// Package config는 YAML 설정 파일을 읽어 검증한다.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// KIS 실전투자 / 모의투자 도메인.
const (
	RealBaseURL  = "https://openapi.koreainvestment.com:9443"
	PaperBaseURL = "https://openapivts.koreainvestment.com:29443"
)

// Config는 설정 파일 전체 구조다.
type Config struct {
	KIS     KIS     `yaml:"kis"`
	Push    Push    `yaml:"push"`
	Market  Market  `yaml:"market"`
	Collect Collect `yaml:"collect"`
	Log     Log     `yaml:"log"`
	Mongo   Mongo   `yaml:"mongo"`
}

// KIS는 한국투자증권 Open API 접속 정보다.
type KIS struct {
	BaseURL   string `yaml:"base_url"`
	AppKey    string `yaml:"app_key"`
	AppSecret string `yaml:"app_secret"`
	// TokenCachePath는 발급한 access token을 보관할 파일 경로다.
	// 프로세스를 재시작해도 토큰을 재사용해 재발급 호출 제한을 피한다.
	TokenCachePath string        `yaml:"token_cache_path"`
	Timeout        time.Duration `yaml:"timeout"`
}

// Push는 VictoriaMetrics push 대상이다.
type Push struct {
	// ImportURL은 Prometheus import 엔드포인트 주소다.
	// 예: http://localhost:8428/api/v1/import/prometheus
	ImportURL string        `yaml:"import_url"`
	Timeout   time.Duration `yaml:"timeout"`
	// Username/Password는 VictoriaMetrics -httpAuth.* 로 건 HTTP Basic 인증 정보다.
	// 둘 다 비어 있으면 인증 없이 보낸다.
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Market은 장 운영 시간이다. 시각은 KST(Asia/Seoul) 기준으로 해석한다.
type Market struct {
	Open  string `yaml:"open"`
	Close string `yaml:"close"`
}

// Collect는 수집 주기다. 수집 대상 종목은 MongoDB corporation 컬렉션에서 읽는다.
type Collect struct {
	Interval time.Duration `yaml:"interval"`
}

// Mongo는 수집 대상 종목을 읽어 올 MongoDB 접속 정보다.
type Mongo struct {
	// URI는 MongoDB 연결 URI다. 예: mongodb://localhost:27017
	URI      string `yaml:"uri"`
	Database string `yaml:"database"`
	// Username/Password가 있으면 URI의 인증 정보 대신 쓴다.
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Symbol은 수집 대상 종목 하나다. Name은 metric label로 그대로 쓰이므로
// 시계열 이름이 굳는 값이다. 정한 뒤 바꾸지 않는 것이 좋다.
type Symbol struct {
	Code string `yaml:"code"`
	Name string `yaml:"name"`
}

// Log는 로그 출력 설정이다. 표준 출력과 함께 Dir 아래 날짜별 파일에도 남긴다.
type Log struct {
	// Level은 debug, info, warn(warning), error 중 하나다.
	Level string `yaml:"level"`
	// Dir은 날짜별 로그 파일(stockagent-YYYY-MM-DD.log)을 둘 디렉터리다.
	Dir string `yaml:"dir"`
}

// Load는 path의 YAML을 읽어 기본값을 채우고 검증한다.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("설정 파일 읽기 실패: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("설정 파일 파싱 실패: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.KIS.BaseURL == "" {
		c.KIS.BaseURL = RealBaseURL
	}
	if c.KIS.TokenCachePath == "" {
		c.KIS.TokenCachePath = ".token_cache.json"
	}
	if c.KIS.Timeout <= 0 {
		c.KIS.Timeout = 10 * time.Second
	}
	if c.Push.Timeout <= 0 {
		c.Push.Timeout = 10 * time.Second
	}
	if c.Market.Open == "" {
		c.Market.Open = "09:00"
	}
	if c.Market.Close == "" {
		c.Market.Close = "15:30"
	}
	if c.Collect.Interval <= 0 {
		c.Collect.Interval = 30 * time.Second
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Dir == "" {
		c.Log.Dir = "logs"
	}
}

func (c *Config) validate() error {
	if c.KIS.AppKey == "" {
		return fmt.Errorf("kis.app_key 가 비어 있다")
	}
	if c.KIS.AppSecret == "" {
		return fmt.Errorf("kis.app_secret 가 비어 있다")
	}
	if c.Push.ImportURL == "" {
		return fmt.Errorf("push.import_url 가 비어 있다")
	}
	if (c.Push.Username == "") != (c.Push.Password == "") {
		return fmt.Errorf("push.username 과 push.password 는 함께 설정해야 한다")
	}
	if c.Mongo.URI == "" {
		return fmt.Errorf("mongo.uri 가 비어 있다")
	}
	if c.Mongo.Database == "" {
		return fmt.Errorf("mongo.database 가 비어 있다")
	}
	if (c.Mongo.Username == "") != (c.Mongo.Password == "") {
		return fmt.Errorf("mongo.username 과 mongo.password 는 함께 설정해야 한다")
	}
	return nil
}
