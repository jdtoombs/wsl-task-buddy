package main

import (
	"testing"
	"time"
)

func TestParseDurationInput(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		// H:MM format
		{"1:30", 1*time.Hour + 30*time.Minute, false},
		{"0:45", 45 * time.Minute, false},
		{"2:00", 2 * time.Hour, false},

		// XhYm format
		{"1h30m", 1*time.Hour + 30*time.Minute, false},
		{"45m", 45 * time.Minute, false},
		{"2h", 2 * time.Hour, false},

		// plain minutes
		{"30", 30 * time.Minute, false},
		{"90", 90 * time.Minute, false},

		// errors
		{"", 0, true},
		{"0:00", 0, true},
		{"-1:30", 0, true},
		{"1:-30", 0, true},
		{"0m", 0, true},
		{"0h", 0, true},
		{"0", 0, true},
		{"-5", 0, true},
		{"xh30m", 0, true},
		{"1hxm", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		got, err := parseDurationInput(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseDurationInput(%q) expected error, got %v", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDurationInput(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseDurationInput(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
