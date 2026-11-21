package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelConcurrencyLimit(t *testing.T) {
	ResetChannelConcurrency()
	t.Cleanup(ResetChannelConcurrency)

	assert.True(t, TryAcquireChannelSlot(1, 2))
	assert.True(t, TryAcquireChannelSlot(1, 2))
	assert.False(t, TryAcquireChannelSlot(1, 2), "the third request exceeds the limit")
	assert.Equal(t, 2, ChannelInFlight(1))

	// Releasing one slot lets exactly one more request through.
	ReleaseChannelSlot(1)
	assert.Equal(t, 1, ChannelInFlight(1))
	assert.True(t, TryAcquireChannelSlot(1, 2))
	assert.False(t, TryAcquireChannelSlot(1, 2))

	// Another channel has its own counter.
	assert.True(t, TryAcquireChannelSlot(2, 2))
	assert.Equal(t, 2, ChannelInFlight(1))
	assert.Equal(t, 1, ChannelInFlight(2))
}

func TestChannelConcurrencyUnlimited(t *testing.T) {
	ResetChannelConcurrency()
	t.Cleanup(ResetChannelConcurrency)

	for i := 0; i < 100; i++ {
		assert.True(t, TryAcquireChannelSlot(1, 0))
	}
	assert.Equal(t, 0, ChannelInFlight(1), "an unlimited limit does not count")
}

func TestReleaseChannelSlotIsSafeWithoutAnAcquire(t *testing.T) {
	ResetChannelConcurrency()
	t.Cleanup(ResetChannelConcurrency)

	assert.NotPanics(t, func() {
		ReleaseChannelSlot(99)
		ReleaseChannelSlot(0)
		ReleaseChannelSlot(-1)
	})
	assert.Equal(t, 0, ChannelInFlight(99))
}

// Under contention the counter must never exceed the limit.
func TestChannelConcurrencyUnderContention(t *testing.T) {
	ResetChannelConcurrency()
	t.Cleanup(ResetChannelConcurrency)

	const limit = 5
	var wg sync.WaitGroup
	var mu sync.Mutex
	acquired := 0

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if TryAcquireChannelSlot(1, limit) {
				mu.Lock()
				acquired++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, limit, acquired, "exactly the limit may be held at once")
	assert.Equal(t, limit, ChannelInFlight(1))
}
