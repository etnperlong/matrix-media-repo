package _routers

import (
	"errors"
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
	"github.com/t2bot/matrix-media-repo/api/_apimeta"
	"github.com/t2bot/matrix-media-repo/api/_responses"
	"github.com/t2bot/matrix-media-repo/common/config"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/matrix"
	"github.com/t2bot/matrix-media-repo/util"
)

func OptionalAccessToken(generator GeneratorWithUserFn) GeneratorFn {
	return func(r *http.Request, ctx rcontext.RequestContext) interface{} {
		accessToken := util.GetAccessTokenFromRequest(r)
		if accessToken == "" {
			return generator(r, ctx, _apimeta.UserInfo{
				UserId:      "",
				AccessToken: "",
				IsShared:    false,
			})
		}
		if config.Get().SharedSecret.Enabled && accessToken == config.Get().SharedSecret.Token {
			ctx = ctx.LogWithFields(logrus.Fields{"sharedSecretAuth": true})
			ctx, user := applyTrustedRepoAdminHeader(r, ctx, _apimeta.UserInfo{
				UserId:      "@sharedsecret",
				AccessToken: accessToken,
				IsShared:    true,
			})
			return generator(r, ctx, user)
		}
		appserviceUserId := util.GetAppserviceUserIdFromRequest(r)
		userId, isGuest, err := getUserIdForToken(ctx, accessToken, appserviceUserId)
		if isGuest {
			return _responses.GuestAuthFailed()
		}
		if err != nil {
			if !errors.Is(err, matrix.ErrInvalidToken) {
				sentry.CaptureException(err)
				ctx.Log.Error("Error verifying token: ", err)
				return _responses.InternalServerError("unexpected error validating access token")
			}

			ctx.Log.Warn("Failed to verify token (non-fatal): ", err)
			userId = ""
		}

		ctx = ctx.LogWithFields(logrus.Fields{"authUserId": userId})
		user := _apimeta.UserInfo{
			UserId:      userId,
			AccessToken: accessToken,
			IsShared:    false,
		}
		if userId != "" {
			ctx, user = applyTrustedRepoAdminHeader(r, ctx, user)
		}
		return generator(r, ctx, user)
	}
}
