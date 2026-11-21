package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/logger"
	"github.com/mazihua-lgtm/relay-forge/model"
	relaycommon "github.com/mazihua-lgtm/relay-forge/relay/common"
	"github.com/mazihua-lgtm/relay-forge/relaykit/types"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
)

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
// 会话已存在时（任务重试换到另一个渠道）把预扣补到 preConsumedQuota，
// 和首次预扣一样必须由余额覆盖。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo != nil && relayInfo.QuotaClamp != nil {
		return types.NewErrorWithStatusCode(
			relayInfo.QuotaClamp,
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("pre-consume quota cannot be negative: %d", preConsumedQuota),
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if relayInfo != nil && relayInfo.Billing != nil {
		if err := relayInfo.Billing.ReserveBeforeSubmit(preConsumedQuota); err != nil {
			var apiErr *types.NewAPIError
			if errors.As(err, &apiErr) {
				return apiErr
			}
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry())
		}
		return nil
	}
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		return apiErr
	}
	relayInfo.Billing = session
	return nil
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		delta := actualQuota - preConsumed

		if delta > 0 {
			logger.LogInfo(ctx, common.LogText("charging the difference after pre-consume: %s (actual %s, pre-consumed %s)",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			logger.LogInfo(ctx, common.LogText("refunding the difference after pre-consume: %s (actual %s, pre-consumed %s)",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, common.LogText("pre-consumed quota matches the actual quota, no adjustment: %s (per-request billing)",
				logger.FormatQuota(actualQuota),
			))
		}

		if err := relayInfo.Billing.Settle(actualQuota); err != nil {
			return err
		}

		// 发送额度通知（订阅计费使用订阅剩余额度）
		if actualQuota != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			} else {
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}
		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		if err := PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true); err != nil {
			return err
		}
	}
	// 抽成按本次实际消耗结算，与预扣差额无关：预扣刚好够用时差额为 0，
	// 但这次消费确实发生了，邀请人仍应拿到抽成。
	settleInviterCommission(relayInfo, actualQuota)
	return nil
}

// ---------------------------------------------------------------------------
// settleInviterCommission — 分销持续抽成
// ---------------------------------------------------------------------------

// settleInviterCommission 在消费扣款成功后给邀请人入账抽成。
// 抽成失败不能让已成功的消费请求失败，只记日志：钱已经扣了，丢的只是返佣。
// 两个结算入口各自调用一次且互斥（BillingSession.Settle 不会走 PostConsumeQuota，
// 反之亦然），同一次消费不会重复抽成。
func settleInviterCommission(relayInfo *relaycommon.RelayInfo, quota int) {
	if relayInfo == nil || relayInfo.UserId <= 0 || quota <= 0 {
		return
	}
	if _, err := model.SettleInviterCommission(relayInfo.UserId, quota); err != nil {
		common.SysLog(common.LogText("failed to settle inviter commission for user %d (quota=%d): %s", relayInfo.UserId, quota, err.Error()))
	}
}
