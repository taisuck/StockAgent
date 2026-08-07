package kis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeAPI는 KIS API를 흉내내는 테스트 서버다.
type fakeAPI struct {
	t *testing.T

	tokenCalls   int
	quoteCalls   int
	holidayCalls int

	expiresIn int64
	// lastQuoteHeaders는 마지막 시세 요청의 헤더다.
	lastQuoteHeaders http.Header
	lastQuoteQuery   string

	quoteBody   string
	holidayBody string
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		f.tokenCalls++

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Errorf("토큰 요청 본문 파싱 실패: %v", err)
		}
		if body["grant_type"] != "client_credentials" {
			f.t.Errorf("grant_type = %q", body["grant_type"])
		}
		if body["appkey"] != "test-key" || body["appsecret"] != "test-secret" {
			f.t.Errorf("appkey/appsecret 이 요청에 실리지 않았다: %v", body)
		}

		expires := f.expiresIn
		if expires == 0 {
			expires = 86400
		}
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"token_type":   "Bearer",
			"expires_in":   expires,
		})
	})

	mux.HandleFunc(quotePath, func(w http.ResponseWriter, r *http.Request) {
		f.quoteCalls++
		f.lastQuoteHeaders = r.Header.Clone()
		f.lastQuoteQuery = r.URL.RawQuery
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(f.quoteBody))
	})

	mux.HandleFunc(holidayPath, func(w http.ResponseWriter, r *http.Request) {
		f.holidayCalls++
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(f.holidayBody))
	})

	return mux
}

const okQuoteBody = `{
  "rt_cd": "0",
  "msg_cd": "MCA00000",
  "msg1": "정상처리 되었습니다.",
  "output": {
    "stck_prpr": "71000",
    "stck_oprc": "70500",
    "stck_hgpr": "71500",
    "stck_lwpr": "70200",
    "acml_vol": "12345678",
    "prdy_ctrt": "-1.25"
  }
}`

func newTestClient(t *testing.T, api *fakeAPI) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(api.handler())
	t.Cleanup(srv.Close)

	cachePath := filepath.Join(t.TempDir(), "token.json")
	return New(srv.URL, "test-key", "test-secret", cachePath, 5*time.Second), srv
}

func TestQuote(t *testing.T) {
	api := &fakeAPI{t: t, quoteBody: okQuoteBody}
	c, _ := newTestClient(t, api)

	q, err := c.Quote(context.Background(), "005930")
	if err != nil {
		t.Fatalf("Quote 실패: %v", err)
	}

	if q.Code != "005930" {
		t.Errorf("Code = %q", q.Code)
	}
	checks := []struct {
		name  string
		field Field
		want  float64
	}{
		{"현재가", q.Price, 71000},
		{"시가", q.Open, 70500},
		{"고가", q.High, 71500},
		{"저가", q.Low, 70200},
		{"거래량", q.Volume, 12345678},
		{"전일대비율", q.ChangeRate, -1.25},
	}
	for _, ch := range checks {
		if !ch.field.OK {
			t.Errorf("%s: OK=false", ch.name)
			continue
		}
		if ch.field.Value != ch.want {
			t.Errorf("%s = %v, want %v", ch.name, ch.field.Value, ch.want)
		}
	}
}

func TestQuoteSendsAuthHeaders(t *testing.T) {
	api := &fakeAPI{t: t, quoteBody: okQuoteBody}
	c, _ := newTestClient(t, api)

	if _, err := c.Quote(context.Background(), "005930"); err != nil {
		t.Fatalf("Quote 실패: %v", err)
	}

	h := api.lastQuoteHeaders
	if got := h.Get("authorization"); got != "Bearer test-token" {
		t.Errorf("authorization = %q", got)
	}
	if got := h.Get("tr_id"); got != quoteTrID {
		t.Errorf("tr_id = %q, want %q", got, quoteTrID)
	}
	if got := h.Get("appkey"); got != "test-key" {
		t.Errorf("appkey = %q", got)
	}
	if got := h.Get("appsecret"); got != "test-secret" {
		t.Errorf("appsecret = %q", got)
	}

	if !strings.Contains(api.lastQuoteQuery, "FID_INPUT_ISCD=005930") {
		t.Errorf("query = %q, 종목 코드가 없다", api.lastQuoteQuery)
	}
	if !strings.Contains(api.lastQuoteQuery, "FID_COND_MRKT_DIV_CODE=J") {
		t.Errorf("query = %q, 시장 구분 코드가 없다", api.lastQuoteQuery)
	}
}

