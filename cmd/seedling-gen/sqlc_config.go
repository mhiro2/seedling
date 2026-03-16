package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SqlcConfig holds resolved configuration from a sqlc.yaml file.
type SqlcConfig struct {
	SchemaFiles    []string // resolved schema file paths
	SqlcDir        string   // resolved sqlc output directory
	SqlcPkg        string   // Go package name
	SqlcImportPath string   // resolved Go import path
}

// ParseSqlcConfig parses a sqlc.yaml (v1 or v2) and resolves schema, output, and import paths.
func ParseSqlcConfig(configPath string) (*SqlcConfig, error) {
	data, err := os.ReadFile(configPath) //nolint:gosec // CLI reads the path provided by the caller.
	if err != nil {
		return nil, fmt.Errorf("read sqlc config: %w", err)
	}

	configDir := filepath.Dir(configPath)
	lines := strings.Split(string(data), "\n")

	ver := detectSqlcConfigVersion(lines)
	var cfg *SqlcConfig
	switch ver {
	case 2:
		cfg = parseSqlcConfigV2(lines, configDir)
	default:
		cfg = parseSqlcConfigV1(lines, configDir)
	}

	if cfg.SqlcDir == "" {
		return nil, fmt.Errorf("parse sqlc config: output directory not found")
	}
	if len(cfg.SchemaFiles) == 0 {
		return nil, fmt.Errorf("parse sqlc config: schema path not found")
	}

	importPath, err := resolveGoImportPath(configDir, cfg.SqlcDir)
	if err != nil {
		return nil, err
	}
	cfg.SqlcImportPath = importPath

	return cfg, nil
}

func detectSqlcConfigVersion(lines []string) int {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if val, ok := strings.CutPrefix(trimmed, "version:"); ok {
			val = strings.TrimSpace(val)
			val = strings.Trim(val, `"'`)
			if val == "2" {
				return 2
			}
			return 1
		}
	}
	return 1
}

// parseSqlcConfigV2 parses v2 format:
//
//	version: "2"
//	sql:
//	  - schema: "schema.sql"
//	    gen:
//	      go:
//	        package: "db"
//	        out: "internal/db"
func parseSqlcConfigV2(lines []string, configDir string) *SqlcConfig {
	cfg := &SqlcConfig{}
	inSQL := false
	inGen := false
	inGo := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "sql:":
			inSQL = true
			inGen = false
			inGo = false
			continue
		case !inSQL:
			continue
		case trimmed == "gen:":
			inGen = true
			inGo = false
			continue
		case trimmed == "go:":
			if inGen {
				inGo = true
			}
			continue
		}

		if inSQL && !inGen && strings.HasPrefix(trimmed, "- schema:") {
			val := extractYAMLValue(trimmed[len("- schema:"):])
			cfg.SchemaFiles = resolveSchemaFiles(val, configDir)
		} else if inSQL && !inGen && strings.HasPrefix(trimmed, "schema:") {
			val := extractYAMLValue(trimmed[len("schema:"):])
			cfg.SchemaFiles = resolveSchemaFiles(val, configDir)
		}

		if inGo {
			if strings.HasPrefix(trimmed, "package:") {
				cfg.SqlcPkg = extractYAMLValue(trimmed[len("package:"):])
			}
			if strings.HasPrefix(trimmed, "out:") {
				outVal := extractYAMLValue(trimmed[len("out:"):])
				cfg.SqlcDir = filepath.Join(configDir, outVal)
			}
		}
	}

	return cfg
}

// parseSqlcConfigV1 parses v1 format:
//
//	version: "1"
//	packages:
//	  - schema: "schema.sql"
//	    name: "db"
//	    path: "internal/db"
func parseSqlcConfigV1(lines []string, configDir string) *SqlcConfig {
	cfg := &SqlcConfig{}
	inPackages := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "packages:" {
			inPackages = true
			continue
		}
		if !inPackages {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "- schema:"):
			val := extractYAMLValue(trimmed[len("- schema:"):])
			cfg.SchemaFiles = resolveSchemaFiles(val, configDir)
		case strings.HasPrefix(trimmed, "schema:"):
			val := extractYAMLValue(trimmed[len("schema:"):])
			cfg.SchemaFiles = resolveSchemaFiles(val, configDir)
		case strings.HasPrefix(trimmed, "name:"):
			cfg.SqlcPkg = extractYAMLValue(trimmed[len("name:"):])
		case strings.HasPrefix(trimmed, "path:"):
			pathVal := extractYAMLValue(trimmed[len("path:"):])
			cfg.SqlcDir = filepath.Join(configDir, pathVal)
		}
	}

	return cfg
}

// extractYAMLValue extracts a simple scalar value from a YAML line fragment.
func extractYAMLValue(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return s
}

// resolveSchemaFiles resolves schema file paths. Handles single files and
// YAML list syntax (e.g., "[a.sql, b.sql]").
func resolveSchemaFiles(val, configDir string) []string {
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		inner := val[1 : len(val)-1]
		parts := strings.Split(inner, ",")
		var files []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			p = strings.Trim(p, `"'`)
			if p != "" {
				files = append(files, filepath.Join(configDir, p))
			}
		}
		return files
	}
	if val != "" {
		return []string{filepath.Join(configDir, val)}
	}
	return nil
}

// resolveGoImportPath reads go.mod from the project root and combines the
// module path with the relative path to dir.
func resolveGoImportPath(baseDir, dir string) (string, error) {
	// Walk up from baseDir to find go.mod.
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve import path: %w", err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve import path: %w", err)
	}

	modulePath, moduleRoot, err := findGoModule(absBase)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(moduleRoot, absDir)
	if err != nil {
		return "", fmt.Errorf("resolve import path: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return modulePath, nil
	}
	return modulePath + "/" + rel, nil
}

// findGoModule walks up from startDir to find go.mod and returns the module path and directory.
func findGoModule(startDir string) (modulePath, moduleDir string, err error) {
	dir := startDir
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if f, e := os.Open(goModPath); e == nil { //nolint:gosec // CLI reads go.mod from project directory.
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "module ") {
					_ = f.Close()
					modulePath = strings.TrimSpace(line[len("module "):])
					return modulePath, dir, nil
				}
			}
			_ = f.Close()
			return "", "", fmt.Errorf("resolve import path: module directive not found in %s", goModPath)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", fmt.Errorf("resolve import path: go.mod not found starting from %s", startDir)
}
