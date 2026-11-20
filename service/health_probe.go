package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/constant"
	"github.com/mazihua-lgtm/relay-forge/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	// DefaultHealthProbeInterval is the period of the automatic channel health
	// probe when the caller does not pass one.
	DefaultHealthProbeInterval = 30 * time.Minute

	// healthProbeMaxAccounts caps how many accounts (keys) of one channel a
	// single sweep probes, so a channel with hundreds of keys stays cheap.
	healthProbeMaxAccounts = 5
	// healthProbeRequestTimeout bounds one probe request. A hanging upstream
	// must never stall the probe goroutine.
	healthProbeRequestTimeout = 10 * time.Second
	// healthProbeChannelWorkers bounds how many channels one sweep probes at
	// the same time.
	healthProbeChannelWorkers = 4
	// healthProbeNetworkDownMinAccounts is how many accounts behind one proxy
	// setting must all fail without any HTTP answer before the sweep reads it as
	// an outage of the probe host's own network or proxy, not as dead upstreams.
	healthProbeNetworkDownMinAccounts = 3
	healthProbeMaxTokens              = 5
	healthProbePrompt                 = "hi"
	healthProbeMaxBodyBytes           = 1 << 20
	healthProbeDetailRunes            = 200

	healthProbeDefaultOpenAIModel    = "gpt-4o-mini"
	healthProbeDefaultAnthropicModel = "claude-3-5-haiku-20241022"
	healthProbeAnthropicVersion      = "2023-06-01"
)

// healthProbeVerdict is what one account answered.
type healthProbeVerdict int

const (
	healthProbeVerdictHealthy healthProbeVerdict = iota
	healthProbeVerdictDegraded
	healthProbeVerdictFailed
)

func (verdict healthProbeVerdict) String() string {
	switch verdict {
	case healthProbeVerdictHealthy:
		return "healthy"
	case healthProbeVerdictDegraded:
		return "degraded"
	default:
		return "failed"
	}
}

// healthProbeProtocol is the wire format a channel speaks for a chat
// completion. Only these two are probed; every other channel type is skipped,
// so the probe never disables a channel whose endpoint it cannot speak to.
type healthProbeProtocol int

const (
	healthProbeProtocolOpenAI healthProbeProtocol = iota
	healthProbeProtocolAnthropic
)

type healthProbeOutcome struct {
	verdict       healthProbeVerdict
	detail        string
	latencyMillis int64
	// noResponse marks a failure with no HTTP answer at all (DNS, connect, TLS,
	// timeout): the path to the upstream is broken, which says nothing about
	// the account itself.
	noResponse bool
}

// healthProbeAccountResult is one probed account. The verdicts are applied to
// the channel rows only after the whole sweep has finished, so an outage of the
// probe host's own network can be told apart from dead upstreams first.
type healthProbeAccountResult struct {
	channel    *model.Channel
	keyIndex   int
	usingKey   string // the key exactly as GetKeys lists it; empty for a single-key channel
	proxyURL   string
	probeModel string
	outcome    healthProbeOutcome
}

type healthProbeStats struct {
	accounts   int
	healthy    int
	degraded   int
	failed     int
	suppressed int
	skipped    int
}

var (
	healthProbeOnce    sync.Once
	healthProbeRunning atomic.Bool
)

// healthProbeDegradedMarkers are the boilerplate answers a degraded or
// throttled upstream returns instead of a real completion. Matching text is a
// strong signal with a prompt as small as "hi".
var healthProbeDegradedMarkers = []string{
	"i'm sorry",
	"i am sorry",
	"i apologize",
	"sorry, i can't",
	"sorry, i cannot",
	"i can't assist",
	"i cannot assist",
	"i can't help",
	"i cannot help",
	"unable to assist",
	"unable to help",
	"as an ai language model",
	"抱歉",
	"很抱歉",
	"无法协助",
	"無法協助",
	"无法提供",
	"無法提供",
}

