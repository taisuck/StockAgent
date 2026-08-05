package vmpush

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPush(t *testing.T) {
	var gotBody, gotContentType, gotMethod string
	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotMethod = r.Method
		gotContentType = r.Header.Get("content-type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()

	body := "kr_stock_price{code=\"005930\"} 71000 1700000000000\n"
	if err := New(srv.URL, 5*time.Second).Push(context.Background(), body); err != nil {
		t.Fatalf("Push 실패: %v", err)
	}

	if calls != 1 {
		t.Errorf("요청 횟수 = %d, want 1", calls)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotBody != body {
		t.Errorf("본문 = %q, want %q", gotBody, body)
	}
	if !strings.HasPrefix(gotContentType, "text/plain") {
		t.Errorf("content-type = %q, want text/plain", gotContentType)
	}
}

// VictoriaMetrics import 엔드포인트는 204를 돌려주기도 한다.
func TestPushAcceptsNoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := New(srv.URL, 5*time.Second).Push(context.Background(), "up 1 0\n"); err != nil {
		t.Errorf("204를 실패로 처리했다: %v", err)
	}
}

func TestPushReportsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("cannot parse metric"))
	}))
	defer srv.Close()

	err := New(srv.URL, 5*time.Second).Push(context.Background(), "up 1 0\n")
	if err == nil {
		t.Fatal("500인데 에러가 없다")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "cannot parse metric") {
		t.Errorf("에러에 상태 코드나 응답 본문이 없다: %v", err)
	}
}

// 보낼 것이 없으면 요청 자체를 만들지 않는다.
func TestPushSkipsEmptyBody(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	for _, body := range []string{"", "   ", "\n"} {
		if err := c.Push(context.Background(), body); err != nil {
			t.Errorf("Push(%q) = %v, want nil", body, err)
		}
	}

	if calls != 0 {
		t.Errorf("요청 횟수 = %d, want 0", calls)
	}
}

func TestPushReportsUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // 즉시 닫아 연결 실패를 만든다.

	if err := New(url, time.Second).Push(context.Background(), "up 1 0\n"); err == nil {
		t.Fatal("연결 실패인데 에러가 없다")
	}
}
