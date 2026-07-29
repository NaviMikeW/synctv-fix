package bootstrap

import (
	"testing"

	"github.com/synctv-org/synctv/internal/conf"
)

func TestSecurityConfigurationLoadsFromUnprefixedEnvironment(t *testing.T) {
	t.Setenv("INITIAL_ROOT_PASSWORD", "Configured1!")
	t.Setenv("LOGIN_RATE_LIMIT_LIMIT", "30")
	t.Setenv("LOGIN_RATE_LIMIT_PERIOD", "2m")

	config := conf.DefaultConfig()
	if err := confFromEnv("", config); err != nil {
		t.Fatalf("confFromEnv() error = %v", err)
	}

	if got := config.Security.InitialRootPassword; got != "Configured1!" {
		t.Fatalf("initial root password = %q, want configured value", got)
	}
	if got := config.Security.LoginRateLimit.Limit; got != 30 {
		t.Fatalf("login rate limit = %d, want 30", got)
	}
	if got := config.Security.LoginRateLimit.Period; got != "2m" {
		t.Fatalf("login rate limit period = %q, want 2m", got)
	}
}

func TestSecurityConfigurationLoadsFromPrefixedEnvironment(t *testing.T) {
	t.Setenv("SYNCTV_INITIAL_ROOT_PASSWORD", "Configured1!")

	config := conf.DefaultConfig()
	if err := confFromEnv("SYNCTV_", config); err != nil {
		t.Fatalf("confFromEnv() error = %v", err)
	}

	if got := config.Security.InitialRootPassword; got != "Configured1!" {
		t.Fatalf("initial root password = %q, want configured value", got)
	}
}
