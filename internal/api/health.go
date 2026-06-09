package api

import (
	"encoding/json"
	"net/http"
)

func handleHealthz(resp http.ResponseWriter, _ *http.Request) error {
	resp.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(resp).Encode(map[string]string{"status": "ok"})
}
