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
