package api

import (
	"encoding/json"
	"net/http"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Connection already closed by client — nothing to do
	}
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

func respondOK(w http.ResponseWriter, data any) {
	if data == nil {
		respondJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	respondJSON(w, 200, data)
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("mustMarshal: " + err.Error())
	}
	return b
}
