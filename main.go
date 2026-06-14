package main

import (
    "encoding/json"
    "net/http"
)

// healthHandler returns a simple JSON status.
func healthHandler(w http.ResponseWriter, r *http.Request) {
    // Handle preflight
    if r.Method == http.MethodOptions {
        setCORSHeaders(w)
        w.WriteHeader(http.StatusOK)
        return
    }

    setCORSHeaders(w)
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// setCORSHeaders adds the minimal CORS headers for local dev.
func setCORSHeaders(w http.ResponseWriter) {
    w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func main() {
    http.HandleFunc("/health", healthHandler)
    http.ListenAndServe(":8080", nil)
}
