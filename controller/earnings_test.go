package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var earningsTestDay = time.Date(2026, 11, 10, 0, 0, 0, 0, time.Local)

// setupEarningsControllerTest installs an isolated log database plus a minimal
// channels table, and disables the channel memory cache so names come from the
// database the way they do on a restarted node.
func setupEarningsControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousCache := common.MemoryCacheEnabled

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, db.Exec("CREATE TABLE channels (id integer primary key, name varchar(255))").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, name) VALUES (1, 'openai-main'), (2, 'claude-main')").Error)

	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.MemoryCacheEnabled = previousCache
		assert.NoError(t, sqlDB.Close())
	})
	return db
}

func seedEarningsControllerLogs(t *testing.T, db *gorm.DB) {
	t.Helper()
	at := func(day int, hour int) int64 {
		return earningsTestDay.AddDate(0, 0, day).Add(time.Duration(hour) * time.Hour).Unix()
	}
	logs := []*model.Log{
		{Type: model.LogTypeConsume, ChannelId: 1, ModelName: "gpt-4o", Quota: 1200, CreatedAt: at(0, 9)},
		{Type: model.LogTypeConsume, ChannelId: 1, ModelName: "gpt-4o-mini", Quota: 600, CreatedAt: at(0, 10)},
		{Type: model.LogTypeConsume, ChannelId: 2, ModelName: "claude-sonnet", Quota: 500, CreatedAt: at(0, 11)},
		{Type: model.LogTypeRefund, ChannelId: 1, ModelName: "gpt-4o", Quota: 200, CreatedAt: at(0, 12)},
		// The next day stays out of a one-day window.
		{Type: model.LogTypeConsume, ChannelId: 1, ModelName: "gpt-4o", Quota: 5000, CreatedAt: at(1, 9)},
	}
	for _, log := range logs {
		require.NoError(t, db.Create(log).Error)
	}
}

// requestEarnings runs one handler and returns the decoded data payload.
func requestEarnings(t *testing.T, handler gin.HandlerFunc, query string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/earnings", handler)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/earnings"+query, nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	if success, _ := body["success"].(bool); !success {
		t.Fatalf("unexpected failure response: %s", recorder.Body.String())
	}
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "response data must be an object: %s", recorder.Body.String())
	return data
}

func requestEarningsFailure(t *testing.T, handler gin.HandlerFunc, query string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/earnings", handler)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/earnings"+query, nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, false, body["success"])
	return body
}

func TestGetEarningsOverviewReportsEstimatedProfit(t *testing.T) {
	db := setupEarningsControllerTest(t)
	seedEarningsControllerLogs(t, db)

	data := requestEarnings(t, GetEarningsOverview, "?start_date=2026-11-10&end_date=2026-11-10")

	assert.EqualValues(t, 2100, data["total_revenue"], "2300 charged - 200 refunded")
	assert.EqualValues(t, 1917, data["total_cost"], "round(2300 / 1.2)")
	assert.EqualValues(t, 183, data["gross_profit"])
	assert.InDelta(t, 0.0871, data["gross_margin"], 1e-9)
	assert.EqualValues(t, 3, data["request_count"])
	assert.EqualValues(t, 200, data["refund_quota"])
	assert.Equal(t, "estimated", data["cost_source"])
	assert.InDelta(t, 1.2, data["cost_ratio"], 1e-9)

	dateRange, ok := data["date_range"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2026-11-10", dateRange["start"])
	assert.Equal(t, "2026-11-10", dateRange["end"])
}

func TestGetEarningsOverviewDefaultsToLastThirtyDays(t *testing.T) {
	setupEarningsControllerTest(t)

	data := requestEarnings(t, GetEarningsOverview, "")

	dateRange, ok := data["date_range"].(map[string]any)
	require.True(t, ok)
	today := time.Now()
	assert.Equal(t, today.Format("2006-01-02"), dateRange["end"])
	assert.Equal(t, today.AddDate(0, 0, -29).Format("2006-01-02"), dateRange["start"])
	assert.EqualValues(t, 0, data["total_revenue"])
	assert.EqualValues(t, 0, data["gross_margin"], "an empty window must not divide by zero")
	assert.EqualValues(t, 0, data["total_cost"])
}

func TestGetEarningsOverviewRejectsInvalidDates(t *testing.T) {
	setupEarningsControllerTest(t)

	body := requestEarningsFailure(t, GetEarningsOverview, "?start_date=2026/11/01")
	assert.Contains(t, body["message"], "start_date")

	body = requestEarningsFailure(t, GetEarningsOverview, "?end_date=yesterday")
	assert.Contains(t, body["message"], "end_date")

	body = requestEarningsFailure(t, GetEarningsOverview, "?start_date=2026-11-10&end_date=2026-11-01")
	assert.Contains(t, body["message"], "start_date")
}

func TestGetEarningsByChannelNamesAndFigures(t *testing.T) {
	db := setupEarningsControllerTest(t)
	seedEarningsControllerLogs(t, db)

	data := requestEarnings(t, GetEarningsByChannel, "?start_date=2026-11-10&end_date=2026-11-10")
	assert.EqualValues(t, 2, data["total"])
	assert.EqualValues(t, 1, data["page"])
	assert.EqualValues(t, 100, data["page_size"])

	items, ok := data["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)

	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 1, first["channel_id"])
	assert.Equal(t, "openai-main", first["channel_name"])
	assert.EqualValues(t, 1600, first["total_revenue"], "1800 charged - 200 refunded")
	assert.EqualValues(t, 1500, first["total_cost"], "round(1800 / 1.2)")
	assert.EqualValues(t, 100, first["gross_profit"])
	assert.EqualValues(t, 2, first["request_count"])

	second, ok := items[1].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 2, second["channel_id"])
	assert.Equal(t, "claude-main", second["channel_name"])
	assert.EqualValues(t, 500, second["total_revenue"])
	assert.EqualValues(t, 417, second["total_cost"], "round(500 / 1.2)")
	assert.EqualValues(t, 83, second["gross_profit"])
}

