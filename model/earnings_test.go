package model

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

const (
	earningsTestWindowStart = int64(1_760_000_000)
	earningsTestWindowEnd   = int64(1_760_600_000)
)

// earningsTestLogs is one month of traffic: two channels, three models, one
// refund, rows outside the window, and log types that never count as revenue.
var earningsTestLogs = []struct {
	logType   int
	channelId int
	modelName string
	quota     int
	createdAt int64
	cost      int
}{
	{LogTypeConsume, 1, "gpt-4o", 1200, earningsTestWindowStart + 10, 1000},
	{LogTypeConsume, 1, "gpt-4o-mini", 600, earningsTestWindowStart + 20, 500},
	{LogTypeConsume, 2, "claude-sonnet", 500, earningsTestWindowEnd, 400},
	{LogTypeRefund, 1, "gpt-4o", 200, earningsTestWindowStart + 30, 0},
	// Outside the window on either side.
	{LogTypeConsume, 1, "gpt-4o", 9999, earningsTestWindowStart - 1, 9999},
	{LogTypeConsume, 2, "claude-sonnet", 9999, earningsTestWindowEnd + 1, 9999},
	// Top-ups and errors never moved money through an upstream channel.
	{LogTypeTopup, 0, "", 100000, earningsTestWindowStart + 40, 0},
	{LogTypeError, 0, "gpt-4o", 0, earningsTestWindowStart + 50, 0},
}

// openEarningsTestDB installs an isolated log database seeded with
// earningsTestLogs. SQLite always runs; MySQL and PostgreSQL run when
// TEST_MYSQL_DSN / TEST_POSTGRES_DSN point at a loopback instance. Without
// withCostQuota the upstream-cost column is dropped again, which is a log table
// from before the column existed (a ClickHouse log store has none); with it the
// column is there and the consume logs that have a cost carry it, the others
// stay NULL, so both the estimated and the recorded cost paths are exercised.
func openEarningsTestDB(t *testing.T, dialect string, withCostQuota bool) {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "sqlite":
		driver = sqlite.Open(":memory:")
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
		driver = postgres.Open(dsn)
	}
	db, err := gorm.Open(driver, &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{TablePrefix: "earnings_test_"},
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// One connection keeps a PostgreSQL search_path and a SQLite :memory: database stable.
	sqlDB.SetMaxOpenConns(1)
	// Cleanups run last-in first-out: register the close first so the drops below still have a connection.
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })

	switch dialect {
	case "postgres":
		// PostgreSQL index names are schema-wide, so build in a throwaway schema.
		schemaName := fmt.Sprintf("earnings_test_%d", time.Now().UnixNano())
		require.NoError(t, db.Exec("CREATE SCHEMA ?", clause.Table{Name: schemaName}).Error)
		require.NoError(t, db.Exec("SET search_path TO ?", clause.Table{Name: schemaName}).Error)
		t.Cleanup(func() { assert.NoError(t, db.Exec("DROP SCHEMA ? CASCADE", clause.Table{Name: schemaName}).Error) })
	case "mysql":
		t.Cleanup(func() { assert.NoError(t, db.Migrator().DropTable(&Log{})) })
	}

	var version string
	versionSQL := "SELECT version()"
	if dialect == "sqlite" {
		versionSQL = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
	t.Logf("database version: %s %s", dialect, version)

	require.NoError(t, db.AutoMigrate(&Log{}))
	statement := &gorm.Statement{DB: db}
	require.NoError(t, statement.Parse(&Log{}))
	table := statement.Schema.Table
	if !withCostQuota {
		require.NoError(t, db.Exec("ALTER TABLE "+table+" DROP COLUMN cost_quota").Error)
	}
	insert := db
	if !withCostQuota {
		// What createLog does against a table without the column.
		insert = db.Omit("cost_quota")
	}
	for _, row := range earningsTestLogs {
		log := &Log{Type: row.logType, ChannelId: row.channelId, ModelName: row.modelName, Quota: row.quota, CreatedAt: row.createdAt, Username: "user-1"}
		require.NoError(t, insert.Create(log).Error)
		if withCostQuota && row.cost != 0 {
			require.NoError(t, db.Exec("UPDATE "+table+" SET cost_quota = ? WHERE id = ?", row.cost, log.Id).Error)
		}
	}

	previousLogDB := LOG_DB
	LOG_DB = db
	t.Cleanup(func() { LOG_DB = previousLogDB })
}

