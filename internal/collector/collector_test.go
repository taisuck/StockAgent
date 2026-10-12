package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/taisuck/StockAgent/internal/config"
	"github.com/taisuck/StockAgent/internal/kis"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeQuotes struct {
	quotes map[string]kis.Quote
	errs   map[string]error
	calls  []string
}

func (f *fakeQuotes) Quote(_ context.Context, code string) (kis.Quote, error) {
	f.calls = append(f.calls, code)
	if err, ok := f.errs[code]; ok {
		return kis.Quote{}, err
	}
	q, ok := f.quotes[code]
	if !ok {
		return kis.Quote{}, errors.New("등록되지 않은 종목")
	}
	return q, nil
}

// fakeSymbols는 고정 목록을 돌려준다. err가 있으면 그것을 돌려준다.
type fakeSymbols struct {
	symbols []config.Symbol
	err     error
	calls   int
}

func (f *fakeSymbols) Symbols(context.Context) ([]config.Symbol, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.symbols, nil
}

type fakePusher struct {
	bodies []string
	err    error
	// signal은 push가 일어났음을 알린다(비동기 테스트용, 없으면 무시).
	signal chan struct{}
}

func (f *fakePusher) Push(_ context.Context, body string) error {
	f.bodies = append(f.bodies, body)
	if f.signal != nil {
		select {
		case f.signal <- struct{}{}:
		default:
		}
	}
	return f.err
}

type fakeGate struct {
	ok     bool
	reason string
	calls  int
}

func (f *fakeGate) ShouldCollect(context.Context) (bool, string) {
	f.calls++
	return f.ok, f.reason
}

func field(v float64) kis.Field { return kis.Field{Value: v, OK: true} }

var samsung = config.Symbol{Code: "005930", Name: "samsung_electronics"}
var hynix = config.Symbol{Code: "000660", Name: "sk_hynix"}

func fullQuote(code string, price float64) kis.Quote {
	return kis.Quote{
		Code:       code,
		Price:      field(price),
		Open:       field(price - 500),
		High:       field(price + 500),
		Low:        field(price - 800),
		Volume:     field(1000),
		ChangeRate: field(1.5),
	}
}

func newTestCollector(quotes *fakeQuotes, pusher *fakePusher, gate *fakeGate, symbols ...config.Symbol) *Collector {
	return New(&fakeSymbols{symbols: symbols}, time.Minute, quotes, pusher, gate, discardLogger())
}

func TestTickPushesAllMetrics(t *testing.T) {
	quotes := &fakeQuotes{quotes: map[string]kis.Quote{"005930": fullQuote("005930", 71000)}}
	pusher := &fakePusher{}
	c := newTestCollector(quotes, pusher, &fakeGate{ok: true, reason: "장중"}, samsung)

	c.tick(context.Background())

	if len(pusher.bodies) != 1 {
		t.Fatalf("push 횟수 = %d, want 1", len(pusher.bodies))
	}
	body := pusher.bodies[0]

	wantLines := []string{
		`kr_stock_price{code="005930",name="samsung_electronics"} 71000`,
		`kr_stock_open_price{code="005930",name="samsung_electronics"} 70500`,
		`kr_stock_high_price{code="005930",name="samsung_electronics"} 71500`,
		`kr_stock_low_price{code="005930",name="samsung_electronics"} 70200`,
		`kr_stock_volume{code="005930",name="samsung_electronics"} 1000`,
		`kr_stock_change_rate{code="005930",name="samsung_electronics"} 1.5`,
	}
	for _, want := range wantLines {
		if !strings.Contains(body, want) {
			t.Errorf("push 본문에 없다: %s\n본문:\n%s", want, body)
		}
	}

	if got := strings.Count(strings.TrimSpace(body), "\n") + 1; got != len(wantLines) {
		t.Errorf("샘플 줄 수 = %d, want %d", got, len(wantLines))
	}
}

// 한 종목 조회가 실패해도 나머지 종목은 push해야 한다.
func TestTickContinuesAfterSymbolError(t *testing.T) {
	quotes := &fakeQuotes{
		quotes: map[string]kis.Quote{"000660": fullQuote("000660", 180000)},
		errs:   map[string]error{"005930": errors.New("조회 실패")},
	}
	pusher := &fakePusher{}
	c := newTestCollector(quotes, pusher, &fakeGate{ok: true}, samsung, hynix)

	c.tick(context.Background())

	if len(pusher.bodies) != 1 {
		t.Fatalf("push 횟수 = %d, want 1", len(pusher.bodies))
	}
	body := pusher.bodies[0]

	if !strings.Contains(body, `name="sk_hynix"`) {
		t.Error("정상 조회된 종목이 push되지 않았다")
	}
	if strings.Contains(body, `name="samsung_electronics"`) {
		t.Error("조회 실패한 종목이 push되었다")
	}
}

// 모든 종목이 실패하면 빈 본문을 보내지 않는다.
func TestTickSkipsPushWhenNoSamples(t *testing.T) {
	quotes := &fakeQuotes{errs: map[string]error{"005930": errors.New("조회 실패")}}
	pusher := &fakePusher{}
	c := newTestCollector(quotes, pusher, &fakeGate{ok: true}, samsung)

	c.tick(context.Background())

	if len(pusher.bodies) != 0 {
		t.Errorf("push 횟수 = %d, want 0", len(pusher.bodies))
	}
}

