package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/constant"
	"github.com/mazihua-lgtm/relay-forge/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The consume log reads the channel's cost ratio from the request context, so
// every channel selection, including each retry, must refresh it.
func TestSetupContextForSelectedChannelRecordsTheCostRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		return c
	}
	priced, cheap := 0.4, 0.2
	channelWith := func(id int, ratio *float64) *model.Channel {
		return &model.Channel{Id: id, Name: "channel", Type: 1, Key: "sk-test", CostRatio: ratio}
	}

	t.Run("a priced channel puts its ratio on the context", func(t *testing.T) {
		c := newContext()
		require.Nil(t, SetupContextForSelectedChannel(c, channelWith(7, &priced), "gpt-4o"))

		ratio, ok := common.GetContextKeyType[float64](c, constant.ContextKeyChannelCostRatio)
		assert.True(t, ok)
		assert.Equal(t, 0.4, ratio)
	})

	t.Run("a channel without a ratio puts 0, which means unknown", func(t *testing.T) {
		c := newContext()
		require.Nil(t, SetupContextForSelectedChannel(c, channelWith(8, nil), "gpt-4o"))

		ratio, ok := common.GetContextKeyType[float64](c, constant.ContextKeyChannelCostRatio)
		assert.True(t, ok)
		assert.Equal(t, 0.0, ratio)
	})

	t.Run("a retry on another channel replaces the ratio", func(t *testing.T) {
		c := newContext()
		require.Nil(t, SetupContextForSelectedChannel(c, channelWith(7, &priced), "gpt-4o"))
		require.Nil(t, SetupContextForSelectedChannel(c, channelWith(9, &cheap), "gpt-4o"))

		ratio, _ := common.GetContextKeyType[float64](c, constant.ContextKeyChannelCostRatio)
		assert.Equal(t, 0.2, ratio, "the log must be priced by the channel that served the request")
	})

	t.Run("a retry on an unpriced channel does not keep the previous price", func(t *testing.T) {
		c := newContext()
		require.Nil(t, SetupContextForSelectedChannel(c, channelWith(7, &priced), "gpt-4o"))
		require.Nil(t, SetupContextForSelectedChannel(c, channelWith(8, nil), "gpt-4o"))

		ratio, _ := common.GetContextKeyType[float64](c, constant.ContextKeyChannelCostRatio)
		assert.Equal(t, 0.0, ratio, "the first channel's price must not leak onto the second channel's request")
	})
}
