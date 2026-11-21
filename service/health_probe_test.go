package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/constant"
	"github.com/mazihua-lgtm/relay-forge/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthProbeEndpoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		baseURL  string
		protocol healthProbeProtocol
		want     string
	}{
		{"openai default", "https://api.openai.com", healthProbeProtocolOpenAI, "https://api.openai.com/v1/chat/completions"},
		{"openai trailing slash", "https://api.openai.com/", healthProbeProtocolOpenAI, "https://api.openai.com/v1/chat/completions"},
		{"openai versioned base", "https://api.deepseek.com/v1", healthProbeProtocolOpenAI, "https://api.deepseek.com/v1/chat/completions"},
		{"openai full endpoint", "https://gw.example.com/v1/chat/completions", healthProbeProtocolOpenAI, "https://gw.example.com/v1/chat/completions"},
		{"proxy prefix", "https://openrouter.ai/api", healthProbeProtocolOpenAI, "https://openrouter.ai/api/v1/chat/completions"},
		{"anthropic default", "https://api.anthropic.com", healthProbeProtocolAnthropic, "https://api.anthropic.com/v1/messages"},
		{"anthropic versioned base", "https://api.anthropic.com/v1", healthProbeProtocolAnthropic, "https://api.anthropic.com/v1/messages"},
		{"anthropic full endpoint", "https://gw.example.com/v1/messages", healthProbeProtocolAnthropic, "https://gw.example.com/v1/messages"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, healthProbeEndpoint(tc.baseURL, tc.protocol))
		})
	}
}

func TestHealthProbeClassifyResponseText(t *testing.T) {
	t.Parallel()

	assert.Equal(t, healthProbeVerdictDegraded, healthProbeClassifyResponseText(""))
	assert.Equal(t, healthProbeVerdictDegraded, healthProbeClassifyResponseText("   \n "))
	assert.Equal(t, healthProbeVerdictDegraded, healthProbeClassifyResponseText("I'm sorry, but I can't help with that."))
	assert.Equal(t, healthProbeVerdictDegraded, healthProbeClassifyResponseText("As an AI language model, I cannot assist."))
	assert.Equal(t, healthProbeVerdictDegraded, healthProbeClassifyResponseText("抱歉，我无法协助处理该请求。"))
	assert.Equal(t, healthProbeVerdictHealthy, healthProbeClassifyResponseText("Hello! How can I help you today?"))
}

func TestHealthProbeResponseText(t *testing.T) {
	t.Parallel()

	openAIPlain := []byte(`{"choices":[{"message":{"content":"Hello!"}}]}`)
	text, err := healthProbeResponseText(healthProbeProtocolOpenAI, openAIPlain)
	require.NoError(t, err)
	assert.Equal(t, "Hello!", text)

	openAIParts := []byte(`{"choices":[{"message":{"content":[{"type":"text","text":"Hi"},{"type":"text","text":"there"}]}}]}`)
	text, err = healthProbeResponseText(healthProbeProtocolOpenAI, openAIParts)
	require.NoError(t, err)
	assert.Equal(t, "Hi\nthere", text)

	openAIEmpty := []byte(`{"choices":[]}`)
	text, err = healthProbeResponseText(healthProbeProtocolOpenAI, openAIEmpty)
	require.NoError(t, err)
	assert.Empty(t, text)

	anthropic := []byte(`{"content":[{"type":"text","text":"Hello"},{"type":"tool_use","id":"x"}]}`)
	text, err = healthProbeResponseText(healthProbeProtocolAnthropic, anthropic)
	require.NoError(t, err)
	assert.Equal(t, "Hello", text)

	_, err = healthProbeResponseText(healthProbeProtocolOpenAI, []byte(`<html>not json</html>`))
	require.Error(t, err)

	text, err = healthProbeResponseText(healthProbeProtocolOpenAI, []byte("  "))
	require.NoError(t, err)
	assert.Empty(t, text)
}

