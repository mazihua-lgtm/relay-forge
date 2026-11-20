package service

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/model"
	"github.com/mazihua-lgtm/relay-forge/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setProbeOperatorPolicy installs the operator's auto-disable and auto-enable
// switches plus the default-shaped rules for one test, and restores whatever
// was there before.
func setProbeOperatorPolicy(t *testing.T, autoDisable bool, autoEnable bool) {
	t.Helper()
	previousDisable := common.AutomaticDisableChannelEnabled
	previousEnable := common.AutomaticEnableChannelEnabled
	previousRanges := operation_setting.AutomaticDisableStatusCodeRanges
	previousKeywords := operation_setting.AutomaticDisableKeywords
	common.AutomaticDisableChannelEnabled = autoDisable
	common.AutomaticEnableChannelEnabled = autoEnable
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: http.StatusUnauthorized, End: http.StatusUnauthorized},
	}
	operation_setting.AutomaticDisableKeywords = []string{
		"Your credit balance is too low",
		"This organization has been disabled.",
	}
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = previousDisable
		common.AutomaticEnableChannelEnabled = previousEnable
		operation_setting.AutomaticDisableStatusCodeRanges = previousRanges
		operation_setting.AutomaticDisableKeywords = previousKeywords
	})
}

func failedProbeAnswer(status int, body string) healthProbeOutcome {
	return healthProbeOutcome{
		verdict:    healthProbeVerdictFailed,
		detail:     fmt.Sprintf("HTTP %d: %s", status, body),
		statusCode: status,
		errorText:  body,
	}
}

func TestHealthProbeShouldDisable(t *testing.T) {
	timeout := healthProbeOutcome{verdict: healthProbeVerdictFailed, detail: "context deadline exceeded", noResponse: true}
	tests := []struct {
		name        string
		autoDisable bool
		outcome     healthProbeOutcome
		want        bool
	}{
		{"a status code the operator configured disables", true, failedProbeAnswer(401, "invalid api key"), true},
		{"a rate limit is inconclusive", true, failedProbeAnswer(429, "rate limited"), false},
		{"an overloaded upstream is inconclusive", true, failedProbeAnswer(529, "overloaded"), false},
		{"a plain 5xx is inconclusive", true, failedProbeAnswer(500, "internal error"), false},
		{"a gateway error is inconclusive", true, failedProbeAnswer(502, "bad gateway"), false},
		{"a timeout is inconclusive", true, timeout, false},
		{"an answer without a status code is inconclusive", true, healthProbeOutcome{verdict: healthProbeVerdictFailed, detail: "build request"}, false},
		{"a configured keyword in the body disables", true, failedProbeAnswer(400, `{"error":{"message":"Your credit balance is too low to access the API"}}`), true},
		{"keywords match regardless of case", true, failedProbeAnswer(400, "THIS ORGANIZATION HAS BEEN DISABLED."), true},
		{"unrelated error text does not disable", true, failedProbeAnswer(400, `{"error":{"message":"max_tokens is too small"}}`), false},
		{"the global switch off disables nothing even for a configured status", false, failedProbeAnswer(401, "invalid api key"), false},
		{"the global switch off disables nothing even for a keyword", false, failedProbeAnswer(400, "Your credit balance is too low"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setProbeOperatorPolicy(t, tt.autoDisable, false)
			assert.Equal(t, tt.want, healthProbeShouldDisable(tt.outcome))
		})
	}
}

func TestHealthProbeHead(t *testing.T) {
	assert.Equal(t, "", healthProbeHead(nil, 8))
	assert.Equal(t, "abc", healthProbeHead([]byte("abc"), 8))
	assert.Equal(t, "abcd", healthProbeHead([]byte("abcdefgh"), 4))
}

func createProbePolicyChannel(t *testing.T, status int, autoBan int) *model.Channel {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}))
	channel := &model.Channel{
		Name: "probe-policy", Type: 1, Key: "sk-test", Status: status,
		Models: "gpt-4o-mini", Group: "default", AutoBan: &autoBan,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: "gpt-4o-mini", ChannelId: channel.Id,
		Enabled: status == common.ChannelStatusEnabled,
	}).Error)
	t.Cleanup(func() {
		model.DB.Where("channel_id = ?", channel.Id).Delete(&model.Ability{})
		model.DB.Delete(&model.Channel{}, channel.Id)
	})
	return channel
}