func TestEarningsDatabaseMatrix(t *testing.T) {
	window := EarningsFilter{StartTimestamp: earningsTestWindowStart, EndTimestamp: earningsTestWindowEnd}
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			t.Run("stock schema leaves cost to the estimate", func(t *testing.T) {
				openEarningsTestDB(t, dialect, false)
				assert.False(t, LogsRecordCostQuota())

				overview, err := GetEarningsOverview(window)
				require.NoError(t, err)
				assert.EqualValues(t, 2300, overview.Revenue, "consume quota inside the window")
				assert.EqualValues(t, 200, overview.Refund)
				assert.EqualValues(t, 0, overview.CostQuota)
				assert.EqualValues(t, 2300, overview.UnrecordedRevenue, "without the column every consume log is unrecorded")
				assert.EqualValues(t, 3, overview.RequestCount, "refunds, top-ups and errors are not requests")

				empty, err := GetEarningsOverview(EarningsFilter{StartTimestamp: earningsTestWindowStart - 100, EndTimestamp: earningsTestWindowStart - 50})
				require.NoError(t, err)
				assert.EqualValues(t, 0, empty.Revenue)
				assert.EqualValues(t, 0, empty.RequestCount)

				channels, total, err := GetEarningsByChannel(window, 100, 0)
				require.NoError(t, err)
				require.Len(t, channels, 2)
				assert.EqualValues(t, 2, total)
				assert.Equal(t, EarningsChannel{ChannelId: 1, Revenue: 1800, Refund: 200, UnrecordedRevenue: 1800, RequestCount: 2}, channels[0])
				assert.Equal(t, EarningsChannel{ChannelId: 2, Revenue: 500, Refund: 0, UnrecordedRevenue: 500, RequestCount: 1}, channels[1])

				models, total, err := GetEarningsByModel(window, 100, 0)
				require.NoError(t, err)
				require.Len(t, models, 3)
				assert.EqualValues(t, 3, total)
				assert.Equal(t, EarningsModel{ModelName: "gpt-4o", Revenue: 1200, Refund: 200, UnrecordedRevenue: 1200, RequestCount: 1}, models[0])
				assert.Equal(t, EarningsModel{ModelName: "gpt-4o-mini", Revenue: 600, UnrecordedRevenue: 600, RequestCount: 1}, models[1])
				assert.Equal(t, EarningsModel{ModelName: "claude-sonnet", Revenue: 500, UnrecordedRevenue: 500, RequestCount: 1}, models[2])

				// Paging keeps the group order and counts groups rather than rows.
				second, total, err := GetEarningsByChannel(window, 1, 1)
				require.NoError(t, err)
				require.Len(t, second, 1)
				assert.EqualValues(t, 2, total)
				assert.Equal(t, 2, second[0].ChannelId)
				beyond, _, err := GetEarningsByModel(window, 10, 10)
				require.NoError(t, err)
				assert.Empty(t, beyond)

				// A missing or negative page bound falls back to the safe defaults.
				defaulted, _, err := GetEarningsByChannel(window, 0, -5)
				require.NoError(t, err)
				assert.Len(t, defaulted, 2)
			})

			t.Run("recorded cost_quota is summed per group", func(t *testing.T) {
				openEarningsTestDB(t, dialect, true)
				assert.True(t, LogsRecordCostQuota())

				overview, err := GetEarningsOverview(window)
				require.NoError(t, err)
				assert.EqualValues(t, 1900, overview.CostQuota, "1000 + 500 + 400, outside-window and refund rows excluded")
				assert.EqualValues(t, 2300, overview.Revenue)
				assert.EqualValues(t, 0, overview.UnrecordedRevenue, "every consume log of the window carries its cost")

				channels, _, err := GetEarningsByChannel(window, 100, 0)
				require.NoError(t, err)
				require.Len(t, channels, 2)
				assert.EqualValues(t, 1500, channels[0].CostQuota)
				assert.EqualValues(t, 400, channels[1].CostQuota)
				assert.EqualValues(t, 0, channels[0].UnrecordedRevenue)
				assert.EqualValues(t, 0, channels[1].UnrecordedRevenue)

				models, _, err := GetEarningsByModel(window, 100, 0)
				require.NoError(t, err)
				require.Len(t, models, 3)
				assert.EqualValues(t, 1000, models[0].CostQuota)
				assert.EqualValues(t, 500, models[1].CostQuota)
				assert.EqualValues(t, 400, models[2].CostQuota)
			})

			// Old logs and channels without a cost ratio have no recorded cost.
			// They must stay out of the recorded sum and be reported as
			// unrecorded revenue for the caller to estimate, not read as free.
			t.Run("logs without a recorded cost are reported as unrecorded revenue", func(t *testing.T) {
				openEarningsTestDB(t, dialect, true)
				require.NoError(t, LOG_DB.Model(&Log{}).
					Where("model_name = ? AND type = ? AND created_at >= ? AND created_at <= ?", "gpt-4o-mini", LogTypeConsume, earningsTestWindowStart, earningsTestWindowEnd).
					Update("cost_quota", nil).Error)

				overview, err := GetEarningsOverview(window)
				require.NoError(t, err)
				assert.EqualValues(t, 2300, overview.Revenue)
				assert.EqualValues(t, 1400, overview.CostQuota, "1000 + 400, the unrecorded row adds nothing")
				assert.EqualValues(t, 600, overview.UnrecordedRevenue, "the gpt-4o-mini row")

				channels, _, err := GetEarningsByChannel(window, 100, 0)
				require.NoError(t, err)
				require.Len(t, channels, 2)
				assert.EqualValues(t, 1000, channels[0].CostQuota)
				assert.EqualValues(t, 600, channels[0].UnrecordedRevenue)
				assert.EqualValues(t, 400, channels[1].CostQuota)
				assert.EqualValues(t, 0, channels[1].UnrecordedRevenue)

				models, _, err := GetEarningsByModel(window, 100, 0)
				require.NoError(t, err)
				require.Len(t, models, 3)
				assert.EqualValues(t, 600, models[1].UnrecordedRevenue)
				assert.EqualValues(t, 0, models[1].CostQuota)
			})
		})
	}
}

func TestEstimateUpstreamCostRoundsHalfUp(t *testing.T) {
	for charged, want := range map[int64]int64{
		0:                 0,
		1:                 1, // 0.83
		3:                 3, // 2.5 rounds up
		4:                 3, // 3.33
		6:                 5,
		500:               417, // 416.67
		1200:              1000,
		2300:              1917,
		6_000_000_000_000: 5_000_000_000_000, // far beyond int32, still exact
	} {
		assert.Equal(t, want, EstimateUpstreamCost(charged), "charged %d", charged)
	}
}
