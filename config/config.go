package config

import (
	"os"
	"strings"
)

// allowedOrigins parses the CORS_ORIGIN env var as a comma-separated list,
// trims spaces and trailing slashes, and defaults to localhost:3000 when
// unset.
func allowedOrigins() []string {
	raw := os.Getenv("CORS_ORIGIN")
	if raw == "" {
		return []string{"http://localhost:3000"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		o := strings.TrimRight(strings.TrimSpace(p), "/")
		if o != "" {
			out = append(out, o)
		}
	}
	if len(out) == 0 {
		return []string{"http://localhost:3000"}
	}
	return out
}

// AllowedOrigin returns the first configured origin. Kept for log/diagnostic
// callers; for actual access control use IsAllowedOrigin.
func AllowedOrigin() string {
	return allowedOrigins()[0]
}

// AllowedOrigins returns every configured origin (browser-style, no trailing slash).
func AllowedOrigins() []string {
	return allowedOrigins()
}

// IsAllowedOrigin reports whether the given browser Origin header value is in
// the configured allow-list. Comparison is exact (case-sensitive scheme+host+port,
// trailing slashes stripped on both sides).
func IsAllowedOrigin(origin string) bool {
	o := strings.TrimRight(origin, "/")
	for _, allowed := range allowedOrigins() {
		if o == allowed {
			return true
		}
	}
	return false
}
