//go:build integration

package testutil

import "testing"

func TestShouldSkipDockerError_DetectsKnownDockerFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "docker binary missing",
			err:  stubError("Docker executable file not found in $PATH"),
			want: true,
		},
		{
			name: "docker socket permission denied",
			err:  stubError("docker socket permission denied"),
			want: true,
		},
		{
			name: "non docker error",
			err:  stubError("database ping timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkipDockerError(tt.err); got != tt.want {
				t.Fatalf("shouldSkipDockerError() = %v, want %v", got, tt.want)
			}
		})
	}
}

type stubError string

func (e stubError) Error() string {
	return string(e)
}
