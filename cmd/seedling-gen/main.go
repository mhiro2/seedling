package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("seedling-gen", flag.ContinueOnError)
	fs.SetOutput(stderr)

	pkg := fs.String("pkg", "blueprints", "package name for generated code")
	out := fs.String("out", "", "output file path (default: stdout)")
	dialect := fs.String("dialect", "auto", "schema dialect: auto, postgres, mysql, sqlite")
	sqlcDir := fs.String("sqlc", "", "path to sqlc-generated Go files directory")
	sqlcPkg := fs.String("sqlc-pkg", "", "Go import path for sqlc package (required with -sqlc)")
	sqlcConfig := fs.String("sqlc-config", "", "path to sqlc.yaml config file (auto-resolves schema, output, and import path)")
	gormDir := fs.String("gorm", "", "path to GORM model Go source files directory")
	gormPkg := fs.String("gorm-pkg", "", "Go import path for GORM models package (required with -gorm)")
	entDir := fs.String("ent", "", "path to ent schema directory")
	entPkg := fs.String("ent-pkg", "", "Go import path for ent client package (required with -ent)")
	atlasFile := fs.String("atlas", "", "path to Atlas HCL schema file")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage: seedling-gen [flags] <schema.sql>\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *showVersion {
		_, _ = fmt.Fprintln(stdout, cliVersion())
		return 0
	}

	// Mutual exclusivity check for adapter modes.
	adapterCount := countNonEmpty(*sqlcConfig, *gormDir, *entDir, *atlasFile)
	if adapterCount > 1 {
		_, _ = fmt.Fprintf(stderr, "Error: only one adapter flag (-sqlc-config, -gorm, -ent, -atlas) can be specified at a time\n")
		return 1
	}

	w := stdout
	var closeOutput func() error
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "Error creating output file: %v\n", err)
			return 1
		}
		w = f
		closeOutput = f.Close
	}

	var genErr error

	switch {
	case *sqlcConfig != "":
		genErr = runSqlcConfig(w, stderr, *pkg, *dialect, *sqlcConfig)
	case *gormDir != "":
		genErr = runGorm(w, stderr, *pkg, *gormDir, *gormPkg)
	case *entDir != "":
		genErr = runEnt(w, stderr, *pkg, *entDir, *entPkg)
	case *atlasFile != "":
		genErr = runAtlas(w, stderr, *pkg, *atlasFile)
	default:
		genErr = runDefault(w, stderr, fs, *pkg, *dialect, *sqlcDir, *sqlcPkg)
	}

	if genErr != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", genErr)
		if closeOutput != nil {
			_ = closeOutput()
		}
		return 1
	}

	if closeOutput != nil {
		if err := closeOutput(); err != nil {
			_, _ = fmt.Fprintf(stderr, "Error closing output file: %v\n", err)
			return 1
		}
	}

	return 0
}

func countNonEmpty(vals ...string) int {
	n := 0
	for _, v := range vals {
		if v != "" {
			n++
		}
	}
	return n
}

func runSqlcConfig(w, _ io.Writer, pkg, dialect, configPath string) error {
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

func runGorm(w, _ io.Writer, pkg, dir, importPath string) error {
	if importPath == "" {
		return fmt.Errorf("-gorm-pkg is required when -gorm is specified")
	}
	models, err := ParseGormDir(dir)
	if err != nil {
		return err
	}
	return GenerateGorm(w, pkg, importPath, models)
}

func runAtlas(w, _ io.Writer, pkg, atlasPath string) error {
	//nolint:gosec // CLI reads the atlas file path provided by the caller.
	data, err := os.ReadFile(atlasPath)
	if err != nil {
		return fmt.Errorf("read atlas file: %w", err)
	}

	tables := ParseAtlasHCL(string(data))
	if len(tables) == 0 {
		return fmt.Errorf("no tables found in %s", atlasPath)
	}

	return Generate(w, pkg, tables)
}

func runEnt(w, _ io.Writer, pkg, dir, importPath string) error {
	if importPath == "" {
		return fmt.Errorf("-ent-pkg is required when -ent is specified")
	}
	schemas, err := ParseEntSchemaDir(dir)
	if err != nil {
		return err
	}
	return GenerateEnt(w, pkg, importPath, schemas)
}

func runDefault(w, _ io.Writer, fs *flag.FlagSet, pkg, dialect, sqlcDir, sqlcPkg string) error {
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("schema file path is required")
	}

	if sqlcDir != "" && sqlcPkg == "" {
		return fmt.Errorf("-sqlc-pkg is required when -sqlc is specified")
	}

	schemaPath := fs.Arg(0)
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

	if sqlcDir != "" {
		sqlcInfo, err := ParseSqlcDir(sqlcDir)
		if err != nil {
			return err
		}
		return GenerateSqlc(w, pkg, sqlcPkg, tables, sqlcInfo)
	}

	return Generate(w, pkg, tables)
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
