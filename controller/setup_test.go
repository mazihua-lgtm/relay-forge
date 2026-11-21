package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applySetupSiteOptions must reject bad white-label input before it reaches the
// options table. The cases below all fail during validation, so they need no
// database.
func TestApplySetupSiteOptionsRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name    string
		req     SetupRequest
		wantErr string
	}{
		{
			name:    "site name longer than 64 characters",
			req:     SetupRequest{SiteName: strings.Repeat("站", 65)},
			wantErr: "site name must be 64 characters or fewer",
		},
		{
			name:    "logo longer than 512 characters",
			req:     SetupRequest{Logo: "https://example.com/" + strings.Repeat("a", 500)},
			wantErr: "logo URL must be 512 characters or fewer",
		},
		{
			name:    "logo without an http scheme or leading slash",
			req:     SetupRequest{Logo: "ftp://example.com/logo.png"},
			wantErr: "logo must be an http(s) URL or an absolute path",
		},
		{
			name:    "logo with a relative path",
			req:     SetupRequest{Logo: "logo.png"},
			wantErr: "logo must be an http(s) URL or an absolute path",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := applySetupSiteOptions(tc.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// Skipping both fields must be a no-op: the wizard lets an operator leave the
// step empty, and that must not touch the stored options.
func TestApplySetupSiteOptionsAcceptsEmptyInput(t *testing.T) {
	assert.NoError(t, applySetupSiteOptions(SetupRequest{}))
	assert.NoError(t, applySetupSiteOptions(SetupRequest{SiteName: "   ", Logo: "  "}))
}
