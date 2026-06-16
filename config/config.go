package config

import "os"

// AllowedOrigin returns the browser origin allowed for both CORS and the
// WebSocket handshake. Configurable via CORS_ORIGIN; defaults to local dev.
func AllowedOrigin() string {
	if v := os.Getenv("CORS_ORIGIN"); v != "" {
		return v
	}
	return "http://localhost:3000"
}