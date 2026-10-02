// Package logging은 로그 레벨 파싱과 날짜별 로그 파일을 다룬다.
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ParseLevel은 debug, info, warn(warning), error 를 slog.Level로 바꾼다.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("알 수 없는 로그 레벨: %q (debug|info|warn|error)", s)
	}
}

// DailyFile은 날짜가 바뀌면 새 파일로 넘어가는 io.Writer다.
// 파일 이름은 <dir>/<prefix>-YYYY-MM-DD.log 이고, 날짜는 loc 기준으로 정한다.
//
// slog 핸들러는 레코드 하나를 Write 한 번으로 쓰므로, 레코드가 두 파일에 걸쳐 나뉘지 않는다.
// 파일은 append 모드로 열기 때문에 같은 날 재시작해도 이어서 쓴다.
type DailyFile struct {
	dir    string
	prefix string
	loc    *time.Location

	mu   sync.Mutex
	day  string
	file *os.File

	// now는 테스트에서 현재 시각을 고정하기 위한 훅이다. 기본값은 time.Now.
	now func() time.Time
}

// NewDailyFile은 dir을 만들고 오늘 날짜의 로그 파일을 연다.
// 시작 시점에 경로 문제를 드러내려고 첫 파일을 미리 연다.
func NewDailyFile(dir, prefix string, loc *time.Location) (*DailyFile, error) {
	d := &DailyFile{dir: dir, prefix: prefix, loc: loc, now: time.Now}
	if err := d.rotate(d.now()); err != nil {
		return nil, err
	}
	return d, nil
}

// Write는 p를 오늘 날짜의 파일에 쓴다. 날짜가 바뀌었으면 먼저 파일을 바꾼다.
func (d *DailyFile) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.rotate(d.now()); err != nil {
		return 0, err
	}
	return d.file.Write(p)
}

// Close는 열려 있는 파일을 닫는다.
func (d *DailyFile) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.file == nil {
		return nil
	}
	err := d.file.Close()
	d.file = nil
	d.day = ""
	return err
}

// Path는 t 시점에 쓰일 파일 경로다.
func (d *DailyFile) Path(t time.Time) string {
	return filepath.Join(d.dir, d.prefix+"-"+t.In(d.loc).Format(time.DateOnly)+".log")
}

// rotate는 t의 날짜가 현재 파일과 다르면 새 파일을 연다. d.mu를 잡은 상태에서 부른다.
func (d *DailyFile) rotate(t time.Time) error {
	day := t.In(d.loc).Format(time.DateOnly)
	if d.file != nil && day == d.day {
		return nil
	}

	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return fmt.Errorf("로그 디렉터리 생성 실패: %w", err)
	}
	f, err := os.OpenFile(d.Path(t), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("로그 파일 열기 실패: %w", err)
	}

	if d.file != nil {
		d.file.Close()
	}
	d.file = f
	d.day = day
	return nil
}
