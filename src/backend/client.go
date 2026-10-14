// Package backend는 백엔드 API를 호출해 수집 대상 종목 목록을 읽는다.
package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/taisuck/StockAgent/src/config"
)

// stockPath는 현재 수집 중인 종목 목록을 돌려주는 API 경로다.
const stockPath = "/stock"

// stock은 stock list 응답 항목 중 수집에 쓰는 필드다.
type stock struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Client는 백엔드 서버 하나를 가리킨다.
type Client struct {
	baseURL string
	http    *http.Client
	log     *slog.Logger
}

// New는 Client를 만든다. baseURL은 백엔드 주소다(예: http://localhost:8080).
func New(baseURL string, timeout time.Duration, log *slog.Logger) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
		log:     log,
	}
}

// Symbols는 GET /stock 응답의 code, name을 수집 대상 종목으로 돌려준다.
// code나 name이 비어 있는 항목은 metric label을 만들 수 없으므로 건너뛴다.
func (c *Client) Symbols(ctx context.Context) ([]config.Symbol, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+stockPath, nil)
	if err != nil {
		return nil, fmt.Errorf("stock list 요청 생성 실패: %w", err)
	}
	req.Header.Set("accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stock list 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("stock list 실패 (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var stocks []stock
	if err := json.NewDecoder(resp.Body).Decode(&stocks); err != nil {
		return nil, fmt.Errorf("stock list 응답 파싱 실패: %w", err)
	}

	symbols := make([]config.Symbol, 0, len(stocks))
	for _, s := range stocks {
		if s.Code == "" || s.Name == "" {
			c.log.Warn("code 또는 name이 비어 있는 종목을 건너뜀", "code", s.Code, "name", s.Name)
			continue
		}
		symbols = append(symbols, config.Symbol{Code: s.Code, Name: s.Name})
	}
	return symbols, nil
}
