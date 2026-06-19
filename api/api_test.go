package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/facu/bingo-back/store"
)

func newTestServer() http.Handler {
	a := &API{Store: store.NewStore()}
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	return WithCORS(mux)
}

func TestCreateRoom_Success(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/rooms", strings.NewReader(`{"adminName":"facu"}`))
	newTestServer().ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID         string `json:"id"`
		ShortCode  string `json:"shortCode"`
		AdminToken string `json:"adminToken"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == "" || resp.AdminToken == "" {
		t.Fatalf("empty id/token: %+v", resp)
	}
	if len(resp.ShortCode) != 5 {
		t.Fatalf("shortCode = %q, want 5-char string", resp.ShortCode)
	}
}

func TestCreateRoom_BadRequest(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/rooms", strings.NewReader(`{}`))
	newTestServer().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestCreateRoom_OptionsPreflight(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/rooms", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	newTestServer().ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:3000")
	}
}

func TestCreateRoom_OptionsPreflight_DisallowedOriginGetsNoCORS(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/rooms", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	newTestServer().ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q for disallowed origin, want empty", got)
	}
}