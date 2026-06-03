package main

import (
	"testing"
	"time"
)

func TestFormatTimeRemaining(t *testing.T) {
	tests := []struct {
		name      string
		remaining time.Duration
		want      string
	}{
		{"zero", 0, "0 min"},
		{"negative", -time.Hour, "0 min"},
		{"days only", 12 * 24 * time.Hour, "12 days"},
		{"one day over 48h", 50 * time.Hour, "2 days"},
		{"just over 48h", 49 * time.Hour, "2 days"},
		{"48h middle tier", 48 * time.Hour, "2 days"},
		{"days and hours", 30 * time.Hour, "1 day 6 hr"},
		{"hours only middle tier", 18 * time.Hour, "18 hr"},
		{"12h middle tier", 12 * time.Hour, "12 hr"},
		{"hours and minutes", 5*time.Hour + 15*time.Minute, "5 hr 15 min"},
		{"minutes only", 45 * time.Minute, "45 min"},
		{"one hour", time.Hour, "1 hr"},
		{"one minute", time.Minute, "1 min"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTimeRemaining(tt.remaining); got != tt.want {
				t.Fatalf("formatTimeRemaining(%v) = %q, want %q", tt.remaining, got, tt.want)
			}
		})
	}
}
