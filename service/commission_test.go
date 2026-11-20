package service

import (
	"testing"

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
// invitee's consumption credits the inviter's configured rate, rounded down,
// through both settlement entries (PostConsumeQuota and the BillingSession).
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

		assert.Equal(t, 100, commissionQuotaOf(t, inviter.Id), "inviter earns 10%% of the consumption")
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

		// 333 * 0.1 = 33.3 → 33
		assert.Equal(t, 33, commissionQuotaOf(t, inviter.Id))
		assert.Equal(t, 4667, commissionQuotaOf(t, invitee.Id))
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
