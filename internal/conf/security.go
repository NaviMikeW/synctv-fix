package conf

// SecurityConfig contains authentication-specific safeguards.
//
// InitialRootPassword and GuardianCredentialKey are deliberately
// environment-only. With the default SYNCTV_ prefix they are never persisted
// to config.yaml.
type SecurityConfig struct {
	InitialRootPassword         string               `env:"INITIAL_ROOT_PASSWORD"               yaml:"-"`
	GuardianCredentialKey       string               `env:"GUARDIAN_CREDENTIAL_KEY"              yaml:"-"`
	GuardianRequireHTTPS        bool                 `env:"GUARDIAN_REQUIRE_HTTPS"                lc:"default: true" yaml:"-"`
	GuardianTrustForwardedProto bool                 `env:"GUARDIAN_TRUST_FORWARDED_PROTO"                         yaml:"-"`
	LoginRateLimit              LoginRateLimitConfig `yaml:"login_rate_limit"`
}

type LoginRateLimitConfig struct {
	Enable                bool   `env:"LOGIN_RATE_LIMIT_ENABLE"                    lc:"default: true" yaml:"enable"`
	Period                string `env:"LOGIN_RATE_LIMIT_PERIOD"                                        yaml:"period"`
	Limit                 int64  `env:"LOGIN_RATE_LIMIT_LIMIT"                                         yaml:"limit"`
	TrustForwardHeader    bool   `env:"LOGIN_RATE_LIMIT_TRUST_FORWARD_HEADER"      lc:"default: false" yaml:"trust_forward_header"     hc:"trust X-Real-IP and X-Forwarded-For only when a trusted reverse proxy overwrites these headers"`
	TrustedClientIPHeader string `env:"LOGIN_RATE_LIMIT_TRUSTED_CLIENT_IP_HEADER"                      yaml:"trusted_client_ip_header" hc:"custom header containing the client IP; use only when a trusted reverse proxy overwrites it"`
}

func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		GuardianRequireHTTPS: true,
		LoginRateLimit: LoginRateLimitConfig{
			Enable:                true,
			Period:                "1m",
			Limit:                 10,
			TrustForwardHeader:    false,
			TrustedClientIPHeader: "",
		},
	}
}
