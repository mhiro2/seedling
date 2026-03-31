package quickstart_test

import (
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/examples/quickstart"
)

func setup(t *testing.T) {
	t.Helper()
	seedling.ResetRegistry()
	quickstart.ResetIDs()
	quickstart.RegisterBlueprints()
}

func TestQuickStart_InsertOneUser(t *testing.T) {
	// Arrange
	setup(t)

	// Act
	user := seedling.InsertOne[quickstart.User](t, nil).Root()

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
	setup(t)
	company := seedling.InsertOne[quickstart.Company](t, nil,
		seedling.Set("Name", "shared-company"),
	).Root()

	// Act
	user := seedling.InsertOne[quickstart.User](t, nil,
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
