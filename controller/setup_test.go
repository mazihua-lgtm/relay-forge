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

// applySetupFirstChannel must reject bad input before it reaches the database.
// Skipping the step is not an error: a blank key means "connect an upstream
// later", which is how a station can be brought up first.
func TestApplySetupFirstChannel(t *testing.T) {
	cases := []struct {
		name    string
		req     *SetupChannel
		wantErr string
	}{
		{
			name: "missing step is a no-op",
			req:  nil,
		},
		{
			name: "blank key skips channel creation",
			req:  &SetupChannel{Type: 1, Key: "   "},
		},
		{
			name:    "unsupported channel type",
			req:     &SetupChannel{Type: 8, Key: "sk-test", Models: "gpt-4o"},
			wantErr: "must be OpenAI-compatible or Anthropic",
		},
		{
			name:    "no models",
			req:     &SetupChannel{Type: 1, Key: "sk-test", Models: " , "},
			wantErr: "at least one model is required",
		},
		{
			name:    "channel name too long",
			req:     &SetupChannel{Type: 1, Key: "sk-test", Models: "gpt-4o", Name: strings.Repeat("渠", 65)},
			wantErr: "channel name must be 64 characters or fewer",
		},
		{
			name:    "base URL without a scheme",
			req:     &SetupChannel{Type: 1, Key: "sk-test", Models: "gpt-4o", BaseURL: "api.example.com"},
			wantErr: "base URL must start with http:// or https://",
		},
		{
			name:    "base URL too long",
			req:     &SetupChannel{Type: 1, Key: "sk-test", Models: "gpt-4o", BaseURL: "https://example.com/" + strings.Repeat("a", 500)},
			wantErr: "base URL must be 512 characters or fewer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := applySetupFirstChannel(tc.req)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestNormalizeSetupModels(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "trims and drops blanks", raw: " gpt-4o , , claude-sonnet-5 ", want: []string{"gpt-4o", "claude-sonnet-5"}},
		{name: "keeps the first occurrence of a duplicate", raw: "b,a,b", want: []string{"b", "a"}},
		{name: "empty input yields no models", raw: "   ", want: []string{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeSetupModels(tc.raw))
		})
	}
}
