package service

import (
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/constant"
	"github.com/mazihua-lgtm/relay-forge/model"
	"github.com/mazihua-lgtm/relay-forge/relaykit/types"
)

// EndUserHeader lets a reseller tag each request with the id of their own end
// user. Without it the smallest unit the platform can act on is the whole API
// key, so one user's policy refusals would suspend every other user of that
// key. With it, enforcement is scoped to key × model × end user.
const EndUserHeader = "X-End-User"

const (
	endUserMaxLength        = 128
	endUserRefusalWindow    = time.Hour
	endUserRefusalThreshold = 5
	// endUserMaxEntries bounds the in-memory table. The header value is chosen
	// by the caller, so without a cap a flood of distinct ids could grow it
	// without limit.
	endUserMaxEntries = 5000
)

// endUserPolicyKeywords match the wording upstream providers use when they
// refuse a request on policy grounds. Only 400 and 403 responses are considered,
// so ordinary client errors cannot trigger a block.
var endUserPolicyKeywords = []string{
	"policy",
	"safety",
	"violat",
	"content filter",
	"content_filter",
	"moderation",
	"refus",
}

type endUserKey struct {
	TokenId int
	Model   string
	EndUser string
}

// EndUserBlock is one active enforcement record, for the admin view.
type EndUserBlock struct {
	TokenId      int    `json:"token_id"`
	Model        string `json:"model"`
	EndUser      string `json:"end_user"`
	Refusals     int    `json:"refusals"`
	BlockedUntil int64  `json:"blocked_until"`
}

var (
	endUserMu       sync.Mutex
	endUserRefusals = map[endUserKey][]time.Time{}
	endUserBlocks   = map[endUserKey]time.Time{}
)

// NormalizeEndUser trims and bounds the header value. An over-long value or one
// carrying control characters is treated as absent rather than trusted.
func NormalizeEndUser(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" || utf8.RuneCountInString(id) > endUserMaxLength {
		return ""
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return id
}

// IsEndUserBlocked reports whether this key × model × end user is currently
// suspended. Expired records are removed as they are encountered.
func IsEndUserBlocked(tokenId int, modelName string, endUser string) bool {
	if tokenId == 0 || modelName == "" || endUser == "" {
		return false
	}
	key := endUserKey{TokenId: tokenId, Model: modelName, EndUser: endUser}

	endUserMu.Lock()
	until, ok := endUserBlocks[key]
	if !ok {
		endUserMu.Unlock()
		return false
	}
	if time.Now().After(until) {
		delete(endUserBlocks, key)
		delete(endUserRefusals, key)
		endUserMu.Unlock()
		forgetPersistedBlock(key)
		return false
	}
	endUserMu.Unlock()
	return true
}

// RecordEndUserRefusal counts one policy refusal and blocks the end user for the
// model once the threshold is reached. It reports whether this call caused a
// block, so the caller can log the transition exactly once.
func RecordEndUserRefusal(tokenId int, modelName string, endUser string) bool {
	if tokenId == 0 || modelName == "" || endUser == "" {
		return false
	}
	key := endUserKey{TokenId: tokenId, Model: modelName, EndUser: endUser}
	now := time.Now()
	cutoff := now.Add(-endUserRefusalWindow)

	endUserMu.Lock()

	if _, tracked := endUserRefusals[key]; !tracked && len(endUserRefusals) >= endUserMaxEntries {
		pruneLocked(cutoff)
	}

	kept := make([]time.Time, 0, endUserRefusalThreshold)
	for _, at := range endUserRefusals[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	kept = append(kept, now)
	// Once the threshold is reached the exact count no longer matters; keeping
	// the slice bounded stops a blocked caller from growing it indefinitely.
	if len(kept) > endUserRefusalThreshold {
		kept = kept[len(kept)-endUserRefusalThreshold:]
	}
	endUserRefusals[key] = kept

	if len(kept) < endUserRefusalThreshold {
		endUserMu.Unlock()
		return false
	}
	if _, already := endUserBlocks[key]; already {
		endUserMu.Unlock()
		return false
	}
	blockedUntil := now.Add(endUserRefusalWindow)
	endUserBlocks[key] = blockedUntil
	refusals := len(kept)
	endUserMu.Unlock()

	// Persist outside the lock: a restart should not forget an active block.
	if err := model.SaveEndUserBlock(key.TokenId, key.Model, key.EndUser, refusals, blockedUntil.Unix()); err != nil {
		common.SysLog(common.LogText("failed to persist end-user block: %s", err.Error()))
	}
	return true
}

// pruneLocked drops expired entries; if that is not enough it drops the entries
// whose most recent refusal is oldest, until there is room for one more.
func pruneLocked(cutoff time.Time) {
	for key, times := range endUserRefusals {
		if _, blocked := endUserBlocks[key]; blocked {
			continue
		}
		if len(times) == 0 || times[len(times)-1].Before(cutoff) {
			delete(endUserRefusals, key)
		}
	}
	if len(endUserRefusals) < endUserMaxEntries {
		return
	}

	type candidate struct {
		key  endUserKey
		last time.Time
	}
	candidates := make([]candidate, 0, len(endUserRefusals))
	for key, times := range endUserRefusals {
		if _, blocked := endUserBlocks[key]; blocked {
			continue
		}
		last := time.Time{}
		if len(times) > 0 {
			last = times[len(times)-1]
		}
		candidates = append(candidates, candidate{key: key, last: last})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].last.Before(candidates[j].last) })

	remove := len(endUserRefusals) - endUserMaxEntries + 1
	for i := 0; i < remove && i < len(candidates); i++ {
		delete(endUserRefusals, candidates[i].key)
	}
}