func TestGetEarningsByChannelPaginatesAndClampsPageSize(t *testing.T) {
	db := setupEarningsControllerTest(t)
	seedEarningsControllerLogs(t, db)

	data := requestEarnings(t, GetEarningsByChannel, "?start_date=2026-11-10&end_date=2026-11-10&p=2&page_size=1")
	assert.EqualValues(t, 2, data["total"], "total counts channels, not rows")
	assert.EqualValues(t, 2, data["page"])
	assert.EqualValues(t, 1, data["page_size"])
	items, ok := data["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.EqualValues(t, 2, items[0].(map[string]any)["channel_id"])

	clamped := requestEarnings(t, GetEarningsByChannel, "?start_date=2026-11-10&end_date=2026-11-10&page_size=10000")
	assert.EqualValues(t, model.EarningsMaxLimit, clamped["page_size"])
}

func TestGetEarningsByModelGroupsByName(t *testing.T) {
	db := setupEarningsControllerTest(t)
	seedEarningsControllerLogs(t, db)

	data := requestEarnings(t, GetEarningsByModel, "?start_date=2026-11-10&end_date=2026-11-10")
	assert.EqualValues(t, 3, data["total"])

	items, ok := data["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 3)

	first := items[0].(map[string]any)
	assert.Equal(t, "gpt-4o", first["model_name"])
	assert.EqualValues(t, 1000, first["total_revenue"], "1200 charged - 200 refunded")
	assert.EqualValues(t, 1000, first["total_cost"], "round(1200 / 1.2)")
	assert.EqualValues(t, 0, first["gross_profit"])

	second := items[1].(map[string]any)
	assert.Equal(t, "gpt-4o-mini", second["model_name"])
	assert.EqualValues(t, 600, second["total_revenue"])
	assert.EqualValues(t, 500, second["total_cost"])

	third := items[2].(map[string]any)
	assert.Equal(t, "claude-sonnet", third["model_name"])
	assert.EqualValues(t, 500, third["total_revenue"])
}

func TestEarningsWindowIncludesWholeEndDay(t *testing.T) {
	db := setupEarningsControllerTest(t)
	seedEarningsControllerLogs(t, db)

	// The last request of 2026-11-10 is a refund at 12:00; the next-day consume
	// at 09:00 must stay outside.
	data := requestEarnings(t, GetEarningsOverview, "?start_date=2026-11-10&end_date=2026-11-10")
	assert.EqualValues(t, 3, data["request_count"], "all three consumes of the end date are inside the window")
	assert.EqualValues(t, 200, data["refund_quota"], "the 12:00 refund is the last request of the day and must be counted")

	nextDay := requestEarnings(t, GetEarningsOverview, "?start_date=2026-11-11&end_date=2026-11-11")
	assert.EqualValues(t, 5000, nextDay["total_revenue"])
	assert.EqualValues(t, 1, nextDay["request_count"])
}
