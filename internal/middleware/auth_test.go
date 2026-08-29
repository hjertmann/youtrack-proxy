package middleware_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"pgregory.net/rapid"

	"github.com/hjertmann/youtrack-proxy/internal/middleware"
)

func basicAuthHeader(user, token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+token))
}

func TestBasicAuth(t *testing.T) {
	okHandler := func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}

	tests := []struct {
		name             string
		expectedUsername string
		authHeader       string // raw Authorization header value; empty means omit
		wantStatus       int
		wantError        string // expected "error" JSON field; empty means no error body check
	}{
		// --- username validation (expectedUsername configured) ---
		{
			name:             "username match allows request",
			expectedUsername: "admin",
			authHeader:       basicAuthHeader("admin", "tok123"),
			wantStatus:       http.StatusOK,
		},
		{
			name:             "username mismatch returns 401",
			expectedUsername: "admin",
			authHeader:       basicAuthHeader("intruder", "tok123"),
			wantStatus:       http.StatusUnauthorized,
			wantError:        "Username is not authorized",
		},

		// --- empty expectedUsername (gate disabled) ---
		{
			name:             "empty expectedUsername allows any username",
			expectedUsername: "",
			authHeader:       basicAuthHeader("whoever", "tok123"),
			wantStatus:       http.StatusOK,
		},
		{
			name:             "empty expectedUsername allows empty username",
			expectedUsername: "",
			authHeader:       basicAuthHeader("", "tok123"),
			wantStatus:       http.StatusOK,
		},

		// --- missing Authorization header ---
		{
			name:             "missing auth header with configured username returns 401",
			expectedUsername: "admin",
			authHeader:       "",
			wantStatus:       http.StatusUnauthorized,
			wantError:        "Basic Authentication required (email:token)",
		},
		{
			name:             "missing auth header with empty expectedUsername returns 401",
			expectedUsername: "",
			authHeader:       "",
			wantStatus:       http.StatusUnauthorized,
			wantError:        "Basic Authentication required (email:token)",
		},

		// --- invalid base64 ---
		{
			name:             "invalid base64 with configured username returns 400",
			expectedUsername: "admin",
			authHeader:       "Basic !!!notbase64!!!",
			wantStatus:       http.StatusBadRequest,
			wantError:        "Invalid Basic Auth encoding",
		},
		{
			name:             "invalid base64 with empty expectedUsername returns 400",
			expectedUsername: "",
			authHeader:       "Basic !!!notbase64!!!",
			wantStatus:       http.StatusBadRequest,
			wantError:        "Invalid Basic Auth encoding",
		},

		// --- missing colon in decoded credentials ---
		{
			name:             "no colon with configured username returns 400",
			expectedUsername: "admin",
			authHeader:       "Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon")),
			wantStatus:       http.StatusBadRequest,
			wantError:        "Invalid Basic Auth format (expected email:token)",
		},
		{
			name:             "no colon with empty expectedUsername returns 400",
			expectedUsername: "",
			authHeader:       "Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon")),
			wantStatus:       http.StatusBadRequest,
			wantError:        "Invalid Basic Auth format (expected email:token)",
		},

		// --- empty token ---
		{
			name:             "empty token with configured username returns 401",
			expectedUsername: "admin",
			authHeader:       basicAuthHeader("admin", ""),
			wantStatus:       http.StatusUnauthorized,
			wantError:        "Token is required in Basic Auth password field",
		},
		{
			name:             "empty token with empty expectedUsername returns 401",
			expectedUsername: "",
			authHeader:       basicAuthHeader("someone", ""),
			wantStatus:       http.StatusUnauthorized,
			wantError:        "Token is required in Basic Auth password field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			handler := middleware.BasicAuth(tt.expectedUsername)(okHandler)
			_ = handler(c)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantError != "" {
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("failed to parse JSON body: %v", err)
				}
				if body["error"] != tt.wantError {
					t.Errorf("error = %q, want %q", body["error"], tt.wantError)
				}
			}
		})
	}
}

// Feature: auth-username-validation, Property 2: Username gate — accept iff match
// **Validates: Requirements 2.1, 2.2, 4.2**
func TestBasicAuth_UsernameGate_Property(t *testing.T) {
	okHandler := func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}

	rapid.Check(t, func(t *rapid.T) {
		expectedUsername := rapid.StringMatching("[a-zA-Z0-9_]{1,50}").Draw(t, "expectedUsername")
		actualUsername := rapid.OneOf(
			rapid.Just(expectedUsername),
			rapid.StringMatching("[a-zA-Z0-9_]{1,50}"),
		).Draw(t, "actualUsername")
		token := rapid.StringMatching("[a-zA-Z0-9]{1,50}").Draw(t, "token")

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", basicAuthHeader(actualUsername, token))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware.BasicAuth(expectedUsername)(okHandler)
		_ = handler(c)

		if actualUsername == expectedUsername {
			if rec.Code != http.StatusOK {
				t.Fatalf("matching username %q: got status %d, want 200", actualUsername, rec.Code)
			}
		} else {
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("mismatched username (expected=%q, actual=%q): got status %d, want 401", expectedUsername, actualUsername, rec.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to parse JSON body: %v", err)
			}
			if body["error"] != "Username is not authorized" {
				t.Fatalf("error = %q, want %q", body["error"], "Username is not authorized")
			}
		}
	})
}

// Feature: auth-username-validation, Property 3: Empty expectedUsername disables gate
// **Validates: Requirements 2.4, 3.1, 4.3**
func TestBasicAuth_EmptyExpectedUsername_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		actualUsername := rapid.StringMatching("[^:]{0,50}").Draw(t, "actualUsername")
		token := rapid.StringMatching("[a-zA-Z0-9]{1,50}").Draw(t, "token")

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", basicAuthHeader(actualUsername, token))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware.BasicAuth("")(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		_ = handler(c)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for any username with empty expectedUsername, got %d (username=%q)", rec.Code, actualUsername)
		}
	})
}
