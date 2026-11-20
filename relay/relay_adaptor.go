package relay

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mazihua-lgtm/relay-forge/constant"
	pluginruntime "github.com/mazihua-lgtm/relay-forge/pkg/jsplugin"
	_ "github.com/mazihua-lgtm/relay-forge/plugins"
	"github.com/mazihua-lgtm/relay-forge/relay/channel"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/advancedcustom"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/ali"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/aws"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/baidu"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/baidu_v2"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/claude"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/cloudflare"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/codex"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/cohere"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/coze"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/deepseek"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/dify"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/gemini"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/jimeng"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/jina"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/minimax"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/mistral"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/mokaai"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/moonshot"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/newapi"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/ollama"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/openai"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/palm"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/perplexity"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/providerplugin"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/replicate"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/siliconflow"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/sub2api"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/submodel"
	jspluginadaptor "github.com/mazihua-lgtm/relay-forge/relay/channel/task/jsplugin"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/tencent"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/vertex"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/volcengine"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/xai"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/xunfei"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/zhipu"
	"github.com/mazihua-lgtm/relay-forge/relay/channel/zhipu_4v"
)

func GetAdaptor(apiType int) channel.Adaptor {
	switch apiType {
	case constant.APITypeAli:
		return &ali.Adaptor{}
	case constant.APITypeAnthropic:
		return &claude.Adaptor{}
	case constant.APITypeBaidu:
		return &baidu.Adaptor{}
	case constant.APITypeGemini:
		return &gemini.Adaptor{}
	case constant.APITypeOpenAI:
		return &openai.Adaptor{}
	case constant.APITypePaLM:
		return &palm.Adaptor{}
	case constant.APITypeTencent:
		return &tencent.DispatchAdaptor{}
	case constant.APITypeXunfei:
		return &xunfei.Adaptor{}
	case constant.APITypeZhipu:
		return &zhipu.Adaptor{}
	case constant.APITypeZhipuV4:
		return &zhipu_4v.Adaptor{}
	case constant.APITypeOllama:
		return &ollama.Adaptor{}
	case constant.APITypePerplexity:
		return &perplexity.Adaptor{}
	case constant.APITypeAws:
		return &aws.Adaptor{}
	case constant.APITypeCohere:
		return &cohere.Adaptor{}
	case constant.APITypeDify:
		return &dify.Adaptor{}
	case constant.APITypeJina:
		return &jina.Adaptor{}
	case constant.APITypeCloudflare:
		return &cloudflare.Adaptor{}
	case constant.APITypeSiliconFlow:
		return &siliconflow.Adaptor{}
	case constant.APITypeVertexAi:
		return &vertex.Adaptor{}
	case constant.APITypeMistral:
		return &mistral.Adaptor{}
	case constant.APITypeDeepSeek:
		return &deepseek.Adaptor{}
	case constant.APITypeMokaAI:
		return &mokaai.Adaptor{}
	case constant.APITypeVolcEngine:
		return &volcengine.Adaptor{}
	case constant.APITypeBaiduV2:
		return &baidu_v2.Adaptor{}
	case constant.APITypeOpenRouter:
		return &openai.Adaptor{}
	case constant.APITypeXinference:
		return &openai.Adaptor{}
	case constant.APITypeXai:
		return &xai.Adaptor{}
	case constant.APITypeCoze:
		return &coze.Adaptor{}
	case constant.APITypeJimeng:
		return &jimeng.Adaptor{}
	case constant.APITypeMoonshot:
		return &moonshot.Adaptor{} // Moonshot uses Claude API
	case constant.APITypeSubmodel:
		return &submodel.Adaptor{}
	case constant.APITypeMiniMax:
		return &minimax.Adaptor{}
	case constant.APITypeReplicate:
		return &replicate.Adaptor{}
	case constant.APITypeCodex:
		return &codex.Adaptor{}
	case constant.APITypeAdvancedCustom:
		return &advancedcustom.Adaptor{}
	case constant.APITypeSub2API:
		return &sub2api.Adaptor{}
	case constant.APITypeNewAPI:
		return &newapi.Adaptor{}
	case constant.APITypeProviderPlugin:
		return &providerplugin.Adaptor{}
	}
	return nil
}

