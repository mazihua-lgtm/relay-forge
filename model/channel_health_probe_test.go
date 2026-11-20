package model

import (
	"testing"

	"github.com/mazihua-lgtm/relay-forge/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// openChannelHealthProbeTestDB swaps the global database for an isolated
// in-memory SQLite one, with the memory cache off so status changes go
// straight through the stored rows.
func openChannelHealthProbeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB, previousType, previousCache := DB, common.MainDatabaseType(), common.MemoryCacheEnabled
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.MemoryCacheEnabled = previousCache
		initCol()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &SystemTask{}))
	return db
}

func createHealthProbeTestChannel(t *testing.T, db *gorm.DB, status int) *Channel {
	t.Helper()
	channel := &Channel{Name: "probe-test", Type: 1, Key: "sk-test", Status: status, Models: "gpt-4o-mini", Group: "default"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{
		Group: "default", Model: "gpt-4o-mini", ChannelId: channel.Id,
		Enabled: status == common.ChannelStatusEnabled,
	}).Error)
	return channel
}

func storedChannelStatus(t *testing.T, db *gorm.DB, id int) int {
	t.Helper()
	var stored Channel
	require.NoError(t, db.Select("status").First(&stored, "id = ?", id).Error)
	return stored.Status
}

func storedAbilityEnabled(t *testing.T, db *gorm.DB, channelID int) bool {
	t.Helper()
	var ability Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", channelID).Error)
	return ability.Enabled
}

// An admin's manual disable survives the automatic statuses, the degraded one
// the health probe sets included, even when the probe read the channel before
// the admin disabled it.
func TestUpdateChannelStatusKeepsManualDisableAgainstAutomaticStatuses(t *testing.T) {
	for name, status := range map[string]int{
		"auto disabled": common.ChannelStatusAutoDisabled,
		"degraded":      common.ChannelStatusDegraded,
	} {
		t.Run(name, func(t *testing.T) {
			db := openChannelHealthProbeTestDB(t)
			channel := createHealthProbeTestChannel(t, db, common.ChannelStatusManuallyDisabled)

			changed := UpdateChannelStatus(channel.Id, "", status, "health probe")

			assert.False(t, changed)
			assert.Equal(t, common.ChannelStatusManuallyDisabled, storedChannelStatus(t, db, channel.Id))
			assert.False(t, storedAbilityEnabled(t, db, channel.Id))
		})
	}
}

// A degraded channel leaves the routing pool and a healthy probe brings it back.
func TestUpdateChannelStatusDegradedLeavesAndRejoinsRotation(t *testing.T) {
	db := openChannelHealthProbeTestDB(t)
	channel := createHealthProbeTestChannel(t, db, common.ChannelStatusEnabled)
	require.True(t, storedAbilityEnabled(t, db, channel.Id))

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusDegraded, "health probe degraded (model=gpt-4o-mini): empty"))
	assert.Equal(t, common.ChannelStatusDegraded, storedChannelStatus(t, db, channel.Id))
	assert.False(t, storedAbilityEnabled(t, db, channel.Id), "a degraded channel must not receive traffic")

	var stored Channel
	require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
	assert.Contains(t, stored.GetOtherInfo()["status_reason"], "health probe degraded")

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, ""))
	assert.Equal(t, common.ChannelStatusEnabled, storedChannelStatus(t, db, channel.Id))
	assert.True(t, storedAbilityEnabled(t, db, channel.Id))
}

// The probe result lands in the channel's existing test fields and its own
// failure counter.
func TestChannelHealthProbeResultFields(t *testing.T) {
	db := openChannelHealthProbeTestDB(t)
	channel := createHealthProbeTestChannel(t, db, common.ChannelStatusEnabled)

	channel.IncrementErrorCount()
	channel.IncrementErrorCount()
	assert.Equal(t, 2, channel.ErrorCount)
	var stored Channel
	require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
	assert.Equal(t, 2, stored.ErrorCount)

	// The in-memory copy of a swept channel does not carry the stored counter;
	// the reset must reach the row regardless.
	fresh := &Channel{Id: channel.Id}
	fresh.ResetErrorCount()
	require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
	assert.Equal(t, 0, stored.ErrorCount)

	channel.UpdateResponseTime(1234)
	require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
	assert.Equal(t, 1234, stored.ResponseTime)
	assert.NotZero(t, stored.TestTime)
}
