package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActualSQLCFixtureLifecycle(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "sqlc_actual")
	schema, err := os.ReadFile(filepath.Join(fixtureDir, "schema.sql"))
	if err != nil {
		t.Fatalf("read schema fixture: %v", err)
	}
	tables, err := ParseSchemaWithDialect(string(schema), "sqlite")
	if err != nil {
		t.Fatalf("ParseSchemaWithDialect: %v", err)
	}
	info, err := ParseSqlcDir(filepath.Join(fixtureDir, "generated"))
	if err != nil {
		t.Fatalf("ParseSqlcDir: %v", err)
	}

	var generated strings.Builder
	if err := GenerateSqlc(
		&generated,
		"compile",
		"github.com/mhiro2/seedling/cmd/seedling-gen/testdata/sqlc_actual/generated",
		tables,
		info,
	); err != nil {
		t.Fatalf("GenerateSqlc: %v", err)
	}
	ensureCompilesAndRuns(t, "actual_sqlc", generated.String(), actualSQLCLifecycleTest)
}

const actualSQLCLifecycleTest = `package compile

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/mhiro2/seedling"
	actualsqlc "github.com/mhiro2/seedling/cmd/seedling-gen/testdata/sqlc_actual/generated"
)

type actualFixtureState struct {
	inserts                          []string
	deletes                          []string
	insertedCompanyCode              string
	insertedCompanySpotifyURL        string
	insertedMembershipOrganizationID int64
	insertedMembershipUserID         int64
	deletedUserID                    int64
	deletedCompanyID                 int64
	deletedLabelID                   int64
	deletedOrganizationID            int64
	deletedMembershipUserID          int64
}

type actualFixtureDriver struct {
	state *actualFixtureState
}

func (d actualFixtureDriver) Open(string) (driver.Conn, error) {
	return &actualFixtureConn{state: d.state}, nil
}

type actualFixtureConn struct {
	state *actualFixtureState
}

func (*actualFixtureConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*actualFixtureConn) Close() error                        { return nil }
func (*actualFixtureConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c *actualFixtureConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "InsertCompany"):
		c.state.inserts = append(c.state.inserts, "company")
		c.state.insertedCompanyCode = args[0].Value.(string)
		c.state.insertedCompanySpotifyURL = args[1].Value.(string)
		return newActualFixtureRows(
			[]string{"id", "code", "spotify_url"},
			[]driver.Value{int64(11), args[0].Value, args[1].Value},
		), nil
	case strings.Contains(query, "InsertUser"):
		c.state.inserts = append(c.state.inserts, "user")
		return newActualFixtureRows(
			[]string{"id", "display_name", "company_code"},
			[]driver.Value{int64(101), args[0].Value, args[1].Value},
		), nil
	case strings.Contains(query, "InsertLabel"):
		c.state.inserts = append(c.state.inserts, "label")
		return newActualFixtureRows(
			[]string{"id", "name"},
			[]driver.Value{int64(202), args[0].Value},
		), nil
	case strings.Contains(query, "InsertMembership"):
		c.state.inserts = append(c.state.inserts, "membership")
		c.state.insertedMembershipOrganizationID = args[0].Value.(int64)
		c.state.insertedMembershipUserID = args[1].Value.(int64)
		return newActualFixtureRows(
			[]string{"organization_id", "user_id"},
			[]driver.Value{args[0].Value, args[1].Value},
		), nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

func (c *actualFixtureConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	switch {
	case strings.Contains(query, "DeleteCompany"):
		c.state.deletes = append(c.state.deletes, "company")
		c.state.deletedCompanyID = args[0].Value.(int64)
	case strings.Contains(query, "DeleteUser"):
		c.state.deletes = append(c.state.deletes, "user")
		c.state.deletedUserID = args[0].Value.(int64)
	case strings.Contains(query, "DeleteLabel"):
		c.state.deletes = append(c.state.deletes, "label")
		c.state.deletedLabelID = args[0].Value.(int64)
	case strings.Contains(query, "DeleteMembership"):
		c.state.deletes = append(c.state.deletes, "membership")
		c.state.deletedOrganizationID = args[0].Value.(int64)
		c.state.deletedMembershipUserID = args[1].Value.(int64)
	default:
		return nil, fmt.Errorf("unexpected exec: %s", query)
	}
	return driver.RowsAffected(1), nil
}

type actualFixtureRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func newActualFixtureRows(columns []string, values []driver.Value) *actualFixtureRows {
	return &actualFixtureRows{columns: columns, values: values}
}

func (r *actualFixtureRows) Columns() []string { return r.columns }
func (*actualFixtureRows) Close() error         { return nil }

func (r *actualFixtureRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}

func TestGeneratedActualSQLCBlueprintLifecycle(t *testing.T) {
	state := &actualFixtureState{}
	driverName := fmt.Sprintf("seedling-actual-sqlc-%p", state)
	sql.Register(driverName, actualFixtureDriver{state: state})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	reg := NewRegistry()
	result, err := seedling.NewSession[actualsqlc.AccountRecord](reg).InsertOneE(t.Context(), db)
	if err != nil {
		t.Fatalf("insert generated sqlc blueprint: %v", err)
	}
	root := result.Root()
	companyNode, ok := result.Node("company")
	if !ok {
		t.Fatal("company node not found")
	}
	company := companyNode.Value().(actualsqlc.Company)
	if root.ID != 101 || root.CompanyCode != company.Code {
		t.Fatalf("inserted user = %+v, company = %+v", root, company)
	}
	if company.Code != "company-code" || company.SpotifyURL != "company-spotify_u_r_l" {
		t.Fatalf("company string fields were cross-wired: %+v", company)
	}
	if state.insertedCompanyCode != company.Code || state.insertedCompanySpotifyURL != company.SpotifyURL {
		t.Fatalf(
			"inserted company arguments = %q/%q, want %q/%q",
			state.insertedCompanyCode,
			state.insertedCompanySpotifyURL,
			company.Code,
			company.SpotifyURL,
		)
	}
	if got, want := strings.Join(state.inserts, ","), "company,user"; got != want {
		t.Fatalf("insert order = %q, want %q", got, want)
	}
	if err := result.CleanupE(t.Context(), db); err != nil {
		t.Fatalf("cleanup generated sqlc blueprint: %v", err)
	}
	if got, want := strings.Join(state.deletes, ","), "user,company"; got != want {
		t.Fatalf("delete order = %q, want %q", got, want)
	}
	if state.deletedUserID != root.ID || state.deletedCompanyID != company.ID {
		t.Fatalf("deleted user/company = %d/%d, want %d/%d", state.deletedUserID, state.deletedCompanyID, root.ID, company.ID)
	}

	labelResult, err := seedling.NewSession[actualsqlc.Label](reg).InsertOneE(t.Context(), db)
	if err != nil {
		t.Fatalf("insert scalar sqlc blueprint: %v", err)
	}
	label := labelResult.Root()
	if label.ID != 202 || label.Name == "" {
		t.Fatalf("inserted scalar label = %+v", label)
	}
	if err := labelResult.CleanupE(t.Context(), db); err != nil {
		t.Fatalf("cleanup scalar sqlc blueprint: %v", err)
	}
	if state.deletedLabelID != label.ID {
		t.Fatalf("deleted label ID = %d, want %d", state.deletedLabelID, label.ID)
	}

	membershipResult, err := seedling.NewSession[actualsqlc.Membership](reg).InsertOneE(
		t.Context(),
		db,
		seedling.Set("OrganizationID", int64(7)),
		seedling.Set("UserID", int64(9)),
	)
	if err != nil {
		t.Fatalf("insert composite sqlc blueprint: %v", err)
	}
	membership := membershipResult.Root()
	if membership.OrganizationID != 7 || membership.UserID != 9 {
		t.Fatalf("inserted membership = %+v, want organization 7 and user 9", membership)
	}
	if state.insertedMembershipOrganizationID != 7 || state.insertedMembershipUserID != 9 {
		t.Fatalf(
			"inserted membership arguments = %d/%d, want 7/9",
			state.insertedMembershipOrganizationID,
			state.insertedMembershipUserID,
		)
	}
	if got, want := strings.Join(state.inserts, ","), "company,user,label,membership"; got != want {
		t.Fatalf("insert order = %q, want %q", got, want)
	}
	if err := membershipResult.CleanupE(t.Context(), db); err != nil {
		t.Fatalf("cleanup composite sqlc blueprint: %v", err)
	}
	if state.deletedOrganizationID != 7 || state.deletedMembershipUserID != 9 {
		t.Fatalf("deleted membership = %d/%d, want 7/9", state.deletedOrganizationID, state.deletedMembershipUserID)
	}
	if got, want := strings.Join(state.deletes, ","), "user,company,label,membership"; got != want {
		t.Fatalf("delete order = %q, want %q", got, want)
	}
}
`
