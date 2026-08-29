package config

import (
	"os"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestLoadConfig_ConcurrencyDefaults(t *testing.T) {
	// Clear env vars to get defaults
	t.Setenv("YT_MAX_CONCURRENCY", "")
	t.Setenv("YT_QUEUE_TIMEOUT_SECONDS", "")
	t.Setenv("YT_REQUEST_TIMEOUT_SECONDS", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConcurrency != 10 {
		t.Errorf("MaxConcurrency = %d, want 10", cfg.MaxConcurrency)
	}
	if cfg.QueueTimeout != 30*time.Second {
		t.Errorf("QueueTimeout = %v, want 30s", cfg.QueueTimeout)
	}
	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("RequestTimeout = %v, want 30s", cfg.RequestTimeout)
	}
}

func TestLoadConfig_ConcurrencyEnvOverrides(t *testing.T) {
	t.Setenv("YT_MAX_CONCURRENCY", "50")
	t.Setenv("YT_QUEUE_TIMEOUT_SECONDS", "60")
	t.Setenv("YT_REQUEST_TIMEOUT_SECONDS", "15")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConcurrency != 50 {
		t.Errorf("MaxConcurrency = %d, want 50", cfg.MaxConcurrency)
	}
	if cfg.QueueTimeout != 60*time.Second {
		t.Errorf("QueueTimeout = %v, want 60s", cfg.QueueTimeout)
	}
	if cfg.RequestTimeout != 15*time.Second {
		t.Errorf("RequestTimeout = %v, want 15s", cfg.RequestTimeout)
	}
}

func TestLoadConfig_ConcurrencyClamping(t *testing.T) {
	tests := []struct {
		name    string
		envKey  string
		envVal  string
		wantInt int
		wantDur time.Duration
		checkFn func(*Config) (any, any) // got, want
	}{
		{"MaxConcurrency below min", "YT_MAX_CONCURRENCY", "0", 1, 0,
			func(c *Config) (any, any) { return c.MaxConcurrency, 1 }},
		{"MaxConcurrency above max", "YT_MAX_CONCURRENCY", "200", 100, 0,
			func(c *Config) (any, any) { return c.MaxConcurrency, 100 }},
		{"MaxConcurrency at min", "YT_MAX_CONCURRENCY", "1", 1, 0,
			func(c *Config) (any, any) { return c.MaxConcurrency, 1 }},
		{"MaxConcurrency at max", "YT_MAX_CONCURRENCY", "100", 100, 0,
			func(c *Config) (any, any) { return c.MaxConcurrency, 100 }},
		{"QueueTimeout below min", "YT_QUEUE_TIMEOUT_SECONDS", "0", 0, 1 * time.Second,
			func(c *Config) (any, any) { return c.QueueTimeout, 1 * time.Second }},
		{"QueueTimeout above max", "YT_QUEUE_TIMEOUT_SECONDS", "999", 0, 300 * time.Second,
			func(c *Config) (any, any) { return c.QueueTimeout, 300 * time.Second }},
		{"RequestTimeout below min", "YT_REQUEST_TIMEOUT_SECONDS", "-5", 0, 1 * time.Second,
			func(c *Config) (any, any) { return c.RequestTimeout, 1 * time.Second }},
		{"RequestTimeout above max", "YT_REQUEST_TIMEOUT_SECONDS", "500", 0, 300 * time.Second,
			func(c *Config) (any, any) { return c.RequestTimeout, 300 * time.Second }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("YT_MAX_CONCURRENCY", "")
			t.Setenv("YT_QUEUE_TIMEOUT_SECONDS", "")
			t.Setenv("YT_REQUEST_TIMEOUT_SECONDS", "")
			t.Setenv(tt.envKey, tt.envVal)

			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			got, want := tt.checkFn(cfg)
			if got != want {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

func TestLoadConfig_ConcurrencyInvalidValue(t *testing.T) {
	t.Setenv("YT_MAX_CONCURRENCY", "notanumber")
	t.Setenv("YT_QUEUE_TIMEOUT_SECONDS", "")
	t.Setenv("YT_REQUEST_TIMEOUT_SECONDS", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConcurrency != 10 {
		t.Errorf("MaxConcurrency = %d, want default 10 on invalid input", cfg.MaxConcurrency)
	}
}

func TestLoadConfig_AuthUsername(t *testing.T) {
	tests := []struct {
		name   string
		setEnv bool
		envVal string
		want   string
	}{
		{"set to mysecret", true, "mysecret", "mysecret"},
		{"set to empty string", true, "", ""},
		{"unset", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv("AUTH_USERNAME", tt.envVal)
			} else {
				t.Setenv("AUTH_USERNAME", "")
			}

			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.AuthUsername != tt.want {
				t.Errorf("AuthUsername = %q, want %q", cfg.AuthUsername, tt.want)
			}
		})
	}
}

// Feature: auth-username-validation, Property 1: Whitespace-only AUTH_USERNAME is treated as empty
// **Validates: Requirements 1.3**
func TestLoadConfig_AuthUsernameWhitespace_Property(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		ws := rapid.StringMatching(`^\s+$`).Draw(rt, "whitespace-only")
		os.Setenv("AUTH_USERNAME", ws)
		defer os.Unsetenv("AUTH_USERNAME")

		cfg, err := LoadConfig()
		if err != nil {
			rt.Fatal(err)
		}
		if cfg.AuthUsername != "" {
			rt.Errorf("AuthUsername = %q, want empty for whitespace-only input %q", cfg.AuthUsername, ws)
		}
	})
}
