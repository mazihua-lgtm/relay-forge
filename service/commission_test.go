package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/model"
	relaycommon "github.com/mazihua-lgtm/relay-forge/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The commission tests own the users/logs tables: each case starts from an
// empty pair of tables so balances and log counts are exact.
func resetCommissionState(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Exec("DELETE FROM logs").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM users").Error)
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM logs")
		model.DB.Exec("DELETE FROM users")
	})
}

func newCommissionTestUser(t *testing.T, username string, quota int, inviterId int, rate float64) model.User {
	t.Helper()
	user := model.User{
		Username:       username,
		Password:       "unused-password-hash",
		Role:           common.RoleCommonUser,
		Status:         common.UserStatusEnabled,
		Group:          "default",
		Quota:          quota,
		AffCode:        username + "-aff",
		InviterId:      inviterId,
		CommissionRate: rate,
		AuthVersion:    1,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	return user
}

func commissionQuotaOf(t *testing.T, userId int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("id", "quota").First(&user, "id = ?", userId).Error)
	return user.Quota
}

func inviterSystemLogs(t *testing.T, userId int) []model.Log {
	t.Helper()
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userId, model.LogTypeSystem).Order("id").Find(&logs).Error)
	return logs
}

// TestCommissionSettle covers the distributor's continuous commission: an
// invitee's consumption credits the inviter's configured rate, rounded down.
// Commission is settled by the callers with the full consumed amount, never with
// the pre-consume delta, so a request whose pre-consumption matched the actual
// cost still pays.
func TestCommissionSettle(t *testing.T) {
	previousBatch, previousRedis := common.BatchUpdateEnabled, common.RedisEnabled
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled, common.RedisEnabled = previousBatch, previousRedis
	})

	t.Run("invitee consumption credits the inviter's rate", func(t *testing.T) {
		resetCommissionState(t)
		inviter := newCommissionTestUser(t, "commission-inviter", 0, 0, 0.1)
		invitee := newCommissionTestUser(t, "commission-invitee", 5000, inviter.Id, 0)

		relayInfo := &relaycommon.RelayInfo{UserId: invitee.Id, IsPlayground: true}
		require.NoError(t, PostConsumeQuota(relayInfo, 1000, 0, false))
		settleInviterCommission(relayInfo, 1000)

		assert.Equal(t, 100, commissionQuotaOf(t, inviter.Id), "inviter earns 10% of the consumption")
		assert.Equal(t, 4000, commissionQuotaOf(t, invitee.Id), "invitee still pays the full consumption")

		logs := inviterSystemLogs(t, inviter.Id)
		require.Len(t, logs, 1)
		assert.Contains(t, logs[0].Content, "下级消费抽成")
	})

	t.Run("commission rounds down to a whole quota", func(t *testing.T) {
		resetCommissionState(t)
		inviter := newCommissionTestUser(t, "commission-floor-inviter", 0, 0, 0.1)
		invitee := newCommissionTestUser(t, "commission-floor-invitee", 5000, inviter.Id, 0)

		relayInfo := &relaycommon.RelayInfo{UserId: invitee.Id, IsPlayground: true}
		require.NoError(t, PostConsumeQuota(relayInfo, 333, 0, false))
		settleInviterCommission(relayInfo, 333)

		// 333 * 0.1 = 33.3 → 33
		assert.Equal(t, 33, commissionQuotaOf(t, inviter.Id))
		assert.Equal(t, 4667, commissionQuotaOf(t, invitee.Id))
	})

	t.Run("a pre-consumption that matches the actual cost still pays commission", func(t *testing.T) {
		resetCommissionState(t)
		inviter := newCommissionTestUser(t, "commission-delta-inviter", 0, 0, 0.1)
		invitee := newCommissionTestUser(t, "commission-delta-invitee", 5000, inviter.Id, 0)

		// 没有 BillingSession，且预扣额度正好等于实际消耗：差额为 0。
		relayInfo := &relaycommon.RelayInfo{
			UserId:                invitee.Id,
			IsPlayground:          true,
			FinalPreConsumedQuota: 500,
		}
		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		require.NoError(t, SettleBilling(ginCtx, relayInfo, 500))

		assert.Equal(t, 50, commissionQuotaOf(t, inviter.Id), "a zero delta must not swallow the commission")
	})

	t.Run("a zero rate pays nothing", func(t *testing.T) {
		resetCommissionState(t)
		inviter := newCommissionTestUser(t, "commission-zero-inviter", 0, 0, 0)
		invitee := newCommissionTestUser(t, "commission-zero-invitee", 5000, inviter.Id, 0)

		relayInfo := &relaycommon.RelayInfo{UserId: invitee.Id, IsPlayground: true}
		require.NoError(t, PostConsumeQuota(relayInfo, 1000, 0, false))

		assert.Equal(t, 0, commissionQuotaOf(t, inviter.Id))
		assert.Empty(t, inviterSystemLogs(t, inviter.Id))
	})

	t.Run("an invitee without inviter pays nothing", func(t *testing.T) {
		resetCommissionState(t)
		invitee := newCommissionTestUser(t, "commission-lone-invitee", 5000, 0, 0)

		relayInfo := &relaycommon.RelayInfo{UserId: invitee.Id, IsPlayground: true}
		require.NoError(t, PostConsumeQuota(relayInfo, 1000, 0, false))

		assert.Equal(t, 4000, commissionQuotaOf(t, invitee.Id))
		assert.Empty(t, inviterSystemLogs(t, invitee.Id))
	})

	t.Run("BillingSession settles the commission once", func(t *testing.T) {
		resetCommissionState(t)
		inviter := newCommissionTestUser(t, "commission-session-inviter", 0, 0, 0.1)
		invitee := newCommissionTestUser(t, "commission-session-invitee", 5000, inviter.Id, 0)

		relayInfo := &relaycommon.RelayInfo{UserId: invitee.Id, IsPlayground: true}
		funding := &WalletFunding{userId: invitee.Id}
		require.NoError(t, funding.PreConsume(1000))
		session := &BillingSession{relayInfo: relayInfo, funding: funding, preConsumedQuota: 1000}

		// The pre-consumed amount covers the actual cost: delta is zero, yet
		// the consumption is real and earns the commission.
		require.NoError(t, session.Settle(1000))
		require.NoError(t, session.Settle(1000), "a second settle is a no-op")

		assert.Equal(t, 100, commissionQuotaOf(t, inviter.Id))
		assert.Equal(t, 4000, commissionQuotaOf(t, invitee.Id))
		require.Len(t, inviterSystemLogs(t, inviter.Id), 1)
	})

	t.Run("BillingSession settles a positive delta and credits commission", func(t *testing.T) {
		resetCommissionState(t)
		inviter := newCommissionTestUser(t, "commission-delta-inviter", 0, 0, 0.2)
		invitee := newCommissionTestUser(t, "commission-delta-invitee", 5000, inviter.Id, 0)

		relayInfo := &relaycommon.RelayInfo{UserId: invitee.Id, IsPlayground: true}
		session := &BillingSession{
			relayInfo:        relayInfo,
			funding:          &WalletFunding{userId: invitee.Id},
			preConsumedQuota: 0,
		}
		require.NoError(t, session.Settle(500))

		assert.Equal(t, 100, commissionQuotaOf(t, inviter.Id), "500 * 0.2")
		assert.Equal(t, 4500, commissionQuotaOf(t, invitee.Id))
	})
}

