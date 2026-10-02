package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "time/tzdata"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"INFO":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		" error ": slog.LevelError,
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseLevel("trace"); err == nil {
		t.Error("알 수 없는 레벨에 오류가 없다")
	}
}

func TestDailyFileRotatesOnKSTDate(t *testing.T) {
	kst, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	// UTC 14:59 는 KST 23:59, UTC 15:00 은 KST 다음 날 00:00 이다.
	now := time.Date(2026, 10, 1, 14, 59, 0, 0, time.UTC)
	d, err := NewDailyFile(dir, "stockagent", kst)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.now = func() time.Time { return now }

	write := func(s string) {
		t.Helper()
		if _, err := d.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	write("first\n")
	now = now.Add(time.Minute)
	write("second\n")

	assertFile(t, filepath.Join(dir, "stockagent-2026-10-01.log"), "first\n")
	assertFile(t, filepath.Join(dir, "stockagent-2026-10-02.log"), "second\n")
}

func TestDailyFileAppendsOnReopen(t *testing.T) {
	dir := t.TempDir()
	for _, s := range []string{"a\n", "b\n"} {
		d, err := NewDailyFile(dir, "app", time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
		d.Close()
	}
	assertFile(t, filepath.Join(dir, "app-"+time.Now().UTC().Format(time.DateOnly)+".log"), "a\nb\n")
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s 읽기 실패: %v", path, err)
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(want) {
		t.Errorf("%s = %q; want %q", path, got, want)
	}
}
