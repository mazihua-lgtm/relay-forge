package model

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/constant"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConsumeCostQuota(t *testing.T) {
	tests := []struct {
		name       string
		quota      int
		groupRatio float64
		costRatio  float64
		wantCost   int
		wantOK     bool
	}{
		{"cost is the base price times the cost ratio", 1200, 1.2, 0.5, 500, true},
		{"a group ratio of 1 leaves the charge as the base price", 1000, 1, 0.4, 400, true},
		{"a discount group ratio does not change the operator's cost", 800, 0.8, 0.4, 400, true},
		{"the result rounds to a whole quota", 100, 3, 1, 33, true},
		{"a half rounds up", 101, 2, 1, 51, true},
		{"decimal arithmetic keeps 100 x 0.29 at 29, not 28", 100, 1, 0.29, 29, true},
		{"a loss-making channel costs more than it earns", 1000, 1, 1.5, 1500, true},
		{"a tiny charge can cost nothing", 1, 1, 0.2, 0, true},
		{"nothing charged derives nothing", 0, 1, 0.5, 0, false},
		{"a negative charge derives nothing", -5, 1, 0.5, 0, false},
		{"a free group hides the base price", 500, 0, 0.5, 0, false},
		{"a negative group ratio is unusable", 500, -1, 0.5, 0, false},
		{"a NaN group ratio is unusable", 500, math.NaN(), 0.5, 0, false},
		{"an infinite group ratio is unusable", 500, math.Inf(1), 0.5, 0, false},
		{"no cost ratio derives nothing", 500, 1, 0, 0, false},
		{"a negative cost ratio is unusable", 500, 1, -0.5, 0, false},
		{"a NaN cost ratio is unusable", 500, 1, math.NaN(), 0, false},
		{"a cost that does not fit the quota column is unknown", math.MaxInt32, 0.01, 1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, ok := ConsumeCostQuota(tt.quota, tt.groupRatio, tt.costRatio)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantCost, cost)
		})
	}
}

func TestValidateChannelCostRatio(t *testing.T) {
	value := func(v float64) *float64 { return &v }
	assert.NoError(t, ValidateChannelCostRatio(nil), "not sent")
	assert.NoError(t, ValidateChannelCostRatio(value(0)), "not set")
	assert.NoError(t, ValidateChannelCostRatio(value(0.4)))
	assert.NoError(t, ValidateChannelCostRatio(value(ChannelCostRatioMax)))
	assert.Error(t, ValidateChannelCostRatio(value(ChannelCostRatioMax+0.01)))
	assert.Error(t, ValidateChannelCostRatio(value(-0.1)))
	assert.Error(t, ValidateChannelCostRatio(value(math.NaN())))
	assert.Error(t, ValidateChannelCostRatio(value(math.Inf(1))))
}

func TestChannelGetCostRatio(t *testing.T) {
	value := func(v float64) *float64 { return &v }
	var missing *Channel
	assert.Equal(t, 0.0, missing.GetCostRatio())
	assert.Equal(t, 0.0, (&Channel{}).GetCostRatio(), "never set")
	assert.Equal(t, 0.4, (&Channel{CostRatio: value(0.4)}).GetCostRatio())
	assert.Equal(t, 0.0, (&Channel{CostRatio: value(-2)}).GetCostRatio(), "a stored negative is unusable")
	assert.Equal(t, 0.0, (&Channel{CostRatio: value(math.NaN())}).GetCostRatio())
}

func TestCostQuotaFromOther(t *testing.T) {
	ratioOf := func(groupRatio any) *LogOther {
		other := NewLogOther()
		other.SetPublic("group_ratio", groupRatio)
		return other
	}
	cost := func(p *int) any {
		if p == nil {
			return nil
		}
		return *p
	}

	assert.Equal(t, 500, cost(costQuotaFromOther(1200, ratioOf(1.2), 0.5)))
	assert.Equal(t, 400, cost(costQuotaFromOther(800, ratioOf(float32(0.8)), 0.4)), "float32 group ratio")
	assert.Equal(t, 500, cost(costQuotaFromOther(1000, ratioOf(2), 1)), "int group ratio")
	assert.Equal(t, 500, cost(costQuotaFromOther(1000, ratioOf(int64(2)), 1)), "int64 group ratio")

	assert.Nil(t, costQuotaFromOther(1200, ratioOf(1.2), 0), "no cost ratio on the channel")
	assert.Nil(t, costQuotaFromOther(1200, NewLogOther(), 0.5), "the log has no group ratio")
	assert.Nil(t, costQuotaFromOther(1200, nil, 0.5), "the log has no other info at all")
	assert.Nil(t, costQuotaFromOther(1200, ratioOf("1.2"), 0.5), "a group ratio of an unexpected type")
	assert.Nil(t, costQuotaFromOther(1200, ratioOf(0.0), 0.5), "a free group")
}

