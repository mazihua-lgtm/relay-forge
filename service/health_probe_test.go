package service

import (
	"testing"

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

func TestHealthProbeCredentials(t *testing.T) {
	t.Parallel()

	assert.Equal(t, common.ChannelStatusDegraded, 4)
	assert.Equal(t, "healthy", healthProbeVerdictHealthy.String())
	assert.Equal(t, "degraded", healthProbeVerdictDegraded.String())
	assert.Equal(t, "failed", healthProbeVerdictFailed.String())
}
