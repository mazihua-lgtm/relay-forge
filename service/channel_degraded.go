package service

import (
	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/i18n"
	"github.com/mazihua-lgtm/relay-forge/model"
	"github.com/mazihua-lgtm/relay-forge/relaykit/types"
)

// DegradeChannel marks a channel, or one key of it, degraded because the health
// probe found it answering with empty or evasive output, and tells the
// administrators who watch channels. It reports whether the status changed;
// only a change is announced, so a channel that stays degraded does not repeat
// the notice on every sweep.
//
// It is DisableChannel for the degraded status, minus two side effects that
// would add behavior the probe never had: active WebSockets are left open, and
// the plugin "scheduled" hook is not run. A degraded channel still answers, so
// there is nothing broken for either to repair, and a probe verdict that turns
// out wrong should not cut live sessions.
func DegradeChannel(channelError types.ChannelError, reason string) bool {
	common.SysLog(common.LogText("channel %q (#%d) is degraded, taking it out of rotation, reason: %s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))

	if !channelError.AutoBan {
		common.SysLog(common.LogText("channel %q (#%d) has automatic disabling turned off, skipping", channelError.ChannelName, channelError.ChannelId))
		return false
	}
	if !model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusDegraded, reason) {
		return false
	}
	params := map[string]any{"Name": channelError.ChannelName, "Id": channelError.ChannelId, "Reason": reason}
	NotifyChannelWatchers(formatNotifyType(channelError.ChannelId, common.ChannelStatusDegraded), func(lang string) (string, string) {
		return i18n.Translate(lang, i18n.MsgChannelNotifyDegradedSubject, params), i18n.Translate(lang, i18n.MsgChannelNotifyDegradedContent, params)
	})
	return true
}
