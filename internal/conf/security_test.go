package conf

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultSecurityConfig(t *testing.T) {
	config := DefaultSecurityConfig()
	if !config.LoginRateLimit.Enable {
		t.Fatal("login rate limiter is disabled by default")
	}
	if config.LoginRateLimit.Period != "1m" {
		t.Fatalf("login rate limit period = %q, want 1m", config.LoginRateLimit.Period)
	}
	if config.LoginRateLimit.Limit != 10 {
		t.Fatalf("login rate limit = %d, want 10", config.LoginRateLimit.Limit)
	}
}

func TestInitialRootPasswordIsNotPersistedToYAML(t *testing.T) {
	config := DefaultConfig()
	config.Security.InitialRootPassword = "DoNotPersist1!"

	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if strings.Contains(string(data), config.Security.InitialRootPassword) {
		t.Fatal("initial root password was persisted to YAML")
	}
}
