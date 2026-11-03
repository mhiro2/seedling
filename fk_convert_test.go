package seedling_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/mhiro2/seedling"
)

type convertTenantID int64

type convertTenant struct {
	ID int64
}

type convertProject struct {
	ID       int64
	TenantID sql.NullInt64
}

type convertMember struct {
	ID       int64
	TenantID *convertTenantID
}

type convertToken struct {
	ID [4]byte
}

type tokenRef [4]byte

type convertSession struct {
	ID      int64
	TokenID tokenRef
}

func nullInt64FK() seedling.FKConverter {
	return seedling.ConvertFK(func(id int64) sql.NullInt64 {
		return sql.NullInt64{Int64: id, Valid: true}
	})
}

// registerConvertTenant registers a tenant whose Insert assigns id, which may be
// zero so tests can tell a converted zero key from a NULL one.
func registerConvertTenant(t *testing.T, reg *seedling.Registry, id int64, inserted *int) {
	t.Helper()
	seedling.MustRegisterTo(reg, seedling.Blueprint[convertTenant]{
		Name:    "convert_tenant",
		PKField: "ID",
		Relations: []seedling.Relation{
			{
				Name:         "projects",
				Kind:         seedling.HasMany,
				LocalField:   "TenantID",
				RefBlueprint: "convert_project",
				Optional:     true,
				Convert:      map[string]seedling.FKConverter{"TenantID": nullInt64FK()},
			},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v convertTenant) (convertTenant, error) {
			if inserted != nil {
				*inserted++
			}
			v.ID = id
			return v, nil
		},
	})
}

func registerConvertProject(t *testing.T, reg *seedling.Registry, convert map[string]seedling.FKConverter) {
	t.Helper()
	seedling.MustRegisterTo(reg, seedling.Blueprint[convertProject]{
		Name:    "convert_project",
		PKField: "ID",
		Relations: []seedling.Relation{
			{Name: "tenant", Kind: seedling.BelongsTo, LocalField: "TenantID", RefBlueprint: "convert_tenant", Convert: convert},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v convertProject) (convertProject, error) {
			v.ID = 10
			return v, nil
		},
	})
}

func TestConvertFK_BelongsToNullableKeepsZeroKeyValid(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	registerConvertTenant(t, reg, 0, nil)
	registerConvertProject(t, reg, map[string]seedling.FKConverter{"TenantID": nullInt64FK()})

	// Act
	plan, err := seedling.NewSession[convertProject](reg).BuildE()
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate plan: %v", err)
	}
	result, err := plan.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got, want := result.Root().TenantID, (sql.NullInt64{Int64: 0, Valid: true}); got != want {
		t.Fatalf("TenantID = %+v, want %+v", got, want)
	}
}

func TestConvertFK_NamedIDIntoPointerField(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	registerConvertTenant(t, reg, 7, nil)
	seedling.MustRegisterTo(reg, seedling.Blueprint[convertMember]{
		Name:    "convert_member",
		PKField: "ID",
		Relations: []seedling.Relation{
			{
				Name:         "tenant",
				Kind:         seedling.BelongsTo,
				LocalField:   "TenantID",
				RefBlueprint: "convert_tenant",
				Convert: map[string]seedling.FKConverter{
					"TenantID": seedling.ConvertFK(func(id int64) convertTenantID { return convertTenantID(id) }),
				},
			},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v convertMember) (convertMember, error) {
			return v, nil
		},
	})

	// Act
	result, err := seedling.NewSession[convertMember](reg).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got := result.Root().TenantID; got == nil || *got != 7 {
		t.Fatalf("TenantID = %v, want pointer to 7", got)
	}
}

