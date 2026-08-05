package market

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeSource struct {
	open  bool
	err   error
	calls int
}

func (f *fakeSource) IsOpenDay(context.Context, time.Time) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	return f.open, nil
}

func TestHolidayCacheCachesResult(t *testing.T) {
	src := &fakeSource{open: true}
	cache := NewHolidayCache(src, discardLogger())
	date := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	for i := range 3 {
		if !cache.IsOpenDay(context.Background(), date, false) {
			t.Fatalf("%d번째 호출: 개장일이어야 한다", i+1)
		}
	}

	if src.calls != 1 {
		t.Errorf("조회 횟수 = %d, want 1 (하루 단위로 캐싱되어야 한다)", src.calls)
	}
}

func TestHolidayCacheSeparatesDates(t *testing.T) {
	src := &fakeSource{open: true}
	cache := NewHolidayCache(src, discardLogger())

	day1 := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)

	cache.IsOpenDay(context.Background(), day1, false)
	cache.IsOpenDay(context.Background(), day2, false)

	if src.calls != 2 {
		t.Errorf("조회 횟수 = %d, want 2 (날짜가 다르면 다시 조회해야 한다)", src.calls)
	}
}

func TestHolidayCacheCachesClosedDay(t *testing.T) {
	src := &fakeSource{open: false}
	cache := NewHolidayCache(src, discardLogger())
	date := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	if cache.IsOpenDay(context.Background(), date, false) {
		t.Fatal("휴장일이어야 한다")
	}
	if cache.IsOpenDay(context.Background(), date, false) {
		t.Fatal("두 번째 호출도 휴장일이어야 한다")
	}
	if src.calls != 1 {
		t.Errorf("조회 횟수 = %d, want 1 (휴장 결과도 캐싱해야 한다)", src.calls)
	}
}

// 조회가 실패하면 주말 판별만 적용한다: 평일은 개장으로 간주해 수집을 계속한다.
func TestHolidayCacheFallsBackOnError(t *testing.T) {
	date := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		weekend bool
		want    bool
	}{
		{"평일이면 개장으로 간주", false, true},
		{"주말이면 휴장으로 간주", true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &fakeSource{err: errors.New("API 장애")}
			cache := NewHolidayCache(src, discardLogger())

			if got := cache.IsOpenDay(context.Background(), date, tc.weekend); got != tc.want {
				t.Errorf("IsOpenDay() = %v, want %v", got, tc.want)
			}
		})
	}
}

// 실패는 캐싱하지 않아 다음 주기에 다시 시도해야 한다.
func TestHolidayCacheDoesNotCacheFailure(t *testing.T) {
	src := &fakeSource{err: errors.New("일시 장애")}
	cache := NewHolidayCache(src, discardLogger())
	date := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	cache.IsOpenDay(context.Background(), date, false)

	// 이제 API가 복구되어 휴장일이라고 답한다.
	src.err = nil
	src.open = false

	if cache.IsOpenDay(context.Background(), date, false) {
		t.Error("복구된 조회 결과(휴장)를 반영해야 한다")
	}
	if src.calls != 2 {
		t.Errorf("조회 횟수 = %d, want 2 (실패를 캐싱하면 안 된다)", src.calls)
	}
}

// 성공한 조회가 캐시에 있으면, 이후 조회가 실패해도 그 값을 쓴다(직전 캐시 우선).
func TestHolidayCacheUsesCacheOverFallback(t *testing.T) {
	src := &fakeSource{open: false}
	cache := NewHolidayCache(src, discardLogger())
	date := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	cache.IsOpenDay(context.Background(), date, false) // 휴장일로 캐싱
	src.err = errors.New("이후 장애")

	if cache.IsOpenDay(context.Background(), date, false) {
		t.Error("캐시된 휴장 결과를 유지해야 한다(폴백으로 개장 처리하면 안 된다)")
	}
}

// clockAt은 현재 시각이 kst로 고정된 Clock을 만든다.
func clockAt(t *testing.T, kst string) *Clock {
	t.Helper()
	c := testClock(t)
	fixed, err := time.Parse(time.RFC3339, kst)
	if err != nil {
		t.Fatalf("시각 파싱 실패: %v", err)
	}
	c.now = func() time.Time { return fixed }
	return c
}

func TestGateShouldCollect(t *testing.T) {
	// 2026-08-05는 수요일.
	const wednesdayNoon = "2026-08-05T12:00:00+09:00"

	tests := []struct {
		name       string
		kst        string
		openDay    bool
		wantOK     bool
		wantReason string
	}{
		{"장중 개장일", wednesdayNoon, true, true, "장중"},
		{"장중 휴장일", wednesdayNoon, false, false, "휴장일"},
		{"개장 전", "2026-08-05T08:30:00+09:00", true, false, "장외 시간"},
		{"폐장 후", "2026-08-05T16:00:00+09:00", true, false, "장외 시간"},
		{"토요일 장중 시각", "2026-08-08T12:00:00+09:00", true, false, "주말"},
		{"일요일 장중 시각", "2026-08-09T12:00:00+09:00", true, false, "주말"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gate := NewGate(clockAt(t, tc.kst), NewHolidayCache(&fakeSource{open: tc.openDay}, discardLogger()))

			ok, reason := gate.ShouldCollect(context.Background())

			if ok != tc.wantOK {
				t.Errorf("ShouldCollect() = %v, want %v (reason=%s)", ok, tc.wantOK, reason)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

// 장외 시간과 주말에는 휴장일 조회 API를 호출하지 않아야 한다.
func TestGateSkipsHolidayLookupWhenClosed(t *testing.T) {
	tests := []struct {
		name string
		kst  string
	}{
		{"개장 전", "2026-08-05T08:00:00+09:00"},
		{"폐장 후", "2026-08-05T20:00:00+09:00"},
		{"주말", "2026-08-08T12:00:00+09:00"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &fakeSource{open: true}
			gate := NewGate(clockAt(t, tc.kst), NewHolidayCache(src, discardLogger()))

			if ok, _ := gate.ShouldCollect(context.Background()); ok {
				t.Error("수집하지 않아야 한다")
			}
			if src.calls != 0 {
				t.Errorf("휴장일 조회 횟수 = %d, want 0", src.calls)
			}
		})
	}
}
