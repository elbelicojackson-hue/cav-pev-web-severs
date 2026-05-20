package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropic-cav/cav-cli/internal/identity"
)

// loadSessionToken reads the JWT from ~/.cav/session.json
func loadSessionToken() (string, error) {
	data, err := os.ReadFile(identity.SessionPath())
	if err != nil {
		return "", fmt.Errorf("no active session (run 'cav-cli auth' first)")
	}
	var session map[string]interface{}
	if err := json.Unmarshal(data, &session); err != nil {
		return "", err
	}
	token, ok := session["token"].(string)
	if !ok || token == "" {
		return "", fmt.Errorf("invalid session file")
	}
	return token, nil
}
