// Package collector는 수집 → 변환 → push 루프를 돌린다.
package collector

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/taisuck/StockAgent/internal/config"
	"github.com/taisuck/StockAgent/internal/kis"
	"github.com/taisuck/StockAgent/internal/metric"
)

// metric 이름. 한 번 push하면 시계열 이름이 굳으므로 바꾸지 않는다.
const (
	metricPrice      = "kr_stock_price"
	metricOpen       = "kr_stock_open_price"
	metricHigh       = "kr_stock_high_price"
	metricLow        = "kr_stock_low_price"
	metricVolume     = "kr_stock_volume"
	metricChangeRate = "kr_stock_change_rate"
)

// SymbolSource는 수집 대상 종목 목록을 돌려준다.
type SymbolSource interface {
	Symbols(ctx context.Context) ([]config.Symbol, error)
}

// QuoteSource는 종목 현재가를 조회한다.
type QuoteSource interface {
	Quote(ctx context.Context, code string) (kis.Quote, error)
}

// Pusher는 exposition 텍스트를 전송한다.
type Pusher interface {
	Push(ctx context.Context, body string) error
}

// Gate는 지금 수집할지 판단한다.
type Gate interface {
	ShouldCollect(ctx context.Context) (bool, string)
}

// Collector는 설정된 주기로 시세를 수집해 push한다.
type Collector struct {
	symbols  SymbolSource
	interval time.Duration
	quotes   QuoteSource
	pusher   Pusher
	gate     Gate
	log      *slog.Logger

	// lastSymbols는 마지막으로 읽어 온 종목 목록이다. 목록 조회가 실패하면 이것으로 수집한다.
	lastSymbols []config.Symbol
}

// New는 Collector를 만든다. 종목 목록은 주기마다 symbols에서 다시 읽으므로
// 대상 종목을 바꿔도 재시작하지 않아도 된다.
func New(symbols SymbolSource, interval time.Duration, quotes QuoteSource, pusher Pusher, gate Gate, log *slog.Logger) *Collector {
	return &Collector{
		symbols:  symbols,
		interval: interval,
		quotes:   quotes,
		pusher:   pusher,
		gate:     gate,
		log:      log,
	}
}

// Run은 ctx가 취소될 때까지 수집 루프를 돈다. 취소로 끝나면 nil을 돌려준다.
func (c *Collector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// 첫 주기를 기다리지 않고 즉시 한 번 수집한다.
	c.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

// tick은 한 주기 분량의 수집과 push를 수행한다.
// 개별 실패는 로그만 남기고 루프를 계속한다. 다음 주기에 다시 시도하면 되기 때문이다.
func (c *Collector) tick(ctx context.Context) {
	ok, reason := c.gate.ShouldCollect(ctx)
	if !ok {
		c.log.Debug("수집 건너뜀", "이유", reason)
		return
	}

	symbols := c.loadSymbols(ctx)
	if len(symbols) == 0 {
		c.log.Warn("수집 대상 종목이 없다")
		return
	}

	samples := c.collect(ctx, symbols)
	if len(samples) == 0 {
		c.log.Warn("push할 샘플이 없다")
		return
	}

	body := metric.Format(samples, time.Now())
	if err := c.pusher.Push(ctx, body); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		c.log.Error("push 실패", "err", err)
	}
}

// loadSymbols는 종목 목록을 읽는다. 실패하면 마지막으로 읽은 목록을 그대로 쓴다.
func (c *Collector) loadSymbols(ctx context.Context) []config.Symbol {
	symbols, err := c.symbols.Symbols(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.log.Error("종목 목록 조회 실패, 이전 목록으로 수집", "symbols", len(c.lastSymbols), "err", err)
		}
		return c.lastSymbols
	}
	c.lastSymbols = symbols
	return symbols
}

// collect는 모든 종목을 순차 조회한다. KIS API 호출 유량 제한을 고려해 병렬로 보내지 않는다.
func (c *Collector) collect(ctx context.Context, symbols []config.Symbol) []metric.Sample {
	samples := make([]metric.Sample, 0, len(symbols)*6)

	for _, sym := range symbols {
		if ctx.Err() != nil {
			return samples
		}

		q, err := c.quotes.Quote(ctx, sym.Code)
		if err != nil {
			c.log.Error("시세 조회 실패", "code", sym.Code, "name", sym.Name, "err", err)
			continue
		}
		samples = append(samples, quoteSamples(q, sym)...)
	}
	return samples
}

// quoteSamples는 시세를 샘플 목록으로 바꾼다. 값이 없는 필드는 제외한다.
func quoteSamples(q kis.Quote, sym config.Symbol) []metric.Sample {
	labels := map[string]string{"code": sym.Code, "name": sym.Name}

	fields := []struct {
		name  string
		field kis.Field
	}{
		{metricPrice, q.Price},
		{metricOpen, q.Open},
		{metricHigh, q.High},
		{metricLow, q.Low},
		{metricVolume, q.Volume},
		{metricChangeRate, q.ChangeRate},
	}

	samples := make([]metric.Sample, 0, len(fields))
	for _, f := range fields {
		if !f.field.OK {
			continue
		}
		samples = append(samples, metric.Sample{
			Name:   f.name,
			Labels: labels,
			Value:  f.field.Value,
		})
	}
	return samples
}
