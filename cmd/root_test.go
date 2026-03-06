package cmd

import (
	"strings"
	"testing"
)

func TestIsValidMatchID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "default sentinel", id: "0", want: true},
		{name: "normal id", id: "12345", want: true},
		{name: "max uint32", id: "4294967295", want: true},
		{name: "overflow uint32", id: "4294967296", want: false},
		{name: "negative number", id: "-1", want: false},
		{name: "not a number", id: "abc", want: false},
		{name: "empty", id: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidMatchID(tt.id)
			if got != tt.want {
				t.Fatalf("isValidMatchID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestValidateTickRate(t *testing.T) {
	tests := []struct {
		name       string
		rate       int
		wantErr    bool
		errContain string
	}{
		{name: "default is valid", rate: defaultTickRateMs, wantErr: false},
		{name: "minimum is valid", rate: minTickRateMs, wantErr: false},
		{name: "maximum is valid", rate: maxTickRateMs, wantErr: false},
		{name: "zero rejected", rate: 0, wantErr: true, errContain: "greater than 0ms"},
		{name: "negative rejected", rate: -5, wantErr: true, errContain: "greater than 0ms"},
		{name: "too low rejected", rate: minTickRateMs - 1, wantErr: true, errContain: "too low"},
		{name: "too high rejected", rate: maxTickRateMs + 1, wantErr: true, errContain: "too high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTickRate(tt.rate)
			if tt.wantErr && err == nil {
				t.Fatalf("validateTickRate(%d) expected error", tt.rate)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateTickRate(%d) unexpected error: %v", tt.rate, err)
			}
			if tt.errContain != "" && err != nil && !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("validateTickRate(%d) error %q does not contain %q", tt.rate, err.Error(), tt.errContain)
			}
		})
	}
}
