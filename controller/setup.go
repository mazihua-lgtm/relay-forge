package controller

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/mazihua-lgtm/relay-forge/common"
	"github.com/mazihua-lgtm/relay-forge/constant"
	"github.com/mazihua-lgtm/relay-forge/model"
	"github.com/mazihua-lgtm/relay-forge/setting/operation_setting"
)

type Setup struct {
	Status       bool   `json:"status"`
	RootInit     bool   `json:"root_init"`
	DatabaseType string `json:"database_type"`
}

type SetupRequest struct {
	Username           string `json:"username"`
	Password           string `json:"password"`
	ConfirmPassword    string `json:"confirmPassword"`
	SelfUseModeEnabled bool   `json:"SelfUseModeEnabled"`
	DemoSiteEnabled    bool   `json:"DemoSiteEnabled"`
	// SiteName and Logo are optional; when empty the built-in defaults stay.
	SiteName string `json:"siteName"`
	Logo     string `json:"logo"`
}

// SetupCheck is one line of the post-install self check.
type SetupCheck struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

const (
	setupCheckOK      = "ok"
	setupCheckWarning = "warning"
	setupCheckError   = "error"
)

func GetSetup(c *gin.Context) {
	setup := Setup{
		Status: constant.Setup,
	}
	if constant.Setup {
		c.JSON(200, gin.H{
			"success": true,
			"data":    setup,
		})
		return
	}
	setup.RootInit = model.RootUserExists()
	setup.DatabaseType = string(common.MainDatabaseType())
	c.JSON(200, gin.H{
		"success": true,
		"data":    setup,
	})
}

func PostSetup(c *gin.Context) {
	// Check if setup is already completed
	if constant.Setup {
		common.ApiErrorT(c, "The system is already initialized")
		return
	}

	// Check if root user already exists
	rootExists := model.RootUserExists()

	var req SetupRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}

	// If root doesn't exist, validate and create admin account
	if !rootExists {
		// Validate username length: max 12 characters to align with model.User validation
		if len(req.Username) > 12 {
			common.ApiErrorT(c, "Username must be {{max}} characters or fewer", map[string]any{"max": 12})
			return
		}
		// Validate password
		if req.Password != req.ConfirmPassword {
			common.ApiErrorT(c, "Passwords do not match")
			return
		}

		if err := common.ValidateNewAccountPassword(req.Password); err != nil {
			c.JSON(200, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		// Create root user
		hashedPassword, err := common.HashAccountPassword(req.Password)
		if err != nil {
			common.ApiErrorT(c, "System error: {{error}}", map[string]any{"error": err.Error()})
			return
		}
		rootUser := model.User{
			Username:    req.Username,
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		err = model.DB.Create(&rootUser).Error
		if err != nil {
			common.ApiErrorT(c, "Failed to create the administrator account: {{error}}", map[string]any{"error": err.Error()})
			return
		}
	}

	// Apply the optional white-label settings. Empty values keep the defaults,
	// so an operator who skips the step sees no change.
	if err := applySetupSiteOptions(req); err != nil {
		common.ApiErrorT(c, "Failed to save the site settings: {{error}}", map[string]any{"error": err.Error()})
		return
	}

	// Set operation modes
	operation_setting.SelfUseModeEnabled = req.SelfUseModeEnabled
	operation_setting.DemoSiteEnabled = req.DemoSiteEnabled

	// Save operation modes to database for persistence
	err = model.UpdateOption("SelfUseModeEnabled", boolToString(req.SelfUseModeEnabled))
	if err != nil {
		common.ApiErrorT(c, "Failed to save the self-use mode setting: {{error}}", map[string]any{"error": err.Error()})
		return
	}

	err = model.UpdateOption("DemoSiteEnabled", boolToString(req.DemoSiteEnabled))
	if err != nil {
		common.ApiErrorT(c, "Failed to save the demo site mode setting: {{error}}", map[string]any{"error": err.Error()})
		return
	}

	// Update setup status
	constant.Setup = true

	setup := model.Setup{
		Version:       common.Version,
		InitializedAt: time.Now().Unix(),
	}
	err = model.DB.Create(&setup).Error
	if err != nil {
		common.ApiErrorT(c, "Failed to initialize the system: {{error}}", map[string]any{"error": err.Error()})
		return
	}

	common.ApiSuccessT(c, "System initialized successfully", nil)
}

// applySetupSiteOptions writes the optional white-label values collected by the
// setup wizard. Each field is independent: an empty one leaves the current
// option untouched, so a partially filled form cannot blank the site name.
func applySetupSiteOptions(req SetupRequest) error {
	siteName := strings.TrimSpace(req.SiteName)
	if siteName != "" {
		if utf8.RuneCountInString(siteName) > 64 {
			return errors.New("site name must be 64 characters or fewer")
		}
		if err := model.UpdateOption("SystemName", siteName); err != nil {
			return err
		}
	}

	logo := strings.TrimSpace(req.Logo)
	if logo == "" {
		return nil
	}
	if utf8.RuneCountInString(logo) > 512 {
		return errors.New("logo URL must be 512 characters or fewer")
	}
	if !strings.HasPrefix(logo, "http://") && !strings.HasPrefix(logo, "https://") && !strings.HasPrefix(logo, "/") {
		return errors.New("logo must be an http(s) URL or an absolute path")
	}
	return model.UpdateOption("Logo", logo)
}

// GetSetupCheck reports what the freshly installed instance can and cannot do.
// It is read-only and anonymous: it runs before any account exists, and it must
// not reveal secrets — only whether a dependency is reachable.
func GetSetupCheck(c *gin.Context) {
	checks := make([]SetupCheck, 0, 4)

	if model.DB != nil {
		checks = append(checks, SetupCheck{
			Key:    "database",
			Status: setupCheckOK,
			Detail: string(common.MainDatabaseType()),
		})
	} else {
		checks = append(checks, SetupCheck{
			Key:    "database",
			Status: setupCheckError,
			Detail: "database is not connected",
		})
	}

	if !common.RedisEnabled {
		checks = append(checks, SetupCheck{
			Key:    "redis",
			Status: setupCheckWarning,
			Detail: "not configured (optional, recommended for multiple instances)",
		})
	} else {
		probeKey := fmt.Sprintf("setup-check:%d", time.Now().UnixNano())
		err := common.RedisSet(probeKey, "1", 30*time.Second)
		if err != nil {
			checks = append(checks, SetupCheck{
				Key:    "redis",
				Status: setupCheckError,
				Detail: err.Error(),
			})
		} else {
			_ = common.RedisDel(probeKey)
			checks = append(checks, SetupCheck{
				Key:    "redis",
				Status: setupCheckOK,
				Detail: "connected",
			})
		}
	}

	scheme := strings.TrimSpace(strings.Split(c.Request.Header.Get("X-Forwarded-Proto"), ",")[0])
	if scheme == "" {
		scheme = "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}
	if scheme == "https" {
		checks = append(checks, SetupCheck{
			Key:    "https",
			Status: setupCheckOK,
			Detail: "https",
		})
	} else {
		checks = append(checks, SetupCheck{
			Key:    "https",
			Status: setupCheckWarning,
			Detail: "serving over http; put a TLS proxy in front before going public",
		})
	}

	checks = append(checks, SetupCheck{
		Key:    "version",
		Status: setupCheckOK,
		Detail: common.Version,
	})

	c.JSON(200, gin.H{
		"success": true,
		"data":    checks,
	})
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
