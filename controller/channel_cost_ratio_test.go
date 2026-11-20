package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// putChannel sends one channel update the way the admin console does and
// returns the decoded response.
func putChannel(t *testing.T, role int, body string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 1)
	context.Set("role", role)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/channel", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(context)
	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response
}

func newCostRatioTestChannel(t *testing.T) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Type: 1, Status: common.ChannelStatusEnabled, Name: "priced", Key: "sk-test",
		Models: "gpt-4o", Group: "default",
	}
	require.NoError(t, channel.Insert())
	return channel
}

func storedCostRatio(t *testing.T, id int) float64 {
	t.Helper()
	stored, err := model.GetChannelById(id, true)
	require.NoError(t, err)
	return stored.GetCostRatio()
}

// updateBody is a full channel update; the console always sends the whole
// channel, so a partial body would exercise a path the console never takes.
func updateBody(id int, name string, extra string) string {
	return fmt.Sprintf(`{"id":%d,"type":1,"name":%q,"key":"sk-test","models":"gpt-4o","group":"default"%s}`, id, name, extra)
}

func TestUpdateChannelPersistsCostRatioAndCanClearIt(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	channel := newCostRatioTestChannel(t)
	assert.Equal(t, 0.0, storedCostRatio(t, channel.Id), "a new channel has no ratio")

	response := putChannel(t, common.RoleRootUser, updateBody(channel.Id, "priced", `,"cost_ratio":0.4`))
	require.Equal(t, true, response["success"], response["message"])
	assert.Equal(t, 0.4, storedCostRatio(t, channel.Id))

	// Leaving the field out of a later edit keeps the price.
	response = putChannel(t, common.RoleRootUser, updateBody(channel.Id, "renamed", ""))
	require.Equal(t, true, response["success"], response["message"])
	assert.Equal(t, 0.4, storedCostRatio(t, channel.Id), "an edit that does not send cost_ratio must not clear it")

	// Sending 0 clears it back to "not set" (a plain struct update would skip it).
	response = putChannel(t, common.RoleRootUser, updateBody(channel.Id, "renamed", `,"cost_ratio":0`))
	require.Equal(t, true, response["success"], response["message"])
	assert.Equal(t, 0.0, storedCostRatio(t, channel.Id))
}

func TestUpdateChannelRejectsAnUnusableCostRatio(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	channel := newCostRatioTestChannel(t)
	response := putChannel(t, common.RoleRootUser, updateBody(channel.Id, "priced", `,"cost_ratio":0.4`))
	require.Equal(t, true, response["success"], response["message"])

	for _, bad := range []string{"-0.1", "100.5", "1e9"} {
		response = putChannel(t, common.RoleRootUser, updateBody(channel.Id, "priced", `,"cost_ratio":`+bad))
		assert.Equal(t, false, response["success"], "cost_ratio %s must be rejected", bad)
		assert.Contains(t, fmt.Sprint(response["message"]), "cost_ratio")
		assert.Equal(t, 0.4, storedCostRatio(t, channel.Id), "a rejected edit leaves the saved ratio alone")
	}
}

func TestAddChannelStoresCostRatio(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	recorder := postAddChannel(t, 1, common.RoleRootUser,
		`{"mode":"single","channel":{"type":1,"name":"new-priced","key":"sk-new","models":"gpt-4o","group":"default","cost_ratio":0.35}}`)
	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	require.Equal(t, true, response["success"], response["message"])

	var channel model.Channel
	require.NoError(t, model.DB.Where("name = ?", "new-priced").First(&channel).Error)
	assert.Equal(t, 0.35, channel.GetCostRatio())

	recorder = postAddChannel(t, 1, common.RoleRootUser,
		`{"mode":"single","channel":{"type":1,"name":"bad-priced","key":"sk-bad","models":"gpt-4o","group":"default","cost_ratio":-1}}`)
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	assert.Equal(t, false, response["success"])
	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("name = ?", "bad-priced").Count(&count).Error)
	assert.EqualValues(t, 0, count, "a rejected channel is not created")
}

// The cost ratio only feeds reporting, so an admin who may edit channels but may
// not touch keys and URLs can still price them.
func TestCostRatioIsNotASensitiveChannelField(t *testing.T) {
	ratio := 0.4
	origin := &model.Channel{Type: 1, Key: "old-key", Models: "gpt-4o", Group: "default"}
	updated := PatchChannel{Channel: *origin}
	updated.CostRatio = &ratio

	assert.False(t, channelHasSensitiveChanges(&updated, origin, map[string]any{"id": 1, "cost_ratio": ratio}))
}
