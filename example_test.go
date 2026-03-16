package seedling_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/faker"
)

type ExCompany struct {
	ID   int
	Name string
}

type ExUser struct {
	ID        int
	CompanyID int
	Name      string
}

func setupExampleBlueprints() {
	seedling.ResetRegistry()

	seedling.MustRegister(seedling.Blueprint[ExCompany]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() ExCompany {
			return ExCompany{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v ExCompany) (ExCompany, error) {
			v.ID = 1
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[ExUser]{
		Name:    "user",
		Table:   "users",
		PKField: "ID",
		Defaults: func() ExUser {
			return ExUser{Name: "test-user"}
		},
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Traits: map[string][]seedling.Option{
			"named": {seedling.Set("Name", "trait-user")},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v ExUser) (ExUser, error) {
			v.ID = 2
			return v, nil
		},
	})
}

func ExampleInsertOne() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil).Root()
	fmt.Printf("User: %s, CompanyID: %d\n", user.Name, user.CompanyID)
	// Output: User: test-user, CompanyID: 1
}

func ExampleInsertMany() {
	setupExampleBlueprints()

	t := &testing.T{}
	companies := seedling.InsertMany[ExCompany](t, nil, 3,
		seedling.Seq("Name", func(i int) string {
			return fmt.Sprintf("company-%d", i)
		}),
	)
	fmt.Printf("%s, %s, %s\n", companies[0].Name, companies[1].Name, companies[2].Name)
	// Output: company-0, company-1, company-2
}

func ExampleBuild() {
	setupExampleBlueprints()

	t := &testing.T{}
	plan := seedling.Build[ExUser](t)
	fmt.Println(plan.DebugString())
	// Output:
	// user
	// └─ company
}

func ExampleSet() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil,
		seedling.Set("Name", "custom-name"),
	).Root()
	fmt.Println(user.Name)
	// Output: custom-name
}

func ExampleRef() {
	setupExampleBlueprints()

	t := &testing.T{}
	plan := seedling.Build[ExUser](t,
		seedling.Ref("company", seedling.Set("Name", "custom-company")),
	)
	result := plan.Insert(t, nil)
	company, ok, err := seedling.NodeAs[ExCompany](result, "company")
	if err != nil || !ok {
		return
	}
	fmt.Println(company.Name)
	// Output: custom-company
}

func ExampleUse() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil,
		seedling.Use("company", ExCompany{ID: 42, Name: "existing-company"}),
	).Root()
	fmt.Println(user.CompanyID)
	// Output: 42
}

func ExampleWith() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil,
		seedling.With(func(u *ExUser) {
			u.Name = "modified-user"
		}),
	).Root()
	fmt.Println(user.Name)
	// Output: modified-user
}

func ExampleBlueprintTrait() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil, seedling.BlueprintTrait("named")).Root()
	fmt.Println(user.Name)
	// Output: trait-user
}

func ExampleFor() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.For[ExUser]().
		Set("Name", "builder-user").
		Insert(t, nil).Root()
	fmt.Println(user.Name)
	// Output: builder-user
}

func ExampleOnly() {
	setupExampleBlueprints()

	t := &testing.T{}
	// Only("company") inserts the root user and its company relation,
	// skipping any other relations that might exist.
	user := seedling.InsertOne[ExUser](t, nil,
		seedling.Only("company"),
	).Root()
	fmt.Printf("User: %s, CompanyID: %d\n", user.Name, user.CompanyID)
	// Output: User: test-user, CompanyID: 1
}

func ExampleOnly_rootOnly() {
	setupExampleBlueprints()

	t := &testing.T{}
	// Only() with no arguments inserts only the root record.
	plan := seedling.Build[ExUser](t, seedling.Only())
	// The plan still shows the full graph:
	fmt.Println(plan.DebugString())
	// Output:
	// user
	// └─ company
}

func ExampleGenerate() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil,
		seedling.WithSeed(42),
		seedling.Generate(func(r *rand.Rand, u *ExUser) {
			f := faker.New(r)
			u.Name = f.Name()
		}),
	).Root()
	fmt.Println(user.Name)
	// Output: Amanda Sanders
}
