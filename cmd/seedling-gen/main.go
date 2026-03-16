package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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

	if fs.NArg() < 1 {
		fs.Usage()
		return 1
	}

	if *sqlcDir != "" && *sqlcPkg == "" {
		_, _ = fmt.Fprintf(stderr, "Error: -sqlc-pkg is required when -sqlc is specified\n")
		return 1
	}

	schemaPath := fs.Arg(0)
	//nolint:gosec // The CLI is expected to read the schema path explicitly provided by the caller.
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error reading schema file: %v\n", err)
		return 1
	}

	tables, err := ParseSchemaWithDialect(string(data), *dialect)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error parsing schema: %v\n", err)
		return 1
	}
	if len(tables) == 0 {
		_, _ = fmt.Fprintf(stderr, "No CREATE TABLE statements found in %s\n", schemaPath)
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
	if *sqlcDir != "" {
		sqlcInfo, err := ParseSqlcDir(*sqlcDir)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "Error parsing sqlc directory: %v\n", err)
			return 1
		}
		genErr = GenerateSqlc(w, *pkg, *sqlcPkg, tables, sqlcInfo)
	} else {
		genErr = Generate(w, *pkg, tables)
	}
	if genErr != nil {
		_, _ = fmt.Fprintf(stderr, "Error generating code: %v\n", genErr)
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
