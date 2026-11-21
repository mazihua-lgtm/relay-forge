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
	defer endUserMu.Unlock()

	until, ok := endUserBlocks[key]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(endUserBlocks, key)
		delete(endUserRefusals, key)
		return false
	}
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
	defer endUserMu.Unlock()

	kept := make([]time.Time, 0, len(endUserRefusals[key])+1)
	for _, at := range endUserRefusals[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	kept = append(kept, now)
	endUserRefusals[key] = kept

	if len(kept) < endUserRefusalThreshold {
		return false
	}
	if _, already := endUserBlocks[key]; already {
		return false
	}
	endUserBlocks[key] = now.Add(endUserRefusalWindow)
	return true
}

// ListEndUserBlocks returns the active blocks, soonest expiry first.
func ListEndUserBlocks() []EndUserBlock {
	now := time.Now()

	endUserMu.Lock()
	defer endUserMu.Unlock()

	blocks := make([]EndUserBlock, 0, len(endUserBlocks))
	for key, until := range endUserBlocks {
		if now.After(until) {
			delete(endUserBlocks, key)
			delete(endUserRefusals, key)
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
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].BlockedUntil < blocks[j].BlockedUntil
	})
	return blocks
}

// ClearEndUserBlock lifts a block and forgets its refusals.
func ClearEndUserBlock(tokenId int, modelName string, endUser string) bool {
	key := endUserKey{TokenId: tokenId, Model: modelName, EndUser: endUser}

	endUserMu.Lock()
	defer endUserMu.Unlock()

	_, blocked := endUserBlocks[key]
	delete(endUserBlocks, key)
	delete(endUserRefusals, key)
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

// ResetEndUserState clears every counter and block. Tests only.
func ResetEndUserState() {
	endUserMu.Lock()
	defer endUserMu.Unlock()
	endUserRefusals = map[endUserKey][]time.Time{}
	endUserBlocks = map[endUserKey]time.Time{}
}
