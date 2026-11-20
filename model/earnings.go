package model

import (
	"database/sql"
	"strings"
	"sync"

	"github.com/mazihua-lgtm/relay-forge/common"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// The default billing multiplier is about 1.2x the upstream price, so a request
// charged Q quota is estimated to have cost Q / 1.2 = Q x 5/6 upstream. The
// ratio is a fraction so the estimate is exact integer arithmetic: aggregated
// quota sums outgrow the int32 range common.QuotaRound saturates at, and that
// helper is meant for single-request charges.
const (
	earningsCostRatioNumerator   = 6
	earningsCostRatioDenominator = 5

	// EarningsCostRatio is the charge-to-cost multiplier behind EstimateUpstreamCost.
	EarningsCostRatio = float64(earningsCostRatioNumerator) / earningsCostRatioDenominator
)

// Page sizes of the grouped earnings reports. They never return an unbounded
// list: an operator with thousands of channels or models pages through them.
const (
	EarningsDefaultLimit = 100
	EarningsMaxLimit     = 500
)

// EstimateUpstreamCost estimates what chargedQuota cost upstream when the log
// table does not record the real figure: chargedQuota / EarningsCostRatio,
// rounded half up.
func EstimateUpstreamCost(chargedQuota int64) int64 {
	return (chargedQuota*earningsCostRatioDenominator + earningsCostRatioNumerator/2) / earningsCostRatioNumerator
}

// EarningsFilter is the time window of an earnings query. Both bounds are
// inclusive unix timestamps.
type EarningsFilter struct {
	StartTimestamp int64
	EndTimestamp   int64
}

// EarningsOverview holds the aggregate sums of one log window. Revenue is the
// gross quota charged to users and Refund the quota given back, so net revenue
// is Revenue - Refund. CostQuota is the recorded upstream cost; it is filled
// only when the log table records it (see LogsRecordCostQuota) and stays zero
// otherwise, in which case callers estimate with EstimateUpstreamCost.
type EarningsOverview struct {
	Revenue      int64 `gorm:"column:revenue"`
	Refund       int64 `gorm:"column:refund"`
	CostQuota    int64 `gorm:"column:recorded_cost"`
	RequestCount int64 `gorm:"column:request_count"`
}

// EarningsChannel is one channel's row of the by-channel earnings report.
type EarningsChannel struct {
	ChannelId    int   `gorm:"column:channel_id"`
	Revenue      int64 `gorm:"column:revenue"`
	Refund       int64 `gorm:"column:refund"`
	CostQuota    int64 `gorm:"column:recorded_cost"`
	RequestCount int64 `gorm:"column:request_count"`
}

// EarningsModel is one model's row of the by-model earnings report.
type EarningsModel struct {
	ModelName    string `gorm:"column:model_name"`
	Revenue      int64  `gorm:"column:revenue"`
	Refund       int64  `gorm:"column:refund"`
	CostQuota    int64  `gorm:"column:recorded_cost"`
	RequestCount int64  `gorm:"column:request_count"`
}

// earningsLogQuery selects the consume and refund logs of the window. Top-ups,
// management and error logs never moved money through an upstream channel, so
// they are excluded by type rather than by quota.
func earningsLogQuery(filter EarningsFilter) *gorm.DB {
	return LOG_DB.Model(&Log{}).
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund}).
		Where("created_at >= ? AND created_at <= ?", filter.StartTimestamp, filter.EndTimestamp)
}

// earningsSelectExpressions returns the aggregate select list and its bind
// arguments. The recorded cost is only summed when the table has the column, so
// the same code runs on stock schemas. Its alias differs from the column name
// because ClickHouse resolves an alias inside the aggregate that defines it.
func earningsSelectExpressions() (string, []any) {
	expressions := []string{
		"COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS revenue",
		"COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS refund",
		"COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0) AS request_count",
	}
	args := []any{LogTypeConsume, LogTypeRefund, LogTypeConsume}
	if LogsRecordCostQuota() {
		expressions = append(expressions,
			"COALESCE(SUM(CASE WHEN type = ? THEN cost_quota ELSE 0 END), 0) AS recorded_cost")
		args = append(args, LogTypeConsume)
	}
	return strings.Join(expressions, ", "), args
}

