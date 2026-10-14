package kis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	quotePath = "/uapi/domestic-stock/v1/quotations/inquire-price"
	quoteTrID = "FHKST01010100"

	// marketDivStock은 조회 구분 코드 중 주식/ETF/ETN이다.
	marketDivStock = "J"
)

// Field는 KIS가 문자열로 내려주는 수치 필드를 파싱한 결과다.
// OK가 false면 응답에 값이 없거나 숫자가 아니라는 뜻이므로 push 대상에서 제외한다.
// 값이 없는 것과 0인 것을 구분하기 위해 필요하다.
type Field struct {
	Value float64
	OK    bool
}

func parseField(s string) Field {
	s = strings.TrimSpace(s)
	if s == "" {
		return Field{}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return Field{}
	}
	return Field{Value: v, OK: true}
}

// Quote는 종목 하나의 현재가 시세다.
type Quote struct {
	Code       string
	Price      Field // 현재가
	Open       Field // 시가
	High       Field // 고가
	Low        Field // 저가
	Volume     Field // 누적 거래량
	ChangeRate Field // 전일 대비율(%)
}

type quoteResponse struct {
	header
	Output struct {
		Price      string `json:"stck_prpr"`
		Open       string `json:"stck_oprc"`
		High       string `json:"stck_hgpr"`
		Low        string `json:"stck_lwpr"`
		Volume     string `json:"acml_vol"`
		ChangeRate string `json:"prdy_ctrt"`
	} `json:"output"`
}

// Quote는 종목 코드의 현재가 시세를 조회한다.
func (c *Client) Quote(ctx context.Context, code string) (Quote, error) {
	query := url.Values{}
	query.Set("FID_COND_MRKT_DIV_CODE", marketDivStock)
	query.Set("FID_INPUT_ISCD", code)

	payload, err := c.get(ctx, quotePath, quoteTrID, query)
	if err != nil {
		return Quote{}, err
	}

	var qr quoteResponse
	if err := json.Unmarshal(payload, &qr); err != nil {
		return Quote{}, fmt.Errorf("시세 응답 파싱 실패 (종목 %s): %w", code, err)
	}
	if err := qr.header.err(quotePath); err != nil {
		return Quote{}, fmt.Errorf("종목 %s: %w", code, err)
	}

	q := Quote{
		Code:       code,
		Price:      parseField(qr.Output.Price),
		Open:       parseField(qr.Output.Open),
		High:       parseField(qr.Output.High),
		Low:        parseField(qr.Output.Low),
		Volume:     parseField(qr.Output.Volume),
		ChangeRate: parseField(qr.Output.ChangeRate),
	}
	if !q.Price.OK {
		return Quote{}, fmt.Errorf("종목 %s: 현재가를 읽을 수 없다 (stck_prpr=%q)", code, qr.Output.Price)
	}
	return q, nil
}
