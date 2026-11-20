package model

import (
	"testing"

	"github.com/mazihua-lgtm/relay-forge/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	earningsTestWindowStart = int64(1_760_000_000)
	earningsTestWindowEnd   = int64(1_760_600_000)
)

// setupEarningsTestDB installs an isolated log database. withCostQuota adds the
// upstream-cost column new-api does not ship, so both the recorded-cost and the
// estimated-cost paths can be exercised.
func setupEarningsTestDB(t *testing.T, withCostQuota bool) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// A shared :memory: database is per connection; keep the pool at one.
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Log{}))
	if withCostQuota {
		require.NoError(t, db.Exec("ALTER TABLE logs ADD COLUMN cost_quota bigint NOT NULL DEFAULT 0").Error)
	}

	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		assert.NoError(t, sqlDB.Close())
	})
	return db
}

// seedEarningsLogs writes one month of representative traffic: two channels,
// three models, one refund, and rows that must stay outside the window or the
// consume/refund type filter.
func seedEarningsLogs(t *testing.T, db *gorm.DB, withCostQuota bool) {
	t.Helper()
	rows := []struct {
		typ       int
		channelId int
		model     string
		quota     int
		createdAt int64
		cost      int
	}{
		{LogTypeConsume, 1, "gpt-4o", 1200, earningsTestWindowStart + 10, 1000},
		{LogTypeConsume, 1, "gpt-4o-mini", 600, earningsTestWindowStart + 20, 500},
		{LogTypeConsume, 2, "claude-sonnet", 500, earningsTestWindowEnd, 400},
		{LogTypeRefund, 1, "gpt-4o", 200, earningsTestWindowStart + 30, 0},
		// Outside the window, and therefore excluded everywhere below.
		{LogTypeConsume, 1, "gpt-4o", 9999, earningsTestWindowStart - 1, 9999},
		{LogTypeConsume, 2, "claude-sonnet", 9999, earningsTestWindowEnd + 1, 9999},
		// Non-consume types never count as revenue.
		{LogTypeTopup, 0, "", 100000, earningsTestWindowStart + 40, 0},
		{LogTypeError, 0, "gpt-4o", 0, earningsTestWindowStart + 50, 0},
	}
	for _, row := range rows {
		log := &Log{
			Type:        row.typ,
			ChannelId:   row.channelId,
			ModelName:   row.model,
			Quota:       row.quota,
			CreatedAt:   row.createdAt,
			RequestId:   "req-" + row.model,
			Username:    "user-1",
			ChannelName: "",
		}
		require.NoError(t, db.Create(log).Error)
		if withCostQuota && row.typ == LogTypeConsume {
			require.NoError(t, db.Exec("UPDATE logs SET cost_quota = ? WHERE id = ?", row.cost, log.Id).Error)
		}
	}
}

func earningsTestFilter() EarningsFilter {
	return EarningsFilter{
		StartTimestamp: earningsTestWindowStart,
		EndTimestamp:   earningsTestWindowEnd,
	}
}

func TestGetEarningsOverviewEstimatesCostWhenLogsLackCostQuota(t *testing.T) {
	db := setupEarningsTestDB(t, false)
	seedEarningsLogs(t, db, false)

	overview, err := GetEarningsOverview(earningsTestFilter())
	require.NoError(t, err)

	assert.False(t, LogsRecordCostQuota())
	assert.EqualValues(t, 2300, overview.Revenue, "gross consume quota inside the window")
	assert.EqualValues(t, 200, overview.Refund)
	assert.EqualValues(t, 0, overview.CostQuota)
	assert.EqualValues(t, 3, overview.RequestCount)
}

func TestGetEarningsOverviewSumsRecordedCostQuota(t *testing.T) {
	db := setupEarningsTestDB(t, true)
	seedEarningsLogs(t, db, true)

	overview, err := GetEarningsOverview(earningsTestFilter())
	require.NoError(t, err)

	assert.True(t, LogsRecordCostQuota())
	assert.EqualValues(t, 2300, overview.Revenue)
	assert.EqualValues(t, 200, overview.Refund)
	assert.EqualValues(t, 1900, overview.CostQuota, "1000 + 500 + 400")
	assert.EqualValues(t, 3, overview.RequestCount)
}

