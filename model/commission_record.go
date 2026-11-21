package model

import "github.com/mazihua-lgtm/relay-forge/common"

// CommissionRecord is one movement of distributor commission. Quota is positive
// when commission is paid and negative when a refund claws it back, so summing
// the column over a window gives what the platform actually paid out.
type CommissionRecord struct {
	Id        int   `json:"id"`
	InviterId int   `json:"inviter_id" gorm:"index"`
	InviteeId int   `json:"invitee_id"`
	Quota     int64 `json:"quota"`
	CreatedAt int64 `json:"created_at" gorm:"bigint;index"`
}

// RecordCommission stores one commission movement. It is best effort: failing to
// write the ledger must not fail the consumption that triggered it.
func RecordCommission(inviterId int, inviteeId int, quota int64) error {
	if DB == nil || inviterId <= 0 || quota == 0 {
		return nil
	}
	return DB.Create(&CommissionRecord{
		InviterId: inviterId,
		InviteeId: inviteeId,
		Quota:     quota,
		CreatedAt: common.GetTimestamp(),
	}).Error
}

// SumCommissionInRange returns the net commission paid in [start, end].
func SumCommissionInRange(start int64, end int64) (int64, error) {
	var total int64
	err := DB.Model(&CommissionRecord{}).
		Where("created_at >= ? AND created_at <= ?", start, end).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&total).Error
	return total, err
}