// healthProbeNonChatMarkers skip models a chat-completions probe cannot use.
var healthProbeNonChatMarkers = []string{
	"embed", "rerank", "whisper", "tts", "moderation", "dall-e", "image",
	"sora", "video", "kling", "jimeng", "vidu", "suno", "midjourney",
	"audio", "realtime", "transcribe", "speech", "bge-", "m3e",
	"reasoner", "codex",
}

// StartHealthProber starts the periodic channel health probe. Call it once
// from main during initialization; extra calls are ignored. Only the master
// node probes, so a multi-node deployment runs one sweep per interval.
//
// The first sweep runs one interval after start, not at boot: right after a
// restart the network or the proxy may not be up yet, and a sweep then would
// take healthy channels out of rotation. An admin can sweep at once through
// POST /api/channel/probe.
func StartHealthProber(interval time.Duration) {
	if interval <= 0 {
		interval = DefaultHealthProbeInterval
	}
	healthProbeOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			common.SysLog(common.LogText("channel health prober started: interval=%s, first sweep in %s", interval, interval))
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				if !TriggerHealthProbeAsync() {
					common.SysLog(common.LogText("channel health probe: previous sweep still running, skipping this tick"))
				}
			}
		})
	})
}

// TriggerHealthProbeAsync starts one full health-probe sweep in the background.
// It reports false when a sweep is already running, so the periodic task and
// the admin endpoint never overlap.
func TriggerHealthProbeAsync() bool {
	if model.DB == nil {
		return false
	}
	if !healthProbeRunning.CompareAndSwap(false, true) {
		return false
	}
	gopool.Go(func() {
		defer healthProbeRunning.Store(false)
		runHealthProbeSweep()
	})
	return true
}

// runHealthProbeSweep probes every channel that is in, or was removed from,
// the routing pool by an automatic decision, then applies the verdicts.
// Manually disabled channels are never probed or re-enabled.
func runHealthProbeSweep() {
	startedAt := time.Now()
	channels, err := loadHealthProbeChannels()
	if err != nil {
		common.SysLog(common.LogText("channel health probe: query channels failed: %v", err))
		return
	}
	if len(channels) == 0 {
		common.SysLog(common.LogText("channel health probe: no channels to probe"))
		return
	}

	type channelProbe struct {
		results []healthProbeAccountResult
		skipped int
	}
	semaphore := make(chan struct{}, healthProbeChannelWorkers)
	probes := make(chan channelProbe, len(channels))
	var wg sync.WaitGroup
	for _, channel := range channels {
		channel := channel
		wg.Add(1)
		semaphore <- struct{}{}
		gopool.Go(func() {
			defer wg.Done()
			defer func() { <-semaphore }()
			probe := channelProbe{}
			defer func() {
				if recovered := recover(); recovered != nil {
					common.SysLog(common.LogText("channel health probe: panic on channel_id=%d: %v", channel.Id, recovered))
					probe = channelProbe{skipped: 1}
				}
				probes <- probe
			}()
			probe.results, probe.skipped = probeChannelHealth(channel)
		})
	}
	wg.Wait()
	close(probes)

	var results []healthProbeAccountResult
	skipped := 0
	for probe := range probes {
		results = append(results, probe.results...)
		skipped += probe.skipped
	}
	summary := applyHealthProbeResults(results)
	summary.skipped += skipped
	common.SysLog(common.LogText(
		"channel health probe finished: channels=%d accounts=%d healthy=%d degraded=%d failed=%d suppressed=%d skipped=%d duration=%s",
		len(channels), summary.accounts, summary.healthy, summary.degraded, summary.failed, summary.suppressed, summary.skipped,
		time.Since(startedAt).Round(time.Millisecond),
	))
}

func loadHealthProbeChannels() ([]*model.Channel, error) {
	var channels []*model.Channel
	err := model.DB.
		Select("id", "name", "type", "key", "base_url", "test_model", "models", "model_mapping", "setting", "status", "channel_info", "auto_ban").
		Where("status IN ?", []int{
			common.ChannelStatusEnabled,
			common.ChannelStatusAutoDisabled,
			common.ChannelStatusDegraded,
		}).
		Order("id asc").
		Find(&channels).Error
	if err != nil {
		return nil, err
	}
	return channels, nil
}

