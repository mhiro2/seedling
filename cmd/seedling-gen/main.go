package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRootUsage(stderr)
		_, _ = fmt.Fprintln(stderr, "Error: command is required")
		return 1
	}

	switch args[0] {
	case "--version", "-version", "version":
		_, _ = fmt.Fprintln(stdout, cliVersion())
		return 0
	case "--help", "-h":
		printRootUsage(stderr)
		return 0
	case "help":
		return runHelp(args[1:], stdout, stderr)
	case "sql":
		return runSQLCmd(args[1:], stdout, stderr)
	case "sqlc":
		return runSQLCCmd(args[1:], stdout, stderr)
	case "gorm":
		return runGormCmd(args[1:], stdout, stderr)
	case "ent":
		return runEntCmd(args[1:], stdout, stderr)
	case "atlas":
		return runAtlasCmd(args[1:], stdout, stderr)
	default:
		printRootUsage(stderr)
		_, _ = fmt.Fprintf(stderr, "Error: unknown command %q\n", args[0])
		return 1
	}
}

// atomicWrite writes to a temporary file and renames it to dest on success.
// On failure the temporary file is removed and no partial output remains.
func atomicWrite(dest string, fn func(w io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(dest), ".seedling-gen-*.go")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := fn(f); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func printRootUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  seedling-gen <command> [flags]

Commands:
  sql      Generate blueprints from SQL DDL
  sqlc     Generate blueprints from sqlc schema and generated Go code
  gorm     Generate blueprints from GORM models
  ent      Generate blueprints from ent schemas
  atlas    Generate blueprints from Atlas HCL
  version  Print version and exit

Run 'seedling-gen <command> -h' for command-specific flags.
`)
}

func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRootUsage(stderr)
		return 0
	}

	switch args[0] {
	case "sql":
		return runSQLCmd([]string{"--help"}, stdout, stderr)
	case "sqlc":
		return runSQLCCmd([]string{"--help"}, stdout, stderr)
	case "gorm":
		return runGormCmd([]string{"--help"}, stdout, stderr)
	case "ent":
		return runEntCmd([]string{"--help"}, stdout, stderr)
	case "atlas":
		return runAtlasCmd([]string{"--help"}, stdout, stderr)
	case "version":
		_, _ = fmt.Fprintln(stdout, cliVersion())
		return 0
	default:
		printRootUsage(stderr)
		_, _ = fmt.Fprintf(stderr, "Error: unknown help topic %q\n", args[0])
		return 1
	}
}

func newFlagSet(name string, stderr io.Writer, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage: %s\n\nFlags:\n", usage)
		fs.PrintDefaults()
	}
	return fs
}

func writeGeneratedOutput(stdout, stderr io.Writer, out string, generate func(w io.Writer) error) int {
	if out != "" {
		if err := atomicWrite(out, generate); err != nil {
			_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0
	}

	if err := generate(stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	return 0
}

func requireNoExtraArgs(fs *flag.FlagSet, stderr io.Writer, noun string) bool {
	if fs.NArg() == 0 {
		return true
	}

	fs.Usage()
	_, _ = fmt.Fprintf(stderr, "Error: unexpected %s argument %q\n", noun, fs.Arg(0))
	return false
}

func requireSingleArg(fs *flag.FlagSet, stderr io.Writer, noun string) (string, bool) {
	if fs.NArg() == 1 {
		return fs.Arg(0), true
	}

	fs.Usage()
	if fs.NArg() == 0 {
		_, _ = fmt.Fprintf(stderr, "Error: %s path is required\n", noun)
		return "", false
	}
	_, _ = fmt.Fprintf(stderr, "Error: expected 1 %s path, got %d\n", noun, fs.NArg())
	return "", false
}

func runSQLCmd(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("sql", stderr, "seedling-gen sql [flags] <schema.sql>")
	pkg := fs.String("pkg", "blueprints", "package name for generated code")
	out := fs.String("out", "", "output file path (default: stdout)")
	dialect := fs.String("dialect", "auto", "schema dialect hint for validation (auto, postgres, mysql, sqlite)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	schemaPath, ok := requireSingleArg(fs, stderr, "schema file")
	if !ok {
		return 1
	}

	return writeGeneratedOutput(stdout, stderr, *out, func(w io.Writer) error {
		return runSQL(w, *pkg, *dialect, schemaPath)
	})
}

func runSQLCCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sqlc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pkg := fs.String("pkg", "blueprints", "package name for generated code")
	out := fs.String("out", "", "output file path (default: stdout)")
	dialect := fs.String("dialect", "auto", "schema dialect hint for validation (auto, postgres, mysql, sqlite)")
	configPath := fs.String("config", "", "path to sqlc config file (auto-resolves schema, output, and import path)")
	sqlcDir := fs.String("dir", "", "path to sqlc-generated Go files directory")
	importPath := fs.String("import-path", "", "Go import path for sqlc package")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, `Usage:
  seedling-gen sqlc [flags] --config <sqlc.yaml>
  seedling-gen sqlc [flags] --dir <sqlc-dir> --import-path <go-import-path> <schema.sql>

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *configPath != "" {
		if *sqlcDir != "" || *importPath != "" {
			fs.Usage()
			_, _ = fmt.Fprintln(stderr, "Error: --config cannot be combined with --dir or --import-path")
			return 1
		}
		if !requireNoExtraArgs(fs, stderr, "schema file") {
			return 1
		}
		return writeGeneratedOutput(stdout, stderr, *out, func(w io.Writer) error {
			return runSqlcConfig(w, *pkg, *dialect, *configPath)
		})
	}

	schemaPath, ok := requireSingleArg(fs, stderr, "schema file")
	if !ok {
		return 1
	}
	if *sqlcDir == "" {
		fs.Usage()
		_, _ = fmt.Fprintln(stderr, "Error: --dir is required when --config is not specified")
		return 1
	}
	if *importPath == "" {
		fs.Usage()
		_, _ = fmt.Fprintln(stderr, "Error: --import-path is required when --config is not specified")
		return 1
	}

	return writeGeneratedOutput(stdout, stderr, *out, func(w io.Writer) error {
		return runSQLCManual(w, *pkg, *dialect, schemaPath, *sqlcDir, *importPath)
	})
}

