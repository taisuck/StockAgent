# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 프로젝트 개요

국장(한국 주식시장) 관심 종목의 시세정보를 수집해서 metric 정보로 변환/수집하는 Go 프로젝트.

## 모듈 및 구조

- 모듈 경로: `github.com/taisuck/StockAgent` (저장소 루트에 단일 모듈)
- 실행 바이너리는 `src/<name>` 에, 공용 패키지는 `internal/` 에 둔다.

```
src/stockagent      수집 루프를 구성해 실행하는 main
internal/config     YAML 설정 로드/검증
internal/kis        KIS API 클라이언트 (토큰, 시세, 휴장일)
internal/market     KST 장 운영 여부 판단 (Clock, HolidayCache, Gate)
internal/metric     Prometheus exposition 포맷 변환
internal/vmpush     VictoriaMetrics push
internal/collector  수집 → 변환 → push 루프
```

의존 방향은 `collector` → (`kis`, `metric`, `config`) 한 방향이다. `collector`는 구체 타입 대신
`QuoteSource`/`Pusher`/`Gate` 인터페이스를 받으므로, 테스트에서 KIS나 VictoriaMetrics 없이 검증한다.

## 주요 명령어

```sh
go build ./...             # 전체 빌드
go run ./src/stockagent -config config.yml -log-level debug
go vet ./...               # 정적 분석
gofmt -l .                 # 포맷되지 않은 파일 목록 (출력이 비어 있으면 정상)
gofmt -w .                 # 제자리에서 포맷 적용
go mod tidy                # import 변경 후 go.mod/go.sum 동기화
go test ./...              # 전체 테스트
go test -run '^TestName$' ./path/to/pkg   # 단일 테스트 실행
```

크로스 빌드(어느 OS에서든 상대 OS용 바이너리를 만들 수 있다. cgo를 쓰지 않는다):

```sh
GOOS=linux   GOARCH=amd64 go build -o dist/stockagent     ./src/stockagent
GOOS=windows GOARCH=amd64 go build -o dist/stockagent.exe ./src/stockagent
GOOS=linux go vet ./...    # 상대 OS 기준 정적 분석
```

같은 빌드를 `scripts/` 의 스크립트로도 할 수 있다. 산출물은 `output/` 에 생긴다(`.gitignore` 처리).

```sh
scripts/build_linux.sh     scripts/build_windows.sh     # Linux 셸에서
scripts\build_linux.bat    scripts\build_windows.bat    # Windows cmd에서
```

## 언어 및 대화 지침 (Language)

- 모든 설명, 질문 답변, 커밋 메시지 작성 시 **한국어(Korean)**를 기본 언어로 사용한다.
- 전문 용어나 코드 상의 명칭(변수명, 함수명, 라이브러리 등)은 필요에 따라 영문 그대로 표기한다.

## 아키텍쳐

데이터 흐름: **KIS REST API 폴링 → 시세를 metric으로 변환 → VictoriaMetrics로 push**.
Prometheus exporter처럼 scrape를 기다리는 방식이 아니라, agent가 직접 밀어넣는 push 모델이다.

### 시세 수집 (한국투자증권 KIS Open API)

- 국내 주식 시세는 한국투자증권 REST API로 수집한다.
- 수집 대상: 삼성전자(`005930`), SK하이닉스(`000660`).
- 인증은 `appkey`/`appsecret`으로 access token을 발급받아 `Bearer` 헤더로 사용한다.
  토큰 재발급에는 호출 제한이 있으므로 **발급한 토큰은 만료 전까지 캐싱해 재사용**한다.
- 요청마다 조회 종류를 지정하는 `tr_id` 헤더가 필요하고, 실전투자와 모의투자는 도메인이 다르다.
  엔드포인트 경로와 `tr_id` 값은 KIS 공식 문서를 확인해 사용한다.

### metric push (VictoriaMetrics)

- Prometheus import 엔드포인트(`/api/v1/import/prometheus`)로 Prometheus exposition
  포맷 텍스트를 POST 한다.
- push 대상(VictoriaMetrics 주소)은 config 파일에 명시한다.
- metric 이름과 label은 `internal/collector` 의 상수로 고정돼 있다. **한 번 push하면 시계열 이름이
  굳으므로 바꾸지 않는다.**

  ```
  kr_stock_price{code="005930",name="samsung_electronics"}   현재가
  kr_stock_open_price / kr_stock_high_price / kr_stock_low_price
  kr_stock_volume        누적 거래량
  kr_stock_change_rate   전일 대비율(%)
  ```

