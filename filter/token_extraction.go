package filter

import (
	"strings"

	"github.com/elsyahtech/gorest/server"
)

func extractTokens(ctx *server.Context) (accessToken, refreshToken string) {
	authorization := strings.Fields(ctx.Get("Authorization"))
	if len(authorization) == 2 && strings.EqualFold(authorization[0], "Bearer") {
		accessToken = authorization[1]
	} else {
		accessToken = ctx.Cookies("access")
	}

	refreshToken = ctx.Cookies("refresh")

	return accessToken, refreshToken
}