func TestHealthProbeProtocolForChannel(t *testing.T) {
	t.Parallel()

	protocol, ok := healthProbeProtocolForChannel(constant.ChannelTypeOpenAI)
	assert.True(t, ok)
	assert.Equal(t, healthProbeProtocolOpenAI, protocol)

	protocol, ok = healthProbeProtocolForChannel(constant.ChannelTypeDeepSeek)
	assert.True(t, ok)
	assert.Equal(t, healthProbeProtocolOpenAI, protocol)

	protocol, ok = healthProbeProtocolForChannel(constant.ChannelTypeAnthropic)
	assert.True(t, ok)
	assert.Equal(t, healthProbeProtocolAnthropic, protocol)

	for _, channelType := range []int{
		constant.ChannelTypeAzure,
		constant.ChannelTypeGemini,
		constant.ChannelTypeMidjourney,
		constant.ChannelTypeCodex,
		constant.ChannelTypeAdvancedCustom,
		constant.ChannelTypeVLLM,
		constant.ChannelTypeSGLang,
		constant.ChannelTypeTaskPlugin,
	} {
		_, ok := healthProbeProtocolForChannel(channelType)
		assert.False(t, ok, "channel type %d must not be probed", channelType)
	}
}

func TestHealthProbeModelForChannel(t *testing.T) {
	t.Parallel()

	testModel := "custom-test-model"
	channel := &model.Channel{TestModel: &testModel, Models: "gpt-4o,gpt-4o-mini"}
	probeModel, ok := healthProbeModelForChannel(channel, healthProbeProtocolOpenAI)
	require.True(t, ok)
	assert.Equal(t, "custom-test-model", probeModel)

	channel = &model.Channel{Models: "gpt-4o,text-embedding-3-large,gpt-4o-mini"}
	probeModel, ok = healthProbeModelForChannel(channel, healthProbeProtocolOpenAI)
	require.True(t, ok)
	assert.Equal(t, "gpt-4o-mini", probeModel)

	channel = &model.Channel{Models: "claude-sonnet-4-5,claude-3-5-haiku-20241022"}
	probeModel, ok = healthProbeModelForChannel(channel, healthProbeProtocolAnthropic)
	require.True(t, ok)
	assert.Equal(t, "claude-3-5-haiku-20241022", probeModel)

	channel = &model.Channel{Models: "deepseek-chat,deepseek-reasoner"}
	probeModel, ok = healthProbeModelForChannel(channel, healthProbeProtocolOpenAI)
	require.True(t, ok)
	assert.Equal(t, "deepseek-chat", probeModel)

	channel = &model.Channel{}
	probeModel, ok = healthProbeModelForChannel(channel, healthProbeProtocolOpenAI)
	require.True(t, ok)
	assert.Equal(t, healthProbeDefaultOpenAIModel, probeModel)

	channel = &model.Channel{}
	probeModel, ok = healthProbeModelForChannel(channel, healthProbeProtocolAnthropic)
	require.True(t, ok)
	assert.Equal(t, healthProbeDefaultAnthropicModel, probeModel)

	channel = &model.Channel{Models: "text-embedding-3-large,bge-m3"}
	_, ok = healthProbeModelForChannel(channel, healthProbeProtocolOpenAI)
	assert.False(t, ok, "an embedding-only channel has no model to probe")
}

func TestHealthProbeMappedModel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "gpt-4o-mini", healthProbeMappedModel("", "gpt-4o-mini"))
	assert.Equal(t, "gpt-4o-mini", healthProbeMappedModel("{}", "gpt-4o-mini"))
	assert.Equal(t, "gpt-4o-mini-2024-07-18",
		healthProbeMappedModel(`{"gpt-4o-mini":"gpt-4o-mini-2024-07-18"}`, "gpt-4o-mini"))
	assert.Equal(t, "c", healthProbeMappedModel(`{"a":"b","b":"c"}`, "a"), "a chain is followed to its end")
	assert.Equal(t, "a", healthProbeMappedModel(`{"a":"b","b":"a"}`, "a"), "a loop keeps the listed name")
	assert.Equal(t, "a", healthProbeMappedModel(`{"a":"a"}`, "a"))
	assert.Equal(t, "a", healthProbeMappedModel(`{"a":""}`, "a"))
	assert.Equal(t, "a", healthProbeMappedModel(`{"other":"x"}`, "a"))
	assert.Equal(t, "a", healthProbeMappedModel(`not json`, "a"))
}

