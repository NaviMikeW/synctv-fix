package bootstrap

import (
	"testing"

	"github.com/synctv-org/synctv/internal/conf"
)

func TestSecurityConfigurationLoadsFromUnprefixedEnvironment(t *testing.T) {
	t.Setenv("INITIAL_ROOT_PASSWORD", "Configured1!")
	t.Setenv(
		"GUARDIAN_CREDENTIAL_KEY",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	t.Setenv("LOGIN_RATE_LIMIT_LIMIT", "30")
	t.Setenv("LOGIN_RATE_LIMIT_PERIOD", "2m")
	t.Setenv("GUARDIAN_REQUIRE_HTTPS", "false")
	t.Setenv("GUARDIAN_TRUST_FORWARDED_PROTO", "true")

	config := conf.DefaultConfig()
	if err := confFromEnv("", config); err != nil {
		t.Fatalf("confFromEnv() error = %v", err)
	}

	if got := config.Security.InitialRootPassword; got != "Configured1!" {
		t.Fatalf("initial root password = %q, want configured value", got)
	}
	if got := config.Security.GuardianCredentialKey; got !=
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("guardian credential key = %q, want configured value", got)
	}
	if config.Security.GuardianRequireHTTPS {
		t.Fatal("guardian HTTPS requirement = true, want configured false")
	}
	if !config.Security.GuardianTrustForwardedProto {
		t.Fatal("guardian trusted forwarded proto = false, want configured true")
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
	t.Setenv(
		"SYNCTV_GUARDIAN_CREDENTIAL_KEY",
		"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	)
	t.Setenv("SYNCTV_GUARDIAN_TRUST_FORWARDED_PROTO", "true")

	config := conf.DefaultConfig()
	if err := confFromEnv("SYNCTV_", config); err != nil {
		t.Fatalf("confFromEnv() error = %v", err)
	}

	if got := config.Security.InitialRootPassword; got != "Configured1!" {
		t.Fatalf("initial root password = %q, want configured value", got)
	}
	if got := config.Security.GuardianCredentialKey; got !=
		"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" {
		t.Fatalf("guardian credential key = %q, want configured value", got)
	}
	if !config.Security.GuardianRequireHTTPS {
		t.Fatal("guardian HTTPS requirement default = false, want true")
	}
	if !config.Security.GuardianTrustForwardedProto {
		t.Fatal("guardian trusted forwarded proto = false, want configured true")
	}
}
