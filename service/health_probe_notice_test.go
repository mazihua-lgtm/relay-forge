package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/constant"
	"github.com/mazihua-lgtm/relay-forge/model"
	"github.com/mazihua-lgtm/relay-forge/relaykit/dto"
	"github.com/mazihua-lgtm/relay-forge/setting/system_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// probeNoticeFixture is an in-memory database with one administrator (root, who
// never chose, so receives channel notices) whose webhook records every notice
// as "<type> <title>", in the order they arrive.
type probeNoticeFixture struct {
	database *gorm.DB
	mutex    sync.Mutex
	notices  []string
}

func newProbeNoticeFixture(t *testing.T) *probeNoticeFixture {
	t.Helper()
	previousDB := model.DB
	previousCache, previousRedis := common.MemoryCacheEnabled, common.RedisEnabled
	previousNotifyLimit := constant.NotifyLimitCount
	previousClient, previousWorker := httpClient, system_setting.WorkerUrl
	fetch := system_setting.GetFetchSetting()
	previousFetch := *fetch
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled, common.RedisEnabled = previousCache, previousRedis
		constant.NotifyLimitCount = previousNotifyLimit
		httpClient, system_setting.WorkerUrl = previousClient, previousWorker
		*fetch = previousFetch
	})

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}))
	model.DB = database
	common.MemoryCacheEnabled, common.RedisEnabled = false, false
	constant.NotifyLimitCount = 100

	fixture := &probeNoticeFixture{database: database}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload WebhookPayload
		if err := common.DecodeJson(r.Body, &payload); err == nil {
			fixture.mutex.Lock()
			fixture.notices = append(fixture.notices, payload.Type+" "+payload.Title)
			fixture.mutex.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	httpClient, system_setting.WorkerUrl = server.Client(), ""
	fetch.EnableSSRFProtection = false

	setting, err := common.Marshal(dto.UserSetting{NotifyType: dto.NotifyTypeWebhook, WebhookUrl: server.URL + "/root"})
	require.NoError(t, err)
	require.NoError(t, database.Create(&model.User{
		Username: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Setting: string(setting), AffCode: "root",
	}).Error)
	return fixture
}

func (f *probeNoticeFixture) received() []string {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return append([]string(nil), f.notices...)
}

// createChannel stores a channel and its ability the way createProbePolicyChannel
// does; keys makes it a multi-key channel.
func (f *probeNoticeFixture) createChannel(t *testing.T, name string, status int, autoBan int, keys ...string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Name: name, Type: 1, Key: "sk-test", Status: status,
		Models: "gpt-4o-mini", Group: "default", AutoBan: &autoBan,
	}
	if len(keys) > 0 {
		channel.Key = ""
		for i, key := range keys {
			if i > 0 {
				channel.Key += "\n"
			}
			channel.Key += key
		}
		channel.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: len(keys)}
	}
	require.NoError(t, f.database.Create(channel).Error)
	require.NoError(t, f.database.Create(&model.Ability{
		Group: "default", Model: "gpt-4o-mini", ChannelId: channel.Id,
		Enabled: status == common.ChannelStatusEnabled,
	}).Error)
	return channel
}

// reload reads the channel back the way a sweep does, so the next verdict sees
// the status the previous one stored.
func (f *probeNoticeFixture) reload(t *testing.T, id int) *model.Channel {
	t.Helper()
	var channel model.Channel
	require.NoError(t, f.database.First(&channel, "id = ?", id).Error)
	return &channel
}

func probeVerdictOn(channel *model.Channel, keyIndex int, outcome healthProbeOutcome) healthProbeAccountResult {
	result := healthProbeAccountResult{channel: channel, keyIndex: keyIndex, probeModel: "gpt-4o-mini", outcome: outcome}
	if channel.ChannelInfo.IsMultiKey {
		result.usingKey = channel.GetKeys()[keyIndex]
	}
	return result
}

// The administrators who watch channels are told when the probe takes a
// channel out of rotation or puts it back, with the same notices a failed
// request and a channel test send, and only when the status really changes: a
// channel that stays down is not announced again on every sweep.
func TestHealthProbeAnnouncesEachStatusChange(t *testing.T) {
	fixture := newProbeNoticeFixture(t)
	setProbeOperatorPolicy(t, true, true)
	channel := fixture.createChannel(t, "probe-notice", common.ChannelStatusEnabled, 1)

	healthy := healthProbeOutcome{verdict: healthProbeVerdictHealthy}
	degraded := healthProbeOutcome{verdict: healthProbeVerdictDegraded, detail: "empty or evasive response: sorry"}
	sweep := func(outcome healthProbeOutcome) {
		applyHealthProbeResults([]healthProbeAccountResult{probeVerdictOn(fixture.reload(t, channel.Id), 0, outcome)})
	}
	title := func(status int, text string) string {
		return fmt.Sprintf(`%s Channel "probe-notice" (#%d) %s`, formatNotifyType(channel.Id, status), channel.Id, text)
	}

	sweep(failedProbeAnswer(401, "invalid api key"))
	assert.Equal(t, common.ChannelStatusAutoDisabled, fixture.reload(t, channel.Id).Status)
	assert.Equal(t, []string{title(common.ChannelStatusAutoDisabled, "has been disabled")}, fixture.received())

	sweep(failedProbeAnswer(401, "invalid api key"))
	assert.Len(t, fixture.received(), 1, "a channel that stays disabled is not announced again")

	sweep(healthy)
	assert.Equal(t, common.ChannelStatusEnabled, fixture.reload(t, channel.Id).Status)
	sweep(degraded)
	assert.Equal(t, common.ChannelStatusDegraded, fixture.reload(t, channel.Id).Status)
	sweep(degraded)
	sweep(healthy)
	assert.Equal(t, common.ChannelStatusEnabled, fixture.reload(t, channel.Id).Status)

	assert.Equal(t, []string{
		title(common.ChannelStatusAutoDisabled, "has been disabled"),
		title(common.ChannelStatusEnabled, "has been enabled"),
		title(common.ChannelStatusDegraded, "has been marked degraded"),
		title(common.ChannelStatusEnabled, "has been enabled"),
	}, fixture.received())
}

