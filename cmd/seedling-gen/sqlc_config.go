package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

// SqlcConfig holds resolved configuration from a sqlc.yaml file.
type SqlcConfig struct {
	SchemaFiles    []string // resolved schema file paths
	SqlcDir        string   // resolved sqlc output directory
	SqlcPkg        string   // Go package name
	SqlcImportPath string   // resolved Go import path
}

// sqlcConfigFile mirrors the subset of sqlc.yaml (v1 and v2) that seedling-gen needs.
// Unknown fields are ignored, but type mismatches on known fields are reported as errors.
type sqlcConfigFile struct {
	Version  sqlcVersion     `yaml:"version"`
	SQL      []sqlcV2SQL     `yaml:"sql"`
	Packages []sqlcV1Package `yaml:"packages"`
}

type sqlcV2SQL struct {
	Schema sqlcPaths `yaml:"schema"`
	Gen    struct {
		Go *sqlcGoGen `yaml:"go"`
	} `yaml:"gen"`
}

type sqlcGoGen struct {
	Package string `yaml:"package"`
	Out     string `yaml:"out"`
}

type sqlcV1Package struct {
	Name   string    `yaml:"name"`
	Path   string    `yaml:"path"`
	Schema sqlcPaths `yaml:"schema"`
}

// sqlcVersion accepts both the quoted ("2") and unquoted (2) spellings sqlc allows.
type sqlcVersion string

func (v *sqlcVersion) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	switch val := raw.(type) {
	case string:
		*v = sqlcVersion(val)
	case int, int64, uint64:
		*v = sqlcVersion(fmt.Sprint(val))
	default:
		return fmt.Errorf("version must be a string or integer, got %T", raw)
	}
	return nil
}

// sqlcPaths accepts a single path or a list of paths, as sqlc does for `schema`.
type sqlcPaths []string

func (p *sqlcPaths) UnmarshalYAML(unmarshal func(any) error) error {
	var single string
	if err := unmarshal(&single); err == nil {
		*p = sqlcPaths{single}
		return nil
	}
	var list []string
	if err := unmarshal(&list); err != nil {
		return fmt.Errorf("schema must be a string or a list of strings: %w", err)
	}
	*p = sqlcPaths(list)
	return nil
}

// ParseSqlcConfig parses a sqlc.yaml (v1 or v2) and resolves schema, output, and import paths.
func ParseSqlcConfig(configPath string) (*SqlcConfig, error) {
	data, err := os.ReadFile(configPath) //nolint:gosec // CLI reads the path provided by the caller.
	if err != nil {
		return nil, fmt.Errorf("read sqlc config: %w", err)
	}

	var file sqlcConfigFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse sqlc config: %w", err)
	}

	var (
		schema  []string
		outDir  string
		pkgName string
		section string
	)
	switch file.Version {
	case "2":
		entry, err := selectSqlcV2Entry(file.SQL)
		if err != nil {
			return nil, err
		}
		schema, outDir, pkgName, section = entry.Schema, entry.Gen.Go.Out, entry.Gen.Go.Package, "sql"
	case "1":
		entry, err := selectSqlcV1Package(file.Packages)
		if err != nil {
			return nil, err
		}
		schema, outDir, pkgName, section = entry.Schema, entry.Path, entry.Name, "packages"
	case "":
		return nil, errors.New("parse sqlc config: version is required")
	default:
		return nil, fmt.Errorf("parse sqlc config: unsupported version %q (expected \"1\" or \"2\")", string(file.Version))
	}

	if outDir == "" {
		return nil, fmt.Errorf("parse sqlc config: %s entry has no Go output directory", section)
	}
	if len(schema) == 0 {
		return nil, fmt.Errorf("parse sqlc config: %s entry has no schema path", section)
	}

	configDir := filepath.Dir(configPath)
	schemaFiles, err := resolveSchemaFiles(schema, configDir)
	if err != nil {
		return nil, err
	}

	cfg := &SqlcConfig{
		SchemaFiles: schemaFiles,
		SqlcDir:     filepath.Join(configDir, outDir),
		SqlcPkg:     pkgName,
	}

	importPath, err := resolveGoImportPath(configDir, cfg.SqlcDir)
	if err != nil {
		return nil, err
	}
	cfg.SqlcImportPath = importPath

	return cfg, nil
}

