package github

import (
	"errors"
	"testing"
)

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"generic error", errors.New("something broke"), false},
		{"HTTP 401", errors.New("HTTP 401: Bad credentials"), true},
		{"Bad credentials", errors.New("Bad credentials"), true},
		{"HTTP 502 server error", errors.New("HTTP 502: Server Error"), false},
		{"HTTP 403 forbidden", errors.New("HTTP 403: Forbidden"), false},
		{"wrapped 401", errors.New("fetching notifications: HTTP 401: Bad credentials (https://api.github.com/notifications)"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuthError(tt.err); got != tt.want {
				t.Errorf("IsAuthError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestHasNextPage(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"empty", "", false},
		{"next only", `<https://api.github.com/notifications?page=2>; rel="next"`, true},
		{"next and last", `<https://api.github.com/notifications?page=2>; rel="next", <https://api.github.com/notifications?page=5>; rel="last"`, true},
		{"prev and last only", `<https://api.github.com/notifications?page=1>; rel="prev", <https://api.github.com/notifications?page=5>; rel="last"`, false},
		{"last only", `<https://api.github.com/notifications?page=5>; rel="last"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasNextPage(tt.header); got != tt.want {
				t.Errorf("hasNextPage(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestIsServerError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"generic error", errors.New("something broke"), false},
		{"HTTP 502", errors.New("HTTP 502: Server Error"), true},
		{"HTTP 503", errors.New("HTTP 503: Service Unavailable"), true},
		{"HTTP 504", errors.New("HTTP 504: Gateway Timeout"), true},
		{"HTTP 401", errors.New("HTTP 401: Bad credentials"), false},
		{"wrapped 502", errors.New("fetching notifications: HTTP 502: Server Error (https://api.github.com/notifications)"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsServerError(tt.err); got != tt.want {
				t.Errorf("IsServerError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
