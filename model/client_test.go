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
			usages, err := GetClientsByUser(1)
			require.NoError(t, err)
			require.Len(t, usages, 1)
			assert.Equal(t, 0, usages[0].KeyCount)
			assert.EqualValues(t, 0, usages[0].UsedQuota)

			// Two keys under the client: consumption is summed, the budget is
			// not touched by listing.
			clientId := client.Id
			require.NoError(t, db.Create(&Token{UserId: 1, Key: "k1", ClientId: &clientId, UsedQuota: 1200}).Error)
			require.NoError(t, db.Create(&Token{UserId: 1, Key: "k2", ClientId: &clientId, UsedQuota: 800}).Error)

			usages, err = GetClientsByUser(1)
			require.NoError(t, err)
			require.Len(t, usages, 1)
			assert.Equal(t, 2, usages[0].KeyCount)
			assert.EqualValues(t, 2000, usages[0].UsedQuota)
			assert.Equal(t, 500000, usages[0].BudgetQuota)

			// A soft-deleted key stops counting.
			require.NoError(t, db.Where("key = ?", "k2").Delete(&Token{}).Error)
			usages, err = GetClientsByUser(1)
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