// probeChannelHealth asks the upstream with up to healthProbeMaxAccounts
// accounts of one channel. It writes nothing: applyHealthProbeResults applies
// the verdicts once the whole sweep is in. skipped counts what it could not
// probe at all.
func probeChannelHealth(channel *model.Channel) (results []healthProbeAccountResult, skipped int) {
	protocol, ok := healthProbeProtocolForChannel(channel.Type)
	if !ok {
		if common.DebugEnabled {
			common.SysLog(common.LogText("channel health probe: skipped channel_id=%d type=%d: channel type has no probe protocol", channel.Id, channel.Type))
		}
		return nil, 1
	}

	probeModel, ok := healthProbeModelForChannel(channel, protocol)
	if !ok {
		common.SysLog(common.LogText("channel health probe: skipped channel_id=%d name=%q: no chat model to probe", channel.Id, channel.Name))
		return nil, 1
	}
	// Clients ask for the listed name; the upstream knows the mapped one.
	probeModel = healthProbeMappedModel(channel.GetModelMapping(), probeModel)

	baseURL := strings.TrimSpace(channel.GetBaseURL())
	if baseURL == "" {
		common.SysLog(common.LogText("channel health probe: skipped channel_id=%d name=%q: empty base url", channel.Id, channel.Name))
		return nil, 1
	}

	// Reach the upstream the way the relay does, through the channel's own
	// proxy and transport settings. Any other route would report an outage
	// that real traffic never sees, and disable a healthy channel for it.
	setting := channel.GetSetting()
	client, err := GetHttpClientWithProxySettings(setting.Proxy, setting)
	if err != nil {
		common.SysLog(common.LogText("channel health probe: skipped channel_id=%d name=%q: proxy setting unusable: %v", channel.Id, channel.Name, err))
		return nil, 1
	}
	proxyURL := strings.TrimSpace(setting.Proxy)

	keys := channel.GetKeys()
	if len(keys) > healthProbeMaxAccounts {
		keys = keys[:healthProbeMaxAccounts]
	}
	if len(keys) == 0 {
		common.SysLog(common.LogText("channel health probe: skipped channel_id=%d name=%q: no account key", channel.Id, channel.Name))
		return nil, 1
	}

	for index, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if !healthProbeIsPlainAPIKey(key) {
			// An empty line or a structured credential (JSON) is no bearer
			// token: probing with it would only report a failure of our own.
			skipped++
			continue
		}

		usingKey := ""
		if channel.ChannelInfo.IsMultiKey {
			// Status updates find the key by the exact string GetKeys lists.
			usingKey = rawKey
		}

		ctx, cancel := context.WithTimeout(context.Background(), healthProbeRequestTimeout)
		outcome := probeChannelAccount(ctx, client, baseURL, protocol, key, probeModel)
		cancel()

		results = append(results, healthProbeAccountResult{
			channel:    channel,
			keyIndex:   index,
			usingKey:   usingKey,
			proxyURL:   proxyURL,
			probeModel: probeModel,
			outcome:    outcome,
		})
	}
	return results, skipped
}

