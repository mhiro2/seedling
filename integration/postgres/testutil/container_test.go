//go:build integration

package testutil

import (
	"os"
	"testing"
)

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

func TestSchemaPath_ResolvesBundledSchema(t *testing.T) {
	path := schemaPath()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("schemaPath() returned unreadable path %q: %v", path, err)
	}
}

type stubError string

func (e stubError) Error() string {
	return string(e)
}
