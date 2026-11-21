package controller

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/model"
)

// clientMaxNameLength matches the column width in model.Client.
const clientMaxNameLength = 64

var (
	errClientNameRequired   = errors.New("client name is required")
	errClientNameTooLong    = errors.New("client name must be 64 characters or fewer")
	errClientBudgetNegative = errors.New("budget cannot be negative")
	errClientBudgetTooLarge = errors.New("budget exceeds the valid range")
)

type clientRequest struct {
	Name        string `json:"name"`
	BudgetQuota int    `json:"budget_quota"`
	Remark      string `json:"remark"`
}

type clientUpdateRequest struct {
	Name        *string `json:"name"`
	BudgetQuota *int    `json:"budget_quota"`
	Status      *int    `json:"status"`
	Remark      *string `json:"remark"`
}

// GetAllClients lists the caller's downstream customers with their key count
// and consumption. It never returns another reseller's clients.
func GetAllClients(c *gin.Context) {
	usages, err := model.GetClientsByUser(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, usages)
}

func AddClient(c *gin.Context) {
	var req clientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	name, err := normalizeClientName(req.Name)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateClientBudget(req.BudgetQuota); err != nil {
		common.ApiError(c, err)
		return
	}

	client := model.Client{
		UserId:      c.GetInt("id"),
		Name:        name,
		BudgetQuota: req.BudgetQuota,
		Remark:      strings.TrimSpace(req.Remark),
		Status:      model.ClientStatusEnabled,
	}
	if err := model.CreateClient(&client); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, client)
}

func UpdateClient(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid client id")
		return
	}

	var req clientUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if req.Name != nil {
		name, err := normalizeClientName(*req.Name)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		req.Name = &name
	}
	if req.BudgetQuota != nil {
		if err := validateClientBudget(*req.BudgetQuota); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	if err := model.UpdateClient(c.GetInt("id"), id, req.Name, req.BudgetQuota, req.Status, req.Remark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func DeleteClient(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid client id")
		return
	}
	if err := model.DeleteClient(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func normalizeClientName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errClientNameRequired
	}
	if utf8.RuneCountInString(name) > clientMaxNameLength {
		return "", errClientNameTooLong
	}
	return name, nil
}

func validateClientBudget(budget int) error {
	if budget < 0 {
		return errClientBudgetNegative
	}
	if budget > maxTokenQuota() {
		return errClientBudgetTooLarge
	}
	return nil
}
