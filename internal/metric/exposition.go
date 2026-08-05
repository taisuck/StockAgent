// Package metric은 시세를 Prometheus exposition 포맷 텍스트로 바꾼다.
//
// VictoriaMetrics의 Prometheus import 엔드포인트가 받는 형식이다:
//
//	metric_name{label="value"} 12345 1700000000000
//
// 마지막 항은 밀리초 단위 타임스탬프다.
package metric

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Sample은 push할 시계열 한 점이다.
type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// Format은 samples를 exposition 포맷 텍스트로 만든다.
// 모든 샘플에 동일한 타임스탬프 ts를 붙여, 한 번의 수집이 같은 시점으로 기록되게 한다.
func Format(samples []Sample, ts time.Time) string {
	millis := strconv.FormatInt(ts.UnixMilli(), 10)

	var b strings.Builder
	for _, s := range samples {
		b.WriteString(s.Name)
		writeLabels(&b, s.Labels)
		b.WriteByte(' ')
		b.WriteString(strconv.FormatFloat(s.Value, 'g', -1, 64))
		b.WriteByte(' ')
		b.WriteString(millis)
		b.WriteByte('\n')
	}
	return b.String()
}

// writeLabels는 label을 이름순으로 정렬해 쓴다. 출력이 항상 같아야 테스트가 안정적이다.
func writeLabels(b *strings.Builder, labels map[string]string) {
	if len(labels) == 0 {
		return
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(escapeLabelValue(labels[k]))
		b.WriteString(`"`)
	}
	b.WriteByte('}')
}

func escapeLabelValue(v string) string {
	if !strings.ContainsAny(v, "\\\"\n") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(v)
}
