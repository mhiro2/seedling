package basic_test

import (
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/examples/basic"
)

func setup(t *testing.T) *seedling.Registry {
	t.Helper()
	reg := seedling.NewRegistry()
	basic.RegisterBlueprints(reg)
	return reg
}

func TestInsertOne_Company(t *testing.T) {
	reg := setup(t)

	company := seedling.NewSession[basic.Company](reg).InsertOne(t, nil)

	if company.Root().ID == 0 {
		t.Fatal("expected company ID to be set")
	}
	if company.Root().Name != "test-company" {
		t.Fatalf("expected Name = %q, got %q", "test-company", company.Root().Name)
	}
}

func TestInsertOne_User(t *testing.T) {
	reg := setup(t)

	// InsertOne[User] automatically creates a parent Company.
	user := seedling.NewSession[basic.User](reg).InsertOne(t, nil)

	if user.Root().ID == 0 {
		t.Fatal("expected user ID to be set")
	}
	if user.Root().CompanyID == 0 {
		t.Fatal("expected CompanyID to be auto-populated")
	}
	if user.Root().Name != "test-user" {
		t.Fatalf("expected Name = %q, got %q", "test-user", user.Root().Name)
	}
}

func TestInsertOne_UserWithSet(t *testing.T) {
	reg := setup(t)

	user := seedling.NewSession[basic.User](reg).InsertOne(t, nil,
		seedling.Set("Name", "alice"),
		seedling.Set("Email", "alice@example.com"),
	)

	if user.Root().Name != "alice" {
		t.Fatalf("expected Name = %q, got %q", "alice", user.Root().Name)
	}
	if user.Root().Email != "alice@example.com" {
		t.Fatalf("expected Email = %q, got %q", "alice@example.com", user.Root().Email)
	}
}
