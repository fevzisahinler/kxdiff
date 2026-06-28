package model

import "testing"

func TestParseEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Environment
		wantErr bool
	}{
		{
			name:  "context and namespace",
			input: "ctx-a/ns-x",
			want:  Environment{Context: "ctx-a", Namespace: "ns-x"},
		},
		{
			name:  "bare token is a namespace in the current context",
			input: "staging",
			want:  Environment{Context: "", Namespace: "staging"},
		},
		{
			name:  "trailing slash means a context across all namespaces",
			input: "ctx-a/",
			want:  Environment{Context: "ctx-a", Namespace: ""},
		},
		{
			name:  "leading slash means a namespace in the current context",
			input: "/ns-x",
			want:  Environment{Context: "", Namespace: "ns-x"},
		},
		{
			name:  "surrounding whitespace is trimmed",
			input: "  ctx-a/ns-x  ",
			want:  Environment{Context: "ctx-a", Namespace: "ns-x"},
		},
		{
			name:    "empty input is rejected",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace-only input is rejected",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "too many path segments are rejected",
			input:   "a/b/c",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnvironment(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseEnvironment(%q): expected an error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEnvironment(%q): unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseEnvironment(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}