// 토큰 재발급에는 호출 제한이 있으므로 유효한 동안 재사용해야 한다.
func TestTokenIsReused(t *testing.T) {
	api := &fakeAPI{t: t, quoteBody: okQuoteBody}
	c, _ := newTestClient(t, api)

	for i := range 3 {
		if _, err := c.Quote(context.Background(), "005930"); err != nil {
			t.Fatalf("%d번째 Quote 실패: %v", i+1, err)
		}
	}

	if api.tokenCalls != 1 {
		t.Errorf("토큰 발급 횟수 = %d, want 1", api.tokenCalls)
	}
	if api.quoteCalls != 3 {
		t.Errorf("시세 조회 횟수 = %d, want 3", api.quoteCalls)
	}
}

// 다른 프로세스가 남긴 파일 캐시를 읽어 재발급을 건너뛴다.
func TestTokenLoadedFromFileCache(t *testing.T) {
	api := &fakeAPI{t: t, quoteBody: okQuoteBody}
	srv := httptest.NewServer(api.handler())
	t.Cleanup(srv.Close)

	cachePath := filepath.Join(t.TempDir(), "token.json")

	first := New(srv.URL, "test-key", "test-secret", cachePath, 5*time.Second)
	if _, err := first.Token(context.Background()); err != nil {
		t.Fatalf("첫 토큰 발급 실패: %v", err)
	}

	second := New(srv.URL, "test-key", "test-secret", cachePath, 5*time.Second)
	token, err := second.Token(context.Background())
	if err != nil {
		t.Fatalf("두 번째 클라이언트 토큰 조회 실패: %v", err)
	}

	if token != "test-token" {
		t.Errorf("token = %q", token)
	}
	if api.tokenCalls != 1 {
		t.Errorf("토큰 발급 횟수 = %d, want 1 (파일 캐시를 재사용해야 한다)", api.tokenCalls)
	}
}

// 캐시 파일이 깨져 있으면 무시하고 새로 발급받는다.
func TestTokenRecoversFromCorruptedCache(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	t.Cleanup(srv.Close)

	cachePath := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(cachePath, []byte(`{"access_to`), 0o600); err != nil {
		t.Fatalf("깨진 캐시 작성 실패: %v", err)
	}

	token, err := New(srv.URL, "test-key", "test-secret", cachePath, 5*time.Second).Token(context.Background())
	if err != nil {
		t.Fatalf("Token 실패: %v", err)
	}
	if token != "test-token" {
		t.Errorf("token = %q", token)
	}
	if api.tokenCalls != 1 {
		t.Errorf("토큰 발급 횟수 = %d, want 1", api.tokenCalls)
	}
}

