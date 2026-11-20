package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/i18n"
	"github.com/mazihua-lgtm/relay-forge/model"
	"github.com/mazihua-lgtm/relay-forge/service"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCommissionRouterTestDB(t *testing.T) {
	t.Helper()
	require.NoError(t, i18n.Init())
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousType, previousRedis, previousSecret := common.MainDatabaseType(), common.RedisEnabled, common.SessionSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuditLog{}, &model.UserAccessToken{}, &model.Option{}))
	model.DB, model.LOG_DB = db, db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.SessionSecret = "commission-router-test-secret"
	require.NoError(t, model.EnsureLegacyAccessTokenRetireAt(time.Now().Unix()))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled, common.SessionSecret = previousRedis, previousSecret
		_ = sqlDB.Close()
	})
}

// The commission rate is an administrator-only write. Through the real router
// and the real AdminAuth chain, an anonymous caller and a signed-in common user
// are both turned away and nothing is written; an administrator gets through.
func TestUserCommissionRouteRequiresAdmin(t *testing.T) {
	setupCommissionRouterTestDB(t)
	engine := newAccessTokenScopeTestEngine(t)

	newUser := func(name string, role int) model.User {
		t.Helper()
		user := model.User{Username: name, Password: "password-placeholder", Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: name + "-aff"}
		require.NoError(t, model.DB.Create(&user).Error)
		return user
	}
	commonUser := newUser("commission-common", common.RoleCommonUser)
	admin := newUser("commission-admin", common.RoleAdminUser)
	target := newUser("commission-route-target", common.RoleCommonUser)

	bearer := func(user model.User) string {
		t.Helper()
		bundle, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "commission-router-test")
		require.NoError(t, err)
		return bundle.AccessToken
	}
	patch := func(token string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPatch, "/api/user/"+strconv.Itoa(target.Id)+"/commission", strings.NewReader(`{"commission_rate":0.1}`))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		return recorder
	}
	rateOfTarget := func() float64 {
		t.Helper()
		var got model.User
		require.NoError(t, model.DB.Select("id", "commission_rate").First(&got, target.Id).Error)
		return got.CommissionRate
	}

	registered := false
	for _, route := range engine.Routes() {
		if route.Method == http.MethodPatch && route.Path == "/api/user/:id/commission" {
			registered = true
		}
	}
	require.True(t, registered, "PATCH /api/user/:id/commission must be registered")

	recorder := patch("")
	assert.Equal(t, http.StatusUnauthorized, recorder.Code, "anonymous: %s", recorder.Body.String())
	assert.Zero(t, rateOfTarget())

	recorder = patch(bearer(commonUser))
	assert.Equal(t, http.StatusForbidden, recorder.Code, "common user: %s", recorder.Body.String())
	assert.Zero(t, rateOfTarget())

	recorder = patch(bearer(admin))
	require.Equal(t, http.StatusOK, recorder.Code, "administrator: %s", recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Equal(t, 0.1, rateOfTarget())
}
