package middleware

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// RequestContextKey is the key used to store/retrieve the RequestContext from the echo context.
const RequestContextKey = "requestCtx"

// BasicAuth returns an Echo middleware that extracts Basic Auth credentials,
// validates the format, and stores a model.RequestContext on the echo context.
// The YouTrack bearer token is taken from the Basic Auth password field.
func BasicAuth(expectedUsername string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Basic ") {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "Basic Authentication required (email:token)",
				})
			}

			encoded := strings.TrimPrefix(auth, "Basic ")
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Invalid Basic Auth encoding",
				})
			}

			credentials := strings.SplitN(string(decoded), ":", 2)
			if len(credentials) != 2 {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "Invalid Basic Auth format (expected email:token)",
				})
			}

			token := credentials[1]
			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "Token is required in Basic Auth password field",
				})
			}

			if expectedUsername != "" && subtle.ConstantTimeCompare([]byte(credentials[0]), []byte(expectedUsername)) == 0 {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "Username is not authorized",
				})
			}

			requestCtx := &model.RequestContext{
				YouTrackToken: token,
			}
			c.Set(RequestContextKey, requestCtx)

			return next(c)
		}
	}
}
