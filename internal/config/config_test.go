package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("임시 설정 파일 작성 실패: %v", err)
	}
	return path
}

const fullConfig = `
kis:
  base_url: "https://example.test"
  app_key: "key"
  app_secret: "secret"
  token_cache_path: "/tmp/token.json"
  timeout: 3s
push:
  import_url: "http://vm.test/api/v1/import/prometheus"
  timeout: 7s
market:
  open: "09:00"
  close: "15:30"
collect:
  interval: 45s
  symbols:
    - code: "005930"
      name: "samsung_electronics"
    - code: "000660"
      name: "sk_hynix"
`

func TestLoadFullConfig(t *testing.T) {
	cfg, err := Load(writeConfig(t, fullConfig))
	if err != nil {
		t.Fatalf("Load 실패: %v", err)
	}

	if cfg.KIS.BaseURL != "https://example.test" {
		t.Errorf("base_url = %q", cfg.KIS.BaseURL)
	}
	// yaml.v3가 "45s" 같은 문자열을 time.Duration으로 해석하는지 함께 확인한다.
	if cfg.Collect.Interval != 45*time.Second {
		t.Errorf("interval = %v, want 45s", cfg.Collect.Interval)
	}
	if cfg.KIS.Timeout != 3*time.Second {
		t.Errorf("kis.timeout = %v, want 3s", cfg.KIS.Timeout)
	}
	if cfg.Push.Timeout != 7*time.Second {
		t.Errorf("push.timeout = %v, want 7s", cfg.Push.Timeout)
	}
	if len(cfg.Collect.Symbols) != 2 {
		t.Fatalf("symbols 개수 = %d, want 2", len(cfg.Collect.Symbols))
	}
	if cfg.Collect.Symbols[0].Code != "005930" || cfg.Collect.Symbols[0].Name != "samsung_electronics" {
		t.Errorf("symbols[0] = %+v", cfg.Collect.Symbols[0])
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
kis:
  app_key: "key"
  app_secret: "secret"
push:
  import_url: "http://vm.test/api/v1/import/prometheus"
collect:
  symbols:
    - code: "005930"
      name: "samsung_electronics"
`))
	if err != nil {
		t.Fatalf("Load 실패: %v", err)
	}

	if cfg.KIS.BaseURL != RealBaseURL {
		t.Errorf("base_url 기본값 = %q, want %q", cfg.KIS.BaseURL, RealBaseURL)
	}
	if cfg.Market.Open != "09:00" || cfg.Market.Close != "15:30" {
		t.Errorf("장 시간 기본값 = %s~%s", cfg.Market.Open, cfg.Market.Close)
	}
	if cfg.Collect.Interval != 30*time.Second {
		t.Errorf("interval 기본값 = %v, want 30s", cfg.Collect.Interval)
	}
	if cfg.KIS.TokenCachePath == "" {
		t.Error("token_cache_path 기본값이 비어 있다")
	}
}

func TestLoadValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "app_key 누락",
			body: "kis:\n  app_secret: s\npush:\n  import_url: u\ncollect:\n  symbols:\n    - code: c\n      name: n\n",
			want: "kis.app_key",
		},
		{
			name: "import_url 누락",
			body: "kis:\n  app_key: k\n  app_secret: s\ncollect:\n  symbols:\n    - code: c\n      name: n\n",
			want: "push.import_url",
		},
		{
			name: "종목 없음",
			body: "kis:\n  app_key: k\n  app_secret: s\npush:\n  import_url: u\n",
			want: "collect.symbols",
		},
		{
			name: "종목 name 누락",
			body: "kis:\n  app_key: k\n  app_secret: s\npush:\n  import_url: u\ncollect:\n  symbols:\n    - code: \"005930\"\n",
			want: "name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tc.body))
			if err == nil {
				t.Fatal("에러를 기대했지만 nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("에러 = %q, %q 를 포함해야 한다", err, tc.want)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "없는파일.yml")); err == nil {
		t.Fatal("없는 파일인데 에러가 없다")
	}
}
