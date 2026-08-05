// Package market은 국내 장 운영 여부를 판단한다.
//
// 모든 시각 계산은 서버 로컬 타임존과 무관하게 KST(Asia/Seoul) 기준으로 한다.
package market

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	// tz 데이터베이스를 바이너리에 내장한다. Windows나 최소 컨테이너 이미지처럼
	// 시스템 tzdata가 없는 환경에서도 Asia/Seoul을 로드할 수 있어야 한다.
	_ "time/tzdata"
)

// TimeZone은 국내 시장 기준 타임존이다.
const TimeZone = "Asia/Seoul"

// Clock은 KST 기준 장 운영 시간을 판단한다.
type Clock struct {
	loc *time.Location
	// 자정 기준 초. 초 단위로 두어 폐장 시각을 정확히 경계로 삼는다.
	openSecond  int
	closeSecond int

	// now는 테스트에서 현재 시각을 고정하기 위한 훅이다. 기본값은 time.Now.
	now func() time.Time
}

// NewClock은 "HH:MM" 형식의 개장/폐장 시각으로 Clock을 만든다.
func NewClock(open, close string) (*Clock, error) {
	loc, err := time.LoadLocation(TimeZone)
	if err != nil {
		return nil, fmt.Errorf("타임존 %s 로드 실패: %w", TimeZone, err)
	}

	openMin, err := parseHHMM(open)
	if err != nil {
		return nil, fmt.Errorf("market.open: %w", err)
	}
	closeMin, err := parseHHMM(close)
	if err != nil {
		return nil, fmt.Errorf("market.close: %w", err)
	}
	if closeMin <= openMin {
		return nil, fmt.Errorf("market.close(%s)가 market.open(%s)보다 늦어야 한다", close, open)
	}

	return &Clock{
		loc:         loc,
		openSecond:  openMin * 60,
		closeSecond: closeMin * 60,
		now:         time.Now,
	}, nil
}

// Now는 현재 시각을 KST로 돌려준다.
func (c *Clock) Now() time.Time {
	return c.now().In(c.loc)
}

// In은 t를 KST로 변환한다.
func (c *Clock) In(t time.Time) time.Time {
	return t.In(c.loc)
}

// WithinHours는 t의 시각이 장 운영 시간 안인지 본다. 날짜(휴장일 여부)는 보지 않는다.
// 개장 시각과 폐장 시각은 포함하고, 폐장 시각을 1초라도 지나면 장외로 본다.
func (c *Clock) WithinHours(t time.Time) bool {
	kst := t.In(c.loc)
	sec := kst.Hour()*3600 + kst.Minute()*60 + kst.Second()
	return sec >= c.openSecond && sec <= c.closeSecond
}

// IsWeekend는 t(KST로 해석)가 주말인지 본다.
func (c *Clock) IsWeekend(t time.Time) bool {
	switch t.In(c.loc).Weekday() {
	case time.Saturday, time.Sunday:
		return true
	default:
		return false
	}
}

func parseHHMM(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return 0, fmt.Errorf("%q는 HH:MM 형식이 아니다", s)
	}
	hour, err := strconv.Atoi(h)
	if err != nil || hour < 0 || hour > 23 {
		return 0, fmt.Errorf("%q의 시(hour)가 올바르지 않다", s)
	}
	minute, err := strconv.Atoi(m)
	if err != nil || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("%q의 분(minute)이 올바르지 않다", s)
	}
	return hour*60 + minute, nil
}
