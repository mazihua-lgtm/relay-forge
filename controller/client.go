package controller

import (
	"errors"
	"fmt"
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
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	usages, total, err := model.GetClientsByUser(c.GetInt("id"), c.Query("keyword"), page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":     usages,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
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

// validateClientKeyQuota keeps a client's keys within its budget. Committed
// quota counts each live key's remaining plus used amount, and the snapshot of
// deleted keys, so editing a key cannot silently overbook the budget.
func validateClientKeyQuota(client *model.Client, excludeTokenId int, unlimited bool, remainQuota int) error {
	if unlimited {
		return errors.New("a key filed under a client cannot have unlimited quota")
	}
	if remainQuota < 0 {
		return errors.New("quota cannot be negative")
	}

	committed, err := model.GetClientCommittedQuota(client.Id, excludeTokenId)
	if err != nil {
		return err
	}
	budget := int64(client.BudgetQuota)
	if committed+int64(remainQuota) > budget {
		return fmt.Errorf("quota exceeds the client's remaining budget: %d of %d already committed", committed, budget)
	}
	return nil
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
