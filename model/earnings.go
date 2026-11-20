package model

import (
	"database/sql"
	"strings"
	"sync"

	"github.com/mazihua-lgtm/relay-forge/common"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// EarningsCostRatio is the charge-to-cost multiplier used to estimate the
// upstream cost of a request whose real cost the log table does not record.
// new-api bills a request at roughly 1.2x what the upstream channel charges,
// so a request billed as quota Q is assumed to cost Q/1.2 upstream. When the
// deployment stores the real figure in a cost_quota column, that sum is used
// instead and this constant only describes what the estimate would have been.
const EarningsCostRatio = 1.2

// Earnings page sizes. The grouped earnings endpoints never return an
// unbounded list: an operator with thousands of channels or models pages
// through them instead of scanning the whole log table into memory.
const (
	EarningsDefaultLimit = 100
	EarningsMaxLimit     = 500
)

// EarningsFilter is the time window of an earnings query. Both bounds are
// inclusive unix timestamps; a zero start means "no lower bound".
type EarningsFilter struct {
	StartTimestamp int64
	EndTimestamp   int64
}

// EarningsOverview holds the aggregate sums of one log window. Revenue is the
// gross quota charged to users and Refund the quota given back, so net revenue
// is Revenue - Refund. CostQuota is populated only when the log table records
// it (see LogsRecordCostQuota); otherwise it stays zero and callers estimate.
type EarningsOverview struct {
	Revenue      int64 `gorm:"column:revenue" json:"revenue"`
	Refund       int64 `gorm:"column:refund" json:"refund"`
	CostQuota    int64 `gorm:"column:cost_quota" json:"cost_quota"`
	RequestCount int64 `gorm:"column:request_count" json:"request_count"`
}

// EarningsChannel is one channel's row of the by-channel earnings report.
type EarningsChannel struct {
	ChannelId    int   `gorm:"column:channel_id" json:"channel_id"`
	Revenue      int64 `gorm:"column:revenue" json:"revenue"`
	Refund       int64 `gorm:"column:refund" json:"refund"`
	CostQuota    int64 `gorm:"column:cost_quota" json:"cost_quota"`
	RequestCount int64 `gorm:"column:request_count" json:"request_count"`
}

// EarningsModel is one model's row of the by-model earnings report.
type EarningsModel struct {
	ModelName    string `gorm:"column:model_name" json:"model_name"`
	Revenue      int64  `gorm:"column:revenue" json:"revenue"`
	Refund       int64  `gorm:"column:refund" json:"refund"`
	CostQuota    int64  `gorm:"column:cost_quota" json:"cost_quota"`
	RequestCount int64  `gorm:"column:request_count" json:"request_count"`
}

// earningsLogQuery builds the consume/refund window both reports aggregate over.
// Top-ups, management and error logs are money that never moved through an
// upstream channel, so they are excluded by type rather than by quota.
func earningsLogQuery(filter EarningsFilter) (*gorm.DB, error) {
	if LOG_DB == nil {
		return nil, common.NewMessage("Log database is not initialized")
	}
	return LOG_DB.Table("logs").
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund}).
		Where("created_at >= ? AND created_at <= ?", filter.StartTimestamp, filter.EndTimestamp), nil
}

// earningsSelectExpressions returns the aggregate select list and its bind
// arguments. cost_quota is only summed when the table has the column, so the
// same code runs on stock new-api schemas.
func earningsSelectExpressions() (string, []any) {
	expressions := []string{
		"COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS revenue",
		"COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS refund",
		"COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END), 0) AS request_count",
	}
	args := []any{LogTypeConsume, LogTypeRefund, LogTypeConsume}
	if LogsRecordCostQuota() {
		expressions = append(expressions,
			"COALESCE(SUM(CASE WHEN type = ? THEN cost_quota ELSE 0 END), 0) AS cost_quota")
		args = append(args, LogTypeConsume)
	}
	return strings.Join(expressions, ", "), args
}

func earningsQueryFailed(operation string, err error) error {
	common.SysError(common.LogText("failed to query %s: %s", operation, err.Error()))
	return common.NewMessage("Failed to query earnings")
}

