package models

import "time"

type TailscaleAuthKey struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Created   time.Time `json:"created"`
	Expires   time.Time `json:"expires"`
	Ephemeral bool      `json:"ephemeral"`
}