// applyHealthProbeResults writes the verdicts of one finished sweep to the
// channel rows and returns the tallies. Disabling is withheld for the accounts
// behind a proxy setting that none of them got any answer through (see
// healthProbeUnreachableProxies): that is the probe host's own network or proxy
// failing, and acting on it would take healthy channels out of rotation.
// Recovery is never withheld.
func applyHealthProbeResults(results []healthProbeAccountResult) healthProbeStats {
	unreachable := healthProbeUnreachableProxies(results)
	for proxyURL := range unreachable {
		common.SysLog(common.LogText(
			"channel health probe: no account behind %s got any answer, treating it as an outage of this host's network or proxy and keeping the channels in rotation this sweep",
			healthProbeRouteName(proxyURL),
		))
	}

	stats := healthProbeStats{accounts: len(results)}
	for _, result := range results {
		channel, outcome := result.channel, result.outcome
		channel.UpdateResponseTime(outcome.latencyMillis)

		switch outcome.verdict {
		case healthProbeVerdictHealthy:
			stats.healthy++
			channel.ResetErrorCount()
			healthProbeRestoreChannel(channel, result.keyIndex, result.usingKey)

		case healthProbeVerdictDegraded:
			stats.degraded++
			common.SysLog(common.LogText(
				"channel health probe: channel_id=%d name=%q model=%s account=%d degraded: %s",
				channel.Id, channel.Name, result.probeModel, result.keyIndex+1, healthProbePreview(outcome.detail),
			))
			if channel.GetAutoBan() {
				model.UpdateChannelStatus(channel.Id, result.usingKey, common.ChannelStatusDegraded,
					healthProbeReason(outcome.verdict, result.probeModel, outcome.detail))
			}

		default:
			stats.failed++
			if outcome.noResponse && unreachable[result.proxyURL] {
				stats.suppressed++
				common.SysLog(common.LogText(
					"channel health probe: channel_id=%d name=%q account=%d unreachable, not counted against the account: %s",
					channel.Id, channel.Name, result.keyIndex+1, healthProbePreview(outcome.detail),
				))
				continue
			}
			channel.IncrementErrorCount()
			common.SysLog(common.LogText(
				"channel health probe: channel_id=%d name=%q model=%s account=%d failed: %s",
				channel.Id, channel.Name, result.probeModel, result.keyIndex+1, healthProbePreview(outcome.detail),
			))
			if channel.GetAutoBan() {
				model.UpdateChannelStatus(channel.Id, result.usingKey, common.ChannelStatusAutoDisabled,
					healthProbeReason(outcome.verdict, result.probeModel, outcome.detail))
			}
		}
	}
	return stats
}

// healthProbeUnreachableProxies returns the proxy settings ("" is the direct
// route) behind which at least healthProbeNetworkDownMinAccounts accounts were
// probed and not one got any HTTP answer. Dead upstreams still answer, with an
// error status; silence from all of them means the way to them is broken.
func healthProbeUnreachableProxies(results []healthProbeAccountResult) map[string]bool {
	type tally struct{ answered, silent int }
	tallies := make(map[string]*tally)
	for _, result := range results {
		t := tallies[result.proxyURL]
		if t == nil {
			t = &tally{}
			tallies[result.proxyURL] = t
		}
		if result.outcome.noResponse {
			t.silent++
		} else {
			t.answered++
		}
	}
	unreachable := make(map[string]bool)
	for proxyURL, t := range tallies {
		if t.answered == 0 && t.silent >= healthProbeNetworkDownMinAccounts {
			unreachable[proxyURL] = true
		}
	}
	return unreachable
}

// healthProbeRouteName names a proxy setting for logs without its credentials.
func healthProbeRouteName(proxyURL string) string {
	if proxyURL == "" {
		return "the direct route"
	}
	if parsed, err := url.Parse(proxyURL); err == nil && parsed.Host != "" {
		return "proxy " + parsed.Host
	}
	return "a channel proxy"
}

// healthProbeRestoreChannel puts a channel, or one key of it, back into
// rotation after a healthy answer. A manual disable always wins.
func healthProbeRestoreChannel(channel *model.Channel, keyIndex int, usingKey string) {
	keyStatus := common.ChannelStatusEnabled
	if channel.ChannelInfo.IsMultiKey && channel.ChannelInfo.MultiKeyStatusList != nil {
		if status, ok := channel.ChannelInfo.MultiKeyStatusList[keyIndex]; ok {
			keyStatus = status
		}
	}
	if keyStatus == common.ChannelStatusEnabled && channel.Status == common.ChannelStatusEnabled {
		return
	}
	// UpdateChannelStatus treats an update to the status the channel already
	// has as a no-op, which is exactly the case of one disabled key inside an
	// enabled multi-key channel: write the key status itself.
	if channel.ChannelInfo.IsMultiKey && channel.Status == common.ChannelStatusEnabled {
		healthProbeRestoreChannelKey(channel.Id, keyIndex)
		return
	}
	stored, err := model.GetChannelById(channel.Id, true)
	if err != nil || stored.Status == common.ChannelStatusManuallyDisabled {
		return
	}
	if model.UpdateChannelStatus(channel.Id, usingKey, common.ChannelStatusEnabled, "") {
		common.SysLog(common.LogText(
			"channel health probe: channel_id=%d name=%q account=%d recovered, status set to enabled",
			channel.Id, channel.Name, keyIndex+1,
		))
	}
}

