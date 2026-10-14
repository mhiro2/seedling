package seedling_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

// insertedUser is the root user and its company from a single-record insert.
type insertedUser struct {
	user    User
	company Company
}

// optionPaths are the insert paths that must resolve options identically.
var optionPaths = []struct {
	name   string
	insert func(ctx context.Context, sess seedling.Session[User], opts []seedling.Option) (insertedUser, error)
}{
	{
		name: "InsertOneE",
		insert: func(ctx context.Context, sess seedling.Session[User], opts []seedling.Option) (insertedUser, error) {
			result, err := sess.InsertOneE(ctx, nil, opts...)
			if err != nil {
				return insertedUser{}, fmt.Errorf("insert user: %w", err)
			}
			return insertedUser{user: result.Root(), company: seedling.MustNodeAs[Company](result, "company")}, nil
		},
	},
	{
		name: "BuildE",
		insert: func(ctx context.Context, sess seedling.Session[User], opts []seedling.Option) (insertedUser, error) {
			plan, err := sess.BuildE(opts...)
			if err != nil {
				return insertedUser{}, fmt.Errorf("insert user: %w", err)
			}
			result, err := plan.InsertE(ctx, nil)
			if err != nil {
				return insertedUser{}, fmt.Errorf("insert user: %w", err)
			}
			return insertedUser{user: result.Root(), company: seedling.MustNodeAs[Company](result, "company")}, nil
		},
	},
	{
		name: "InsertManyE",
		insert: func(ctx context.Context, sess seedling.Session[User], opts []seedling.Option) (insertedUser, error) {
			result, err := sess.InsertManyE(ctx, nil, 1, opts...)
			if err != nil {
				return insertedUser{}, fmt.Errorf("insert user: %w", err)
			}
			return insertedUser{user: result.MustRootAt(0), company: seedling.MustNodeAs[Company](result, "company")}, nil
		},
	},
	{
		name: "Builder.InsertE",
		insert: func(ctx context.Context, sess seedling.Session[User], opts []seedling.Option) (insertedUser, error) {
			result, err := seedling.ForSession(sess).Apply(opts...).InsertE(ctx, nil)
			if err != nil {
				return insertedUser{}, fmt.Errorf("insert user: %w", err)
			}
			return insertedUser{user: result.Root(), company: seedling.MustNodeAs[Company](result, "company")}, nil
		},
	},
	{
		name: "Builder.InsertManyE",
		insert: func(ctx context.Context, sess seedling.Session[User], opts []seedling.Option) (insertedUser, error) {
			result, err := seedling.ForSession(sess).Apply(opts...).InsertManyE(ctx, nil, 1)
			if err != nil {
				return insertedUser{}, fmt.Errorf("insert user: %w", err)
			}
			return insertedUser{user: result.MustRootAt(0), company: seedling.MustNodeAs[Company](result, "company")}, nil
		},
	},
}