func earningsQueryFailed(operation string, err error) error {
	common.SysError(common.LogText("failed to query %s: %s", operation, err.Error()))
	return common.NewMessage("Failed to query statistics")
}

// GetEarningsOverview sums revenue, refunds, cost and request count over the
// window in a single pass.
func GetEarningsOverview(filter EarningsFilter) (EarningsOverview, error) {
	var overview EarningsOverview
	selects, args := earningsSelectExpressions()
	if err := earningsLogQuery(filter).Select(selects, args...).Scan(&overview).Error; err != nil {
		return overview, earningsQueryFailed("earnings overview", err)
	}
	return overview, nil
}

// GetEarningsByChannel groups the window by channel, largest revenue first. The
// returned total counts channels, so a caller can page through the report.
func GetEarningsByChannel(filter EarningsFilter, limit int, offset int) ([]EarningsChannel, int64, error) {
	return getEarningsGroups[EarningsChannel](filter, "channel_id", limit, offset)
}

// GetEarningsByModel groups the window by model name, largest revenue first.
func GetEarningsByModel(filter EarningsFilter, limit int, offset int) ([]EarningsModel, int64, error) {
	return getEarningsGroups[EarningsModel](filter, "model_name", limit, offset)
}

// getEarningsGroups aggregates the window per value of column and counts the
// groups. column is a constant chosen by the exported wrappers, never request
// input, so it is safe to put in the SQL text.
func getEarningsGroups[T any](filter EarningsFilter, column string, limit int, offset int) ([]T, int64, error) {
	if limit <= 0 {
		limit = EarningsDefaultLimit
	}
	limit = min(limit, EarningsMaxLimit)
	offset = max(offset, 0)

	selects, args := earningsSelectExpressions()
	rows := make([]T, 0)
	if err := earningsLogQuery(filter).
		Select(selects+", "+column, args...).
		Group(column).
		Order("revenue DESC, " + column + " ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error; err != nil {
		return nil, 0, earningsQueryFailed("earnings by "+column, err)
	}
	var total int64
	if err := earningsLogQuery(filter).Select("COUNT(DISTINCT " + column + ")").Scan(&total).Error; err != nil {
		return nil, 0, earningsQueryFailed("earnings group count", err)
	}
	return rows, total, nil
}

var (
	earningsCostProbeMu  sync.Mutex
	earningsCostProbeDB  *gorm.DB
	earningsCostRecorded bool
)

// LogsRecordCostQuota reports whether the log table stores each request's real
// upstream cost in a cost_quota column. The stock schema does not, so this is
// normally false and earnings estimate the cost from quota. A definitive answer
// is remembered per log database handle, which lets a later schema change take
// effect without a code change; a failed probe (for example a dropped
// connection) is not remembered and is retried on the next call.
func LogsRecordCostQuota() bool {
	db := LOG_DB
	earningsCostProbeMu.Lock()
	defer earningsCostProbeMu.Unlock()
	if earningsCostProbeDB == db {
		return earningsCostRecorded
	}

	// A missing column is the expected answer, so keep gorm from logging the
	// probe's SQL error.
	silent := db.Session(&gorm.Session{Logger: db.Logger.LogMode(gormlogger.Silent)})
	var row struct {
		CostQuota sql.NullInt64 `gorm:"column:cost_quota"`
	}
	err := silent.Model(&Log{}).Select("cost_quota").Limit(1).Scan(&row).Error
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "cost_quota") {
		return false
	}
	earningsCostProbeDB = db
	earningsCostRecorded = err == nil
	return earningsCostRecorded
}

// GetChannelNameMap resolves channel ids to display names for the earnings
// report. It is best effort: a channel that was deleted simply has no name.
func GetChannelNameMap(ids []int) map[int]string {
	names := make(map[int]string, len(ids))
	pending := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if common.MemoryCacheEnabled {
			if channel, err := CacheGetChannel(id); err == nil {
				names[id] = channel.Name
				continue
			}
		}
		pending = append(pending, id)
	}
	if len(pending) == 0 {
		return names
	}
	var rows []struct {
		Id   int    `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	if err := DB.Table("channels").Select("id, name").Where("id IN ?", pending).Find(&rows).Error; err != nil {
		common.SysError(common.LogText("failed to resolve channel names: %s", err.Error()))
		return names
	}
	for _, row := range rows {
		names[row.Id] = row.Name
	}
	return names
}
