package metric

import (
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	ts := time.UnixMilli(1_700_000_000_000)

	got := Format([]Sample{
		{Name: "kr_stock_price", Labels: map[string]string{"name": "samsung_electronics", "code": "005930"}, Value: 71000},
		{Name: "kr_stock_change_rate", Labels: map[string]string{"code": "005930"}, Value: -1.25},
	}, ts)

	// label은 이름순(code → name)으로 정렬되어야 한다.
	want := "kr_stock_price{code=\"005930\",name=\"samsung_electronics\"} 71000 1700000000000\n" +
		"kr_stock_change_rate{code=\"005930\"} -1.25 1700000000000\n"

	if got != want {
		t.Errorf("Format() =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatEmpty(t *testing.T) {
	if got := Format(nil, time.UnixMilli(0)); got != "" {
		t.Errorf("Format(nil) = %q, want 빈 문자열", got)
	}
}

func TestFormatNoLabels(t *testing.T) {
	got := Format([]Sample{{Name: "up", Value: 1}}, time.UnixMilli(5))
	if want := "up 1 5\n"; got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestFormatEscapesLabelValue(t *testing.T) {
	got := Format([]Sample{
		{Name: "m", Labels: map[string]string{"l": `a"b\c` + "\n"}, Value: 1},
	}, time.UnixMilli(0))

	want := `m{l="a\"b\\c\n"} 1 0` + "\n"
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestFormatLargeValue(t *testing.T) {
	// 거래량처럼 큰 값은 지수 표기가 된다. Prometheus 포맷이 허용하는 표기이고
	// client_golang도 같은 방식('g')을 쓰므로 VictoriaMetrics가 그대로 받는다.
	got := Format([]Sample{{Name: "kr_stock_volume", Value: 12345678}}, time.UnixMilli(0))
	if want := "kr_stock_volume 1.2345678e+07 0\n"; got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}