// healthProbeRestoreChannelKey clears one key's disabled status inside a
// multi-key channel whose own status stays enabled.
func healthProbeRestoreChannelKey(channelId int, keyIndex int) {
	written, err := model.UpdateChannelOtherInfo(channelId, func(stored *model.Channel) (map[string]any, error) {
		if !stored.ChannelInfo.IsMultiKey ||
			stored.Status == common.ChannelStatusManuallyDisabled ||
			stored.ChannelInfo.MultiKeyStatusList == nil {
			return nil, nil
		}
		if _, disabled := stored.ChannelInfo.MultiKeyStatusList[keyIndex]; !disabled {
			return nil, nil
		}
		delete(stored.ChannelInfo.MultiKeyStatusList, keyIndex)
		delete(stored.ChannelInfo.MultiKeyDisabledReason, keyIndex)
		delete(stored.ChannelInfo.MultiKeyDisabledTime, keyIndex)
		return map[string]any{"status": stored.Status, "channel_info": stored.ChannelInfo}, nil
	})
	if err != nil {
		common.SysLog(common.LogText(
			"channel health probe: failed to re-enable channel key: channel_id=%d key_index=%d error=%v",
			channelId, keyIndex, err,
		))
		return
	}
	if written {
		model.BumpSyncTopic(model.SyncTopicChannels)
		common.SysLog(common.LogText(
			"channel health probe: channel_id=%d key_index=%d recovered, key set to enabled",
			channelId, keyIndex,
		))
	}
}

// probeChannelAccount sends one minimal chat request through client and
// classifies the answer. ctx bounds the whole exchange, the body read
// included, so a hanging upstream cannot stall the caller. The key never
// reaches the returned detail, which is logged and stored with the channel.
func probeChannelAccount(ctx context.Context, client *http.Client, baseURL string, protocol healthProbeProtocol, apiKey string, probeModel string) healthProbeOutcome {
	outcome := doProbeChannelAccount(ctx, client, baseURL, protocol, apiKey, probeModel)
	if apiKey != "" {
		outcome.detail = strings.ReplaceAll(outcome.detail, apiKey, "***")
	}
	return outcome
}

func doProbeChannelAccount(ctx context.Context, client *http.Client, baseURL string, protocol healthProbeProtocol, apiKey string, probeModel string) healthProbeOutcome {
	payload, err := healthProbeRequestBody(protocol, probeModel)
	if err != nil {
		return healthProbeOutcome{verdict: healthProbeVerdictFailed, detail: "build request body: " + err.Error()}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, healthProbeEndpoint(baseURL, protocol), bytes.NewReader(payload))
	if err != nil {
		return healthProbeOutcome{verdict: healthProbeVerdictFailed, detail: "build request: " + err.Error()}
	}
	request.Header.Set("Content-Type", "application/json")
	switch protocol {
	case healthProbeProtocolAnthropic:
		request.Header.Set("x-api-key", apiKey)
		request.Header.Set("anthropic-version", healthProbeAnthropicVersion)
	default:
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	startedAt := time.Now()
	response, err := client.Do(request)
	latencyMillis := time.Since(startedAt).Milliseconds()
	if err != nil {
		return healthProbeOutcome{verdict: healthProbeVerdictFailed, detail: err.Error(), latencyMillis: latencyMillis, noResponse: true}
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, healthProbeMaxBodyBytes))
	if err != nil {
		return healthProbeOutcome{verdict: healthProbeVerdictFailed, detail: "read response: " + err.Error(), latencyMillis: latencyMillis}
	}
	if response.StatusCode != http.StatusOK {
		return healthProbeOutcome{
			verdict:       healthProbeVerdictFailed,
			detail:        fmt.Sprintf("HTTP %d: %s", response.StatusCode, healthProbePreview(string(body))),
			latencyMillis: latencyMillis,
		}
	}

	text, err := healthProbeResponseText(protocol, body)
	if err != nil {
		// A 200 whose body is not the protocol's JSON is unusable for a client,
		// which is what degraded marks.
		return healthProbeOutcome{verdict: healthProbeVerdictDegraded, detail: "unreadable response body: " + err.Error(), latencyMillis: latencyMillis}
	}
	if verdict := healthProbeClassifyResponseText(text); verdict != healthProbeVerdictHealthy {
		return healthProbeOutcome{verdict: verdict, detail: "empty or evasive response: " + healthProbePreview(text), latencyMillis: latencyMillis}
	}
	return healthProbeOutcome{verdict: healthProbeVerdictHealthy, detail: healthProbePreview(text), latencyMillis: latencyMillis}
}

