package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// openClientTestDB runs the client tests against SQLite always, and against
// MySQL/PostgreSQL when their DSNs are configured, matching the matrix the
// project requires for database behaviour.
func openClientTestDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()

	var driver gorm.Dialector
	switch dialect {
	case "sqlite":
		driver = sqlite.Open(filepath.Join(t.TempDir(), "client.db"))
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
		driver = postgres.Open(dsn)
	default:
		t.Fatalf("unknown dialect %q", dialect)
	}

	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Client{}, &Token{}))

	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	return db
}

func TestClientLifecycleDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openClientTestDB(t, dialect)

			client := Client{UserId: 1, Name: "Acme", BudgetQuota: 500000}
			require.NoError(t, CreateClient(&client))
			require.NotZero(t, client.Id)

			// A fresh client has no keys and no consumption.
			usages, total, err := GetClientsByUser(1, "", 1, 20)
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			require.Len(t, usages, 1)
			assert.Equal(t, 0, usages[0].KeyCount)
			assert.EqualValues(t, 0, usages[0].UsedQuota)

			// Two keys under the client: consumption is summed, the budget is
			// not touched by listing.
			clientId := client.Id
			require.NoError(t, db.Create(&Token{UserId: 1, Key: "k1", ClientId: &clientId, UsedQuota: 1200}).Error)
			require.NoError(t, db.Create(&Token{UserId: 1, Key: "k2", ClientId: &clientId, UsedQuota: 800}).Error)

			usages, total, err = GetClientsByUser(1, "", 1, 20)
			require.NoError(t, err)
			require.Len(t, usages, 1)
			assert.Equal(t, 2, usages[0].KeyCount)
			assert.EqualValues(t, 2000, usages[0].UsedQuota)
			assert.Equal(t, 500000, usages[0].BudgetQuota)

			// A soft-deleted key stops counting.
			require.NoError(t, db.Where("key = ?", "k2").Delete(&Token{}).Error)
			usages, total, err = GetClientsByUser(1, "", 1, 20)
			require.NoError(t, err)
			assert.Equal(t, 1, usages[0].KeyCount)
			assert.EqualValues(t, 1200, usages[0].UsedQuota)
		})
	}
}

func TestClientIsScopedToItsOwner(t *testing.T) {
	openClientTestDB(t, "sqlite")

	client := Client{UserId: 1, Name: "Acme", BudgetQuota: 100}
	require.NoError(t, CreateClient(&client))

	// Another account cannot read, update or delete it.
	_, err := GetClientById(2, client.Id)
	require.Error(t, err)

	name := "Hijacked"
	require.Error(t, UpdateClient(2, client.Id, &name, nil, nil, nil))

	require.Error(t, DeleteClient(2, client.Id))

	// The owner still sees the untouched record.
	owned, err := GetClientById(1, client.Id)
	require.NoError(t, err)
	assert.Equal(t, "Acme", owned.Name)
}

func TestDeleteClientRefusesWhileKeysRemain(t *testing.T) {
	db := openClientTestDB(t, "sqlite")

	client := Client{UserId: 1, Name: "Acme", BudgetQuota: 100}
	require.NoError(t, CreateClient(&client))

	clientId := client.Id
	token := Token{UserId: 1, Key: "attached", ClientId: &clientId}
	require.NoError(t, db.Create(&token).Error)

	err := DeleteClient(1, client.Id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still has API keys")

	require.NoError(t, db.Delete(&token).Error)
	require.NoError(t, DeleteClient(1, client.Id))
}

func TestCreateClientValidatesInput(t *testing.T) {
	openClientTestDB(t, "sqlite")

	require.Error(t, CreateClient(&Client{UserId: 1, Name: "   "}))
	require.Error(t, CreateClient(&Client{UserId: 1, Name: "Acme", BudgetQuota: -1}))

	client := Client{UserId: 1, Name: "  Acme  ", BudgetQuota: 10}
	require.NoError(t, CreateClient(&client))
	assert.Equal(t, "Acme", client.Name)
	assert.Equal(t, ClientStatusEnabled, client.Status)
	assert.Equal(t, common.GetTimestamp(), client.CreatedTime)
}

func TestUpdateClientRejectsInvalidValues(t *testing.T) {
	openClientTestDB(t, "sqlite")

	client := Client{UserId: 1, Name: "Acme", BudgetQuota: 100}
	require.NoError(t, CreateClient(&client))

	empty := "  "
	require.Error(t, UpdateClient(1, client.Id, &empty, nil, nil, nil))

	negative := -5
	require.Error(t, UpdateClient(1, client.Id, nil, &negative, nil, nil))

	badStatus := 9
	require.Error(t, UpdateClient(1, client.Id, nil, nil, &badStatus, nil))

	budget := 250
	status := ClientStatusDisabled
	require.NoError(t, UpdateClient(1, client.Id, nil, &budget, &status, nil))

	updated, err := GetClientById(1, client.Id)
	require.NoError(t, err)
	assert.Equal(t, 250, updated.BudgetQuota)
	assert.Equal(t, ClientStatusDisabled, updated.Status)
}

// Deleting a key must not erase what it consumed: the client's total keeps it.
func TestDeletingAKeyKeepsItsUsageInTheClientTotal(t *testing.T) {
	db := openClientTestDB(t, "sqlite")

	client := Client{UserId: 1, Name: "Acme", BudgetQuota: 10000}
	require.NoError(t, CreateClient(&client))

	clientId := client.Id
	token := Token{UserId: 1, Key: "k-keep", ClientId: &clientId, RemainQuota: 4000, UsedQuota: 1200}
	require.NoError(t, db.Create(&token).Error)

	usages, _, err := GetClientsByUser(1, "", 1, 20)
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.EqualValues(t, 1200, usages[0].UsedQuota)

	require.NoError(t, token.Delete())

	usages, _, err = GetClientsByUser(1, "", 1, 20)
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, 0, usages[0].KeyCount, "the key is gone")
	assert.EqualValues(t, 1200, usages[0].UsedQuota, "its usage is not")
}