// ListEndUserBlocks returns the active blocks, soonest expiry first.
func ListEndUserBlocks() []EndUserBlock {
	now := time.Now()

	endUserMu.Lock()
	blocks := make([]EndUserBlock, 0, len(endUserBlocks))
	expired := make([]endUserKey, 0)
	for key, until := range endUserBlocks {
		if now.After(until) {
			delete(endUserBlocks, key)
			delete(endUserRefusals, key)
			expired = append(expired, key)
			continue
		}
		blocks = append(blocks, EndUserBlock{
			TokenId:      key.TokenId,
			Model:        key.Model,
			EndUser:      key.EndUser,
			Refusals:     len(endUserRefusals[key]),
			BlockedUntil: until.Unix(),
		})
	}
	endUserMu.Unlock()

	for _, key := range expired {
		forgetPersistedBlock(key)
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].BlockedUntil < blocks[j].BlockedUntil
	})
	return blocks
}

// ClearEndUserBlock lifts a block and forgets its refusals.
func ClearEndUserBlock(tokenId int, modelName string, endUser string) bool {
	key := endUserKey{TokenId: tokenId, Model: modelName, EndUser: endUser}

	endUserMu.Lock()
	_, blocked := endUserBlocks[key]
	delete(endUserBlocks, key)
	delete(endUserRefusals, key)
	endUserMu.Unlock()

	forgetPersistedBlock(key)
	return blocked
}

// IsEndUserPolicyRefusal reports whether an upstream error looks like a policy
// refusal rather than a malformed request.
func IsEndUserPolicyRefusal(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode != 400 && err.StatusCode != 403 {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, keyword := range endUserPolicyKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

// RecordEndUserRefusalFromContext attributes a policy refusal to the end user
// named on the request. Requests without the header are left alone: the key
// keeps its existing behaviour.
func RecordEndUserRefusalFromContext(c *gin.Context, err *types.NewAPIError) bool {
	if c == nil || !IsEndUserPolicyRefusal(err) {
		return false
	}
	endUser := common.GetContextKeyString(c, constant.ContextKeyEndUser)
	if endUser == "" {
		return false
	}
	return RecordEndUserRefusal(c.GetInt("token_id"), c.GetString("original_model"), endUser)
}

// LoadEndUserBlocks restores persisted blocks after a restart.
func LoadEndUserBlocks() {
	records, err := model.GetActiveEndUserBlocks()
	if err != nil {
		common.SysLog(common.LogText("failed to load end-user blocks: %s", err.Error()))
		return
	}

	endUserMu.Lock()
	defer endUserMu.Unlock()
	for _, record := range records {
		key := endUserKey{TokenId: record.TokenId, Model: record.Model, EndUser: record.EndUser}
		endUserBlocks[key] = time.Unix(record.BlockedUntil, 0)
	}
	if len(records) > 0 {
		common.SysLog(common.LogText("restored %d end-user blocks", len(records)))
	}
}

// ResetEndUserState clears every counter and block. Tests only.
func ResetEndUserState() {
	endUserMu.Lock()
	defer endUserMu.Unlock()
	endUserRefusals = map[endUserKey][]time.Time{}
	endUserBlocks = map[endUserKey]time.Time{}
}

func forgetPersistedBlock(key endUserKey) {
	if err := model.DeleteEndUserBlock(key.TokenId, key.Model, key.EndUser); err != nil {
		common.SysLog(common.LogText("failed to delete end-user block: %s", err.Error()))
	}
}
