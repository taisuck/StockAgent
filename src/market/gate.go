package market

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const dateKeyLayout = "20060102"

// OpenDaySource는 특정 날짜가 개장일인지 알려준다(KIS 휴장일 조회 API).
type OpenDaySource interface {
	IsOpenDay(ctx context.Context, date time.Time) (bool, error)
}

// HolidayCache는 개장일 조회 결과를 날짜 단위로 캐싱한다.
// 휴장일은 하루 안에 바뀌지 않으므로 폴링 주기마다 조회하지 않는다.
type HolidayCache struct {
	src OpenDaySource
	log *slog.Logger

	mu    sync.Mutex
	cache map[string]bool
}

// NewHolidayCache는 src를 감싼 캐시를 만든다.
func NewHolidayCache(src OpenDaySource, log *slog.Logger) *HolidayCache {
	return &HolidayCache{src: src, log: log, cache: make(map[string]bool)}
}

// IsOpenDay는 date가 개장일인지 돌려준다.
//
// 조회가 실패하면 그 날짜의 캐시된 값을 쓰고, 캐시도 없으면 주말 판별만 적용한다
// (평일이면 개장으로 본다). 일시적인 API 장애로 장중 수집이 멈추지 않게 하려는 것이다.
func (h *HolidayCache) IsOpenDay(ctx context.Context, date time.Time, weekend bool) bool {
	key := date.Format(dateKeyLayout)

	h.mu.Lock()
	cached, hit := h.cache[key]
	h.mu.Unlock()
	if hit {
		return cached
	}

	open, err := h.src.IsOpenDay(ctx, date)
	if err != nil {
		fallback := !weekend
		h.log.Warn("휴장일 조회 실패, 주말 판별로 대체",
			"date", key, "개장으로_간주", fallback, "err", err)
		return fallback
	}

	h.mu.Lock()
	h.cache[key] = open
	h.mu.Unlock()
	return open
}

// Gate는 지금 수집해야 하는지 판단한다.
type Gate struct {
	clock    *Clock
	holidays *HolidayCache
}

// NewGate는 Gate를 만든다.
func NewGate(clock *Clock, holidays *HolidayCache) *Gate {
	return &Gate{clock: clock, holidays: holidays}
}

// ShouldCollect는 수집 여부와 그 이유를 돌려준다. 이유는 로그용이다.
//
// 휴장일 조회는 장 시간 안일 때만 하므로, 장외 시간에는 API를 건드리지 않는다.
func (g *Gate) ShouldCollect(ctx context.Context) (bool, string) {
	now := g.clock.Now()

	if g.clock.IsWeekend(now) {
		return false, "주말"
	}
	if !g.clock.WithinHours(now) {
		return false, "장외 시간"
	}
	if !g.holidays.IsOpenDay(ctx, now, false) {
		return false, "휴장일"
	}
	return true, "장중"
}
