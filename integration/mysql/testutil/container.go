//go:build integration

package testutil

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	mysqlmodule "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func runMySQLContainer(ctx context.Context) (_ *mysqlmodule.MySQLContainer, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("mysql container startup panicked: %v", r)
		}
	}()

	container, err := mysqlmodule.Run(ctx,
		"mysql:8",
		mysqlmodule.WithDatabase("seedling_test"),
		mysqlmodule.WithUsername("seedling"),
		mysqlmodule.WithPassword("seedling"),
		mysqlmodule.WithScripts(schemaPath()),
	)
	if err != nil {
		return nil, fmt.Errorf("start mysql container: %w", err)
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
		panic("mysql testutil: unable to resolve schema path")
	}

	return filepath.Join(filepath.Dir(filepath.Dir(file)), "testdata", "schema.sql")
}
