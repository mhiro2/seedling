package sqlc_test

import (
	"fmt"
	"testing"

	"github.com/mhiro2/seedling"
	example "github.com/mhiro2/seedling/examples/sqlc"
)

func setup(t *testing.T) {
	t.Helper()
	seedling.ResetRegistry()
	example.SetupBlueprints()
}

func TestInsertOne_Member(t *testing.T) {
	setup(t)

	// seedling auto-creates the parent Organization.
	member := seedling.InsertOne[example.Member](t, nil)

	if member.Root().ID == 0 {
		t.Fatal("expected member ID to be set")
	}
	if member.Root().OrganizationID == 0 {
		t.Fatal("expected OrganizationID to be auto-populated")
	}
	if member.Root().Name != "test-member" {
		t.Fatalf("expected Name = %q, got %q", "test-member", member.Root().Name)
	}
}

func TestInsertOne_Organization(t *testing.T) {
	setup(t)

	org := seedling.InsertOne[example.Organization](t, nil,
		seedling.Set("Name", "Acme Corp"),
	)

	if org.Root().ID == 0 {
		t.Fatal("expected org ID to be set")
	}
	if org.Root().Name != "Acme Corp" {
		t.Fatalf("expected Name = %q, got %q", "Acme Corp", org.Root().Name)
	}
}

func TestInsertMany_Members(t *testing.T) {
	setup(t)

	members := seedling.InsertMany[example.Member](t, nil, 3,
		seedling.Seq("Name", func(i int) string {
			return fmt.Sprintf("member-%d", i)
		}),
	)

	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
	for i, m := range members {
		expected := fmt.Sprintf("member-%d", i)
		if m.Name != expected {
			t.Fatalf("members[%d].Name = %q, want %q", i, m.Name, expected)
		}
	}
}