// openLogCostTestDB installs an isolated database with the logs and channels
// tables, and consume logging on.
func openLogCostTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Log{}, &Channel{}))

	previousDB, previousLogDB := DB, LOG_DB
	previousConsume, previousCache := common.LogConsumeEnabled, common.MemoryCacheEnabled
	DB, LOG_DB = db, db
	common.LogConsumeEnabled = true
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.LogConsumeEnabled, common.MemoryCacheEnabled = previousConsume, previousCache
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func logCostTestContext(costRatio *float64) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if costRatio != nil {
		common.SetContextKey(c, constant.ContextKeyChannelCostRatio, *costRatio)
	}
	return c
}

func storedConsumeLogs(t *testing.T, db *gorm.DB) []Log {
	t.Helper()
	var logs []Log
	require.NoError(t, db.Order("id").Find(&logs).Error)
	return logs
}

func TestRecordConsumeLogSnapshotsTheUpstreamCost(t *testing.T) {
	ratio := 0.5
	billed := func(groupRatio float64) *LogOther {
		other := NewLogOther()
		other.SetPublic("group_ratio", groupRatio)
		return other
	}

	t.Run("a channel with a cost ratio records the cost", func(t *testing.T) {
		db := openLogCostTestDB(t)
		RecordConsumeLog(logCostTestContext(&ratio), 1, RecordConsumeLogParams{ChannelId: 3, ModelName: "gpt-4o", Quota: 1200, Other: billed(1.2)})

		logs := storedConsumeLogs(t, db)
		require.Len(t, logs, 1)
		require.NotNil(t, logs[0].CostQuota)
		assert.Equal(t, 500, *logs[0].CostQuota, "1200 / 1.2 * 0.5")
		assert.Equal(t, 1200, logs[0].Quota, "what the user paid is untouched")
	})

	t.Run("a channel without a cost ratio leaves the cost unknown", func(t *testing.T) {
		db := openLogCostTestDB(t)
		RecordConsumeLog(logCostTestContext(nil), 1, RecordConsumeLogParams{ChannelId: 3, ModelName: "gpt-4o", Quota: 1200, Other: billed(1.2)})

		logs := storedConsumeLogs(t, db)
		require.Len(t, logs, 1)
		assert.Nil(t, logs[0].CostQuota)
	})

	t.Run("a log that did not record its group ratio leaves the cost unknown", func(t *testing.T) {
		db := openLogCostTestDB(t)
		RecordConsumeLog(logCostTestContext(&ratio), 1, RecordConsumeLogParams{ChannelId: 3, ModelName: "gpt-4o", Quota: 1200, Other: NewLogOther()})

		logs := storedConsumeLogs(t, db)
		require.Len(t, logs, 1)
		assert.Nil(t, logs[0].CostQuota)
	})

	// The cost column is optional: a table that does not have it (a ClickHouse
	// log store, or a node that started before the master migrated the schema)
	// must keep accepting consume logs. Losing a billing record over an optional
	// column would be far worse than not knowing the cost.
	t.Run("a log table without the cost column still records the consume log", func(t *testing.T) {
		db := openLogCostTestDB(t)
		require.NoError(t, db.Exec("ALTER TABLE logs DROP COLUMN cost_quota").Error)
		assert.False(t, LogsRecordCostQuota())

		RecordConsumeLog(logCostTestContext(&ratio), 1, RecordConsumeLogParams{ChannelId: 3, ModelName: "gpt-4o", Quota: 1200, Other: billed(1.2)})

		var count int64
		require.NoError(t, db.Model(&Log{}).Omit("cost_quota").Count(&count).Error)
		assert.EqualValues(t, 1, count, "the consume log is stored")
	})
}

// The cost is the operator's margin data. It must never reach a user, and the
// self-service log APIs return Log as JSON.
func TestLogCostQuotaNeverLeavesTheServer(t *testing.T) {
	cost := 500
	raw, err := json.Marshal(Log{Quota: 1200, CostQuota: &cost})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "cost")
	assert.Contains(t, string(raw), `"quota":1200`)
}