func runGormCmd(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("gorm", stderr, "seedling-gen gorm [flags]")
	pkg := fs.String("pkg", "blueprints", "package name for generated code")
	out := fs.String("out", "", "output file path (default: stdout)")
	dir := fs.String("dir", "", "path to GORM model Go source files directory")
	importPath := fs.String("import-path", "", "Go import path for GORM models package")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if !requireNoExtraArgs(fs, stderr, "positional") {
		return 1
	}
	if *dir == "" {
		fs.Usage()
		_, _ = fmt.Fprintln(stderr, "Error: --dir is required")
		return 1
	}
	if *importPath == "" {
		fs.Usage()
		_, _ = fmt.Fprintln(stderr, "Error: --import-path is required")
		return 1
	}

	return writeGeneratedOutput(stdout, stderr, *out, func(w io.Writer) error {
		return runGorm(w, *pkg, *dir, *importPath)
	})
}

func runEntCmd(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("ent", stderr, "seedling-gen ent [flags]")
	pkg := fs.String("pkg", "blueprints", "package name for generated code")
	out := fs.String("out", "", "output file path (default: stdout)")
	dir := fs.String("dir", "", "path to ent schema directory")
	importPath := fs.String("import-path", "", "Go import path for ent client package")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if !requireNoExtraArgs(fs, stderr, "positional") {
		return 1
	}
	if *dir == "" {
		fs.Usage()
		_, _ = fmt.Fprintln(stderr, "Error: --dir is required")
		return 1
	}
	if *importPath == "" {
		fs.Usage()
		_, _ = fmt.Fprintln(stderr, "Error: --import-path is required")
		return 1
	}

	return writeGeneratedOutput(stdout, stderr, *out, func(w io.Writer) error {
		return runEnt(w, *pkg, *dir, *importPath)
	})
}

func runAtlasCmd(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("atlas", stderr, "seedling-gen atlas [flags] <schema.hcl>")
	pkg := fs.String("pkg", "blueprints", "package name for generated code")
	out := fs.String("out", "", "output file path (default: stdout)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	atlasPath, ok := requireSingleArg(fs, stderr, "Atlas HCL file")
	if !ok {
		return 1
	}

	return writeGeneratedOutput(stdout, stderr, *out, func(w io.Writer) error {
		return runAtlas(w, *pkg, atlasPath)
	})
}

func runSqlcConfig(w io.Writer, pkg, dialect, configPath string) error {
	cfg, err := ParseSqlcConfig(configPath)
	if err != nil {
		return err
	}

	schemaSQL, err := readSchemaFiles(cfg.SchemaFiles)
	if err != nil {
		return err
	}

	tables, err := ParseSchemaWithDialect(schemaSQL, dialect)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("no CREATE TABLE statements found in schema files")
	}

	sqlcInfo, err := ParseSqlcDir(cfg.SqlcDir)
	if err != nil {
		return err
	}

	return GenerateSqlc(w, pkg, cfg.SqlcImportPath, tables, sqlcInfo)
}

func runGorm(w io.Writer, pkg, dir, importPath string) error {
	models, err := ParseGormDir(dir)
	if err != nil {
		return err
	}
	return GenerateGorm(w, pkg, importPath, models)
}

func runAtlas(w io.Writer, pkg, atlasPath string) error {
	//nolint:gosec // CLI reads the atlas file path provided by the caller.
	data, err := os.ReadFile(atlasPath)
	if err != nil {
		return fmt.Errorf("read atlas file: %w", err)
	}

	tables, err := ParseAtlasHCL(string(data))
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("no tables found in %s", atlasPath)
	}

	return Generate(w, pkg, tables)
}

func runEnt(w io.Writer, pkg, dir, importPath string) error {
	schemas, err := ParseEntSchemaDir(dir)
	if err != nil {
		return err
	}
	return GenerateEnt(w, pkg, importPath, schemas)
}

func runSQL(w io.Writer, pkg, dialect, schemaPath string) error {
	//nolint:gosec // The CLI is expected to read the schema path explicitly provided by the caller.
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema file: %w", err)
	}

	tables, err := ParseSchemaWithDialect(string(data), dialect)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("no CREATE TABLE statements found in %s", schemaPath)
	}

	return Generate(w, pkg, tables)
}

func runSQLCManual(w io.Writer, pkg, dialect, schemaPath, sqlcDir, importPath string) error {
	//nolint:gosec // The CLI is expected to read the schema path explicitly provided by the caller.
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema file: %w", err)
	}

	tables, err := ParseSchemaWithDialect(string(data), dialect)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("no CREATE TABLE statements found in %s", schemaPath)
	}

	sqlcInfo, err := ParseSqlcDir(sqlcDir)
	if err != nil {
		return err
	}

	return GenerateSqlc(w, pkg, importPath, tables, sqlcInfo)
}

// readSchemaFiles reads and concatenates multiple schema files.
func readSchemaFiles(paths []string) (string, error) {
	var sb strings.Builder
	for _, p := range paths {
		//nolint:gosec // CLI reads schema paths resolved from config.
		data, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("read schema file %s: %w", p, err)
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.Write(data)
	}
	return sb.String(), nil
}