func healthProbeRequestBody(protocol healthProbeProtocol, probeModel string) ([]byte, error) {
	if protocol == healthProbeProtocolAnthropic {
		return json.Marshal(map[string]any{
			"model":      probeModel,
			"max_tokens": healthProbeMaxTokens,
			"messages": []map[string]any{
				{"role": "user", "content": healthProbePrompt},
			},
		})
	}
	return json.Marshal(map[string]any{
		"model": probeModel,
		"messages": []map[string]any{
			{"role": "user", "content": healthProbePrompt},
		},
		"max_tokens": healthProbeMaxTokens,
		"stream":     false,
	})
}

// healthProbeEndpoint builds the probe URL from the channel base URL, keeping
// an already versioned base (…/v1) or a full endpoint path untouched.
func healthProbeEndpoint(baseURL string, protocol healthProbeProtocol) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path := "/v1/chat/completions"
	if protocol == healthProbeProtocolAnthropic {
		path = "/v1/messages"
	}
	switch {
	case strings.HasSuffix(base, "/chat/completions"), strings.HasSuffix(base, "/messages"):
		return base
	case strings.HasSuffix(base, "/v1"):
		return base + strings.TrimPrefix(path, "/v1")
	default:
		return base + path
	}
}

// healthProbeProtocolForChannel maps channel types to a protocol the probe can
// speak. Deployment-scoped (Azure), native (Gemini/Baidu/…), subscription
// (Codex), video, task and advanced-custom channels (vLLM and SGLang among
// them, whose route presets can move the chat path) are skipped on purpose.
func healthProbeProtocolForChannel(channelType int) (healthProbeProtocol, bool) {
	switch channelType {
	case constant.ChannelTypeAnthropic:
		return healthProbeProtocolAnthropic, true
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeOpenAIMax,
		constant.ChannelTypeOhMyGPT,
		constant.ChannelTypeCustom,
		constant.ChannelTypeAILS,
		constant.ChannelTypeAIProxy,
		constant.ChannelTypeAPI2GPT,
		constant.ChannelTypeAIGC2D,
		constant.ChannelTypeOpenRouter,
		constant.ChannelTypeAIProxyLibrary,
		constant.ChannelTypeMoonshot,
		constant.ChannelTypeSiliconFlow,
		constant.ChannelTypeMistral,
		constant.ChannelTypeDeepSeek,
		constant.ChannelTypeXai,
		constant.ChannelTypeSubmodel,
		constant.ChannelTypeSub2API,
		constant.ChannelTypeNewAPI:
		return healthProbeProtocolOpenAI, true
	default:
		return healthProbeProtocolOpenAI, false
	}
}