func TestRecordTaskBillingLogSnapshotsTheChannelCost(t *testing.T) {
	billed := func(groupRatio float64) *LogOther {
		other := NewLogOther()
		other.SetPublic("group_ratio", groupRatio)
		return other
	}
	ratio := 0.4
	newChannel := func(t *testing.T, db *gorm.DB, costRatio *float64) int {
		t.Helper()
		channel := &Channel{Name: "video", Type: 1, Key: "sk", Status: common.ChannelStatusEnabled, CostRatio: costRatio}
		require.NoError(t, db.Create(channel).Error)
		return channel.Id
	}

	t.Run("a consume log takes the cost from the task's channel", func(t *testing.T) {
		db := openLogCostTestDB(t)
		channelId := newChannel(t, db, &ratio)
		RecordTaskBillingLog(RecordTaskBillingLogParams{
			UserId: 1, LogType: LogTypeConsume, ChannelId: channelId, ModelName: "video-1", Quota: 1000, Other: billed(2),
		})

		logs := storedConsumeLogs(t, db)
		require.Len(t, logs, 1)
		require.NotNil(t, logs[0].CostQuota)
		assert.Equal(t, 200, *logs[0].CostQuota, "1000 / 2 * 0.4")
	})

	t.Run("a channel without a ratio leaves the cost unknown", func(t *testing.T) {
		db := openLogCostTestDB(t)
		channelId := newChannel(t, db, nil)
		RecordTaskBillingLog(RecordTaskBillingLogParams{
			UserId: 1, LogType: LogTypeConsume, ChannelId: channelId, ModelName: "video-1", Quota: 1000, Other: billed(2),
		})

		logs := storedConsumeLogs(t, db)
		require.Len(t, logs, 1)
		assert.Nil(t, logs[0].CostQuota)
	})

	t.Run("a refund never carries a cost", func(t *testing.T) {
		db := openLogCostTestDB(t)
		channelId := newChannel(t, db, &ratio)
		RecordTaskBillingLog(RecordTaskBillingLogParams{
			UserId: 1, LogType: LogTypeRefund, ChannelId: channelId, ModelName: "video-1", Quota: 1000, Other: billed(2),
		})

		logs := storedConsumeLogs(t, db)
		require.Len(t, logs, 1)
		assert.Nil(t, logs[0].CostQuota)
	})

	t.Run("a deleted channel is an unknown cost, not an error", func(t *testing.T) {
		db := openLogCostTestDB(t)
		RecordTaskBillingLog(RecordTaskBillingLogParams{
			UserId: 1, LogType: LogTypeConsume, ChannelId: 999, ModelName: "video-1", Quota: 1000, Other: billed(2),
		})

		logs := storedConsumeLogs(t, db)
		require.Len(t, logs, 1)
		assert.Nil(t, logs[0].CostQuota)
	})
}

func TestGetChannelCostRatio(t *testing.T) {
	db := openLogCostTestDB(t)
	ratio := 0.4
	channel := &Channel{Name: "priced", Type: 1, Key: "sk", Status: common.ChannelStatusEnabled, CostRatio: &ratio}
	require.NoError(t, db.Create(channel).Error)
	plain := &Channel{Name: "plain", Type: 1, Key: "sk", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(plain).Error)

	assert.Equal(t, 0.4, GetChannelCostRatio(channel.Id))
	assert.Equal(t, 0.0, GetChannelCostRatio(plain.Id), "default 0 means not set")
	assert.Equal(t, 0.0, GetChannelCostRatio(999999), "a missing channel")
	assert.Equal(t, 0.0, GetChannelCostRatio(0))
	assert.Equal(t, 0.0, GetChannelCostRatio(-1))
}

// A node that starts before the master has migrated the schema sees no cost
// column. It must notice the column once it exists, not only after a restart.
func TestLogsRecordCostQuotaNoticesAColumnAddedLater(t *testing.T) {
	db := openLogCostTestDB(t)
	require.NoError(t, db.Exec("ALTER TABLE logs DROP COLUMN cost_quota").Error)
	assert.False(t, LogsRecordCostQuota(), "the table has no cost column yet")

	require.NoError(t, db.Exec("ALTER TABLE logs ADD COLUMN cost_quota integer").Error)
	assert.False(t, LogsRecordCostQuota(), "a recent 'no' is trusted until the retry window passes")

	// Age the remembered answer past the retry window, as a minute would.
	state := logCostProbeState.Load()
	require.NotNil(t, state)
	logCostProbeState.Store(&logCostProbe{db: state.db, recorded: false, checkedAt: time.Now().Add(-2 * logCostProbeRetry)})
	assert.True(t, LogsRecordCostQuota(), "after the retry window the new column is noticed")

	// A 'yes' is kept without asking again.
	assert.True(t, LogsRecordCostQuota())
}
