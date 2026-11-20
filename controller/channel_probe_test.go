package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mazihua-lgtm/relay-forge/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The probe endpoint answers 202 at once; the sweep itself runs in the
// background. Without a database no sweep starts, which keeps this test from
// touching a database another test may have left behind.
func TestTriggerHealthProbeAnswersAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	model.DB = nil
	t.Cleanup(func() { model.DB = previousDB })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/probe", nil)

	TriggerHealthProbe(c)

	assert.Equal(t, http.StatusAccepted, recorder.Code)
	var body struct {
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.True(t, body.Success)
}