func TestGetClientCommittedQuota(t *testing.T) {
	db := openClientTestDB(t, "sqlite")

	client := Client{UserId: 1, Name: "Acme", BudgetQuota: 10000}
	require.NoError(t, CreateClient(&client))

	clientId := client.Id
	require.NoError(t, db.Create(&Token{UserId: 1, Key: "a", ClientId: &clientId, RemainQuota: 500, UsedQuota: 100}).Error)
	require.NoError(t, db.Create(&Token{UserId: 1, Key: "b", ClientId: &clientId, RemainQuota: 700, UsedQuota: 50}).Error)

	committed, err := GetClientCommittedQuota(client.Id, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1350, committed)

	// Excluding a key leaves the other one in the total.
	committed, err = GetClientCommittedQuota(client.Id, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 750, committed)
}

func TestClientListPaginatesAndFiltersByKeyword(t *testing.T) {
	openClientTestDB(t, "sqlite")

	for _, name := range []string{"Acme", "Beta", "Acme Labs"} {
		client := Client{UserId: 1, Name: name, BudgetQuota: 100}
		require.NoError(t, CreateClient(&client))
	}

	firstPage, total, err := GetClientsByUser(1, "", 1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Len(t, firstPage, 2)

	secondPage, total, err := GetClientsByUser(1, "", 2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Len(t, secondPage, 1)

	filtered, total, err := GetClientsByUser(1, "Acme", 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, filtered, 2)
}

// A disabled client must actually stop serving: the relay never looks at the
// client, so its keys have to be switched off. Re-enabling revives only the keys
// this cascade switched off, not ones the user disabled by hand.
func TestClientStatusCascadesToItsKeys(t *testing.T) {
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })

	db := openClientTestDB(t, "sqlite")

	client := Client{UserId: 1, Name: "Acme", BudgetQuota: 10000}
	require.NoError(t, CreateClient(&client))

	clientId := client.Id
	live := Token{UserId: 1, Key: "live", ClientId: &clientId, Status: common.TokenStatusEnabled}
	manual := Token{UserId: 1, Key: "manual", ClientId: &clientId, Status: common.TokenStatusDisabled}
	require.NoError(t, db.Create(&live).Error)
	require.NoError(t, db.Create(&manual).Error)

	disabled := ClientStatusDisabled
	require.NoError(t, UpdateClient(1, client.Id, nil, nil, &disabled, nil))

	var got Token
	require.NoError(t, db.First(&got, live.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, got.Status)
	assert.True(t, got.DisabledByClient)

	enabled := ClientStatusEnabled
	require.NoError(t, UpdateClient(1, client.Id, nil, nil, &enabled, nil))

	require.NoError(t, db.First(&got, live.Id).Error)
	assert.Equal(t, common.TokenStatusEnabled, got.Status)
	assert.False(t, got.DisabledByClient)

	var manualGot Token
	require.NoError(t, db.First(&manualGot, manual.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, manualGot.Status, "a hand-disabled key stays off")
	assert.False(t, manualGot.DisabledByClient)
}