func GetTaskPlatform(c *gin.Context) constant.TaskPlatform {
	if pluginKey := c.GetString("task_plugin_key"); pluginKey != "" {
		return constant.TaskPlatform(pluginKey)
	}
	channelType := c.GetInt("channel_type")
	if channelType > 0 {
		return constant.TaskPlatform(strconv.Itoa(channelType))
	}
	return constant.TaskPlatform(c.GetString("platform"))
}

var taskPluginKeys = map[constant.TaskPlatform]string{
	constant.TaskPlatformSuno:                                            "sunoapi",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAli)):         "alibaba",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeKling)):       "kling",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeJimeng)):      "jimeng",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVidu)):        "vidu",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)): "doubao",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVolcEngine)):  "doubao",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeGemini)):      "google",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMax)):     "hailuo",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora)):        "sora",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeOpenAI)):      "sora",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVertexAi)):    "vertex-ai",
}

func ResolveTaskPluginForPlatform(generation *pluginruntime.RoutingGeneration, platform constant.TaskPlatform) (*pluginruntime.LoadedPlugin, bool) {
	if generation == nil {
		return nil, false
	}
	if key, ok := taskPluginKeys[platform]; ok {
		if plugin, found := generation.Get(key); found {
			return plugin, true
		}
	}
	return generation.Get(string(platform))
}

// TaskPlatformUnavailableError explains why no adaptor serves the platform:
// the task-plugin system is switched off, the resolved plugin is disabled,
// or the platform simply names nothing. The distinction is user-actionable,
// so it must survive into the client-facing message.
func TaskPlatformUnavailableError(platform constant.TaskPlatform) (string, string) {
	if !pluginruntime.DefaultRegistry.Enabled() {
		return "task_plugin_system_disabled", "the task plugin system is disabled on this gateway"
	}
	key := string(platform)
	if mapped, ok := taskPluginKeys[platform]; ok {
		key = mapped
	}
	for _, meta := range pluginruntime.DefaultRegistry.Snapshot().Factory {
		if meta.Key == key {
			return "task_plugin_disabled", fmt.Sprintf("task plugin %q is disabled on this gateway", key)
		}
	}
	return "invalid_api_platform", fmt.Sprintf("invalid api platform: %s", platform)
}

func GetTaskAdaptor(platform constant.TaskPlatform) channel.TaskAdaptor {
	plugin, ok := ResolveTaskPluginForPlatform(pluginruntime.DefaultRegistry.Generation(), platform)
	if !ok {
		return nil
	}
	return jspluginadaptor.New(plugin)
}

// getTaskAdaptorForRequest preserves the exact plugin object pinned by the
// declarative or shared-endpoint router. Legacy task routes are pinned here
// from one registry generation before the adaptor is returned.
func getTaskAdaptorForRequest(c *gin.Context, platform constant.TaskPlatform) (constant.TaskPlatform, channel.TaskAdaptor) {
	if c != nil {
		if value, exists := c.Get(pluginruntime.ContextKeyPinnedPlugin); exists {
			if pinned, ok := value.(pluginruntime.PinnedPlugin); ok && pinned.Plugin != nil {
				platform = constant.TaskPlatform(pinned.Plugin.Meta.Key)
				return platform, jspluginadaptor.New(pinned.Plugin)
			}
			return platform, nil
		}
		if value, exists := c.Get(pluginruntime.ContextKeyPinnedEndpoint); exists {
			if pinned, ok := value.(pluginruntime.PinnedEndpoint); ok && pinned.Plugin != nil {
				platform = constant.TaskPlatform(pinned.Plugin.Meta.Key)
				return platform, jspluginadaptor.New(pinned.Plugin)
			}
			return platform, nil
		}
		if value, exists := c.Get(pluginruntime.ContextKeyPinnedRoute); exists {
			if pinned, ok := value.(pluginruntime.PinnedRoute); ok && pinned.Plugin != nil {
				platform = constant.TaskPlatform(pinned.Plugin.Meta.Key)
				return platform, jspluginadaptor.New(pinned.Plugin)
			}
			return platform, nil
		}
	}
	generation := pluginruntime.DefaultRegistry.Generation()
	plugin, ok := ResolveTaskPluginForPlatform(generation, platform)
	if !ok {
		return platform, nil
	}
	if c != nil {
		c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{
			Generation: generation,
			Plugin:     plugin,
		})
	}
	return platform, jspluginadaptor.New(plugin)
}
