package filter

import "github.com/elsyahtech/gorest/server"

func deleteCookie(ctx server.Context, cookieName string, cookie *Cookie) {
	ctx.Cookie(&server.Cookie{
		// Name of the cookie to be deleted
		Name: cookieName,

		// Value is set to empty to clear the cookie contents
		Value: "",

		// Domain scope where the cookie is valid
		Domain: GetDefaultCookiesDomain(cookie.Domain),

		// HTTPOnly flag restricting client-side script access
		HTTPOnly: cookie.HTTPOnly,

		// Secure flag restricting cookie transmission to HTTPS only
		Secure: cookie.Secure,

		// SameSite attribute policy for cross-site request security
		SameSite: GetDefaultCookiesSameSite(cookie.SameSite),

		// Path scope must match the original cookie to remove it.
		Path: cookie.Path,

		// Expires timestamp to immediately invalidate the cookie
		Expires: cookie.Expires,
	})
}
