package custom

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t2bot/matrix-media-repo/api/_apimeta"
	"github.com/t2bot/matrix-media-repo/api/_responses"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
)

func TestSynGetUsersMediaStatsTrustedRepoAdminBypassesLocalAdminProbe(t *testing.T) {
	oldIsUserAdmin := _apimeta.IsUserAdminForRequest
	t.Cleanup(func() {
		_apimeta.IsUserAdminForRequest = oldIsUserAdmin
	})

	called := false
	_apimeta.IsUserAdminForRequest = func(ctx rcontext.RequestContext, serverName string, accessToken string, ipAddr string) (bool, error) {
		called = true
		return false, nil
	}

	req := httptest.NewRequest("GET", "http://example.com/_synapse/admin/v1/statistics/users/media", nil)
	req.Host = "example.com"
	req.RemoteAddr = "127.0.0.1:12345"

	resp, ok := SynGetUsersMediaStats(req, rcontext.InitialNoConfig(), _apimeta.UserInfo{UserId: "@user:example.com", IsTrustedHeaderAdmin: true}).(*_responses.ErrorResponse)
	require.True(t, ok)
	assert.False(t, called)
	assert.Equal(t, _responses.BadRequest("Query parameter 'dir' must be one of ['f', 'b']"), resp)
}

func TestSynGetUsersMediaStatsRejectsNonAdmin(t *testing.T) {
	oldIsUserAdmin := _apimeta.IsUserAdminForRequest
	t.Cleanup(func() {
		_apimeta.IsUserAdminForRequest = oldIsUserAdmin
	})

	_apimeta.IsUserAdminForRequest = func(ctx rcontext.RequestContext, serverName string, accessToken string, ipAddr string) (bool, error) {
		return false, nil
	}

	req := httptest.NewRequest("GET", "http://example.com/_synapse/admin/v1/statistics/users/media", nil)
	req.Host = "example.com"
	req.RemoteAddr = "127.0.0.1:12345"

	resp, ok := SynGetUsersMediaStats(req, rcontext.InitialNoConfig(), _apimeta.UserInfo{UserId: "@user:example.com"}).(*_responses.ErrorResponse)
	require.True(t, ok)
	assert.Equal(t, _responses.AuthFailed(), resp)
}
