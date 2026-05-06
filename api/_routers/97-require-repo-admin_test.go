package _routers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t2bot/matrix-media-repo/api/_apimeta"
	"github.com/t2bot/matrix-media-repo/api/_responses"
	"github.com/t2bot/matrix-media-repo/common/config"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/matrix"
)

func TestRequireRepoAdminSupportsConfigSharedSecretAndTrustedHeader(t *testing.T) {
	cfg := config.Get()
	oldAdmins := append([]string(nil), cfg.Admins...)
	oldSharedSecret := cfg.SharedSecret
	oldTrustedHeader := cfg.TrustedAdminHeader
	oldGetUserId := getUserIdForToken
	t.Cleanup(func() {
		cfg.Admins = oldAdmins
		cfg.SharedSecret = oldSharedSecret
		cfg.TrustedAdminHeader = oldTrustedHeader
		getUserIdForToken = oldGetUserId
	})

	getUserIdForToken = func(ctx rcontext.RequestContext, accessToken string, appserviceUserId string) (string, bool, error) {
		return "@user:example.com", false, nil
	}

	handler := RequireRepoAdmin(func(r *http.Request, ctx rcontext.RequestContext, user _apimeta.UserInfo) interface{} {
		return "ok"
	})

	t.Run("config admin passes", func(t *testing.T) {
		cfg.Admins = []string{"@user:example.com"}
		cfg.SharedSecret = config.SharedSecretConfig{}
		cfg.TrustedAdminHeader = config.TrustedAdminHeaderConfig{}

		req := httptest.NewRequest("GET", "http://example.com/_matrix/media/unstable/admin/tasks/all", nil)
		req.Header.Set("Authorization", "Bearer user-token")

		assert.Equal(t, "ok", handler(req, rcontext.InitialNoConfig()))
	})

	t.Run("shared secret passes", func(t *testing.T) {
		cfg.Admins = nil
		cfg.SharedSecret = config.SharedSecretConfig{Enabled: true, Token: "shared-token"}
		cfg.TrustedAdminHeader = config.TrustedAdminHeaderConfig{}

		req := httptest.NewRequest("GET", "http://example.com/_matrix/media/unstable/admin/tasks/all", nil)
		req.Header.Set("Authorization", "Bearer shared-token")

		assert.Equal(t, "ok", handler(req, rcontext.InitialNoConfig()))
	})

	t.Run("trusted header passes", func(t *testing.T) {
		cfg.Admins = nil
		cfg.SharedSecret = config.SharedSecretConfig{}
		cfg.TrustedAdminHeader = config.TrustedAdminHeaderConfig{Enabled: true, Header: "X-Matrix-Repo-Admin", Value: "true"}

		req := httptest.NewRequest("GET", "http://example.com/_matrix/media/unstable/admin/tasks/all", nil)
		req.Header.Set("Authorization", "Bearer user-token")
		req.Header.Set("X-Matrix-Repo-Admin", "true")

		assert.Equal(t, "ok", handler(req, rcontext.InitialNoConfig()))
	})

	t.Run("regular user fails", func(t *testing.T) {
		cfg.Admins = nil
		cfg.SharedSecret = config.SharedSecretConfig{}
		cfg.TrustedAdminHeader = config.TrustedAdminHeaderConfig{Enabled: true, Header: "X-Matrix-Repo-Admin", Value: "true"}

		req := httptest.NewRequest("GET", "http://example.com/_matrix/media/unstable/admin/tasks/all", nil)
		req.Header.Set("Authorization", "Bearer user-token")

		resp, ok := handler(req, rcontext.InitialNoConfig()).(*_responses.ErrorResponse)
		require.True(t, ok)
		assert.Equal(t, _responses.AuthFailed(), resp)
	})

	t.Run("wrong trusted value fails", func(t *testing.T) {
		cfg.Admins = nil
		cfg.SharedSecret = config.SharedSecretConfig{}
		cfg.TrustedAdminHeader = config.TrustedAdminHeaderConfig{Enabled: true, Header: "X-Matrix-Repo-Admin", Value: "true"}

		req := httptest.NewRequest("GET", "http://example.com/_matrix/media/unstable/admin/tasks/all", nil)
		req.Header.Set("Authorization", "Bearer user-token")
		req.Header.Set("X-Matrix-Repo-Admin", "TRUE")

		resp, ok := handler(req, rcontext.InitialNoConfig()).(*_responses.ErrorResponse)
		require.True(t, ok)
		assert.Equal(t, _responses.AuthFailed(), resp)
	})
}

func TestOptionalAccessTokenDoesNotGrantTrustedHeaderOnInvalidToken(t *testing.T) {
	cfg := config.Get()
	oldTrustedHeader := cfg.TrustedAdminHeader
	oldGetUserId := getUserIdForToken
	t.Cleanup(func() {
		cfg.TrustedAdminHeader = oldTrustedHeader
		getUserIdForToken = oldGetUserId
	})

	cfg.TrustedAdminHeader = config.TrustedAdminHeaderConfig{Enabled: true, Header: "X-Matrix-Repo-Admin", Value: "true"}
	getUserIdForToken = func(ctx rcontext.RequestContext, accessToken string, appserviceUserId string) (string, bool, error) {
		return "", false, matrix.ErrInvalidToken
	}

	handler := OptionalAccessToken(func(r *http.Request, ctx rcontext.RequestContext, user _apimeta.UserInfo) interface{} {
		return user
	})

	req := httptest.NewRequest("GET", "http://example.com/_matrix/media/r0/download/example.com/id", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	req.Header.Set("X-Matrix-Repo-Admin", "true")

	user, ok := handler(req, rcontext.InitialNoConfig()).(_apimeta.UserInfo)
	require.True(t, ok)
	assert.Empty(t, user.UserId)
	assert.False(t, user.IsTrustedHeaderAdmin)
	assert.False(t, user.IsRepoAdmin())
}