func TestHealthProbeIsPlainAPIKey(t *testing.T) {
	t.Parallel()

	assert.True(t, healthProbeIsPlainAPIKey("sk-abc123"))
	assert.True(t, healthProbeIsPlainAPIKey("sk-ant-api03-xyz"))
	assert.False(t, healthProbeIsPlainAPIKey(""))
	assert.False(t, healthProbeIsPlainAPIKey(`{"type":"service_account"}`))
	assert.False(t, healthProbeIsPlainAPIKey(`["sk-a","sk-b"]`))
	assert.False(t, healthProbeIsPlainAPIKey(`"sk-quoted"`))
}

func TestHealthProbeRouteNameHidesCredentials(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "the direct route", healthProbeRouteName(""))
	name := healthProbeRouteName("http://user:secret@127.0.0.1:1082")
	assert.Equal(t, "proxy 127.0.0.1:1082", name)
	assert.NotContains(t, name, "secret")
	assert.Equal(t, "a channel proxy", healthProbeRouteName("::not a url"))
}

func TestHealthProbeVerdictStrings(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 4, common.ChannelStatusDegraded)
	assert.Equal(t, "healthy", healthProbeVerdictHealthy.String())
	assert.Equal(t, "degraded", healthProbeVerdictDegraded.String())
	assert.Equal(t, "failed", healthProbeVerdictFailed.String())
}

func healthProbeSilentResult(proxyURL string) healthProbeAccountResult {
	return healthProbeAccountResult{
		proxyURL: proxyURL,
		outcome:  healthProbeOutcome{verdict: healthProbeVerdictFailed, noResponse: true},
	}
}

func healthProbeAnsweredResult(proxyURL string, verdict healthProbeVerdict) healthProbeAccountResult {
	return healthProbeAccountResult{
		proxyURL: proxyURL,
		outcome:  healthProbeOutcome{verdict: verdict},
	}
}

func TestHealthProbeUnreachableProxies(t *testing.T) {
	t.Parallel()

	t.Run("silence from enough accounts is an outage", func(t *testing.T) {
		t.Parallel()
		results := []healthProbeAccountResult{
			healthProbeSilentResult(""), healthProbeSilentResult(""), healthProbeSilentResult(""),
		}
		assert.Equal(t, map[string]bool{"": true}, healthProbeUnreachableProxies(results))
	})

	t.Run("too few accounts to tell", func(t *testing.T) {
		t.Parallel()
		results := []healthProbeAccountResult{healthProbeSilentResult(""), healthProbeSilentResult("")}
		assert.Empty(t, healthProbeUnreachableProxies(results))
	})

	t.Run("one answer proves the route works", func(t *testing.T) {
		t.Parallel()
		results := []healthProbeAccountResult{
			healthProbeSilentResult(""), healthProbeSilentResult(""), healthProbeSilentResult(""),
			healthProbeAnsweredResult("", healthProbeVerdictFailed),
		}
		assert.Empty(t, healthProbeUnreachableProxies(results))
	})

	t.Run("dead upstreams that still answer are not an outage", func(t *testing.T) {
		t.Parallel()
		results := []healthProbeAccountResult{
			healthProbeAnsweredResult("", healthProbeVerdictFailed),
			healthProbeAnsweredResult("", healthProbeVerdictFailed),
			healthProbeAnsweredResult("", healthProbeVerdictFailed),
		}
		assert.Empty(t, healthProbeUnreachableProxies(results))
	})

	t.Run("each proxy is judged on its own", func(t *testing.T) {
		t.Parallel()
		const downProxy = "http://127.0.0.1:1082"
		results := []healthProbeAccountResult{
			healthProbeSilentResult(downProxy), healthProbeSilentResult(downProxy), healthProbeSilentResult(downProxy),
			healthProbeAnsweredResult("", healthProbeVerdictHealthy),
			healthProbeAnsweredResult("", healthProbeVerdictHealthy),
		}
		assert.Equal(t, map[string]bool{downProxy: true}, healthProbeUnreachableProxies(results))
	})
}

