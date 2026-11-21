package controller

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mazihua-lgtm/relay-forge/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openClientBudgetDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "budget.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Client{}, &model.Token{}))

	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
}

// A key filed under a client must fit inside that client's budget, counting the
// quota its sibling keys have already committed.
func TestValidateClientKeyQuota(t *testing.T) {
	openClientBudgetDB(t)

	client := model.Client{UserId: 1, Name: "Acme", BudgetQuota: 1000}
	require.NoError(t, model.CreateClient(&client))

	clientId := client.Id
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: 1, Key: "existing", ClientId: &clientId, RemainQuota: 400, UsedQuota: 100,
	}).Error)

	cases := []struct {
		name        string
		exclude     int
		unlimited   bool
		remainQuota int
		wantErr     string
	}{
		{
			name:        "unlimited quota is rejected under a client",
			unlimited:   true,
			remainQuota: 0,
			wantErr:     "cannot have unlimited quota",
		},
		{
			name:        "negative quota is rejected",
			remainQuota: -1,
			wantErr:     "quota cannot be negative",
		},
		{
			name:        "quota that fits is accepted",
			remainQuota: 500,
		},
		{
			name:        "quota beyond the remaining budget is rejected",
			remainQuota: 501,
			wantErr:     "exceeds the client's remaining budget",
		},
		{
			name:        "editing a key excludes its own committed quota",
			exclude:     1,
			remainQuota: 900,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateClientKeyQuota(&client, tc.exclude, tc.unlimited, tc.remainQuota)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// A client whose budget is fully committed cannot take another key at all.
func TestValidateClientKeyQuotaRejectsExhaustedBudget(t *testing.T) {
	openClientBudgetDB(t)

	client := model.Client{UserId: 1, Name: "Acme", BudgetQuota: 1000}
	require.NoError(t, model.CreateClient(&client))

	clientId := client.Id
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: 1, Key: "full", ClientId: &clientId, RemainQuota: 900, UsedQuota: 100,
	}).Error)

	err := validateClientKeyQuota(&client, 0, false, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already committed")
}