// A notice goes out only for a status the probe really changed. Everything the
// operator did not opt into, and every verdict one probe cannot trust, stays
// silent, exactly as it leaves the status alone.
func TestHealthProbeStaysSilentWhenItChangesNothing(t *testing.T) {
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
	}{
		{"auto-disable off: a configured status", false, true, 1, common.ChannelStatusEnabled, failedProbeAnswer(401, "invalid api key")},
		{"auto-disable off: a degraded answer", false, true, 1, common.ChannelStatusEnabled, degraded},
		{"the channel opted out of auto-ban: a configured status", true, true, 0, common.ChannelStatusEnabled, failedProbeAnswer(401, "invalid api key")},
		{"the channel opted out of auto-ban: a degraded answer", true, true, 0, common.ChannelStatusEnabled, degraded},
		{"a rate limit", true, true, 1, common.ChannelStatusEnabled, failedProbeAnswer(429, "rate limited")},
		{"a timeout", true, true, 1, common.ChannelStatusEnabled, timeout},
		{"auto-enable off: a healthy answer", true, false, 1, common.ChannelStatusAutoDisabled, healthy},
		{"a manual disable wins over a healthy answer", true, true, 1, common.ChannelStatusManuallyDisabled, healthy},
		{"a healthy answer from a channel already enabled", true, true, 1, common.ChannelStatusEnabled, healthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newProbeNoticeFixture(t)
			setProbeOperatorPolicy(t, tt.autoDisable, tt.autoEnable)
			channel := fixture.createChannel(t, "probe-quiet", tt.start, tt.autoBan)

			applyHealthProbeResults([]healthProbeAccountResult{probeVerdictOn(channel, 0, tt.outcome)})

			assert.Equal(t, tt.start, fixture.reload(t, channel.Id).Status)
			assert.Empty(t, fixture.received())
		})
	}
}

// In a multi-key channel the probe acts on one key. The notice says which one,
// both when the key goes and when it comes back, and the channel itself stays
// enabled while another key still works.
func TestHealthProbeAnnouncesKeyChangesInAMultiKeyChannel(t *testing.T) {
	fixture := newProbeNoticeFixture(t)
	setProbeOperatorPolicy(t, true, true)
	channel := fixture.createChannel(t, "probe-keys", common.ChannelStatusEnabled, 1, "sk-first", "sk-second")

	applyHealthProbeResults([]healthProbeAccountResult{
		probeVerdictOn(fixture.reload(t, channel.Id), 1, failedProbeAnswer(401, "invalid api key")),
	})

	stored := fixture.reload(t, channel.Id)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[1])
	assert.Contains(t, stored.ChannelInfo.MultiKeyDisabledReason[1], "key #2: health probe failed")
	assert.Equal(t, []string{
		fmt.Sprintf(`%s Channel "probe-keys" (#%d) has been disabled`, formatNotifyType(channel.Id, common.ChannelStatusAutoDisabled), channel.Id),
	}, fixture.received())

	applyHealthProbeResults([]healthProbeAccountResult{
		probeVerdictOn(fixture.reload(t, channel.Id), 1, healthProbeOutcome{verdict: healthProbeVerdictHealthy}),
	})

	stored = fixture.reload(t, channel.Id)
	assert.Empty(t, stored.ChannelInfo.MultiKeyStatusList)
	assert.Equal(t, []string{
		fmt.Sprintf(`%s Channel "probe-keys" (#%d) has been disabled`, formatNotifyType(channel.Id, common.ChannelStatusAutoDisabled), channel.Id),
		fmt.Sprintf(`%s Channel "probe-keys" (#%d) has been enabled`, formatNotifyType(channel.Id, common.ChannelStatusEnabled), channel.Id),
	}, fixture.received())
}

// A single-key channel keeps the reason as it was: only a multi-key channel
// needs the key named.
func TestHealthProbeStatusReasonNamesTheKeyOnlyInAMultiKeyChannel(t *testing.T) {
	outcome := failedProbeAnswer(401, "invalid api key")
	single := healthProbeAccountResult{channel: &model.Channel{}, keyIndex: 0, probeModel: "gpt-4o-mini", outcome: outcome}
	multi := healthProbeAccountResult{
		channel:  &model.Channel{ChannelInfo: model.ChannelInfo{IsMultiKey: true}},
		keyIndex: 2, probeModel: "gpt-4o-mini", outcome: outcome,
	}

	assert.Equal(t, "health probe failed (model=gpt-4o-mini): HTTP 401: invalid api key", healthProbeStatusReason(single))
	assert.Equal(t, "key #3: health probe failed (model=gpt-4o-mini): HTTP 401: invalid api key", healthProbeStatusReason(multi))
}
