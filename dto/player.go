package dto

// PlayerInfo is sent to clients – no token, only JSON‑safe data.
// It is deliberately independent of the domain package; it uses basic types.
// The conversion helper that turns a store.Player into this type lives in
// the websocket layer.
//
// Fields order follows the JSON naming desired by the front‑end.
//`json` tags are camelCase to match typical JavaScript conventions.

type PlayerInfo struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    IsAdmin     bool   `json:"isAdmin"`
    IsConnected bool   `json:"isConnected"`
}