// newOptionResolutionSession registers a company and a user that belongs to
// it, each with a trait that sets its Name.
func newOptionResolutionSession(tb testing.TB) seedling.Session[User] {
	tb.Helper()
	reg := seedling.NewRegistry()
	registerCompanyWithTraits(tb, reg, map[string][]seedling.Option{
		"big": {seedling.Set("Name", "trait-company")},
	})
	ids := seedlingtest.NewIDSequence()
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name:     "user",
		Table:    "users",
		PKField:  "ID",
		Defaults: func() User { return User{Name: "default"} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Traits: map[string][]seedling.Option{
			"named":  {seedling.Set("Name", "trait-user")},
			"nested": {seedling.InlineTrait(seedling.BlueprintTrait("named")), seedling.Ref("company", seedling.BlueprintTrait("big"))},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	return seedling.NewSession[User](reg)
}

func TestOptionResolution_PrecedenceIsSharedByEveryInsertPath(t *testing.T) {
	existing := Company{ID: 41, Name: "existing"}
	other := Company{ID: 42, Name: "other"}

	tests := []struct {
		name        string
		opts        []seedling.Option
		wantUser    string
		wantCompany string
		wantID      int // expected CompanyID; zero means an auto-created company
	}{
		{
			name:        "later Set wins",
			opts:        []seedling.Option{seedling.Set("Name", "first"), seedling.Set("Name", "second")},
			wantUser:    "second",
			wantCompany: "default",
		},
		{
			name: "Set overrides Generate written after it",
			opts: []seedling.Option{
				seedling.Set("Name", "set"),
				seedling.Generate(func(_ *rand.Rand, u *User) { u.Name = "generated" }),
			},
			wantUser:    "set",
			wantCompany: "default",
		},
		{
			name: "With overrides Set written after it",
			opts: []seedling.Option{
				seedling.With(func(u *User) { u.Name = "with" }),
				seedling.Set("Name", "set"),
			},
			wantUser:    "with",
			wantCompany: "default",
		},
		{
			name:        "Set after a trait wins",
			opts:        []seedling.Option{seedling.BlueprintTrait("named"), seedling.Set("Name", "explicit")},
			wantUser:    "explicit",
			wantCompany: "default",
		},
		{
			name:        "trait after a Set wins",
			opts:        []seedling.Option{seedling.Set("Name", "explicit"), seedling.BlueprintTrait("named")},
			wantUser:    "trait-user",
			wantCompany: "default",
		},
		{
			name:        "inline trait expands in place",
			opts:        []seedling.Option{seedling.InlineTrait(seedling.Set("Name", "inline")), seedling.Set("Name", "explicit")},
			wantUser:    "explicit",
			wantCompany: "default",
		},
		{
			name:        "nested traits and Ref traits expand in place",
			opts:        []seedling.Option{seedling.BlueprintTrait("nested")},
			wantUser:    "trait-user",
			wantCompany: "trait-company",
		},
		{
			name: "Refs to the same relation merge in written order",
			opts: []seedling.Option{
				seedling.Ref("company", seedling.Set("Name", "first")),
				seedling.Ref("company", seedling.BlueprintTrait("big")),
				seedling.Ref("company", seedling.Set("Name", "last")),
			},
			wantUser:    "default",
			wantCompany: "last",
		},
		{
			name:        "later Use wins",
			opts:        []seedling.Option{seedling.Use("company", other), seedling.Use("company", existing)},
			wantUser:    "default",
			wantCompany: "existing",
			wantID:      existing.ID,
		},
		{
			name:        "Use inside a trait",
			opts:        []seedling.Option{seedling.InlineTrait(seedling.Use("company", existing))},
			wantUser:    "default",
			wantCompany: "existing",
			wantID:      existing.ID,
		},
	}

	for _, path := range optionPaths {
		for _, tt := range tests {
			t.Run(path.name+"/"+tt.name, func(t *testing.T) {
				// Arrange
				sess := newOptionResolutionSession(t)

				// Act
				got, err := path.insert(t.Context(), sess, tt.opts)
				// Assert
				if err != nil {
					t.Fatal(err)
				}
				if got.user.Name != tt.wantUser {
					t.Errorf("user Name = %q, want %q", got.user.Name, tt.wantUser)
				}
				if got.company.Name != tt.wantCompany {
					t.Errorf("company Name = %q, want %q", got.company.Name, tt.wantCompany)
				}
				if got.user.CompanyID != got.company.ID {
					t.Errorf("user CompanyID = %d, want company ID %d", got.user.CompanyID, got.company.ID)
				}
				if tt.wantID != 0 && got.company.ID != tt.wantID {
					t.Errorf("company ID = %d, want %d", got.company.ID, tt.wantID)
				}
			})
		}
	}
}

func TestOptionResolution_ContextIsSharedByEveryInsertPath(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name       string
		ctx        context.Context
		opts       []seedling.Option
		wantCancel bool
	}{
		{
			name: "caller context applies without WithContext",
			ctx:  canceled, wantCancel: true,
		},
		{
			name: "WithContext overrides the caller context",
			ctx:  canceled,
			opts: []seedling.Option{seedling.WithContext(context.Background())},
		},
		{
			name:       "later WithContext wins",
			ctx:        context.Background(),
			opts:       []seedling.Option{seedling.WithContext(context.Background()), seedling.WithContext(canceled)},
			wantCancel: true,
		},
		{
			name:       "WithContext inside a trait applies",
			ctx:        context.Background(),
			opts:       []seedling.Option{seedling.InlineTrait(seedling.WithContext(canceled))},
			wantCancel: true,
		},
		{
			name: "WithContext after a trait overrides it",
			ctx:  context.Background(),
			opts: []seedling.Option{
				seedling.InlineTrait(seedling.WithContext(canceled)),
				seedling.WithContext(context.Background()),
			},
		},
	}

	for _, path := range optionPaths {
		for _, tt := range tests {
			t.Run(path.name+"/"+tt.name, func(t *testing.T) {
				// Arrange
				sess := newOptionResolutionSession(t)

				// Act
				_, err := path.insert(tt.ctx, sess, tt.opts)

				// Assert
				if tt.wantCancel {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("got %v, want %v", err, context.Canceled)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestOptionResolution_PlacementRulesAreSharedByEveryInsertPath(t *testing.T) {
	tests := []struct {
		name string
		opts []seedling.Option
	}{
		{
			name: "WithContext under Ref",
			opts: []seedling.Option{seedling.Ref("company", seedling.WithContext(context.Background()))},
		},
		{
			name: "Only under Ref inside a trait",
			opts: []seedling.Option{seedling.InlineTrait(seedling.Ref("company", seedling.Only()))},
		},
		{
			name: "AfterInsert for another type",
			opts: []seedling.Option{seedling.AfterInsert(func(Company, seedling.DBTX) {})},
		},
		{
			name: "unknown trait",
			opts: []seedling.Option{seedling.Ref("company", seedling.BlueprintTrait("missing"))},
		},
	}

	for _, path := range optionPaths {
		for _, tt := range tests {
			t.Run(path.name+"/"+tt.name, func(t *testing.T) {
				// Arrange
				sess := newOptionResolutionSession(t)

				// Act
				_, err := path.insert(t.Context(), sess, tt.opts)

				// Assert
				if !errors.Is(err, seedling.ErrInvalidOption) {
					t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
				}
			})
		}
	}
}
