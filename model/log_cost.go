package model

import (
	"fmt"
	"math"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/constant"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// Upstream cost of a consume log.
//
// A consume log may carry cost_quota: what the upstream charged the operator
// for that request, in quota, snapshotted when the request is logged. The
// snapshot keeps history stable: raising a channel's purchase price later does
// not rewrite last month's profit. NULL means unknown (the channel has no cost
// ratio, or the cost cannot be derived), and the earnings report prices those
// rows with its estimate instead.
//
// The cost is derived, not metered. The quota a user was charged is the model
// base price times the user group ratio, so the base price is quota divided by
// the group ratio, and the operator's cost is that base price times the
// channel's cost ratio:
//
//	cost_quota = quota / group_ratio * cost_ratio

// ChannelCostRatioMax bounds the purchase ratio an operator can enter. A
// channel bought at more than a hundred times the base price is a typo, not a
// price.
const ChannelCostRatioMax = 100.0

// ValidateChannelCostRatio checks the cost ratio of a channel being saved. Nil
// (not sent) and 0 (not set) are both fine.
func ValidateChannelCostRatio(ratio *float64) error {
	if ratio == nil {
		return nil
	}
	if math.IsNaN(*ratio) || math.IsInf(*ratio, 0) || *ratio < 0 || *ratio > ChannelCostRatioMax {
		return fmt.Errorf("cost_ratio must be between 0 and %g", ChannelCostRatioMax)
	}
	return nil
}

// ConsumeCostQuota derives the upstream cost of a request that charged quota
// under groupRatio, on a channel bought at costRatio of the base price. The
// result is rounded to a whole quota. ok is false when the cost cannot be
// derived: nothing was charged, a ratio is missing or unusable (a free group
// has ratio 0, which hides the base price), or the cost would not fit the
// quota column.
func ConsumeCostQuota(quota int, groupRatio float64, costRatio float64) (cost int, ok bool) {
	if quota <= 0 || !usableRatio(groupRatio) || !usableRatio(costRatio) {
		return 0, false
	}
	// Decimal arithmetic keeps a ratio like 1.2 exact, so the same request
	// always snapshots the same cost.
	derived := decimal.NewFromInt(int64(quota)).
		Div(decimal.NewFromFloat(groupRatio)).
		Mul(decimal.NewFromFloat(costRatio)).
		Round(0)
	if derived.GreaterThan(decimal.NewFromInt(math.MaxInt32)) {
		return 0, false
	}
	return int(derived.IntPart()), true
}

func usableRatio(ratio float64) bool {
	return ratio > 0 && !math.IsNaN(ratio) && !math.IsInf(ratio, 0)
}

// publicNumber reads a numeric public log field, whichever numeric type the
// writer stored.
func (o *LogOther) publicNumber(key string) (float64, bool) {
	if o == nil {
		return 0, false
	}
	switch value := o.public[key].(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	}
	return 0, false
}

// costQuotaFromOther snapshots the cost of a log from the group ratio the
// request was billed with (public field group_ratio) and the channel's cost
// ratio. It returns nil, meaning unknown, whenever the cost cannot be derived.
func costQuotaFromOther(quota int, other *LogOther, costRatio float64) *int {
	if !usableRatio(costRatio) {
		return nil
	}
	groupRatio, ok := other.publicNumber("group_ratio")
	if !ok {
		return nil
	}
	cost, ok := ConsumeCostQuota(quota, groupRatio, costRatio)
	if !ok {
		return nil
	}
	return &cost
}

// consumeLogCostQuota is the snapshot for a consume log written while serving a
// request: the channel that served it put its cost ratio on the context when it
// was selected, so the log needs no channel lookup.
func consumeLogCostQuota(c *gin.Context, quota int, other *LogOther) *int {
	if c == nil {
		return nil
	}
	costRatio, _ := common.GetContextKeyType[float64](c, constant.ContextKeyChannelCostRatio)
	return costQuotaFromOther(quota, other, costRatio)
}

// taskLogCostQuota is the snapshot for the billing log of an asynchronous task.
// No request context exists when a task settles, so the channel is looked up;
// task settlements are rare enough for that. The lookup is skipped when the log
// cannot yield a cost anyway.
func taskLogCostQuota(channelId int, quota int, other *LogOther) *int {
	if _, ok := other.publicNumber("group_ratio"); !ok || quota <= 0 {
		return nil
	}
	return costQuotaFromOther(quota, other, GetChannelCostRatio(channelId))
}

// GetChannelCostRatio returns the cost ratio of a channel, 0 when it has none
// or does not exist. The memory cache answers when it is on.
func GetChannelCostRatio(channelId int) float64 {
	if channelId <= 0 {
		return 0
	}
	if common.MemoryCacheEnabled {
		if channel, err := CacheGetChannel(channelId); err == nil && channel != nil {
			return channel.GetCostRatio()
		}
	}
	// A slice, not Take: a deleted channel is an empty answer rather than a
	// logged record-not-found.
	var rows []struct {
		CostRatio *float64 `gorm:"column:cost_ratio"`
	}
	if err := DB.Table("channels").Select("cost_ratio").Where("id = ?", channelId).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return 0
	}
	return (&Channel{CostRatio: rows[0].CostRatio}).GetCostRatio()
}