// A refunded task must take the commission back; otherwise a user could farm
// commission by submitting tasks that always fail.
func TestCommissionClawbackOnRefund(t *testing.T) {
	previousBatch, previousRedis := common.BatchUpdateEnabled, common.RedisEnabled
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled, common.RedisEnabled = previousBatch, previousRedis
	})

	resetCommissionState(t)
	inviter := newCommissionTestUser(t, "clawback-inviter", 0, 0, 0.1)
	invitee := newCommissionTestUser(t, "clawback-invitee", 5000, inviter.Id, 0)

	relayInfo := &relaycommon.RelayInfo{UserId: invitee.Id, IsPlayground: true}
	require.NoError(t, PostConsumeQuota(relayInfo, 1000, 0, false))
	settleInviterCommission(relayInfo, 1000)
	require.Equal(t, 100, commissionQuotaOf(t, inviter.Id))

	clawed, err := model.ClawbackInviterCommission(invitee.Id, 1000)
	require.NoError(t, err)
	assert.Equal(t, 100, clawed)
	assert.Equal(t, 0, commissionQuotaOf(t, inviter.Id), "the commission is reversed")

	logs := inviterSystemLogs(t, inviter.Id)
	require.Len(t, logs, 2)
	assert.Contains(t, logs[1].Content, "任务退款冲回抽成")
}

// The clawback is proportional: a partial refund only takes back its share.
func TestCommissionClawbackIsProportional(t *testing.T) {
	previousBatch, previousRedis := common.BatchUpdateEnabled, common.RedisEnabled
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled, common.RedisEnabled = previousBatch, previousRedis
	})

	resetCommissionState(t)
	inviter := newCommissionTestUser(t, "clawback-part-inviter", 0, 0, 0.1)
	invitee := newCommissionTestUser(t, "clawback-part-invitee", 5000, inviter.Id, 0)

	relayInfo := &relaycommon.RelayInfo{UserId: invitee.Id, IsPlayground: true}
	require.NoError(t, PostConsumeQuota(relayInfo, 1000, 0, false))
	settleInviterCommission(relayInfo, 1000)
	require.Equal(t, 100, commissionQuotaOf(t, inviter.Id))

	clawed, err := model.ClawbackInviterCommission(invitee.Id, 400)
	require.NoError(t, err)
	assert.Equal(t, 40, clawed)
	assert.Equal(t, 60, commissionQuotaOf(t, inviter.Id))
}
