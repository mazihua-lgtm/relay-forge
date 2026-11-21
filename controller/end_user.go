package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/service"
)

// GetEndUserBlocks lists the end users currently suspended for a model.
func GetEndUserBlocks(c *gin.Context) {
	common.ApiSuccess(c, service.ListEndUserBlocks())
}

// ClearEndUserBlock lifts one suspension early.
func ClearEndUserBlock(c *gin.Context) {
	tokenId, err := strconv.Atoi(c.Query("token_id"))
	if err != nil || tokenId <= 0 {
		common.ApiErrorT(c, "Invalid token id")
		return
	}
	modelName := c.Query("model")
	endUser := c.Query("end_user")
	if modelName == "" || endUser == "" {
		common.ApiErrorT(c, "Model and end user are required")
		return
	}

	service.ClearEndUserBlock(tokenId, modelName, endUser)
	common.ApiSuccess(c, nil)
}
