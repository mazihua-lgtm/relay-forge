package controller

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/model"

	"github.com/gin-gonic/gin"
)

const (
	// earningsDefaultRangeDays is the window used when the caller names no dates.
	earningsDefaultRangeDays = 30
	earningsDateLayout       = "2006-01-02"

	earningsCostSourceRecorded  = "recorded"
	earningsCostSourceEstimated = "estimated"
)

// earningsWindow is one resolved reporting window. The dates are kept in the
// caller's local calendar so the response echoes what was asked for.
type earningsWindow struct {
	startTimestamp int64
	endTimestamp   int64
	startDate      string
	endDate        string
}

func (w earningsWindow) dateRange() earningsDateRange {
	return earningsDateRange{Start: w.startDate, End: w.endDate}
}

// parseEarningsWindow reads start_date and end_date (YYYY-MM-DD, local calendar).
// A missing end_date is today and a missing start_date reaches back 30 days from
// end_date; the end date includes its whole day. The window must stay within the
// longest log time range, so one request cannot aggregate the whole log table.
func parseEarningsWindow(c *gin.Context) (earningsWindow, error) {
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if raw := strings.TrimSpace(c.Query("end_date")); raw != "" {
		parsed, err := time.ParseInLocation(earningsDateLayout, raw, time.Local)
		if err != nil {
			return earningsWindow{}, common.NewMessage("Invalid {{field}}, expected YYYY-MM-DD", map[string]any{"field": "end_date"})
		}
		end = parsed
	}
	start := end.AddDate(0, 0, -(earningsDefaultRangeDays - 1))
	if raw := strings.TrimSpace(c.Query("start_date")); raw != "" {
		parsed, err := time.ParseInLocation(earningsDateLayout, raw, time.Local)
		if err != nil {
			return earningsWindow{}, common.NewMessage("Invalid {{field}}, expected YYYY-MM-DD", map[string]any{"field": "start_date"})
		}
		start = parsed
	}
	if end.Before(start) {
		return earningsWindow{}, common.NewMessage("start_date must not be later than end_date")
	}
	window := earningsWindow{
		startTimestamp: start.Unix(),
		endTimestamp:   end.AddDate(0, 0, 1).Unix() - 1,
		startDate:      start.Format(earningsDateLayout),
		endDate:        end.Format(earningsDateLayout),
	}
	if _, err := limitLogTimeRange(window.startTimestamp, window.endTimestamp); err != nil {
		return earningsWindow{}, err
	}
	return window, nil
}

// earningsPageQuery reads the p/page_size pair used by the grouped reports.
// Both are clamped, so no request can ask for an unbounded result set or push
// the row offset out of range.
func earningsPageQuery(c *gin.Context) (page int, pageSize int) {
	page, _ = strconv.Atoi(c.Query("p"))
	page = min(max(page, 1), math.MaxInt32)
	pageSize, _ = strconv.Atoi(c.Query("page_size"))
	if pageSize < 1 {
		pageSize = model.EarningsDefaultLimit
	}
	return page, min(pageSize, model.EarningsMaxLimit)
}

// earningsPageOffset converts a 1-based page to a row offset. Page and page size
// are already clamped, so the product fits in int64; the offset itself stays
// within int32.
func earningsPageOffset(page int, pageSize int) int {
	return int(min(int64(page-1)*int64(pageSize), math.MaxInt32))
}

// earningsFigures is the money block shared by the overview and both grouped
// reports. total_revenue is net of refunds; refund_quota is what was given back,
// so gross charged quota is total_revenue + refund_quota.
type earningsFigures struct {
	TotalRevenue int64   `json:"total_revenue"`
	TotalCost    int64   `json:"total_cost"`
	GrossProfit  int64   `json:"gross_profit"`
	GrossMargin  float64 `json:"gross_margin"`
	RequestCount int64   `json:"request_count"`
	RefundQuota  int64   `json:"refund_quota"`
}

type earningsDateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type earningsOverviewResponse struct {
	earningsFigures
	CostSource string            `json:"cost_source"`
	CostRatio  float64           `json:"cost_ratio"`
	DateRange  earningsDateRange `json:"date_range"`
}

type earningsChannelItem struct {
	earningsFigures
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
}

type earningsModelItem struct {
	earningsFigures
	ModelName string `json:"model_name"`
}