- 값이 없는 필드는 0으로 push하지 않고 제외한다. "값 없음"과 "0"을 구분해야 하므로
  `kis.Field` 가 파싱 성공 여부를 함께 들고 다닌다.

### 동작 시간

- 폴링 주기는 config의 `collect.interval` (기본 30s). 시작 직후 첫 주기를 기다리지 않고 1회 수집한다.
- 장외 시간과 휴장일에는 수집을 멈춘다. 한국 시장 기준이므로 시간 계산은 **KST(Asia/Seoul)** 로 한다.
  시스템 tzdata가 없는 환경(Windows, 최소 컨테이너)에서도 동작하도록 `time/tzdata` 를 내장한다.
- 주말 외의 휴장일 판별 방법 : 휴장일 조회 API 호출
  - 이 조회도 KIS API이므로 토큰과 전용 `tr_id`가 필요하다.
  - 휴장일은 하루 단위로 바뀌지 않으니 **조회 결과를 캐싱**하고, 폴링 주기마다 호출하지 않는다.
    장외 시간·주말에는 조회 자체를 하지 않는다.
  - 조회 실패 시: 그 날짜의 캐시된 값이 있으면 그것을 쓰고, 없으면 주말 판별만 적용한다
    (평일이면 개장으로 간주). 실패는 캐싱하지 않아 다음 주기에 다시 시도한다.

### 장애 처리 원칙

한 주기의 실패가 루프를 멈추지 않는다. 다음 주기에 다시 시도하면 되기 때문이다.

- 종목 하나의 시세 조회가 실패해도 나머지 종목은 그대로 push한다.
- push가 실패하면 로그만 남기고 다음 주기로 넘어간다.
- 수집할 샘플이 하나도 없으면 빈 본문을 전송하지 않는다.

### 설정 파일

- 형식은 YAML. 인증 정보(appkey/appsecret), push 대상, 수집 대상 종목을 여기에 둔다.
- 실제 키가 담기는 파일이므로 저장소에 커밋하지 않는다. `config.example.yml` 만 두고
  `config.yml` 과 토큰 캐시(`.token_cache.json`)는 `.gitignore` 처리한다.
- 발급한 access token은 `kis.token_cache_path` 파일에 보관해 재시작 후에도 재사용한다.
  재발급 호출 제한을 피하려는 것이므로, 이 캐시를 지우는 코드를 넣지 않는다.

### 플랫폼 (Windows / Linux)

개발은 Windows, 배포는 Linux처럼 두 OS를 오가는 것을 전제로 한다. OS별 분기 코드나
빌드 태그는 두지 않고, 양쪽에서 같게 동작하는 방법을 택한다.

- `go.mod` 의 go 지시문은 코드가 실제로 요구하는 최소 버전만 적는다. 패치 버전까지 박으면
  다른 머신에서 툴체인을 강제로 내려받는다.
- 시간 계산은 `time/tzdata` 내장으로 해결한다. 시스템 tzdata 설치 여부에 기대지 않는다.
- 토큰 캐시는 임시 파일에 쓴 뒤 `os.Rename` 으로 교체한다. rename은 양쪽 OS 모두 기존 파일을
  덮어쓰므로, 쓰다 중단돼도 캐시가 깨지지 않는다.
- 파일 권한(`0600`)은 Linux에서만 의미가 있다. Windows는 모드를 무시하고 디렉터리 ACL을 따르므로,
  토큰 캐시를 접근이 제한된 위치에 두는 것은 배포 시 판단할 문제다.
- `SIGTERM` 은 Linux(systemd, `docker stop`)에서만 전달된다. Windows 콘솔에서는 Ctrl+C(`os.Interrupt`)만
  온다. 둘 다 받아 정상 종료한다.
- 설정 파일은 CRLF나 UTF-8 BOM이 섞여도 읽힌다(Windows 편집기 산출물). 회귀 테스트로 고정해 뒀다.
- `.gitattributes` 로 저장소 안 텍스트 파일은 LF로 고정한다.

## 주의사항 & 금지 항목

- 기존 코드를 리팩토링할 때는 요청 받은 부분 외에는 건드리지 말 것.