// 캐시 파일은 rename으로 교체하므로 임시 파일이 남지 않아야 한다.
func TestSaveCacheLeavesNoTempFile(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "token.json")

	if _, err := New(srv.URL, "test-key", "test-secret", cachePath, 5*time.Second).Token(context.Background()); err != nil {
		t.Fatalf("Token 실패: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("디렉터리 읽기 실패: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "token.json" {
		t.Errorf("디렉터리 내용 = %v, want [token.json]", names)
	}
}

// 캐시 경로의 상위 디렉터리가 없으면 만들어야 한다.
func TestSaveCacheCreatesParentDir(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	t.Cleanup(srv.Close)

	cachePath := filepath.Join(t.TempDir(), "cache", "sub", "token.json")

	if _, err := New(srv.URL, "test-key", "test-secret", cachePath, 5*time.Second).Token(context.Background()); err != nil {
		t.Fatalf("Token 실패: %v", err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("캐시 파일이 만들어지지 않았다: %v", err)
	}
}

// 만료가 임박한 토큰은 재발급한다.
func TestTokenRefreshedWhenNearExpiry(t *testing.T) {
	api := &fakeAPI{t: t, quoteBody: okQuoteBody, expiresIn: 60}
	c, _ := newTestClient(t, api)

	if _, err := c.Token(context.Background()); err != nil {
		t.Fatalf("첫 토큰 발급 실패: %v", err)
	}
	if _, err := c.Token(context.Background()); err != nil {
		t.Fatalf("두 번째 토큰 조회 실패: %v", err)
	}

	// expires_in(60초)이 refreshMargin(10분)보다 짧으므로 매번 재발급된다.
	if api.tokenCalls != 2 {
		t.Errorf("토큰 발급 횟수 = %d, want 2", api.tokenCalls)
	}
}

func TestQuoteRejectsErrorResponse(t *testing.T) {
	api := &fakeAPI{t: t, quoteBody: `{"rt_cd":"1","msg_cd":"EGW00123","msg1":"기간이 만료된 token 입니다."}`}
	c, _ := newTestClient(t, api)

	_, err := c.Quote(context.Background(), "005930")
	if err == nil {
		t.Fatal("rt_cd가 0이 아닌데 에러가 없다")
	}
	if !strings.Contains(err.Error(), "EGW00123") {
		t.Errorf("에러에 msg_cd가 없다: %v", err)
	}
}

func TestQuoteRejectsMissingPrice(t *testing.T) {
	api := &fakeAPI{t: t, quoteBody: `{"rt_cd":"0","output":{"stck_prpr":""}}`}
	c, _ := newTestClient(t, api)

	if _, err := c.Quote(context.Background(), "005930"); err == nil {
		t.Fatal("현재가가 없는데 에러가 없다")
	}
}

func TestIsOpenDay(t *testing.T) {
	date := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "개장일",
			body: `{"rt_cd":"0","output":[{"bass_dt":"20260805","opnd_yn":"Y"}]}`,
			want: true,
		},
		{
			name: "휴장일",
			body: `{"rt_cd":"0","output":[{"bass_dt":"20260805","opnd_yn":"N"}]}`,
			want: false,
		},
		{
			name: "여러 날짜 중 해당 날짜를 골라낸다",
			body: `{"rt_cd":"0","output":[
				{"bass_dt":"20260804","opnd_yn":"N"},
				{"bass_dt":"20260805","opnd_yn":"Y"},
				{"bass_dt":"20260806","opnd_yn":"N"}
			]}`,
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{t: t, holidayBody: tc.body}
			c, _ := newTestClient(t, api)

			got, err := c.IsOpenDay(context.Background(), date)
			if err != nil {
				t.Fatalf("IsOpenDay 실패: %v", err)
			}
			if got != tc.want {
				t.Errorf("IsOpenDay() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsOpenDayMissingDate(t *testing.T) {
	api := &fakeAPI{t: t, holidayBody: `{"rt_cd":"0","output":[{"bass_dt":"20260806","opnd_yn":"Y"}]}`}
	c, _ := newTestClient(t, api)

	_, err := c.IsOpenDay(context.Background(), time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("응답에 해당 날짜가 없는데 에러가 없다")
	}
	if !strings.Contains(err.Error(), "20260805") {
		t.Errorf("에러에 요청 날짜가 없다: %v", err)
	}
}

func TestParseField(t *testing.T) {
	tests := []struct {
		in     string
		wantOK bool
		want   float64
	}{
		{"71000", true, 71000},
		{"-1.25", true, -1.25},
		{" 500 ", true, 500},
		{"0", true, 0},
		{"", false, 0},
		{"   ", false, 0},
		{"N/A", false, 0},
	}

	for _, tc := range tests {
		got := parseField(tc.in)
		if got.OK != tc.wantOK {
			t.Errorf("parseField(%q).OK = %v, want %v", tc.in, got.OK, tc.wantOK)
			continue
		}
		if got.OK && got.Value != tc.want {
			t.Errorf("parseField(%q).Value = %v, want %v", tc.in, got.Value, tc.want)
		}
	}
}