// healthProbeTestContext bounds one probe in a test.
func healthProbeTestContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func TestProbeChannelAccountOpenAIRequestAndHealthyAnswer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Stream    bool   `json:"stream"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "gpt-4o-mini", body.Model)
		assert.Equal(t, 5, body.MaxTokens)
		assert.False(t, body.Stream)
		if assert.Len(t, body.Messages, 1) {
			assert.Equal(t, "user", body.Messages[0].Role)
			assert.Equal(t, "hi", body.Messages[0].Content)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Hello! How can I help?"}}]}`))
	}))
	t.Cleanup(server.Close)

	outcome := probeChannelAccount(healthProbeTestContext(t, 5*time.Second), server.Client(), server.URL,
		healthProbeProtocolOpenAI, "sk-test", "gpt-4o-mini")
	assert.Equal(t, healthProbeVerdictHealthy, outcome.verdict, outcome.detail)
	assert.False(t, outcome.noResponse)
}

func TestProbeChannelAccountAnthropicRequestAndHealthyAnswer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "sk-ant-test", r.Header.Get("x-api-key"))
		assert.Equal(t, healthProbeAnthropicVersion, r.Header.Get("anthropic-version"))
		assert.Empty(t, r.Header.Get("Authorization"))

		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "claude-3-5-haiku-20241022", body.Model)
		assert.Equal(t, 5, body.MaxTokens)

		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Hi!"}]}`))
	}))
	t.Cleanup(server.Close)

	outcome := probeChannelAccount(healthProbeTestContext(t, 5*time.Second), server.Client(), server.URL,
		healthProbeProtocolAnthropic, "sk-ant-test", "claude-3-5-haiku-20241022")
	assert.Equal(t, healthProbeVerdictHealthy, outcome.verdict, outcome.detail)
}

func TestProbeChannelAccountClassifiesAnswers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		status      int
		body        string
		wantVerdict healthProbeVerdict
		wantDetail  string
	}{
		{"empty content is degraded", http.StatusOK, `{"choices":[{"message":{"content":""}}]}`, healthProbeVerdictDegraded, "empty or evasive"},
		{"no choices is degraded", http.StatusOK, `{"choices":[]}`, healthProbeVerdictDegraded, "empty or evasive"},
		{"evasive boilerplate is degraded", http.StatusOK, `{"choices":[{"message":{"content":"I'm sorry, I can't help with that."}}]}`, healthProbeVerdictDegraded, "empty or evasive"},
		{"unreadable 200 body is degraded", http.StatusOK, `<html>gateway page</html>`, healthProbeVerdictDegraded, "unreadable"},
		{"401 is failed", http.StatusUnauthorized, `{"error":{"message":"invalid api key"}}`, healthProbeVerdictFailed, "HTTP 401"},
		{"429 is failed", http.StatusTooManyRequests, `{"error":{"message":"rate limited"}}`, healthProbeVerdictFailed, "HTTP 429"},
		{"500 is failed", http.StatusInternalServerError, `upstream exploded`, healthProbeVerdictFailed, "HTTP 500"},
		{"204 is failed", http.StatusNoContent, ``, healthProbeVerdictFailed, "HTTP 204"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)

			outcome := probeChannelAccount(healthProbeTestContext(t, 5*time.Second), server.Client(), server.URL,
				healthProbeProtocolOpenAI, "sk-test", "gpt-4o-mini")
			assert.Equal(t, tc.wantVerdict, outcome.verdict, outcome.detail)
			assert.Contains(t, outcome.detail, tc.wantDetail)
			assert.False(t, outcome.noResponse, "the upstream did answer")
		})
	}
}

// A hanging upstream must cost one timeout, not the probe goroutine.
func TestProbeChannelAccountTimeoutDoesNotHang(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	// Cleanups run last in, first out: the handler is freed before Close waits.
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	startedAt := time.Now()
	outcome := probeChannelAccount(healthProbeTestContext(t, 200*time.Millisecond), server.Client(), server.URL,
		healthProbeProtocolOpenAI, "sk-test", "gpt-4o-mini")
	elapsed := time.Since(startedAt)

	assert.Equal(t, healthProbeVerdictFailed, outcome.verdict)
	assert.True(t, outcome.noResponse, "no answer arrived before the deadline")
	assert.Less(t, elapsed, 3*time.Second, "the probe must give up at its deadline")
}