// 장외 시간이면 KIS API를 호출하지 않는다.
func TestTickSkipsWhenGateClosed(t *testing.T) {
	quotes := &fakeQuotes{quotes: map[string]kis.Quote{"005930": fullQuote("005930", 71000)}}
	pusher := &fakePusher{}
	gate := &fakeGate{ok: false, reason: "장외 시간"}
	c := newTestCollector(quotes, pusher, gate, samsung)

	c.tick(context.Background())

	if len(quotes.calls) != 0 {
		t.Errorf("시세 조회 횟수 = %d, want 0", len(quotes.calls))
	}
	if len(pusher.bodies) != 0 {
		t.Errorf("push 횟수 = %d, want 0", len(pusher.bodies))
	}
	if gate.calls != 1 {
		t.Errorf("gate 확인 횟수 = %d, want 1", gate.calls)
	}
}

// push가 실패해도 루프가 죽지 않고 다음 주기를 계속한다.
func TestTickSurvivesPushError(t *testing.T) {
	quotes := &fakeQuotes{quotes: map[string]kis.Quote{"005930": fullQuote("005930", 71000)}}
	pusher := &fakePusher{err: errors.New("VM 연결 실패")}
	c := newTestCollector(quotes, pusher, &fakeGate{ok: true}, samsung)

	c.tick(context.Background())
	c.tick(context.Background())

	if len(pusher.bodies) != 2 {
		t.Errorf("push 시도 횟수 = %d, want 2", len(pusher.bodies))
	}
}

// 종목 목록은 주기마다 다시 읽고, 조회가 실패하면 마지막 목록으로 수집한다.
func TestTickUsesLastSymbolsWhenSourceFails(t *testing.T) {
	quotes := &fakeQuotes{quotes: map[string]kis.Quote{
		"005930": fullQuote("005930", 71000),
		"000660": fullQuote("000660", 180000),
	}}
	pusher := &fakePusher{}
	symbols := &fakeSymbols{symbols: []config.Symbol{samsung}}
	c := New(symbols, time.Minute, quotes, pusher, &fakeGate{ok: true}, discardLogger())

	c.tick(context.Background())
	symbols.symbols = []config.Symbol{samsung, hynix}
	c.tick(context.Background())
	symbols.err = errors.New("MongoDB 연결 끊김")
	c.tick(context.Background())

	if symbols.calls != 3 {
		t.Errorf("종목 목록 조회 횟수 = %d, want 3", symbols.calls)
	}
	if len(pusher.bodies) != 3 {
		t.Fatalf("push 횟수 = %d, want 3", len(pusher.bodies))
	}
	if strings.Contains(pusher.bodies[0], `name="sk_hynix"`) {
		t.Error("목록에 없던 종목이 첫 주기에 push되었다")
	}
	for i, body := range pusher.bodies[1:] {
		if !strings.Contains(body, `name="sk_hynix"`) {
			t.Errorf("%d번째 주기에 갱신된 목록이 반영되지 않았다", i+2)
		}
	}
}

// 종목 목록이 비어 있고 이전 목록도 없으면 KIS를 호출하지 않는다.
func TestTickSkipsWhenNoSymbols(t *testing.T) {
	quotes := &fakeQuotes{}
	pusher := &fakePusher{}
	c := New(&fakeSymbols{err: errors.New("조회 실패")}, time.Minute, quotes, pusher, &fakeGate{ok: true}, discardLogger())

	c.tick(context.Background())

	if len(quotes.calls) != 0 || len(pusher.bodies) != 0 {
		t.Errorf("시세 조회 %d회, push %d회; want 0, 0", len(quotes.calls), len(pusher.bodies))
	}
}

// 값이 없는 필드는 0으로 push하지 않고 제외한다.
func TestQuoteSamplesSkipsMissingFields(t *testing.T) {
	q := kis.Quote{
		Code:  "005930",
		Price: field(71000),
		// Open/High/Low/Volume/ChangeRate는 값 없음
	}

	samples := quoteSamples(q, samsung)

	if len(samples) != 1 {
		t.Fatalf("샘플 수 = %d, want 1", len(samples))
	}
	if samples[0].Name != metricPrice {
		t.Errorf("metric = %q, want %q", samples[0].Name, metricPrice)
	}
	if samples[0].Labels["code"] != "005930" || samples[0].Labels["name"] != "samsung_electronics" {
		t.Errorf("labels = %v", samples[0].Labels)
	}
}

// ctx가 취소되면 남은 종목 조회를 중단한다.
func TestCollectStopsOnCanceledContext(t *testing.T) {
	quotes := &fakeQuotes{quotes: map[string]kis.Quote{
		"005930": fullQuote("005930", 71000),
		"000660": fullQuote("000660", 180000),
	}}
	c := newTestCollector(quotes, &fakePusher{}, &fakeGate{ok: true}, samsung, hynix)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if got := c.collect(ctx, []config.Symbol{samsung, hynix}); len(got) != 0 {
		t.Errorf("샘플 수 = %d, want 0", len(got))
	}
	if len(quotes.calls) != 0 {
		t.Errorf("시세 조회 횟수 = %d, want 0", len(quotes.calls))
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	quotes := &fakeQuotes{quotes: map[string]kis.Quote{"005930": fullQuote("005930", 71000)}}
	// interval을 1시간으로 두어, push가 일어난다면 그것은 시작 직후 1회뿐이다.
	pusher := &fakePusher{signal: make(chan struct{}, 1)}
	c := New(&fakeSymbols{symbols: []config.Symbol{samsung}}, time.Hour, quotes, pusher, &fakeGate{ok: true}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	// 첫 주기를 기다리지 않고 즉시 수집하는지 확인한 뒤 취소한다.
	select {
	case <-pusher.signal:
	case <-time.After(2 * time.Second):
		t.Fatal("시작 직후 push가 일어나지 않았다")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run이 ctx 취소 후에도 끝나지 않았다")
	}

	if len(pusher.bodies) != 1 {
		t.Errorf("push 횟수 = %d, want 1 (시작 직후 1회)", len(pusher.bodies))
	}
}