// GetEarningsOverview sums revenue, refunds, cost and request count over the
// window in a single pass.
func GetEarningsOverview(filter EarningsFilter) (EarningsOverview, error) {
	var overview EarningsOverview
	tx, err := earningsLogQuery(filter)
	if err != nil {
		return overview, err
	}
	selects, args := earningsSelectExpressions()
	if err := tx.Select(selects, args...).Scan(&overview).Error; err != nil {
		return overview, earningsQueryFailed("earnings overview", err)
	}
	return overview, nil
}

// GetEarningsByChannel groups the window by channel, largest revenue first.
// The returned total counts groups, so a caller can page through the report.
func GetEarningsByChannel(filter EarningsFilter, limit int, offset int) ([]EarningsChannel, int64, error) {
	rows := make([]EarningsChannel, 0)
	limit, offset = normalizeEarningsPage(limit, offset)
	tx, err := earningsLogQuery(filter)
	if err != nil {
		return rows, 0, err
	}
	selects, args := earningsSelectExpressions()
	if err := tx.Select(selects+", channel_id AS channel_id", args...).
		Group("channel_id").
		Order("revenue DESC, channel_id ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error; err != nil {
		return rows, 0, earningsQueryFailed("earnings by channel", err)
	}
	total, err := earningsGroupCount(filter, "channel_id")
	if err != nil {
		return rows, 0, err
	}
	return rows, total, nil
}

// GetEarningsByModel groups the window by model name, largest revenue first.
func GetEarningsByModel(filter EarningsFilter, limit int, offset int) ([]EarningsModel, int64, error) {
	rows := make([]EarningsModel, 0)
	limit, offset = normalizeEarningsPage(limit, offset)
	tx, err := earningsLogQuery(filter)
	if err != nil {
		return rows, 0, err
	}
	selects, args := earningsSelectExpressions()
	if err := tx.Select(selects+", model_name AS model_name", args...).
		Group("model_name").
		Order("revenue DESC, model_name ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error; err != nil {
		return rows, 0, earningsQueryFailed("earnings by model", err)
	}
	total, err := earningsGroupCount(filter, "model_name")
	if err != nil {
		return rows, 0, err
	}
	return rows, total, nil
}

func earningsGroupCount(filter EarningsFilter, column string) (int64, error) {
	tx, err := earningsLogQuery(filter)
	if err != nil {
		return 0, err
	}
	var total int64
	if err := tx.Select("COUNT(DISTINCT " + column + ")").Scan(&total).Error; err != nil {
		return 0, earningsQueryFailed("earnings group count", err)
	}
	return total, nil
}

func normalizeEarningsPage(limit int, offset int) (int, int) {
	if limit <= 0 {
		limit = EarningsDefaultLimit
	}
	if limit > EarningsMaxLimit {
		limit = EarningsMaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

var (
	earningsCostProbeMu  sync.Mutex
	earningsCostProbeDB  *gorm.DB
	earningsCostRecorded bool
)

// LogsRecordCostQuota reports whether the log table stores each request's real
// upstream cost in a cost_quota column. The stock new-api schema does not, so
// this is normally false and earnings estimates cost from quota. The probe runs
// once per log database handle, which keeps a later schema migration from
// requiring a code change.
func LogsRecordCostQuota() bool {
	db := LOG_DB
	if db == nil {
		return false
	}
	earningsCostProbeMu.Lock()
	defer earningsCostProbeMu.Unlock()
	if earningsCostProbeDB == db {
		return earningsCostRecorded
	}
	earningsCostRecorded = probeLogCostQuota(db)
	earningsCostProbeDB = db
	return earningsCostRecorded
}

func probeLogCostQuota(db *gorm.DB) bool {
	// A missing column is the expected answer, so keep gorm from logging the
	// probe's SQL error on every process start.
	silent := db.Session(&gorm.Session{Logger: db.Logger.LogMode(gormlogger.Silent)})
	var row struct {
		CostQuota sql.NullInt64 `gorm:"column:cost_quota"`
	}
	return silent.Table("logs").Select("cost_quota").Limit(1).Scan(&row).Error == nil
}

// GetChannelNameMap resolves channel ids to display names for the earnings
// report. It is best effort: a channel that was deleted simply has no name.
func GetChannelNameMap(ids []int) map[int]string {
	names := make(map[int]string, len(ids))
	if len(ids) == 0 {
		return names
	}
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
	if len(pending) == 0 || DB == nil {
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
