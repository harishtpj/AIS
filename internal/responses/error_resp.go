package responses

import (
	"encoding/json"
	"net/http"
)

type Error struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Error{Error: code, Message: message})
}
