package controller

import (
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/model"
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

// GetMyEndUserBlocks lists the suspensions that belong to the caller's own keys.
func GetMyEndUserBlocks(c *gin.Context) {
	tokenIds, err := model.GetTokenIdsByUser(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, service.ListEndUserBlocksForTokens(tokenIds))
}

// ClearMyEndUserBlock lifts a suspension on one of the caller's own keys.
func ClearMyEndUserBlock(c *gin.Context) {
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

	tokenIds, err := model.GetTokenIdsByUser(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !slices.Contains(tokenIds, tokenId) {
		common.ApiErrorT(c, "Token not found")
		return
	}

	service.ClearEndUserBlock(tokenId, modelName, endUser)
	common.ApiSuccess(c, nil)
}
