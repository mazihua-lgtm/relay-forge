package model

import (
	"math"
	"testing"

	"github.com/mazihua-lgtm/relay-forge/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createCommissionTestUser(t *testing.T, username string, inviterId int, rate float64) User {
	t.Helper()
	user := User{
		Username:       username,
		Password:       "unused-password-hash",
		Role:           common.RoleCommonUser,
		Status:         common.UserStatusEnabled,
		Group:          "default",
		AuthVersion:    1,
		AffCode:        username + "-aff",
		InviterId:      inviterId,
		CommissionRate: rate,
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func commissionSystemLogCount(t *testing.T, userId int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", userId, LogTypeSystem).Count(&count).Error)
	return count
}

// The commission is the inviter's rate of what the invitee consumed, rounded
// down to a whole quota. A commission that rounds to nothing credits and logs
// nothing.
func TestSettleInviterCommissionRoundsDown(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rate  float64
		quota int
		want  int
	}{
		// 100 * 0.29 evaluates to 28.999999999999996 in float64; decimal math keeps it at 29.
		{"float noise does not shave a quota", 0.29, 100, 29},
		// MySQL's float column is single precision: 0.29 comes back as 0.28999999165534973.
		{"a rate read back from a single-precision column", float64(float32(0.29)), 100, 29},
		{"fraction is dropped", 0.1, 333, 33},
		{"full rate", 1, 7, 7},
		{"less than one quota pays nothing", 0.5, 1, 0},
		{"a rate above 1 is clamped to 1", 5, 40, 40},
		{"zero rate", 0, 1000, 0},
		{"no consumption", 0.5, 0, 0},
		{"a refund earns no commission", 0.5, -100, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupUserUpdateTestState(t)
			inviter := createCommissionTestUser(t, "rounding-inviter", 0, tc.rate)
			invitee := createCommissionTestUser(t, "rounding-invitee", inviter.Id, 0)

			got, err := SettleInviterCommission(invitee.Id, tc.quota)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)

			var reloaded User
			require.NoError(t, DB.Select("id", "quota").First(&reloaded, inviter.Id).Error)
			assert.Equal(t, tc.want, reloaded.Quota)

			wantLogs := int64(0)
			if tc.want > 0 {
				wantLogs = 1
			}
			assert.Equal(t, wantLogs, commissionSystemLogCount(t, inviter.Id))
		})
	}
}

// Only a live, different inviter earns commission: no inviter, an inviter that
// is the invitee itself, and a deleted inviter all resolve to nobody.
func TestGetInviterCommissionIgnoresInvalidInviters(t *testing.T) {
	setupUserUpdateTestState(t)

	inviter := createCommissionTestUser(t, "lookup-inviter", 0, 0.2)
	invitee := createCommissionTestUser(t, "lookup-invitee", inviter.Id, 0)

	id, rate, err := GetInviterCommission(invitee.Id)
	require.NoError(t, err)
	assert.Equal(t, inviter.Id, id)
	assert.Equal(t, 0.2, rate)

	for name, inviteeId := range map[string]int{
		"user without an inviter": inviter.Id,
		"zero id":                 0,
		"negative id":             -1,
		"unknown user":            987654,
	} {
		id, rate, err = GetInviterCommission(inviteeId)
		require.NoError(t, err, name)
		assert.Zero(t, id, name)
		assert.Zero(t, rate, name)
	}

	self := createCommissionTestUser(t, "lookup-self", 0, 0.5)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", self.Id).Update("inviter_id", self.Id).Error)
	id, rate, err = GetInviterCommission(self.Id)
	require.NoError(t, err)
	assert.Zero(t, id, "a user who is its own inviter earns nothing from itself")
	assert.Zero(t, rate)

	require.NoError(t, DB.Delete(&User{}, inviter.Id).Error)
	id, rate, err = GetInviterCommission(invitee.Id)
	require.NoError(t, err)
	assert.Zero(t, id, "a deleted inviter earns nothing")
	assert.Zero(t, rate)
}

// The rate is only writable through UpdateUserCommissionRate. A profile update
// built from a stale snapshot must not roll back what an admin set meanwhile.
func TestUserUpdateDoesNotOverwriteCommissionRate(t *testing.T) {
	setupUserUpdateTestState(t)
	user := createCommissionTestUser(t, "commission-update-user", 0, 0.3)

	stale, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	require.NoError(t, UpdateUserCommissionRate(user.Id, 0.5))

	stale.DisplayName = "renamed"
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return stale.UpdateWithTx(tx, false)
	}))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "renamed", got.DisplayName)
	assert.Equal(t, 0.5, got.CommissionRate, "the stale snapshot must not roll the rate back")
}

func TestUpdateUserCommissionRate(t *testing.T) {
	setupUserUpdateTestState(t)
	user := createCommissionTestUser(t, "rate-user", 0, 0.3)

	rateOf := func() float64 {
		var got User
		require.NoError(t, DB.Select("id", "commission_rate").First(&got, user.Id).Error)
		return got.CommissionRate
	}

	require.NoError(t, UpdateUserCommissionRate(user.Id, 0))
	assert.Equal(t, 0.0, rateOf(), "zero is a real value and must be written")
	require.NoError(t, UpdateUserCommissionRate(user.Id, 1))
	assert.Equal(t, 1.0, rateOf())
	require.NoError(t, UpdateUserCommissionRate(user.Id, 1), "writing the stored value again is not an error")

	for _, rate := range []float64{-0.01, 1.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		assert.Error(t, UpdateUserCommissionRate(user.Id, rate), "rate %v", rate)
	}
	assert.Equal(t, 1.0, rateOf(), "a rejected rate leaves the stored one alone")

	assert.ErrorIs(t, UpdateUserCommissionRate(424242, 0.5), gorm.ErrRecordNotFound)
	assert.Error(t, UpdateUserCommissionRate(0, 0.5))
}
