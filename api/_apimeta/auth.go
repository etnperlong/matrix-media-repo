package _apimeta

import (
	"net/http"

	"github.com/getsentry/sentry-go"

	"github.com/t2bot/matrix-media-repo/common/config"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/matrix"
	"github.com/t2bot/matrix-media-repo/util"
)

type UserInfo struct {
	UserId               string
	AccessToken          string
	IsShared             bool
	IsTrustedHeaderAdmin bool
}

var IsUserAdminForRequest = matrix.IsUserAdmin

type ServerInfo struct {
	ServerName string
}

type AuthContext struct {
	User   UserInfo
	Server ServerInfo
}

func (a AuthContext) IsAuthenticated() bool {
	return a.User.UserId != "" || a.Server.ServerName != ""
}

func (u UserInfo) IsRepoAdmin() bool {
	return u.IsShared || util.IsGlobalAdmin(u.UserId) || (u.UserId != "" && u.IsTrustedHeaderAdmin)
}

func RequestClaimsRepoAdmin(r *http.Request) bool {
	cfg := config.Get().TrustedAdminHeader
	if !cfg.Enabled || cfg.Header == "" || cfg.Value == "" {
		return false
	}
	return r.Header.Get(cfg.Header) == cfg.Value
}

func GetRequestUserAdminStatus(r *http.Request, rctx rcontext.RequestContext, user UserInfo) (bool, bool) {
	isGlobalAdmin := user.IsRepoAdmin()
	if isGlobalAdmin {
		return true, false
	}
	isLocalAdmin, err := IsUserAdminForRequest(rctx, r.Host, user.AccessToken, r.RemoteAddr)
	if err != nil {
		sentry.CaptureException(err)
		rctx.Log.Debug("Error verifying local admin: ", err)
		return isGlobalAdmin, false
	}

	return isGlobalAdmin, isLocalAdmin
}