func TestConvertFK_NamedSourceReachesUnnamedInput(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[convertToken]{
		Name:    "convert_token",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v convertToken) (convertToken, error) {
			v.ID = [4]byte{1, 2, 3, 4}
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[convertSession]{
		Name:    "convert_session",
		PKField: "ID",
		Relations: []seedling.Relation{
			{
				Name:         "token",
				Kind:         seedling.BelongsTo,
				LocalField:   "TokenID",
				RefBlueprint: "convert_token",
				Convert: map[string]seedling.FKConverter{
					"TokenID": seedling.ConvertFK(func(id [4]byte) tokenRef { return tokenRef(id) }),
				},
			},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v convertSession) (convertSession, error) {
			return v, nil
		},
	})

	// Act
	result, err := seedling.NewSession[convertSession](reg).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got, want := result.Root().TokenID, (tokenRef{1, 2, 3, 4}); got != want {
		t.Fatalf("TokenID = %v, want %v", got, want)
	}
}

func TestConvertFK_HasManyConvertsChildFK(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	registerConvertTenant(t, reg, 3, nil)
	seedling.MustRegisterTo(reg, seedling.Blueprint[convertProject]{
		Name:    "convert_project",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v convertProject) (convertProject, error) {
			return v, nil
		},
	})

	// Act
	result, err := seedling.NewSession[convertTenant](reg).InsertOneE(t.Context(), nil,
		seedling.Ref("projects", seedling.Count(2)),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	projects := result.Nodes("convert_project")
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(projects))
	}
	for _, node := range projects {
		if got, want := node.Value().(convertProject).TenantID, (sql.NullInt64{Int64: 3, Valid: true}); got != want {
			t.Fatalf("TenantID = %+v, want %+v", got, want)
		}
	}
}

type convertUser struct {
	ID int64
}

type convertGroup struct {
	ID int64
}

type convertMembership struct {
	UserID  sql.NullInt64
	GroupID convertTenantID
}

