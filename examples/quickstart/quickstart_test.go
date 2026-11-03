package quickstart_test

import (
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/examples/quickstart"
)

func setup(t *testing.T) *seedling.Registry {
	t.Helper()
	quickstart.ResetIDs()
	return quickstart.NewRegistry()
}

func TestQuickStart_InsertOneUser(t *testing.T) {
	// Arrange
	reg := setup(t)

	// Act
	user := seedling.NewSession[quickstart.User](reg).InsertOne(t, nil).Root()

	// Assert
	if user.ID == 0 {
		t.Fatal("expected user ID to be set")
	}
	if user.CompanyID == 0 {
		t.Fatal("expected CompanyID to be set")
	}
	if user.Name != "test-user" {
		t.Fatalf("expected Name = %q, got %q", "test-user", user.Name)
	}
}

func TestQuickStart_ReuseCompanyWithUse(t *testing.T) {
	// Arrange
	reg := setup(t)
	company := seedling.NewSession[quickstart.Company](reg).InsertOne(t, nil,
		seedling.Set("Name", "shared-company"),
	).Root()

	// Act
	user := seedling.NewSession[quickstart.User](reg).InsertOne(t, nil,
		seedling.Set("Name", "alice"),
		seedling.Use("company", company),
	).Root()

	// Assert
	if user.CompanyID != company.ID {
		t.Fatalf("expected CompanyID = %d, got %d", company.ID, user.CompanyID)
	}
	if user.Name != "alice" {
		t.Fatalf("expected Name = %q, got %q", "alice", user.Name)
	}
}

func TestQuickStart_CreateUsersFromCompany(t *testing.T) {
	// Arrange
	reg := setup(t)

	// Act
	result := seedling.NewSession[quickstart.Company](reg).InsertOne(t, nil,
		seedling.Ref("users",
			seedling.Count(3),
			seedling.At(1, seedling.Set("Name", "admin")),
		),
	)

	// Assert
	users, err := seedling.NodesAs[quickstart.User](result, "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}
	admins := 0
	for _, user := range users {
		if user.CompanyID != result.Root().ID {
			t.Fatalf("expected CompanyID = %d, got %d", result.Root().ID, user.CompanyID)
		}
		if user.Name == "admin" {
			admins++
		}
	}
	if admins != 1 {
		t.Fatalf("expected exactly one admin, got %d", admins)
	}
}

func TestQuickStart_TypedFixtures(t *testing.T) {
	// Arrange
	f := quickstart.NewFixtures(setup(t))

	// Act
	user := f.User().
		Name("alice").
		Company(f.Company().Name("acme")).
		Insert(t, nil)

	// Assert
	company, ok, err := seedling.NodeAs[quickstart.Company](user, "company")
	if err != nil || !ok {
		t.Fatalf("expected the company node, ok=%v err=%v", ok, err)
	}
	if company.Name != "acme" || user.Root().Name != "alice" || user.Root().CompanyID != company.ID {
		t.Fatalf("got user %+v and company %+v", user.Root(), company)
	}
}

func TestQuickStart_TypedFixturesChildrenAndPresets(t *testing.T) {
	// Arrange
	f := quickstart.NewFixtures(setup(t))
	admin := func(b quickstart.UserBuilder) quickstart.UserBuilder {
		return b.Name("admin").Email("admin@example.com")
	}
	base := f.User()

	// Act
	result := f.Company().Users(base, base.Apply(admin), base).Insert(t, nil)

	// Assert
	users, err := seedling.NodesAs[quickstart.User](result, "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}
	admins := 0
	for _, user := range users {
		if user.Name == "admin" && user.Email == "admin@example.com" {
			admins++
		}
	}
	if admins != 1 {
		t.Fatalf("expected one admin, got %d", admins)
	}
}

func TestQuickStart_TypedFixturesInScenario(t *testing.T) {
	// Arrange
	reg := setup(t)
	f := quickstart.NewFixtures(reg)
	s := seedling.NewScenario(reg)
	company := f.Company().Name("acme").Add(s)
	alice := f.User().Name("alice").LinkCompany(company).Add(s)
	bob := f.User().Name("bob").LinkCompany(company).Add(s)

	// Act
	result := s.Insert(t, nil)

	// Assert
	companyID := company.Get(result).ID
	if alice.Get(result).CompanyID != companyID || bob.Get(result).CompanyID != companyID {
		t.Fatalf("expected both users in company %d", companyID)
	}
	if alice.Get(result).Name != "alice" || bob.Get(result).Name != "bob" {
		t.Fatal("expected users to be read back by role")
	}
}

func TestQuickStart_TypedFixturesUseExisting(t *testing.T) {
	// Arrange
	f := quickstart.NewFixtures(setup(t))
	company := f.Company().Name("existing").Insert(t, nil).Root()

	// Act
	user := f.User().UseCompany(company).With(func(u *quickstart.User) {
		u.Email = "custom@example.com"
	}).Insert(t, nil).Root()

	// Assert
	if user.CompanyID != company.ID || user.Email != "custom@example.com" {
		t.Fatalf("got user %+v", user)
	}
}
