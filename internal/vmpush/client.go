// Package vmpush는 VictoriaMetrics의 Prometheus import 엔드포인트로 metric을 push한다.
package vmpush

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client는 import 엔드포인트 하나를 가리킨다.
type Client struct {
	url  string
	http *http.Client

	// username이 비어 있지 않으면 요청에 HTTP Basic 인증을 붙인다.
	username string
	password string
}

// New는 Client를 만든다. url은 /api/v1/import/prometheus 까지 포함한 전체 주소다.
func New(url string, timeout time.Duration) *Client {
	return &Client{url: url, http: &http.Client{Timeout: timeout}}
}

// WithBasicAuth는 요청마다 HTTP Basic 인증을 붙이도록 설정하고 c를 돌려준다.
// VictoriaMetrics의 -httpAuth.username / -httpAuth.password 에 대응한다.
func (c *Client) WithBasicAuth(username, password string) *Client {
	c.username = username
	c.password = password
	return c
}

// Push는 exposition 포맷 텍스트를 전송한다. body가 비어 있으면 아무것도 하지 않는다.
func (c *Client) Push(ctx context.Context, body string) error {
	if strings.TrimSpace(body) == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("push 요청 생성 실패: %w", err)
	}
	req.Header.Set("content-type", "text/plain; charset=utf-8")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("push 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	// 본문을 끝까지 읽어야 연결이 재사용된다.
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("push 실패 (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}
