package models

type Config struct {
	TargetPort        int
	Hostname          string
	LogLevel          string
	AuthKeyExpiryDays int // Validity duration for the Tailscale auth key in days
}