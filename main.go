package main

// TODO: General:
//		 - graceful shutdown (close Tailscale node, close listener, etc.), with signal handling (SIGINT, SIGTERM)
//		 - add Error handling to LSP pinged issues, and to the GetAccessToken function from oauth.go
//		 - Logging overview
//		 - security, general cleanup, and optimization overview
//		 - support multiple concurrent reverse proxies instead of making the user run multiple instances of the program
//		 - detection (and handling) of the case where the user tries to run the program with the same hostname and target port
//		 - detection and integration to the proxied service - deteced if port is listning, graceful shutdown, etc.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"tailscale.com/tsnet"
)

type Config struct {
	TargetPort int
	Hostname   string
	LogLevel   string
}

const (
	tailscaleAuthURL  = "https://api.tailscale.com/api/v2/oauth/token"
	tailscaleAPIBase  = "https://api.tailscale.com/api/v2"
	authKeyExpiryDays = 14 // TODO: Make this configurable
)

type TailscaleAuthKey struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Created   time.Time `json:"created"`
	Expires   time.Time `json:"expires"`
	Ephemeral bool      `json:"ephemeral"`
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.IntVar(&cfg.TargetPort, "target-port", 0, "Local port to forward to")
	flag.StringVar(&cfg.Hostname, "hostname", "", "Desired Tailscale hostname")
	flag.StringVar(&cfg.LogLevel, "log-level", "error", "Log level (error, info, debug)")
	flag.Parse()

	if cfg.TargetPort == 0 || cfg.Hostname == "" {
		flag.Usage()
		os.Exit(1)
	}

	return cfg
}

func setupLogging(level string) {
	switch strings.ToLower(level) {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
}

func obscureCredential(cred string) string {
	if len(cred) <= 8 {
		return "***"
	}
	return cred[:4] + "..." + cred[len(cred)-4:]
}

func loadEnvConfig() error {
	// Try to load from .env file in the same directory as the executable
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	envPath := filepath.Join(filepath.Dir(exePath), ".env")
	if err := godotenv.Load(envPath); err != nil {
		// If not found in executable directory, try current working directory
		if err := godotenv.Load(); err != nil {
			return fmt.Errorf("no .env file found in executable directory or current directory")
		}
	}

	return nil
}

func generateAuthKey(ctx context.Context, client *http.Client, tailnet string) (*TailscaleAuthKey, error) {
	endpoint := fmt.Sprintf("%s/tailnet/%s/keys", tailscaleAPIBase, tailnet)
	log.WithField("endpoint", endpoint).Debug("Generating new auth key")

	expiry := time.Now().Add(authKeyExpiryDays * 24 * time.Hour)

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
		"expirySeconds": int(expiry.Sub(time.Now()).Seconds()),
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

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"status_code": resp.StatusCode,
			"endpoint":    endpoint,
			"response":    string(bodyBytes),
		}).Debug("Auth key request failed")
		return nil, fmt.Errorf("failed to generate auth key: HTTP %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var authKey TailscaleAuthKey
	if err := json.Unmarshal(bodyBytes, &authKey); err != nil {
		return nil, fmt.Errorf("failed to decode auth key response: %v", err)
	}

	log.WithFields(log.Fields{
		"key_id":   authKey.ID,
		"expires":  authKey.Expires,
		"endpoint": endpoint,
		"response": string(bodyBytes),
	}).Debug("Generated new auth key")
	return &authKey, nil
}

func main() {
	cfg := parseFlags()
	setupLogging(cfg.LogLevel)

	// Load environment variables from .env file
	if err := loadEnvConfig(); err != nil {
		log.Debug(err)
	}

	// Get tailnet name from environment
	tailnet := os.Getenv("TS_TAILNET")
	if tailnet == "" {
		log.Fatal("TS_TAILNET environment variable is required")
	}

	// Get OAuth token
	ctx := context.Background()
	client, _ := GetAccessToken(ctx) // TODO: add error handling

	// Test the token with a devices list request
	testEndpoint := fmt.Sprintf("%s/tailnet/%s/devices", tailscaleAPIBase, tailnet)
	req, _ := http.NewRequestWithContext(ctx, "GET", testEndpoint, nil)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to test OAuth token: %v", err)
	}
	resp.Body.Close()
	log.WithField("status", resp.StatusCode).Debug("OAuth token test request completed")

	// Generate auth key
	authKey, err := generateAuthKey(ctx, client, tailnet)
	if err != nil {
		log.Fatalf("Failed to generate auth key: %v", err)
	}

	// separate config dirs to avoide conflicting states
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("failed to get user config directory: %v", err)
	}
	instanceDir := filepath.Join(userConfigDir, "tsrouter", cfg.Hostname)

	// Create and configure the Tailscale node
	s := &tsnet.Server{
		Hostname: cfg.Hostname,
		AuthKey:  authKey.Key,
		Dir:      instanceDir,
	}

	log.Debug("Starting Tailscale node...")
	if err := s.Start(); err != nil {
		log.Fatalf("Failed to start Tailscale node: %v", err)
	}

	// Create the reverse proxy
	targetURL := fmt.Sprintf("http://localhost:%d", cfg.TargetPort)
	target, err := url.Parse(targetURL)
	if err != nil {
		log.Fatalf("Failed to parse target URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Get a listener on the Tailscale network
	ln, err := s.ListenTLS("tcp", ":443")
	if err != nil {
		log.Fatalf("Failed to create Tailscale listener: %v", err)
	}
	log.Infof("Service available at %s.%s -> localhost:%d", cfg.Hostname, tailnet, cfg.TargetPort)
	if err := http.Serve(ln, proxy); err != nil {
		log.Fatalf("Failed to serve proxy: %v", err)
	}
	defer s.Close()
}
