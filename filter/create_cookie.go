package filter

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elsyahtech/gorest/server"
)

func (filter *Filter) createCookie(ctx server.Context, cookieName, token string, expiresAt time.Time) (string, int, error) {
	if filter == nil || filter.Config == nil {
		return filterConfigurationMessage, http.StatusNotImplemented,
			errors.New("filter configuration is error: config is nil")
	}

	// Validate that the cookie name is provided
	if cookieName == "" {
		const message = "Ensure the cookie name is included to set the cookie"

		return message, http.StatusBadRequest, errors.New("missing cookie name")
	}

	// Validate that the token value is provided
	if token == "" {
		const message = "Ensure the token is included to set the cookie"

		return message, http.StatusBadRequest, errors.New("missing token")
	}

	// Build the cookie configuration using global settings and computed expiration time
	remaining := time.Until(expiresAt)

	maxAge := int(remaining / time.Second)
	if remaining%time.Second != 0 {
		maxAge++
	}

	cookie := Cookie{
		Name:        cookieName,
		Value:       token,
		Path:        filter.Config.CookiePath,
		Domain:      filter.Config.CookieDomain,
		SameSite:    filter.Config.CookieSameSite,
		Secure:      filter.Config.CookieSecure,
		HTTPOnly:    filter.Config.CookieHTTPOnly,
		SessionOnly: filter.Config.CookieSessionOnly,
	}
	if !cookie.SessionOnly {
		cookie.Expires = expiresAt
		cookie.MaxAge = maxAge
	}

	// Invoke CreateCookie to attach the single cookie onto the response context
	troubleShootMsg, httpCode, err := setCookie(ctx, &cookie)
	if err != nil {
		return troubleShootMsg, httpCode, fmt.Errorf("set cookie: %w", err)
	}

	// Return
	return troubleShootMsg, httpCode, nil
}

func setCookie(ctx server.Context, cookie *Cookie) (string, int, error) {
	// Validate that the cookie name is provided
	if cookie.Name == "" {
		const message = "Ensure the cookie name is included to create the cookies"

		return message, http.StatusBadRequest, errors.New("missing cookies name")
	}

	// Validate that the cookie value is provided
	if cookie.Value == "" {
		const message = "Ensure the value is included to create the cookies"

		return message, http.StatusBadRequest, errors.New("token detail is empty")
	}

	// Construct the Fiber cookie configuration using default fallbacks and input parameters
	cookieCreated := &server.Cookie{
		Name:        cookie.Name,
		Value:       cookie.Value,
		Domain:      GetDefaultCookiesDomain(cookie.Domain),
		Path:        cookie.Path,
		MaxAge:      cookie.MaxAge,
		Expires:     cookie.Expires,
		Secure:      cookie.Secure,
		HTTPOnly:    cookie.HTTPOnly,
		SameSite:    GetDefaultCookiesSameSite(cookie.SameSite),
		SessionOnly: cookie.SessionOnly,
	}

	// Attach the cookie to the response context
	ctx.Cookie(cookieCreated)

	return "Cookie saved successfully", http.StatusOK, nil
}
