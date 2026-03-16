package customdefaults_test

import (
	"testing"

	"github.com/mhiro2/seedling"
	customdefaults "github.com/mhiro2/seedling/examples/custom-defaults"
)

func setup(t *testing.T) {
	t.Helper()
	seedling.ResetRegistry()
	customdefaults.SetupBlueprints()
}

func TestDefaultUser(t *testing.T) {
	setup(t)

	user := seedling.InsertOne[customdefaults.User](t, nil)

	if user.Root().Role != "member" {
		t.Fatalf("expected Role = %q, got %q", "member", user.Root().Role)
	}
	if user.Root().Status != "active" {
		t.Fatalf("expected Status = %q, got %q", "active", user.Root().Status)
	}
}

func TestAdminUser(t *testing.T) {
	setup(t)

	// Use the AdminUser() helper for type-safe default customization.
	admin := seedling.InsertOne[customdefaults.User](t, nil,
		customdefaults.AdminUser(),
	)

	if admin.Root().Role != "admin" {
		t.Fatalf("expected Role = %q, got %q", "admin", admin.Root().Role)
	}
	if admin.Root().Email != "admin@example.com" {
		t.Fatalf("expected Email = %q, got %q", "admin@example.com", admin.Root().Email)
	}
	// Status should keep the default.
	if admin.Root().Status != "active" {
		t.Fatalf("expected Status = %q, got %q", "active", admin.Root().Status)
	}
}

func TestInactiveUser(t *testing.T) {
	setup(t)

	user := seedling.InsertOne[customdefaults.User](t, nil,
		customdefaults.InactiveUser(),
	)

	if user.Root().Status != "inactive" {
		t.Fatalf("expected Status = %q, got %q", "inactive", user.Root().Status)
	}
	// Role should keep the default.
	if user.Root().Role != "member" {
		t.Fatalf("expected Role = %q, got %q", "member", user.Root().Role)
	}
}

func TestCombineWithOptions(t *testing.T) {
	setup(t)

	// Multiple With options can be combined.
	user := seedling.InsertOne[customdefaults.User](t, nil,
		customdefaults.AdminUser(),
		customdefaults.InactiveUser(),
	)

	if user.Root().Role != "admin" {
		t.Fatalf("expected Role = %q, got %q", "admin", user.Root().Role)
	}
	if user.Root().Status != "inactive" {
		t.Fatalf("expected Status = %q, got %q", "inactive", user.Root().Status)
	}
}

func TestWithInline(t *testing.T) {
	setup(t)

	// With() can also be used inline for one-off customizations.
	user := seedling.InsertOne[customdefaults.User](t, nil,
		seedling.With(func(u *customdefaults.User) {
			u.Name = "custom-name"
			u.Role = "viewer"
		}),
	)

	if user.Root().Name != "custom-name" {
		t.Fatalf("expected Name = %q, got %q", "custom-name", user.Root().Name)
	}
	if user.Root().Role != "viewer" {
		t.Fatalf("expected Role = %q, got %q", "viewer", user.Root().Role)
	}
}