// TestApplyHealthProbeResultsFollowsOperatorPolicy runs one verdict through the
// real status update and checks the stored channel. The probe must not change
// routing the operator did not opt into, and must never take a channel out of
// rotation because of a failure one probe cannot tell from a busy upstream.
func TestApplyHealthProbeResultsFollowsOperatorPolicy(t *testing.T) {
	healthy := healthProbeOutcome{verdict: healthProbeVerdictHealthy}
	degraded := healthProbeOutcome{verdict: healthProbeVerdictDegraded, detail: "empty or evasive response: sorry"}
	timeout := healthProbeOutcome{verdict: healthProbeVerdictFailed, detail: "context deadline exceeded", noResponse: true}

	tests := []struct {
		name        string
		autoDisable bool
		autoEnable  bool
		autoBan     int
		start       int
		outcome     healthProbeOutcome
		wantStatus  int
		wantErrors  int
	}{
		{"a configured status disables the channel", true, false, 1, common.ChannelStatusEnabled, failedProbeAnswer(401, "invalid api key"), common.ChannelStatusAutoDisabled, 1},
		{"a configured keyword disables the channel", true, false, 1, common.ChannelStatusEnabled, failedProbeAnswer(400, "Your credit balance is too low"), common.ChannelStatusAutoDisabled, 1},
		{"a rate limit keeps the channel in rotation but is counted", true, false, 1, common.ChannelStatusEnabled, failedProbeAnswer(429, "rate limited"), common.ChannelStatusEnabled, 1},
		{"an overloaded upstream keeps the channel in rotation", true, false, 1, common.ChannelStatusEnabled, failedProbeAnswer(529, "overloaded"), common.ChannelStatusEnabled, 1},
		{"a timeout keeps the channel in rotation", true, false, 1, common.ChannelStatusEnabled, timeout, common.ChannelStatusEnabled, 1},
		{"auto-disable off: a configured status changes nothing", false, false, 1, common.ChannelStatusEnabled, failedProbeAnswer(401, "invalid api key"), common.ChannelStatusEnabled, 1},
		{"the channel opted out of auto-ban", true, false, 0, common.ChannelStatusEnabled, failedProbeAnswer(401, "invalid api key"), common.ChannelStatusEnabled, 1},
		{"a degraded answer marks the channel degraded", true, false, 1, common.ChannelStatusEnabled, degraded, common.ChannelStatusDegraded, 0},
		{"auto-disable off: a degraded answer changes nothing", false, false, 1, common.ChannelStatusEnabled, degraded, common.ChannelStatusEnabled, 0},
		{"a degraded answer respects the channel's auto-ban opt-out", true, false, 0, common.ChannelStatusEnabled, degraded, common.ChannelStatusEnabled, 0},
		{"auto-enable on: a healthy answer restores an auto-disabled channel", false, true, 1, common.ChannelStatusAutoDisabled, healthy, common.ChannelStatusEnabled, 0},
		{"auto-enable on: a healthy answer restores a degraded channel", false, true, 1, common.ChannelStatusDegraded, healthy, common.ChannelStatusEnabled, 0},
		{"auto-enable off: a healthy answer leaves it auto-disabled", false, false, 1, common.ChannelStatusAutoDisabled, healthy, common.ChannelStatusAutoDisabled, 0},
		{"a manual disable always wins over a healthy answer", false, true, 1, common.ChannelStatusManuallyDisabled, healthy, common.ChannelStatusManuallyDisabled, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setProbeOperatorPolicy(t, tt.autoDisable, tt.autoEnable)
			channel := createProbePolicyChannel(t, tt.start, tt.autoBan)

			applyHealthProbeResults([]healthProbeAccountResult{{
				channel: channel, probeModel: "gpt-4o-mini", outcome: tt.outcome,
			}})

			var stored model.Channel
			require.NoError(t, model.DB.First(&stored, "id = ?", channel.Id).Error)
			assert.Equal(t, tt.wantStatus, stored.Status)
			assert.Equal(t, tt.wantErrors, stored.ErrorCount)
		})
	}
}
