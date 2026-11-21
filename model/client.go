package model

import (
	"errors"
	"strings"

	"github.com/mazihua-lgtm/relay-forge/common"
	"gorm.io/gorm"
)

// Client status values. They are deliberately separate from the user and
// channel constants so a change to either cannot silently change this one.
const (
	ClientStatusEnabled  = 1
	ClientStatusDisabled = 2
)

// Client is a downstream customer of a reseller. The reseller (UserId) creates
// one per customer, gives it a budget — what the customer paid — and issues API
// keys under it. Consumption is read from those keys, so the margin is
// budget minus what the customer actually used.
type Client struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"index"`
	Name        string `json:"name" gorm:"type:varchar(64);not null"`
	BudgetQuota int    `json:"budget_quota" gorm:"bigint;default:0"`
	// UsedQuotaSnapshot keeps the consumption of keys that were deleted. Without
	// it, deleting a key would silently erase that usage from the client's total
	// and overstate the margin.
	UsedQuotaSnapshot int64  `json:"used_quota_snapshot" gorm:"bigint;default:0"`
	Status            int    `json:"status" gorm:"default:1"`
	Remark            string `json:"remark" gorm:"type:varchar(255);default:''"`
	CreatedTime       int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime       int64  `json:"updated_time" gorm:"bigint"`
}

// ClientUsage is a client plus what its keys have consumed.
type ClientUsage struct {
	Client
	KeyCount  int   `json:"key_count"`
	UsedQuota int64 `json:"used_quota"`
}

type clientKeyAggregate struct {
	ClientId  int
	KeyCount  int
	UsedQuota int64
}

// CreateClient stores a client owned by userId.
func CreateClient(client *Client) error {
	client.Name = strings.TrimSpace(client.Name)
	if client.Name == "" {
		return errors.New("client name is required")
	}
	if client.BudgetQuota < 0 {
		return errors.New("budget cannot be negative")
	}
	if client.Status == 0 {
		client.Status = ClientStatusEnabled
	}
	now := common.GetTimestamp()
	client.CreatedTime = now
	client.UpdatedTime = now
	return DB.Create(client).Error
}

// GetClientById returns a client only when it belongs to userId, so one
// reseller can never read another's customer.
func GetClientById(userId int, id int) (*Client, error) {
	var client Client
	err := DB.Where("id = ? AND user_id = ?", id, userId).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// GetClientsByUser lists a reseller's clients with their key count and
// consumption. Aggregation happens in a second query rather than a GROUP BY on
// the client row, which keeps the result identical on SQLite, MySQL and
// PostgreSQL.
func GetClientsByUser(userId int, keyword string, page int, pageSize int) ([]ClientUsage, int64, error) {
	query := DB.Model(&Client{}).Where("user_id = ?", userId)
	if trimmed := strings.TrimSpace(keyword); trimmed != "" {
		query = query.Where("name LIKE ?", "%"+trimmed+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var clients []Client
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&clients).Error; err != nil {
		return nil, 0, err
	}
	usages := make([]ClientUsage, 0, len(clients))
	if len(clients) == 0 {
		return usages, total, nil
	}

	ids := make([]int, 0, len(clients))
	for _, client := range clients {
		ids = append(ids, client.Id)
	}

	var aggregates []clientKeyAggregate
	err := DB.Model(&Token{}).
		Select("client_id, COUNT(*) AS key_count, COALESCE(SUM(used_quota), 0) AS used_quota").
		Where("client_id IN ?", ids).
		Group("client_id").
		Scan(&aggregates).Error
	if err != nil {
		return nil, 0, err
	}

	byClient := make(map[int]clientKeyAggregate, len(aggregates))
	for _, aggregate := range aggregates {
		byClient[aggregate.ClientId] = aggregate
	}

	for _, client := range clients {
		usage := ClientUsage{Client: client}
		if aggregate, ok := byClient[client.Id]; ok {
			usage.KeyCount = aggregate.KeyCount
			usage.UsedQuota = aggregate.UsedQuota
		}
		usage.UsedQuota += client.UsedQuotaSnapshot
		usages = append(usages, usage)
	}
	return usages, total, nil
}

// GetClientCommittedQuota returns what the client's keys have committed: each
// live key's remaining plus used quota, plus the snapshot of deleted keys. A new
// or edited key must keep the total within the client's budget.
func GetClientCommittedQuota(clientId int, excludeTokenId int) (int64, error) {
	query := DB.Model(&Token{}).Where("client_id = ?", clientId)
	if excludeTokenId > 0 {
		query = query.Where("id <> ?", excludeTokenId)
	}

	var committed int64
	if err := query.Select("COALESCE(SUM(remain_quota + used_quota), 0)").Scan(&committed).Error; err != nil {
		return 0, err
	}

	var client Client
	if err := DB.Select("used_quota_snapshot").Where("id = ?", clientId).First(&client).Error; err != nil {
		return 0, err
	}
	return committed + client.UsedQuotaSnapshot, nil
}

// AddClientUsedQuota folds a deleted key's consumption into the client total.
func AddClientUsedQuota(clientId int, delta int64) error {
	if clientId == 0 || delta <= 0 {
		return nil
	}
	return DB.Model(&Client{}).Where("id = ?", clientId).
		UpdateColumn("used_quota_snapshot", gorm.Expr("used_quota_snapshot + ?", delta)).Error
}

// UpdateClient applies the editable fields of a client the reseller owns.
// A nil field is left unchanged.
func UpdateClient(userId int, id int, name *string, budget *int, status *int, remark *string) error {
	updates := map[string]any{"updated_time": common.GetTimestamp()}

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return errors.New("client name is required")
		}
		updates["name"] = trimmed
	}
	if budget != nil {
		if *budget < 0 {
			return errors.New("budget cannot be negative")
		}
		updates["budget_quota"] = *budget
	}
	if status != nil {
		if *status != ClientStatusEnabled && *status != ClientStatusDisabled {
			return errors.New("invalid client status")
		}
		updates["status"] = *status
	}
	if remark != nil {
		updates["remark"] = strings.TrimSpace(*remark)
	}

	result := DB.Model(&Client{}).Where("id = ? AND user_id = ?", id, userId).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("client not found")
	}

	if status != nil {
		if err := applyClientStatusToKeys(id, *status); err != nil {
			return err
		}
	}
	return nil
}

// applyClientStatusToKeys makes a client's suspension effective on the relay
// path: the distributor never looks at the client, so a disabled client must
// actually switch its keys off. Keys the user disabled by hand are left alone —
// only the ones this function switched off are revived.
func applyClientStatusToKeys(clientId int, status int) error {
	where := "client_id = ? AND status = ?"
	args := []any{clientId, common.TokenStatusEnabled}
	updates := map[string]any{"status": common.TokenStatusDisabled, "disabled_by_client": true}
	if status != ClientStatusDisabled {
		where = "client_id = ? AND disabled_by_client = ?"
		args = []any{clientId, true}
		updates = map[string]any{"status": common.TokenStatusEnabled, "disabled_by_client": false}
	}

	// The relay accepts a key from cache, so the cached copy has to go with the
	// row; otherwise a suspended key keeps working until its cache entry expires.
	var keys []string
	if err := DB.Model(&Token{}).Where(where, args...).Pluck("key", &keys).Error; err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	if err := DB.Model(&Token{}).Where(where, args...).Updates(updates).Error; err != nil {
		return err
	}
	for _, key := range keys {
		if err := invalidateTokenCacheForMutation(key); err != nil {
			common.SysLog(common.LogText("failed to invalidate token cache after a client status change: %s", err.Error()))
		}
	}
	return nil
}

// DeleteClient removes a client only when no API key still points at it, so a
// delete can never orphan a key that is in active use.
func DeleteClient(userId int, id int) error {
	var keyCount int64
	if err := DB.Model(&Token{}).Where("client_id = ?", id).Count(&keyCount).Error; err != nil {
		return err
	}
	if keyCount > 0 {
		return errors.New("client still has API keys; remove or reassign them first")
	}

	result := DB.Where("id = ? AND user_id = ?", id, userId).Delete(&Client{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("client not found")
	}
	return nil
}