// selectSqlcV2Entry picks the single `sql` entry that generates Go code.
// Multiple Go-generating entries are ambiguous and reported as an error.
func selectSqlcV2Entry(entries []sqlcV2SQL) (*sqlcV2SQL, error) {
	if len(entries) == 0 {
		return nil, errors.New("parse sqlc config: no sql entries found")
	}
	var goEntries []*sqlcV2SQL
	for i := range entries {
		if entries[i].Gen.Go != nil {
			goEntries = append(goEntries, &entries[i])
		}
	}
	switch len(goEntries) {
	case 0:
		return nil, errors.New("parse sqlc config: no sql entry has a gen.go block; only Go code generation is supported")
	case 1:
		return goEntries[0], nil
	default:
		outs := make([]string, 0, len(goEntries))
		for _, e := range goEntries {
			outs = append(outs, fmt.Sprintf("%s (out: %s)", e.Gen.Go.Package, e.Gen.Go.Out))
		}
		return nil, fmt.Errorf("parse sqlc config: %d sql entries generate Go code [%s]; split them into separate config files and run seedling-gen once per file",
			len(goEntries), strings.Join(outs, ", "))
	}
}

// selectSqlcV1Package picks the single `packages` entry; multiple entries are ambiguous.
func selectSqlcV1Package(pkgs []sqlcV1Package) (*sqlcV1Package, error) {
	switch len(pkgs) {
	case 0:
		return nil, errors.New("parse sqlc config: no packages entries found")
	case 1:
		return &pkgs[0], nil
	default:
		names := make([]string, 0, len(pkgs))
		for _, p := range pkgs {
			names = append(names, fmt.Sprintf("%s (path: %s)", p.Name, p.Path))
		}
		return nil, fmt.Errorf("parse sqlc config: %d packages entries found [%s]; split them into separate config files and run seedling-gen once per file",
			len(pkgs), strings.Join(names, ", "))
	}
}

// resolveSchemaFiles resolves schema paths relative to configDir the way sqlc
// does: a file path is used as written, while glob patterns and directories
// are expanded and then filtered to non-hidden *.sql files that are not
// *.down.sql migrations, in lexical order.
func resolveSchemaFiles(paths []string, configDir string) ([]string, error) {
	files := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			return nil, errors.New("parse sqlc config: schema path must not be empty")
		}
		full := filepath.Join(configDir, p)
		if strings.ContainsAny(p, "*?[") {
			matches, err := filepath.Glob(full)
			if err != nil {
				return nil, fmt.Errorf("parse sqlc config: schema pattern %s: %w", p, err)
			}
			expanded, err := expandSchemaMatches(matches)
			if err != nil {
				return nil, err
			}
			if len(expanded) == 0 {
				return nil, fmt.Errorf("parse sqlc config: schema pattern %s matched no schema files", p)
			}
			files = append(files, expanded...)
			continue
		}
		info, err := os.Stat(full)
		if err != nil {
			return nil, fmt.Errorf("parse sqlc config: schema path: %w", err)
		}
		if !info.IsDir() {
			files = append(files, full)
			continue
		}
		expanded, err := listSchemaDir(full)
		if err != nil {
			return nil, err
		}
		if len(expanded) == 0 {
			return nil, fmt.Errorf("parse sqlc config: schema directory %s contains no schema files", full)
		}
		files = append(files, expanded...)
	}
	return files, nil
}

// expandSchemaMatches turns glob matches into schema files: directories are
// listed and plain files are kept only when they pass isSchemaFile.
func expandSchemaMatches(matches []string) ([]string, error) {
	slices.Sort(matches)
	files := make([]string, 0, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			return nil, fmt.Errorf("parse sqlc config: schema path: %w", err)
		}
		if info.IsDir() {
			listed, err := listSchemaDir(m)
			if err != nil {
				return nil, err
			}
			files = append(files, listed...)
			continue
		}
		if isSchemaFile(filepath.Base(m)) {
			files = append(files, m)
		}
	}
	return files, nil
}

// listSchemaDir returns the schema files directly inside dir in lexical order.
func listSchemaDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("parse sqlc config: schema directory %s: %w", dir, err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !isSchemaFile(e.Name()) {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	slices.Sort(files)
	return files, nil
}

// isSchemaFile applies sqlc's directory filter: visible *.sql files that are
// not *.down.sql migrations.
func isSchemaFile(name string) bool {
	return !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".sql") && !strings.HasSuffix(name, ".down.sql")
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