func TestConvertFK_ManyToManyConvertsLocalAndRemoteFields(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[convertUser]{
		Name:    "convert_user",
		PKField: "ID",
		Relations: []seedling.Relation{
			{
				Name:             "groups",
				Kind:             seedling.ManyToMany,
				LocalField:       "UserID",
				RemoteField:      "GroupID",
				RefBlueprint:     "convert_group",
				ThroughBlueprint: "convert_membership",
				Count:            1,
				Convert: map[string]seedling.FKConverter{
					"UserID":  nullInt64FK(),
					"GroupID": seedling.ConvertFK(func(id int64) convertTenantID { return convertTenantID(id) }),
				},
			},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v convertUser) (convertUser, error) {
			v.ID = 1
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[convertGroup]{
		Name:    "convert_group",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v convertGroup) (convertGroup, error) {
			v.ID = 2
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[convertMembership]{
		Name: "convert_membership",
		Insert: func(_ context.Context, _ seedling.DBTX, v convertMembership) (convertMembership, error) {
			return v, nil
		},
	})

	// Act
	result, err := seedling.NewSession[convertUser](reg).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	node, ok := result.Node("convert_membership")
	if !ok {
		t.Fatal("membership not inserted")
	}
	got := node.Value().(convertMembership)
	if want := (convertMembership{UserID: sql.NullInt64{Int64: 1, Valid: true}, GroupID: 2}); got != want {
		t.Fatalf("membership = %+v, want %+v", got, want)
	}
}

func TestConvertFK_DryRunShowsConversion(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	registerConvertTenant(t, reg, 1, nil)
	registerConvertProject(t, reg, map[string]seedling.FKConverter{"TenantID": nullInt64FK()})

	// Act
	plan, err := seedling.NewSession[convertProject](reg).BuildE()
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got, want := plan.DryRunString(), "SET TenantID ← sql.NullInt64(convert_tenant.ID)"; !strings.Contains(got, want) {
		t.Fatalf("dry run = %q, want it to contain %q", got, want)
	}
}

func TestConvertFK_WithoutConverterFailsBeforeInsert(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	inserted := 0
	registerConvertTenant(t, reg, 1, &inserted)
	registerConvertProject(t, reg, nil)

	// Act
	plan, err := seedling.NewSession[convertProject](reg).BuildE()
	if err != nil {
		t.Fatal(err)
	}
	validateErr := plan.Validate()
	_, insertErr := plan.InsertE(t.Context(), nil)

	// Assert
	if validateErr == nil || !strings.Contains(validateErr.Error(), "Relation.Convert") {
		t.Fatalf("Validate error = %v, want a hint about Relation.Convert", validateErr)
	}
	if !errors.Is(insertErr, seedling.ErrTypeMismatch) {
		t.Fatalf("InsertE error = %v, want ErrTypeMismatch", insertErr)
	}
	if inserted != 0 {
		t.Fatalf("inserted %d tenants before failing, want 0", inserted)
	}
}

func TestConvertFK_ConverterTypeMismatchFailsBeforeInsert(t *testing.T) {
	tests := []struct {
		name      string
		converter seedling.FKConverter
		want      string
	}{
		{
			name:      "input",
			converter: seedling.ConvertFK(func(id string) sql.NullInt64 { return sql.NullInt64{} }),
			want:      "converter input",
		},
		{
			name:      "output",
			converter: seedling.ConvertFK(func(id int64) sql.NullInt32 { return sql.NullInt32{} }),
			want:      "converter output",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			reg := seedling.NewRegistry()
			inserted := 0
			registerConvertTenant(t, reg, 1, &inserted)
			registerConvertProject(t, reg, map[string]seedling.FKConverter{"TenantID": tt.converter})

			// Act
			plan, err := seedling.NewSession[convertProject](reg).BuildE()
			if err != nil {
				t.Fatal(err)
			}
			validateErr := plan.Validate()
			_, insertErr := plan.InsertE(t.Context(), nil)

			// Assert
			if validateErr == nil {
				t.Fatal("Validate succeeded, want a type mismatch")
			}
			if !errors.Is(insertErr, seedling.ErrTypeMismatch) || !strings.Contains(insertErr.Error(), tt.want) {
				t.Fatalf("InsertE error = %v, want ErrTypeMismatch mentioning %q", insertErr, tt.want)
			}
			if inserted != 0 {
				t.Fatalf("inserted %d tenants before failing, want 0", inserted)
			}
		})
	}
}

func TestConvertFK_RegistrationRejectsInvalidConverters(t *testing.T) {
	tests := []struct {
		name     string
		relation seedling.Relation
		want     string
	}{
		{
			name: "unknown field",
			relation: seedling.Relation{
				Name: "tenant", Kind: seedling.BelongsTo, LocalField: "TenantID", RefBlueprint: "convert_tenant",
				Convert: map[string]seedling.FKConverter{"TenantId": nullInt64FK()},
			},
			want: `converter key "TenantId" names no FK field`,
		},
		{
			name: "zero converter",
			relation: seedling.Relation{
				Name: "tenant", Kind: seedling.BelongsTo, LocalField: "TenantID", RefBlueprint: "convert_tenant",
				Convert: map[string]seedling.FKConverter{"TenantID": {}},
			},
			want: `converter for FK field "TenantID" is not set`,
		},
		{
			name: "nil function",
			relation: seedling.Relation{
				Name: "tenant", Kind: seedling.BelongsTo, LocalField: "TenantID", RefBlueprint: "convert_tenant",
				Convert: map[string]seedling.FKConverter{"TenantID": seedling.ConvertFK[int64, sql.NullInt64](nil)},
			},
			want: `converter for FK field "TenantID" is not set`,
		},
		{
			name: "remote field outside many-to-many",
			relation: seedling.Relation{
				Name: "tenant", Kind: seedling.BelongsTo, LocalField: "TenantID", RemoteField: "OtherID", RefBlueprint: "convert_tenant",
				Convert: map[string]seedling.FKConverter{"OtherID": nullInt64FK()},
			},
			want: `converter key "OtherID" names no FK field`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := seedling.RegisterTo(seedling.NewRegistry(), seedling.Blueprint[convertProject]{
				Name:      "convert_project",
				PKField:   "ID",
				Relations: []seedling.Relation{tt.relation},
				Insert: func(_ context.Context, _ seedling.DBTX, v convertProject) (convertProject, error) {
					return v, nil
				},
			})

			// Assert
			if !errors.Is(err, seedling.ErrInvalidOption) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RegisterTo error = %v, want ErrInvalidOption containing %q", err, tt.want)
			}
		})
	}
}
