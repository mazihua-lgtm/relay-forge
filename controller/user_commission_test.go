package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// performUpdateUserCommissionRequest drives UpdateUserCommission the way
// AdminAuth hands a request over: the operator's id and role are on the context.
func performUpdateUserCommissionRequest(t *testing.T, role int, id string, body string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/user/"+id+"/commission", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Set("id", 9999)
	c.Set("role", role)
	c.Set("username", "root-operator")
	c.Set(common.RequestIdKey, "commission-test-request")
	UpdateUserCommission(c)

	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestUpdateUserCommission(t *testing.T) {
	db := setupManageUserTestDB(t)
	createQuotaTestOperator(t, db, common.RoleRootUser)

	newUser := func(name string, role int) model.User {
		t.Helper()
		user := model.User{Username: name, Password: "unused-password-hash", Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: name + "-aff"}
		require.NoError(t, db.Create(&user).Error)
		return user
	}
	rateOf := func(id int) float64 {
		t.Helper()
		var user model.User
		require.NoError(t, db.Select("id", "commission_rate").First(&user, "id = ?", id).Error)
		return user.CommissionRate
	}
	target := newUser("commission-target", common.RoleCommonUser)
	peerAdmin := newUser("commission-peer-admin", common.RoleAdminUser)
	targetID := strconv.Itoa(target.Id)

	t.Run("sets every rate from 0 to 1 and audits the change", func(t *testing.T) {
		for _, tc := range []struct {
			body string
			want float64
		}{
			{`{"commission_rate":0.1}`, 0.1},
			{`{"commission_rate":1}`, 1},
			{`{"commission_rate":0}`, 0},
			{`{"commission_rate":0.333}`, 0.333},
		} {
			response := performUpdateUserCommissionRequest(t, common.RoleRootUser, targetID, tc.body)
			require.Equal(t, true, response["success"], tc.body)
			assert.Equal(t, tc.want, rateOf(target.Id), tc.body)
		}

		var audit model.AuditLog
		require.NoError(t, db.Where("action = ?", "user.commission").Order("id").First(&audit).Error)
		assert.Equal(t, "Set commission rate of user commission-target (ID: "+targetID+") to 0.1", audit.Content)
	})

	t.Run("rejects an out of range or malformed rate and keeps the stored one", func(t *testing.T) {
		require.NoError(t, model.UpdateUserCommissionRate(target.Id, 0.25))
		for _, body := range []string{
			`{"commission_rate":-0.01}`,
			`{"commission_rate":1.01}`,
			`{"commission_rate":100}`,
			`{"commission_rate":"0.5"}`,
			`{"commission_rate":null}`,
			`{}`,
			`not json`,
			``,
		} {
			response := performUpdateUserCommissionRequest(t, common.RoleRootUser, targetID, body)
			assert.Equal(t, false, response["success"], "body %q", body)
			assert.Equal(t, 0.25, rateOf(target.Id), "body %q", body)
		}
	})

	t.Run("rejects an unusable or unknown user id", func(t *testing.T) {
		require.NoError(t, model.UpdateUserCommissionRate(target.Id, 0.25))
		for _, id := range []string{"abc", "0", "-1", "424242"} {
			response := performUpdateUserCommissionRequest(t, common.RoleRootUser, id, `{"commission_rate":0.5}`)
			assert.Equal(t, false, response["success"], "id %q", id)
		}
		assert.Equal(t, 0.25, rateOf(target.Id))
	})

	t.Run("an administrator may not touch a peer administrator, root may", func(t *testing.T) {
		require.NoError(t, model.UpdateUserCommissionRate(peerAdmin.Id, 0.25))
		peerID := strconv.Itoa(peerAdmin.Id)

		response := performUpdateUserCommissionRequest(t, common.RoleAdminUser, peerID, `{"commission_rate":0.9}`)
		assert.Equal(t, false, response["success"])
		assert.Equal(t, 0.25, rateOf(peerAdmin.Id))

		response = performUpdateUserCommissionRequest(t, common.RoleAdminUser, targetID, `{"commission_rate":0.9}`)
		assert.Equal(t, true, response["success"], "an administrator may set a common user's rate")
		assert.Equal(t, 0.9, rateOf(target.Id))

		response = performUpdateUserCommissionRequest(t, common.RoleRootUser, peerID, `{"commission_rate":0.9}`)
		assert.Equal(t, true, response["success"])
		assert.Equal(t, 0.9, rateOf(peerAdmin.Id))
	})
}
