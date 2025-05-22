package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2/clientcredentials"
)

func GetAccessToken(ctx context.Context) (*http.Client, error) {
	err := godotenv.Load()
	if err != nil {
		// It's okay if .env is not found, vars might be in the environment
		log.Println("No .env file found, attempting to use system environment variables.")
	}

	clientID := os.Getenv("TS_CLIENT_ID")
	if clientID == "" {
		return nil, fmt.Errorf("missing required environment variable: TS_CLIENT_ID")
	}
	clientSecret := os.Getenv("TS_CLIENT_SECRET")
	if clientSecret == "" {
		return nil, fmt.Errorf("missing required environment variable: TS_CLIENT_SECRET")
	}
	tailnet := os.Getenv("TS_TAILNET")
	if tailnet == "" {
		// This default is for local testing or specific use cases,
		// but a missing tailnet might be an issue for other functions.
		// For GetAccessToken, it's not directly used, so we allow it to be empty or default.
		log.Println("TS_TAILNET environment variable is not set or empty, defaulting to 'example'. This might not be intended for production.")
		tailnet = "example" // Or consider if this should also be an error.
	}

	oauthConfig := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     "https://api.tailscale.com/api/v2/oauth/token",
	}
	// Using context.Background() here is acceptable as the client itself is context-aware for its requests.
	// The oauthConfig.Client method itself doesn't typically return an error directly.
	// Errors usually occur when the client makes a request.
	client := oauthConfig.Client(context.Background()) // ctx is passed to individual requests made by the client
	return client, nil
}
