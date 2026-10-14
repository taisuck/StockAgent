package market

import (
	"testing"
	"time"
)

func testClock(t *testing.T) *Clock {
	t.Helper()
	c, err := NewClock("09:00", "15:30")
	if err != nil {
		t.Fatalf("NewClock 실패: %v", err)
	}
	return c
}

func TestWithinHours(t *testing.T) {
	c := testClock(t)

	tests := []struct {
		name string
		kst  string // KST 기준 시각
		want bool
	}{
		{"개장 직전", "2026-08-05T08:59:59+09:00", false},
		{"개장 시각", "2026-08-05T09:00:00+09:00", true},
		{"장중", "2026-08-05T12:30:00+09:00", true},
		{"폐장 시각", "2026-08-05T15:30:00+09:00", true},
		{"폐장 직후", "2026-08-05T15:30:01+09:00", false},
		{"새벽", "2026-08-05T03:00:00+09:00", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts, err := time.Parse(time.RFC3339, tc.kst)
			if err != nil {
				t.Fatalf("시각 파싱 실패: %v", err)
			}
			if got := c.WithinHours(ts); got != tc.want {
				t.Errorf("WithinHours(%s) = %v, want %v", tc.kst, got, tc.want)
			}
		})
	}
}

// 서버 타임존이 KST가 아니어도 KST로 변환해 판단해야 한다.
func TestWithinHoursConvertsTimeZone(t *testing.T) {
	c := testClock(t)

	// 2026-08-05 01:00 UTC = 같은 날 10:00 KST → 장중
	utcDuringSession := time.Date(2026, 8, 5, 1, 0, 0, 0, time.UTC)
	if !c.WithinHours(utcDuringSession) {
		t.Error("01:00 UTC(=10:00 KST)는 장중이어야 한다")
	}

	// 2026-08-05 09:00 UTC = 같은 날 18:00 KST → 장외
	utcAfterClose := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	if c.WithinHours(utcAfterClose) {
		t.Error("09:00 UTC(=18:00 KST)는 장외여야 한다")
	}
}

func TestIsWeekend(t *testing.T) {
	c := testClock(t)

	// 2026-08-08은 토요일, 08-09는 일요일, 08-10은 월요일.
	tests := []struct {
		date string
		want bool
	}{
		{"2026-08-07T12:00:00+09:00", false},
		{"2026-08-08T12:00:00+09:00", true},
		{"2026-08-09T12:00:00+09:00", true},
		{"2026-08-10T12:00:00+09:00", false},
	}

	for _, tc := range tests {
		ts, err := time.Parse(time.RFC3339, tc.date)
		if err != nil {
			t.Fatalf("시각 파싱 실패: %v", err)
		}
		if got := c.IsWeekend(ts); got != tc.want {
			t.Errorf("IsWeekend(%s) = %v, want %v", tc.date, got, tc.want)
		}
	}
}

// UTC로는 금요일이지만 KST로는 토요일인 시각은 주말로 봐야 한다.
func TestIsWeekendAcrossDateLine(t *testing.T) {
	c := testClock(t)

	// 2026-08-07 16:00 UTC(금) = 2026-08-08 01:00 KST(토)
	ts := time.Date(2026, 8, 7, 16, 0, 0, 0, time.UTC)
	if !c.IsWeekend(ts) {
		t.Error("KST로 토요일인 시각은 주말이어야 한다")
	}
}

func TestNewClockRejectsBadInput(t *testing.T) {
	tests := []struct {
		name        string
		open, close string
	}{
		{"형식 오류", "0900", "15:30"},
		{"시 범위 초과", "25:00", "26:00"},
		{"분 범위 초과", "09:60", "15:30"},
		{"폐장이 개장보다 이름", "15:30", "09:00"},
		{"개장과 폐장 동일", "09:00", "09:00"},
		{"빈 값", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewClock(tc.open, tc.close); err == nil {
				t.Errorf("NewClock(%q, %q): 에러를 기대했지만 nil", tc.open, tc.close)
			}
		})
	}
}
