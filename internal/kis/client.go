// Package kis는 한국투자증권(KIS) Open API REST 클라이언트다.
//
// 엔드포인트 경로와 tr_id 값은 KIS 공식 문서 기준이다. API가 개정되면
// 이 패키지의 상수만 확인하면 된다.
package kis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	tokenPath = "/oauth2/tokenP"

	// refreshMargin 만큼 남았으면 미리 재발급한다.
	refreshMargin = 10 * time.Minute
)

// Client는 KIS API 호출과 access token 수명 관리를 담당한다.
// 토큰 재발급에는 호출 제한이 있으므로 만료 전까지 캐싱해 재사용한다.
type Client struct {
	baseURL   string
	appKey    string
	appSecret string
	cachePath string
	http      *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// New는 Client를 만든다. cachePath가 비어 있지 않으면 토큰을 그 파일에
// 보관해서 프로세스 재시작 후에도 재사용한다.
func New(baseURL, appKey, appSecret, cachePath string, timeout time.Duration) *Client {
	return &Client{
		baseURL:   baseURL,
		appKey:    appKey,
		appSecret: appSecret,
		cachePath: cachePath,
		http:      &http.Client{Timeout: timeout},
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type cachedToken struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Token은 유효한 access token을 돌려준다. 메모리 → 파일 캐시 → 신규 발급
// 순서로 확인하므로, 유효한 토큰이 있는 동안 발급 API를 호출하지 않는다.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.valid(time.Now()) {
		return c.token, nil
	}
	if c.loadCache() && c.valid(time.Now()) {
		return c.token, nil
	}
	if err := c.issue(ctx); err != nil {
		return "", err
	}
	return c.token, nil
}

// valid는 호출자가 c.mu를 잡은 상태를 전제한다.
func (c *Client) valid(now time.Time) bool {
	return c.token != "" && now.Add(refreshMargin).Before(c.expiresAt)
}

func (c *Client) issue(ctx context.Context) error {
	body, err := json.Marshal(map[string]string{
		"grant_type": "client_credentials",
		"appkey":     c.appKey,
		"appsecret":  c.appSecret,
	})
	if err != nil {
		return fmt.Errorf("토큰 요청 본문 생성 실패: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+tokenPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("토큰 요청 생성 실패: %w", err)
	}
	req.Header.Set("content-type", "application/json; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("토큰 발급 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("토큰 응답 읽기 실패: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("토큰 발급 실패 (HTTP %d): %s", resp.StatusCode, truncate(payload))
	}

	var tr tokenResponse
	if err := json.Unmarshal(payload, &tr); err != nil {
		return fmt.Errorf("토큰 응답 파싱 실패: %w", err)
	}
	if tr.AccessToken == "" {
		return fmt.Errorf("토큰 응답에 access_token 이 없다: %s", truncate(payload))
	}

	expiresIn := tr.ExpiresIn
	if expiresIn <= 0 {
		// 문서상 24시간이지만 응답에 값이 없으면 보수적으로 잡는다.
		expiresIn = int64((1 * time.Hour).Seconds())
	}

	c.token = tr.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	c.saveCache()
	return nil
}

// loadCache는 파일 캐시를 메모리로 읽어온다. 캐시가 없거나 깨져 있으면
// 신규 발급으로 넘어가면 되므로 실패를 에러로 올리지 않는다.
func (c *Client) loadCache() bool {
	if c.cachePath == "" {
		return false
	}
	raw, err := os.ReadFile(c.cachePath)
	if err != nil {
		return false
	}
	var ct cachedToken
	if err := json.Unmarshal(raw, &ct); err != nil || ct.AccessToken == "" {
		return false
	}
	c.token = ct.AccessToken
	c.expiresAt = ct.ExpiresAt
	return true
}

func (c *Client) saveCache() {
	if c.cachePath == "" {
		return
	}
	raw, err := json.Marshal(cachedToken{AccessToken: c.token, ExpiresAt: c.expiresAt})
	if err != nil {
		return
	}
	if dir := filepath.Dir(c.cachePath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o700)
	}
	// 토큰은 자격증명이므로 소유자만 읽도록 둔다.
	_ = os.WriteFile(c.cachePath, raw, 0o600)
}

// get은 인증 헤더를 붙여 GET 요청을 보내고 응답 본문을 돌려준다.
func (c *Client) get(ctx context.Context, path, trID string, query url.Values) ([]byte, error) {
	token, err := c.Token(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("요청 생성 실패: %w", err)
	}
	req.Header.Set("content-type", "application/json; charset=utf-8")
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("appkey", c.appKey)
	req.Header.Set("appsecret", c.appSecret)
	req.Header.Set("tr_id", trID)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s 요청 실패: %w", path, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s 응답 읽기 실패: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s 실패 (HTTP %d): %s", path, resp.StatusCode, truncate(payload))
	}
	return payload, nil
}

// header는 모든 KIS 응답에 공통으로 실리는 처리 결과다.
type header struct {
	RtCd  string `json:"rt_cd"`
	MsgCd string `json:"msg_cd"`
	Msg1  string `json:"msg1"`
}

// err는 rt_cd가 "0"(정상)이 아니면 에러를 만든다.
func (h header) err(path string) error {
	if h.RtCd == "0" {
		return nil
	}
	return fmt.Errorf("%s 응답 오류 (rt_cd=%s, msg_cd=%s): %s", path, h.RtCd, h.MsgCd, h.Msg1)
}

func truncate(b []byte) string {
	const max = 300
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}
