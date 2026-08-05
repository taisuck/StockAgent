package kis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

const (
	holidayPath = "/uapi/domestic-stock/v1/quotations/chk-holiday"
	holidayTrID = "CTCA0903R"

	// dateLayout은 KIS가 쓰는 날짜 포맷(BASS_DT)이다.
	dateLayout = "20060102"
)

type holidayResponse struct {
	header
	Output []struct {
		BaseDate string `json:"bass_dt"`
		// OpenedYN은 개장 여부다("Y"/"N"). 주말과 공휴일 모두 "N"으로 온다.
		OpenedYN string `json:"opnd_yn"`
	} `json:"output"`
}

// IsOpenDay는 date가 개장일인지 조회한다. date는 KST 기준 날짜여야 한다.
//
// 응답은 BASS_DT 이후 여러 날짜를 함께 담고 있어, 그중 요청한 날짜를 찾아 쓴다.
func (c *Client) IsOpenDay(ctx context.Context, date time.Time) (bool, error) {
	want := date.Format(dateLayout)

	query := url.Values{}
	query.Set("BASS_DT", want)
	query.Set("CTX_AREA_NK", "")
	query.Set("CTX_AREA_FK", "")

	payload, err := c.get(ctx, holidayPath, holidayTrID, query)
	if err != nil {
		return false, err
	}

	var hr holidayResponse
	if err := json.Unmarshal(payload, &hr); err != nil {
		return false, fmt.Errorf("휴장일 응답 파싱 실패: %w", err)
	}
	if err := hr.header.err(holidayPath); err != nil {
		return false, err
	}

	for _, o := range hr.Output {
		if o.BaseDate == want {
			return o.OpenedYN == "Y", nil
		}
	}
	return false, fmt.Errorf("휴장일 응답에 %s 항목이 없다", want)
}
