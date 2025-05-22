package main

// TODO: General:
//		 - Logging overview (Error handling improved, further review may be needed)
//		 - security, general cleanup, and optimization overview
//		 - support multiple concurrent reverse proxies instead of making the user run multiple instances of the program
//		 - detection (and handling) of the case where the user tries to run the program with the same hostname and target port
//		 - detection and integration to the proxied service - deteced if port is listning, graceful shutdown, etc.

import (
	"context"
	// "encoding/json" // No longer needed here after moving GenerateAuthKey
	"flag"
	"fmt"
	// "io" // No longer needed after removing duplicated generateAuthKey
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/whitehawk2/tsrouter/models"
	"github.com/whitehawk2/tsrouter/tailscaleapi" // New import
	log "github.com/sirupsen/logrus"
	"tailscale.com/tsnet"
)

const (
	tailscaleAuthURL = "https://api.tailscale.com/api/v2/oauth/token"
	// tailscaleAPIBase has been moved to tailscaleapi package
	// authKeyExpiryDays constant is now replaced by a configurable flag
)

func parseFlags() *models.Config {
	cfg := &models.Config{}

	flag.IntVar(&cfg.TargetPort, "target-port", 0, "Local port to forward to")
	flag.StringVar(&cfg.Hostname, "hostname", "", "Desired Tailscale hostname")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "Log level (error, info, debug)")
	flag.IntVar(&cfg.AuthKeyExpiryDays, "auth-key-expiry-days", 14, "Validity duration for the Tailscale auth key in days")
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

// generateAuthKey has been moved to the tailscaleapi package.
// Ensure all calls are made to tailscaleapi.GenerateAuthKey.

func main() {
	cfg := parseFlags()
	setupLogging(cfg.LogLevel)

	// Load environment variables from .env file
	if err := loadEnvConfig(); err != nil {
		log.Warnf("Failed to load .env file: %v. Continuing with system environment variables.", err)
	}

	// Get tailnet name from environment
	tailnet := os.Getenv("TS_TAILNET")
	if tailnet == "" {
		log.Errorf("TS_TAILNET environment variable is required")
		os.Exit(1)
	}

	// Get OAuth token
	ctx := context.Background() // Main context for setup
	client, err := GetAccessToken(ctx)
	if err != nil {
		log.Errorf("Failed to get OAuth token: %v", err)
		os.Exit(1)
	}

	// Test the token with a devices list request
	// tailscaleAPIBase is defined in the tailscaleapi package, so we construct the URL here.
	// Consider moving this test into the tailscaleapi package in the future.
	apiBase := "https://api.tailscale.com/api/v2" // TODO: Get this from a shared constant or config
	testEndpoint := fmt.Sprintf("%s/tailnet/%s/devices", apiBase, tailnet)
	req, err := http.NewRequestWithContext(ctx, "GET", testEndpoint, nil)
	if err != nil {
		log.Errorf("Failed to create request for OAuth token test: %v", err)
		os.Exit(1)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Failed to test OAuth token: %v", err)
		os.Exit(1)
	}
	resp.Body.Close()
	log.WithField("status", resp.StatusCode).Debug("OAuth token test request completed")

	// Calculate expiry for auth key using the configured value
	expiryDuration := time.Duration(cfg.AuthKeyExpiryDays) * 24 * time.Hour
	expirySeconds := int(expiryDuration.Seconds()) // Total lifetime in seconds

	// Generate auth key using the new package
	authKey, err := tailscaleapi.GenerateAuthKey(ctx, client, tailnet, expirySeconds)
	if err != nil {
		log.Errorf("Failed to generate auth key: %v", err)
		os.Exit(1)
	}

	// separate config dirs to avoide conflicting states
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		log.Errorf("Failed to get user config directory: %v", err)
		os.Exit(1)
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
		log.Errorf("Failed to start Tailscale node: %v", err)
		os.Exit(1)
	}

	// Create the reverse proxy
	targetURL := fmt.Sprintf("http://localhost:%d", cfg.TargetPort)
	target, err := url.Parse(targetURL)
	if err != nil {
		log.Errorf("Failed to parse target URL '%s': %v", targetURL, err)
		os.Exit(1)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Get a listener on the Tailscale network
	ln, err := s.ListenTLS("tcp", ":443")
	if err != nil {
		log.Errorf("Failed to create Tailscale listener: %v", err)
		if s != nil { // s should not be nil here, but good practice to check
			if closeErr := s.Close(); closeErr != nil {
				log.Errorf("Failed to close Tailscale node during error handling for ListenTLS: %v", closeErr)
			}
		}
		os.Exit(1)
	}
	log.Infof("Service available at %s.%s -> localhost:%d", cfg.Hostname, tailnet, cfg.TargetPort)

	// Create an http.Server instance
	httpServer := &http.Server{
		Handler: proxy,
		// Consider adding timeouts like ReadTimeout, WriteTimeout, IdleTimeout for robustness
	}

	// Start the HTTP server in a goroutine
	// Set up signal handling for graceful shutdown
	quit := make(chan os.Signal, 1) // quit channel needs to be defined before the goroutine that might use it.

	go func() {
		log.Info("Starting HTTP server on Tailscale listener...")
		// ServeTLS with empty cert/key files because ln is already a TLS listener
		if err := httpServer.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			log.Errorf("HTTP server failed: %v. Initiating shutdown.", err)
			quit <- syscall.SIGINT // Send signal to trigger graceful shutdown
		}
	}()

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	receivedSignal := <-quit // Block until a signal is received

	log.Infof("Received signal: %v. Starting graceful shutdown...", receivedSignal)

	// Create a context with a timeout for the shutdown process

	// Create a context with a timeout for the shutdown process
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	// Shutdown the HTTP server
	if err := httpServer.Shutdown(ctxShutdown); err != nil {
		log.Errorf("HTTP server shutdown error: %v", err)
	} else {
		log.Info("HTTP server gracefully shut down.")
	}

	// Close the Tailscale node
	// s.Close() should be called after the http server using its listener has shut down.
	if err := s.Close(); err != nil {
		log.Errorf("Tailscale node shutdown error: %v", err)
	} else {
		log.Info("Tailscale node gracefully shut down.")
	}

	log.Info("Shutdown complete.")
}
