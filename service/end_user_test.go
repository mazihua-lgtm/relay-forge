package service

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/mazihua-lgtm/relay-forge/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeEndUser(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "trims surrounding spaces", raw: "  user-1  ", want: "user-1"},
		{name: "empty stays empty", raw: "   ", want: ""},
		{name: "rejects a value longer than 128 characters", raw: strings.Repeat("a", 129), want: ""},
		{name: "rejects control characters", raw: "user\n1", want: ""},
		{name: "keeps a normal id", raw: "acme:42", want: "acme:42"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NormalizeEndUser(tc.raw))
		})
	}
}

func TestEndUserBlocksOnlyAfterTheThreshold(t *testing.T) {
	ResetEndUserState()
	t.Cleanup(ResetEndUserState)

	for i := 1; i < endUserRefusalThreshold; i++ {
		blocked := RecordEndUserRefusal(1, "gpt-4o", "user-1")
		assert.False(t, blocked, "refusal %d must not block yet", i)
		assert.False(t, IsEndUserBlocked(1, "gpt-4o", "user-1"))
	}

	assert.True(t, RecordEndUserRefusal(1, "gpt-4o", "user-1"))
	assert.True(t, IsEndUserBlocked(1, "gpt-4o", "user-1"))

	// A second refusal past the threshold does not report a new transition.
	assert.False(t, RecordEndUserRefusal(1, "gpt-4o", "user-1"))
}

func TestEndUserBlockIsScopedToKeyModelAndUser(t *testing.T) {
	ResetEndUserState()
	t.Cleanup(ResetEndUserState)

	for i := 0; i < endUserRefusalThreshold; i++ {
		RecordEndUserRefusal(1, "gpt-4o", "user-1")
	}
	require.True(t, IsEndUserBlocked(1, "gpt-4o", "user-1"))

	assert.False(t, IsEndUserBlocked(2, "gpt-4o", "user-1"), "another key is unaffected")
	assert.False(t, IsEndUserBlocked(1, "claude-sonnet-5", "user-1"), "another model is unaffected")
	assert.False(t, IsEndUserBlocked(1, "gpt-4o", "user-2"), "another end user is unaffected")
}

func TestClearEndUserBlockLiftsAndForgets(t *testing.T) {
	ResetEndUserState()
	t.Cleanup(ResetEndUserState)

	for i := 0; i < endUserRefusalThreshold; i++ {
		RecordEndUserRefusal(1, "gpt-4o", "user-1")
	}
	require.True(t, IsEndUserBlocked(1, "gpt-4o", "user-1"))

	assert.True(t, ClearEndUserBlock(1, "gpt-4o", "user-1"))
	assert.False(t, IsEndUserBlocked(1, "gpt-4o", "user-1"))
	assert.Empty(t, ListEndUserBlocks())

	// Clearing an unknown block reports no change.
	assert.False(t, ClearEndUserBlock(1, "gpt-4o", "user-1"))
}

func TestListEndUserBlocksReportsActiveBlocks(t *testing.T) {
	ResetEndUserState()
	t.Cleanup(ResetEndUserState)

	for i := 0; i < endUserRefusalThreshold; i++ {
		RecordEndUserRefusal(1, "gpt-4o", "user-1")
	}

	blocks := ListEndUserBlocks()
	require.Len(t, blocks, 1)
	assert.Equal(t, 1, blocks[0].TokenId)
	assert.Equal(t, "gpt-4o", blocks[0].Model)
	assert.Equal(t, "user-1", blocks[0].EndUser)
	assert.Equal(t, endUserRefusalThreshold, blocks[0].Refusals)
	assert.NotZero(t, blocks[0].BlockedUntil)
}

func TestIsEndUserPolicyRefusal(t *testing.T) {
	policy := types.NewErrorWithStatusCode(errors.New("request blocked by safety policy"), types.ErrorCodeInvalidRequest, 400)
	assert.True(t, IsEndUserPolicyRefusal(policy))

	contentFilter := types.NewErrorWithStatusCode(errors.New("content_filter triggered"), types.ErrorCodeInvalidRequest, 403)
	assert.True(t, IsEndUserPolicyRefusal(contentFilter))

	badRequest := types.NewErrorWithStatusCode(errors.New("missing field messages"), types.ErrorCodeInvalidRequest, 400)
	assert.False(t, IsEndUserPolicyRefusal(badRequest), "an ordinary 400 is not a policy refusal")

	rateLimited := types.NewErrorWithStatusCode(errors.New("rate limit exceeded"), types.ErrorCodeInvalidRequest, 429)
	assert.False(t, IsEndUserPolicyRefusal(rateLimited), "429 is never treated as a policy refusal")

	assert.False(t, IsEndUserPolicyRefusal(nil))
}

// The header value is caller-controlled, so the table must stay bounded no
// matter how many distinct ids arrive.
func TestEndUserTableStaysBounded(t *testing.T) {
	ResetEndUserState()
	t.Cleanup(ResetEndUserState)

	for i := 0; i < endUserMaxEntries+50; i++ {
		RecordEndUserRefusal(1, "gpt-4o", "user-"+strconv.Itoa(i))
	}

	endUserMu.Lock()
	size := len(endUserRefusals)
	endUserMu.Unlock()
	assert.LessOrEqual(t, size, endUserMaxEntries)
}

// A blocked caller that keeps sending refusals must not grow the counter.
func TestBlockedEndUserRefusalsStayCapped(t *testing.T) {
	ResetEndUserState()
	t.Cleanup(ResetEndUserState)

	for i := 0; i < endUserRefusalThreshold; i++ {
		RecordEndUserRefusal(1, "gpt-4o", "user-1")
	}
	require.True(t, IsEndUserBlocked(1, "gpt-4o", "user-1"))

	for i := 0; i < 100; i++ {
		RecordEndUserRefusal(1, "gpt-4o", "user-1")
	}

	endUserMu.Lock()
	size := len(endUserRefusals[endUserKey{TokenId: 1, Model: "gpt-4o", EndUser: "user-1"}])
	endUserMu.Unlock()
	assert.LessOrEqual(t, size, endUserRefusalThreshold)
}