func TestGetEarningsOverviewEmptyWindow(t *testing.T) {
	setupEarningsTestDB(t, false)

	overview, err := GetEarningsOverview(EarningsFilter{
		StartTimestamp: earningsTestWindowStart - 100,
		EndTimestamp:   earningsTestWindowStart - 50,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 0, overview.Revenue)
	assert.EqualValues(t, 0, overview.Refund)
	assert.EqualValues(t, 0, overview.RequestCount)
}

func TestGetEarningsByChannelGroupsAndOrdersByRevenue(t *testing.T) {
	db := setupEarningsTestDB(t, false)
	seedEarningsLogs(t, db, false)

	rows, total, err := GetEarningsByChannel(earningsTestFilter(), 100, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.EqualValues(t, 2, total)

	assert.Equal(t, 1, rows[0].ChannelId)
	assert.EqualValues(t, 1800, rows[0].Revenue)
	assert.EqualValues(t, 200, rows[0].Refund)
	assert.EqualValues(t, 2, rows[0].RequestCount)

	assert.Equal(t, 2, rows[1].ChannelId)
	assert.EqualValues(t, 500, rows[1].Revenue)
	assert.EqualValues(t, 0, rows[1].Refund)
	assert.EqualValues(t, 1, rows[1].RequestCount)
}

func TestGetEarningsByChannelSumsRecordedCostPerChannel(t *testing.T) {
	db := setupEarningsTestDB(t, true)
	seedEarningsLogs(t, db, true)

	rows, _, err := GetEarningsByChannel(earningsTestFilter(), 100, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.EqualValues(t, 1500, rows[0].CostQuota, "1000 + 500")
	assert.EqualValues(t, 400, rows[1].CostQuota)
}

func TestGetEarningsByChannelPaginates(t *testing.T) {
	db := setupEarningsTestDB(t, false)
	seedEarningsLogs(t, db, false)

	firstPage, total, err := GetEarningsByChannel(earningsTestFilter(), 1, 0)
	require.NoError(t, err)
	require.Len(t, firstPage, 1)
	assert.EqualValues(t, 2, total, "total counts groups, not rows")
	assert.Equal(t, 1, firstPage[0].ChannelId)

	secondPage, total, err := GetEarningsByChannel(earningsTestFilter(), 1, 1)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	assert.EqualValues(t, 2, total)
	assert.Equal(t, 2, secondPage[0].ChannelId)

	beyond, total, err := GetEarningsByChannel(earningsTestFilter(), 10, 10)
	require.NoError(t, err)
	assert.Empty(t, beyond)
	assert.EqualValues(t, 2, total)
}

func TestGetEarningsByModelGroupsAndOrdersByRevenue(t *testing.T) {
	db := setupEarningsTestDB(t, false)
	seedEarningsLogs(t, db, false)

	rows, total, err := GetEarningsByModel(earningsTestFilter(), 100, 0)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.EqualValues(t, 3, total)

	assert.Equal(t, "gpt-4o", rows[0].ModelName)
	assert.EqualValues(t, 1200, rows[0].Revenue)
	assert.EqualValues(t, 200, rows[0].Refund)
	assert.EqualValues(t, 1, rows[0].RequestCount)

	assert.Equal(t, "gpt-4o-mini", rows[1].ModelName)
	assert.EqualValues(t, 600, rows[1].Revenue)

	assert.Equal(t, "claude-sonnet", rows[2].ModelName)
	assert.EqualValues(t, 500, rows[2].Revenue)
}

func TestGetEarningsByModelPaginates(t *testing.T) {
	db := setupEarningsTestDB(t, false)
	seedEarningsLogs(t, db, false)

	rows, total, err := GetEarningsByModel(earningsTestFilter(), 1, 2)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 3, total)
	assert.Equal(t, "claude-sonnet", rows[0].ModelName)
}

func TestGetEarningsByModelSumsRecordedCostPerModel(t *testing.T) {
	db := setupEarningsTestDB(t, true)
	seedEarningsLogs(t, db, true)

	rows, _, err := GetEarningsByModel(earningsTestFilter(), 100, 0)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.EqualValues(t, 1000, rows[0].CostQuota)
	assert.EqualValues(t, 500, rows[1].CostQuota)
	assert.EqualValues(t, 400, rows[2].CostQuota)
}