type earningsListResponse struct {
	Items      any               `json:"items"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	CostSource string            `json:"cost_source"`
	CostRatio  float64           `json:"cost_ratio"`
	DateRange  earningsDateRange `json:"date_range"`
}

func earningsCostSource(recorded bool) string {
	if recorded {
		return earningsCostSourceRecorded
	}
	return earningsCostSourceEstimated
}

// buildEarningsFigures turns raw log sums into the reported money block. The
// cost is the recorded cost_quota sum when the log table has one, and otherwise
// the quota-based estimate. A refund hands quota back to the user but does not
// show that the upstream refunded the channel, so the estimate prices the gross
// charge, the conservative side for a profit figure. gross_margin is profit over
// net revenue, rounded to four decimals, and 0 when there is no net revenue.
func buildEarningsFigures(revenue int64, refund int64, recordedCost int64, requestCount int64, costRecorded bool) earningsFigures {
	netRevenue := revenue - refund
	cost := recordedCost
	if !costRecorded {
		cost = model.EstimateUpstreamCost(revenue)
	}
	profit := netRevenue - cost
	margin := 0.0
	if netRevenue > 0 {
		margin = math.Round(float64(profit)/float64(netRevenue)*10000) / 10000
	}
	return earningsFigures{
		TotalRevenue: netRevenue,
		TotalCost:    cost,
		GrossProfit:  profit,
		GrossMargin:  margin,
		RequestCount: requestCount,
		RefundQuota:  refund,
	}
}

// GetEarningsOverview reports revenue, upstream cost and gross profit for the
// whole site over the requested window.
func GetEarningsOverview(c *gin.Context) {
	window, err := parseEarningsWindow(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	overview, err := model.GetEarningsOverview(model.EarningsFilter{
		StartTimestamp: window.startTimestamp,
		EndTimestamp:   window.endTimestamp,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	costRecorded := model.LogsRecordCostQuota()
	common.ApiSuccess(c, earningsOverviewResponse{
		earningsFigures: buildEarningsFigures(overview.Revenue, overview.Refund, overview.CostQuota, overview.RequestCount, costRecorded),
		CostSource:      earningsCostSource(costRecorded),
		CostRatio:       model.EarningsCostRatio,
		DateRange:       window.dateRange(),
	})
}

// GetEarningsByChannel reports the same figures per upstream channel.
func GetEarningsByChannel(c *gin.Context) {
	window, err := parseEarningsWindow(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page, pageSize := earningsPageQuery(c)
	rows, total, err := model.GetEarningsByChannel(model.EarningsFilter{
		StartTimestamp: window.startTimestamp,
		EndTimestamp:   window.endTimestamp,
	}, pageSize, earningsPageOffset(page, pageSize))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	costRecorded := model.LogsRecordCostQuota()
	channelIds := make([]int, 0, len(rows))
	for _, row := range rows {
		channelIds = append(channelIds, row.ChannelId)
	}
	channelNames := model.GetChannelNameMap(channelIds)
	items := make([]earningsChannelItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, earningsChannelItem{
			earningsFigures: buildEarningsFigures(row.Revenue, row.Refund, row.CostQuota, row.RequestCount, costRecorded),
			ChannelId:       row.ChannelId,
			ChannelName:     channelNames[row.ChannelId],
		})
	}
	common.ApiSuccess(c, earningsListResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		CostSource: earningsCostSource(costRecorded),
		CostRatio:  model.EarningsCostRatio,
		DateRange:  window.dateRange(),
	})
}

// GetEarningsByModel reports the same figures per model name.
func GetEarningsByModel(c *gin.Context) {
	window, err := parseEarningsWindow(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page, pageSize := earningsPageQuery(c)
	rows, total, err := model.GetEarningsByModel(model.EarningsFilter{
		StartTimestamp: window.startTimestamp,
		EndTimestamp:   window.endTimestamp,
	}, pageSize, earningsPageOffset(page, pageSize))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	costRecorded := model.LogsRecordCostQuota()
	items := make([]earningsModelItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, earningsModelItem{
			earningsFigures: buildEarningsFigures(row.Revenue, row.Refund, row.CostQuota, row.RequestCount, costRecorded),
			ModelName:       row.ModelName,
		})
	}
	common.ApiSuccess(c, earningsListResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		CostSource: earningsCostSource(costRecorded),
		CostRatio:  model.EarningsCostRatio,
		DateRange:  window.dateRange(),
	})
}
