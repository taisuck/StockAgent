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
}

// Market은 장 운영 시간이다. 시각은 KST(Asia/Seoul) 기준으로 해석한다.
type Market struct {
	Open  string `yaml:"open"`
	Close string `yaml:"close"`
}

// Collect는 수집 주기와 대상 종목이다.
type Collect struct {
	Interval time.Duration `yaml:"interval"`
	Symbols  []Symbol      `yaml:"symbols"`
}

// Symbol은 수집 대상 종목 하나다. Name은 metric label로 그대로 쓰이므로
// 시계열 이름이 굳는 값이다. 정한 뒤 바꾸지 않는 것이 좋다.
type Symbol struct {
	Code string `yaml:"code"`
	Name string `yaml:"name"`
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
	if len(c.Collect.Symbols) == 0 {
		return fmt.Errorf("collect.symbols 가 비어 있다")
	}
	for i, s := range c.Collect.Symbols {
		if s.Code == "" {
			return fmt.Errorf("collect.symbols[%d].code 가 비어 있다", i)
		}
		if s.Name == "" {
			return fmt.Errorf("collect.symbols[%d].name 가 비어 있다 (종목 %s)", i, s.Code)
		}
	}
	return nil
}
