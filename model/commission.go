package model

import (
	"errors"
	"math"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 分销持续抽成：下级用户每次消费时，按邀请人自己配置的比例（commission_rate）
// 给邀请人返佣。与注册时一次性奖励（AffQuota）无关，比例由管理员通过
// PATCH /api/user/:id/commission 设置。

// GetInviterCommission 返回 inviteeId 的邀请人 ID 及其抽成比例。
// 用户没有邀请人（或邀请人已删除）时返回 (0, 0, nil)。
func GetInviterCommission(inviteeId int) (inviterId int, rate float64, err error) {
	if inviteeId <= 0 {
		return 0, 0, nil
	}
	var row struct {
		Id             int     `gorm:"column:id"`
		CommissionRate float64 `gorm:"column:commission_rate"`
	}
	// Find（而不是 Take）让“没有邀请人”成为空结果而不是 ErrRecordNotFound，
	// 免得每个无邀请人的用户消费都打一条错误日志。
	err = DB.Table("users AS invitee").
		Select("inviter.id AS id, inviter.commission_rate AS commission_rate").
		Joins("JOIN users AS inviter ON inviter.id = invitee.inviter_id").
		Where("invitee.id = ? AND invitee.inviter_id > 0 AND invitee.inviter_id <> invitee.id", inviteeId).
		Where("invitee.deleted_at IS NULL AND inviter.deleted_at IS NULL").
		Limit(1).
		Find(&row).Error
	if err != nil {
		return 0, 0, err
	}
	if row.Id <= 0 {
		return 0, 0, nil
	}
	return row.Id, row.CommissionRate, nil
}

// SettleInviterCommission 为 inviteeId 的一次消费结算邀请人抽成。
// quota 是下级本次实际消费的额度，抽成 = quota * 邀请人 commission_rate，
// 按整数向下取整。返回实际入账的抽成额度，0 表示本次无需抽成。
func SettleInviterCommission(inviteeId int, quota int) (int, error) {
	if quota <= 0 {
		return 0, nil
	}
	inviterId, rate, err := GetInviterCommission(inviteeId)
	if err != nil {
		return 0, err
	}
	if inviterId <= 0 || inviterId == inviteeId || rate <= 0 {
		return 0, nil
	}
	// 比例由管理员写入，仍防御脏数据：非有限值或越界一律按 0-1 处理。
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, nil
	}
	if rate > 1 {
		rate = 1
	}
	// 用十进制计算避免浮点误差把 100*0.29 算成 28。MySQL 的 float 列是单精度，
	// 读回的 0.29 可能是 0.28999999…，先按 6 位小数取整，去掉存储带来的噪声。
	commission := decimal.NewFromFloat(rate).
		Round(6).
		Mul(decimal.NewFromInt(int64(quota))).
		Floor().
		IntPart()
	if commission <= 0 {
		return 0, nil
	}
	if commission > int64(math.MaxInt) {
		return 0, errors.New("commission exceeds the wallet quota range")
	}
	if err := IncreaseUserQuota(inviterId, int(commission), false); err != nil {
		return 0, err
	}
	RecordLog(inviterId, LogTypeSystem, common.NewMessage("下级消费抽成 {{quota}}（下级 {{invitee}} 本次消费 {{consumed}}）", map[string]any{
		"quota":    logger.FormatQuota(int(commission)),
		"invitee":  inviteeId,
		"consumed": logger.FormatQuota(quota),
	}))
	return int(commission), nil
}

// UpdateUserCommissionRate 写入管理员为用户设置的持续抽成比例（0 ≤ rate ≤ 1）。
// 该列只允许从这里修改，用户资料更新路径已在 UpdateWithTx 中剔除。
func UpdateUserCommissionRate(userId int, rate float64) error {
	if userId <= 0 {
		return common.NewMessage("ID is empty!")
	}
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > 1 {
		return errors.New("commission rate must be between 0 and 1")
	}
	result := DB.Model(&User{}).Where("id = ?", userId).Update("commission_rate", rate)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// MySQL 在值未变化时也会报 0 行，只有记录不存在才算错误。
		var count int64
		if err := DB.Model(&User{}).Where("id = ?", userId).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}
