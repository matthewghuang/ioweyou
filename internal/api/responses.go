package api

import (
	"encoding/json"
	"net/http"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
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
