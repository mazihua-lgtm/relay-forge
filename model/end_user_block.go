package model

import (
	"time"
)

// EndUserBlockRecord persists an end-user suspension so it survives a restart.
// The runtime check stays in memory; this table is written when a block starts
// or ends and read once at startup.
type EndUserBlockRecord struct {
	Id           int    `json:"id"`
	TokenId      int    `json:"token_id" gorm:"uniqueIndex:idx_end_user_block"`
	Model        string `json:"model" gorm:"type:varchar(128);uniqueIndex:idx_end_user_block"`
	EndUser      string `json:"end_user" gorm:"type:varchar(128);uniqueIndex:idx_end_user_block"`
	Refusals     int    `json:"refusals"`
	BlockedUntil int64  `json:"blocked_until" gorm:"bigint"`
}

// SaveEndUserBlock writes or refreshes one block record.
func SaveEndUserBlock(tokenId int, modelName string, endUser string, refusals int, blockedUntil int64) error {
	// Persistence is best effort: enforcement must keep working even before the
	// database is wired (and in unit tests that do not open one).
	if DB == nil {
		return nil
	}
	record := EndUserBlockRecord{
		TokenId:      tokenId,
		Model:        modelName,
		EndUser:      endUser,
		Refusals:     refusals,
		BlockedUntil: blockedUntil,
	}
	var existing EndUserBlockRecord
	err := DB.Where("token_id = ? AND model = ? AND end_user = ?", tokenId, modelName, endUser).First(&existing).Error
	if err != nil {
		return DB.Create(&record).Error
	}
	return DB.Model(&existing).Updates(map[string]any{
		"refusals":      refusals,
		"blocked_until": blockedUntil,
	}).Error
}

// DeleteEndUserBlock removes one persisted block.
func DeleteEndUserBlock(tokenId int, modelName string, endUser string) error {
	if DB == nil {
		return nil
	}
	return DB.Where("token_id = ? AND model = ? AND end_user = ?", tokenId, modelName, endUser).
		Delete(&EndUserBlockRecord{}).Error
}

// GetActiveEndUserBlocks returns the blocks that have not expired yet and
// drops the ones that have.
func GetActiveEndUserBlocks() ([]EndUserBlockRecord, error) {
	if DB == nil {
		return nil, nil
	}
	now := time.Now().Unix()

	if err := DB.Where("blocked_until <= ?", now).Delete(&EndUserBlockRecord{}).Error; err != nil {
		return nil, err
	}

	var records []EndUserBlockRecord
	err := DB.Where("blocked_until > ?", now).Find(&records).Error
	return records, err
}
