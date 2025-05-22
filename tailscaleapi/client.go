package tailscaleapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	// "time" // No longer needed

	"github.com/whitehawk2/tsrouter/models"
	log "github.com/sirupsen/logrus"
)

const tailscaleAPIBase = "https://api.tailscale.com/api/v2"

// GenerateAuthKey creates a new Tailscale auth key.
// client is an HTTP client authenticated to make requests to the Tailscale API.
// tailnet is the name of the tailnet to create the key for.
// expirySeconds is the number of seconds until the key expires.
func GenerateAuthKey(ctx context.Context, client *http.Client, tailnet string, expirySeconds int) (*models.TailscaleAuthKey, error) {
	endpoint := fmt.Sprintf("%s/tailnet/%s/keys", tailscaleAPIBase, tailnet)
	log.WithField("endpoint", endpoint).Debug("Generating new auth key")

	reqBody := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"devices": map[string]interface{}{
				"create": map[string]interface{}{
					"reusable":      false,
					"ephemeral":     true,
					"preauthorized": true,
					"tags":          []string{"tag:server"}, // TODO: make this configurable
				},
			},
		},
		"expirySeconds": expirySeconds,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auth key request: %v", err)
	}

	log.WithFields(log.Fields{
		"endpoint": endpoint,
		"body":     string(jsonBody),
	}).Debug("Sending auth key request")

	req, err := http.NewRequestWithContext(ctx, "POST",
		endpoint,
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create auth key request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send auth key request: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body) // Read body for logging, even on error
	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"status_code": resp.StatusCode,
			"endpoint":    endpoint,
			"response":    string(bodyBytes),
		}).Debug("Auth key request failed")
		return nil, fmt.Errorf("failed to generate auth key: HTTP %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var authKey models.TailscaleAuthKey
	if err := json.Unmarshal(bodyBytes, &authKey); err != nil {
		return nil, fmt.Errorf("failed to decode auth key response: %v", err)
	}

	// Log success with key ID and expiry (from response, not calculated)
	log.WithFields(log.Fields{
		"key_id":   authKey.ID,
		"expires":  authKey.Expires, // This comes from the API response
		"endpoint": endpoint,
		// "response": string(bodyBytes), // Potentially too verbose for successful requests
	}).Debug("Generated new auth key")
	return &authKey, nil
}
