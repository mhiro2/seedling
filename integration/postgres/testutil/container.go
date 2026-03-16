//go:build integration

package testutil

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func runPostgresContainer(ctx context.Context) (_ *postgres.PostgresContainer, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("postgres container startup panicked: %v", r)
		}
	}()

	container, err := postgres.Run(ctx,
		"postgres:18",
		postgres.WithDatabase("seedling_test"),
		postgres.WithUsername("seedling"),
		postgres.WithPassword("seedling"),
		postgres.WithInitScripts(schemaPath()),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}
	return container, nil
}

func shouldSkipDockerError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "docker") && (strings.Contains(msg, "not found") ||
		strings.Contains(msg, "cannot connect") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "rootless") ||
		strings.Contains(msg, "socket"))
}

func schemaPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("postgres testutil: unable to resolve schema path")
	}

	return filepath.Join(filepath.Dir(filepath.Dir(file)), "testdata", "schema.sql")
}