// A server that answers headers and then stalls must not hold the body read.
func TestProbeChannelAccountBodyStallDoesNotHang(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	startedAt := time.Now()
	outcome := probeChannelAccount(healthProbeTestContext(t, 200*time.Millisecond), server.Client(), server.URL,
		healthProbeProtocolOpenAI, "sk-test", "gpt-4o-mini")
	elapsed := time.Since(startedAt)

	assert.Equal(t, healthProbeVerdictFailed, outcome.verdict)
	assert.Less(t, elapsed, 3*time.Second, "the body read is bounded by the same deadline")
}

func TestProbeChannelAccountRefusedConnectionIsSilence(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := server.URL
	server.Close()

	outcome := probeChannelAccount(healthProbeTestContext(t, 5*time.Second), &http.Client{}, closedURL,
		healthProbeProtocolOpenAI, "sk-test", "gpt-4o-mini")
	assert.Equal(t, healthProbeVerdictFailed, outcome.verdict)
	assert.True(t, outcome.noResponse)
}

func TestProbeChannelAccountNeverReportsTheKey(t *testing.T) {
	t.Parallel()

	const apiKey = "sk-secret-0123456789"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Incorrect API key provided: ` + apiKey + `"}`))
	}))
	t.Cleanup(server.Close)

	outcome := probeChannelAccount(healthProbeTestContext(t, 5*time.Second), server.Client(), server.URL,
		healthProbeProtocolOpenAI, apiKey, "gpt-4o-mini")
	assert.Equal(t, healthProbeVerdictFailed, outcome.verdict)
	assert.NotContains(t, outcome.detail, apiKey)
	assert.Contains(t, outcome.detail, "***")
	assert.NotContains(t, healthProbeReason(outcome.verdict, "gpt-4o-mini", outcome.detail), apiKey)
}

// A single empty answer must not take a channel out of rotation: the probe asks
// again, and a healthy second answer wins.
func TestProbeChannelAccountRetriesADegradedAnswerOnce(t *testing.T) {
	t.Parallel()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Hello!"}}]}`))
	}))
	t.Cleanup(server.Close)

	outcome := probeChannelAccount(healthProbeTestContext(t, 5*time.Second), server.Client(), server.URL,
		healthProbeProtocolOpenAI, "sk-test", "gpt-4o-mini")

	assert.Equal(t, healthProbeVerdictHealthy, outcome.verdict, outcome.detail)
	assert.Equal(t, 2, calls, "the probe asks once more before judging")
}

// Two degraded answers in a row still count as degraded.
func TestProbeChannelAccountKeepsDegradedAfterTwoEmptyAnswers(t *testing.T) {
	t.Parallel()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	t.Cleanup(server.Close)

	outcome := probeChannelAccount(healthProbeTestContext(t, 5*time.Second), server.Client(), server.URL,
		healthProbeProtocolOpenAI, "sk-test", "gpt-4o-mini")

	assert.Equal(t, healthProbeVerdictDegraded, outcome.verdict)
	assert.Equal(t, 2, calls)
}

// Reasoning models spend the tiny probe budget on thinking and answer with an
// empty message, so they must never be chosen as the probe model.
func TestHealthProbeSkipsReasoningModels(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"deepseek-r1", "qwq-32b", "o1-mini", "o3", "o4-mini", "o5-preview",
		"claude-3-7-thinking", "gemini-2.5-pro-reasoning", "gpt-5-reasoner",
	} {
		assert.True(t, healthProbeIsNonChatModel(name), "%s must not be used for probing", name)
	}
	for _, name := range []string{"gpt-4o-mini", "claude-haiku-4-5", "gemini-2.5-flash"} {
		assert.False(t, healthProbeIsNonChatModel(name), "%s is a normal chat model", name)
	}
}