// healthProbeModelForChannel picks the cheapest chat model the channel lists,
// so a probe never asks an upstream for a model it does not serve.
func healthProbeModelForChannel(channel *model.Channel, protocol healthProbeProtocol) (string, bool) {
	if channel.TestModel != nil {
		if testModel := strings.TrimSpace(*channel.TestModel); testModel != "" {
			return testModel, true
		}
	}
	models := channel.GetModels()
	best := ""
	bestScore := -1
	for _, candidate := range models {
		name := strings.TrimSpace(candidate)
		if name == "" || healthProbeIsNonChatModel(name) {
			continue
		}
		if score := healthProbeChatModelScore(name); score > bestScore {
			best, bestScore = name, score
		}
	}
	if best != "" {
		return best, true
	}
	if len(models) == 0 {
		if protocol == healthProbeProtocolAnthropic {
			return healthProbeDefaultAnthropicModel, true
		}
		return healthProbeDefaultOpenAIModel, true
	}
	return "", false
}

func healthProbeIsNonChatModel(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, marker := range healthProbeNonChatMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, prefix := range []string{"o1", "o3", "o4"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func healthProbeChatModelScore(name string) int {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "gpt-4o-mini"):
		return 100
	case strings.Contains(lower, "haiku"):
		return 95
	case strings.Contains(lower, "mini"):
		return 85
	case strings.Contains(lower, "flash"):
		return 80
	case strings.Contains(lower, "turbo"):
		return 75
	case strings.Contains(lower, "chat"):
		return 60
	default:
		return 10
	}
}

func healthProbeResponseText(protocol healthProbeProtocol, body []byte) (string, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return "", nil
	}
	if protocol == healthProbeProtocolAnthropic {
		var response struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return "", err
		}
		parts := make([]string, 0, len(response.Content))
		for _, block := range response.Content {
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
			parts = append(parts, block.Text)
		}
		return strings.Join(parts, "\n"), nil
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", nil
	}
	text := healthProbeOpenAIContentText(response.Choices[0].Message.Content)
	if strings.TrimSpace(text) == "" {
		text = response.Choices[0].Text
	}
	return text, nil
}

// healthProbeOpenAIContentText accepts both a plain string content and the
// content-parts array some providers return.
func healthProbeOpenAIContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		texts = append(texts, part.Text)
	}
	return strings.Join(texts, "\n")
}

// healthProbeClassifyResponseText treats an empty answer or evasive
// boilerplate as degraded.
func healthProbeClassifyResponseText(text string) healthProbeVerdict {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return healthProbeVerdictDegraded
	}
	lower := strings.ToLower(trimmed)
	for _, marker := range healthProbeDegradedMarkers {
		if strings.Contains(lower, marker) {
			return healthProbeVerdictDegraded
		}
	}
	return healthProbeVerdictHealthy
}

func healthProbeReason(verdict healthProbeVerdict, probeModel string, detail string) string {
	return fmt.Sprintf("health probe %s (model=%s): %s", verdict, probeModel, healthProbePreview(detail))
}

// healthProbeIsPlainAPIKey reports a key that can be sent as a bearer token:
// non-empty and not a structured credential such as a JSON service account.
func healthProbeIsPlainAPIKey(key string) bool {
	if key == "" {
		return false
	}
	switch key[0] {
	case '{', '[', '"':
		return false
	}
	return true
}

// healthProbeMappedModel resolves name through a channel's model_mapping the
// way the relay does: it follows the chain to its end and keeps name when the
// mapping is absent, unreadable or loops.
func healthProbeMappedModel(modelMapping string, name string) string {
	modelMapping = strings.TrimSpace(modelMapping)
	if modelMapping == "" || modelMapping == "{}" {
		return name
	}
	mapping := make(map[string]string)
	if err := json.Unmarshal([]byte(modelMapping), &mapping); err != nil {
		return name
	}
	current := name
	visited := map[string]bool{current: true}
	for {
		next, ok := mapping[current]
		if !ok || next == "" || next == current {
			return current
		}
		if visited[next] {
			return name
		}
		visited[next] = true
		current = next
	}
}

// healthProbePreview collapses a detail to one short, single-line snippet for
// logs and the stored status reason.
func healthProbePreview(text string) string {
	compact := strings.Join(strings.Fields(text), " ")
	runes := []rune(compact)
	if len(runes) <= healthProbeDetailRunes {
		return compact
	}
	return string(runes[:healthProbeDetailRunes]) + "..."
}
