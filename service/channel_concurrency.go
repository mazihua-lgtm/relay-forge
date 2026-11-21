package service

import "sync"

// In-flight counters are per node: each process limits the requests it is
// itself sending to a channel. A shared counter would need a Redis round trip
// on every request just to enter and leave the relay path, which costs more
// than it saves for the deployments this protects.

var (
	channelConcurrencyMu sync.Mutex
	channelInFlight      = map[int]int{}
)

// TryAcquireChannelSlot reserves one in-flight slot for the channel. It returns
// false when the channel already has limit requests in flight. A limit of zero
// or less means unlimited and always succeeds.
func TryAcquireChannelSlot(channelId int, limit int) bool {
	if limit <= 0 || channelId <= 0 {
		return true
	}

	channelConcurrencyMu.Lock()
	defer channelConcurrencyMu.Unlock()

	if channelInFlight[channelId] >= limit {
		return false
	}
	channelInFlight[channelId]++
	return true
}

// ReleaseChannelSlot returns a slot taken by TryAcquireChannelSlot. It is safe to
// call for a channel that never acquired one.
func ReleaseChannelSlot(channelId int) {
	if channelId <= 0 {
		return
	}

	channelConcurrencyMu.Lock()
	defer channelConcurrencyMu.Unlock()

	if channelInFlight[channelId] <= 1 {
		delete(channelInFlight, channelId)
		return
	}
	channelInFlight[channelId]--
}

// ChannelInFlight reports the current in-flight count, for the admin view and
// tests.
func ChannelInFlight(channelId int) int {
	channelConcurrencyMu.Lock()
	defer channelConcurrencyMu.Unlock()
	return channelInFlight[channelId]
}

// ResetChannelConcurrency clears every counter. Tests only.
func ResetChannelConcurrency() {
	channelConcurrencyMu.Lock()
	defer channelConcurrencyMu.Unlock()
	channelInFlight = map[int]int{}
}
