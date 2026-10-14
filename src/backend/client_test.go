package backend

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taisuck/StockAgent/src/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSymbols(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`[
			{"id": "1", "name": "samsung_electronics", "code": "005930"},
			{"id": "2", "name": "", "code": "000660"},
			{"id": "3", "name": "naver", "code": ""},
			{"id": "4", "name": "kakao", "code": "035720"}
		]`))
	}))
	defer srv.Close()

	// 주소 끝의 슬래시는 경로에 겹치지 않아야 한다.
	got, err := New(srv.URL+"/", 5*time.Second, discardLogger()).Symbols(context.Background())
	if err != nil {
		t.Fatalf("Symbols 실패: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/stock" {
		t.Errorf("path = %q, want /stock", gotPath)
	}
	want := []config.Symbol{
		{Code: "005930", Name: "samsung_electronics"},
		{Code: "035720", Name: "kakao"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("symbols = %+v, want %+v", got, want)
	}
}

func TestSymbolsReportsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("db down"))
	}))
	defer srv.Close()

	_, err := New(srv.URL, 5*time.Second, discardLogger()).Symbols(context.Background())
	if err == nil {
		t.Fatal("500 응답인데 에러가 없다")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "db down") {
		t.Errorf("에러 = %q, 상태 코드와 본문을 포함해야 한다", err)
	}
}

func TestSymbolsReportsBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code": "005930"`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL, 5*time.Second, discardLogger()).Symbols(context.Background()); err == nil {
		t.Fatal("깨진 JSON인데 에러가 없다")
	}
}
