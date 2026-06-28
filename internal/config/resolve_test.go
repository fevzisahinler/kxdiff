package config

import (
	"strings"
	"testing"
)

func TestResolveContext(t *testing.T) {
	available := []string{"eks-staging", "eks-prod", "gke-dev"}

	tests := []struct {
		name        string
		available   []string
		current     string
		requested   string
		want        string
		wantErr     bool
		errContains []string // substrings the error must include
		errExcludes []string // substrings the error must NOT include
	}{
		{
			name:      "explicit context that exists",
			available: available,
			current:   "eks-staging",
			requested: "eks-prod",
			want:      "eks-prod",
		},
		{
			name:      "empty request falls back to current context",
			available: available,
			current:   "eks-staging",
			requested: "",
			want:      "eks-staging",
		},
		{
			name:        "empty request with no current context",
			available:   available,
			current:     "",
			requested:   "",
			wantErr:     true,
			errContains: []string{"current"},
		},
		{
			name:        "current context is not defined in kubeconfig",
			available:   available,
			current:     "deleted-ctx",
			requested:   "",
			wantErr:     true,
			errContains: []string{"deleted-ctx"},
		},
		{
			name:        "typo suggests the closest context",
			available:   available,
			current:     "eks-staging",
			requested:   "eks-prd",
			wantErr:     true,
			errContains: []string{"eks-prd", "did you mean", "eks-prod"},
		},
		{
			name:        "unrelated name does not suggest but lists contexts",
			available:   available,
			current:     "eks-staging",
			requested:   "zzzzzzzz",
			wantErr:     true,
			errContains: []string{"zzzzzzzz", "eks-prod"},
			errExcludes: []string{"did you mean"},
		},
		{
			name:        "no contexts in kubeconfig",
			available:   []string{},
			current:     "",
			requested:   "eks-prod",
			wantErr:     true,
			errContains: []string{"no contexts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveContext(tt.available, tt.current, tt.requested)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil (result %q)", got)
				}
				msg := err.Error()
				for _, sub := range tt.errContains {
					if !strings.Contains(msg, sub) {
						t.Errorf("error %q should contain %q", msg, sub)
					}
				}
				for _, sub := range tt.errExcludes {
					if strings.Contains(msg, sub) {
						t.Errorf("error %q should not contain %q", msg, sub)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveContext() = %q, want %q", got, tt.want)
			}
		})
	}
}
