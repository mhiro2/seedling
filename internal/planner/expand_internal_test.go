package planner

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
)

type helperCompany struct {
	ID   int
	Name string
}

type helperDepartment struct {
	ID int
}

func TestValidateUseValueType(t *testing.T) {
	t.Parallel()

	// Arrange
	valueType := reflect.TypeFor[helperCompany]()
	pointerType := reflect.TypeFor[*helperCompany]()

	tests := []struct {
		name     string
		expected reflect.Type
		value    any
		wantErr  error
	}{
		{name: "value matches value", expected: valueType, value: helperCompany{}},
		{name: "pointer matches value", expected: valueType, value: &helperCompany{}},
		{name: "value matches pointer", expected: pointerType, value: helperCompany{}},
		{name: "pointer matches pointer", expected: pointerType, value: &helperCompany{}},
		{name: "mismatch", expected: valueType, value: helperDepartment{}, wantErr: errx.ErrTypeMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := validateUseValueType("company", tt.expected, tt.value)

			// Assert
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewProvidedNode_ClonesInput(t *testing.T) {
	t.Parallel()

	// Arrange
	bp := &BlueprintDef{
		Name:     "company",
		PKFields: []string{"ID"},
	}
	original := &helperCompany{ID: 10, Name: "acme"}

	// Act
	node := newProvidedNode(bp, "task.company", original)

	// Assert
	cloned, ok := node.Value.(*helperCompany)
	if !ok {
		t.Fatalf("expected pointer value, got %T", node.Value)
	}
	if original == cloned {
		t.Errorf("expected provided value to be cloned")
	}

	original.Name = "changed"
	if cloned.Name != "acme" {
		t.Errorf("expected cloned value to remain unchanged: got %v, want %v", cloned.Name, "acme")
	}
	if !node.IsProvided {
		t.Errorf("expected node to be marked as provided")
	}
	if len(node.PrimaryKeyFields()) != 1 {
		t.Fatalf("got len %d, want %d", len(node.PrimaryKeyFields()), 1)
	}
	if node.PrimaryKeyFields()[0] != "ID" {
		t.Errorf("got %v, want %v", node.PrimaryKeyFields()[0], "ID")
	}
}

func TestBuildRelationBindings(t *testing.T) {
	t.Parallel()

	t.Run("local and remote bindings", func(t *testing.T) {
		// Arrange
		rel := RelationDef{
			Name:         "company",
			LocalFields:  []string{"CompanyID"},
			RemoteFields: []string{"RemoteCompanyID"},
		}

		// Act
		localBindings, err := buildLocalBindings("user", rel, []string{"ID"})
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if len(localBindings) != 1 {
			t.Fatalf("got len %d, want %d", len(localBindings), 1)
		}
		if localBindings[0].ParentField != "ID" {
			t.Errorf("got %v, want %v", localBindings[0].ParentField, "ID")
		}
		if localBindings[0].ChildField != "CompanyID" {
			t.Errorf("got %v, want %v", localBindings[0].ChildField, "CompanyID")
		}

		// Act
		remoteBindings, err := buildRemoteBindings("user", rel, []string{"ID"})
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if len(remoteBindings) != 1 {
			t.Fatalf("got len %d, want %d", len(remoteBindings), 1)
		}
		if remoteBindings[0].ParentField != "ID" {
			t.Errorf("got %v, want %v", remoteBindings[0].ParentField, "ID")
		}
		if remoteBindings[0].ChildField != "RemoteCompanyID" {
			t.Errorf("got %v, want %v", remoteBindings[0].ChildField, "RemoteCompanyID")
		}
	})

	t.Run("invalid mapping returns error", func(t *testing.T) {
		// Arrange
		rel := RelationDef{Name: "company"}

		// Act
		_, err := buildLocalBindings("user", rel, []string{"ID"})

		// Assert
		if !errors.Is(err, errx.ErrInvalidOption) {
			t.Fatalf("got %v, want %v", err, errx.ErrInvalidOption)
		}
	})
}

func TestHasManyBindings_ReturnsMatchingBelongsToRelations(t *testing.T) {
	t.Parallel()

	// Arrange
	childBP := &BlueprintDef{
		Name: "employee",
		Relations: []RelationDef{
			{Name: "department", Kind: BelongsTo, LocalFields: []string{"DepartmentID"}, RefBlueprint: "department"},
			{Name: "company", Kind: BelongsTo, LocalFields: []string{"CompanyID"}, RefBlueprint: "company"},
		},
	}
	parentNode := &graph.Node{BlueprintName: "department"}
	rel := RelationDef{Name: "employees", Kind: HasMany, LocalFields: []string{"DepartmentID"}}

	// Act
	bindings := hasManyBindings(childBP, parentNode, rel)

	// Assert
	if len(bindings) != 1 {
		t.Fatalf("expected exactly one matching binding: got len %d, want %d", len(bindings), 1)
	}
	if bindings["department"] != parentNode {
		t.Errorf("expected department relation to be pre-bound to the parent node: got %v, want %v", bindings["department"], parentNode)
	}
}

func TestManyToManyBindings(t *testing.T) {
	t.Parallel()

	// Arrange
	parentNode := &graph.Node{BlueprintName: "article"}
	childNode := &graph.Node{BlueprintName: "tag"}
	rel := RelationDef{
		Name:         "tags",
		Kind:         ManyToMany,
		LocalFields:  []string{"ArticleID"},
		RemoteFields: []string{"TagID"},
	}

	tests := []struct {
		name            string
		joinRelations   []RelationDef
		wantBindingKeys []string
		wantParentBound bool
		wantChildBound  bool
	}{
		{
			name: "both sides bound",
			joinRelations: []RelationDef{
				{Name: "article", Kind: BelongsTo, LocalFields: []string{"ArticleID"}, RefBlueprint: "article"},
				{Name: "tag", Kind: BelongsTo, LocalFields: []string{"TagID"}, RefBlueprint: "tag"},
			},
			wantBindingKeys: []string{"article", "tag"},
			wantParentBound: true,
			wantChildBound:  true,
		},
		{
			name: "child binding missing",
			joinRelations: []RelationDef{
				{Name: "article", Kind: BelongsTo, LocalFields: []string{"ArticleID"}, RefBlueprint: "article"},
			},
			wantBindingKeys: []string{"article"},
			wantParentBound: true,
			wantChildBound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			joinBP := &BlueprintDef{Name: "article_tag", Relations: tt.joinRelations}

			// Act
			bindings, parentBound, childBound := manyToManyBindings(joinBP, parentNode, childNode, rel)

			// Assert
			if parentBound != tt.wantParentBound {
				t.Fatalf("parentBound mismatch: got %v, want %v", parentBound, tt.wantParentBound)
			}
			if childBound != tt.wantChildBound {
				t.Fatalf("childBound mismatch: got %v, want %v", childBound, tt.wantChildBound)
			}
			if len(bindings) != len(tt.wantBindingKeys) {
				t.Fatalf("got len %d, want %d", len(bindings), len(tt.wantBindingKeys))
			}
			for _, key := range tt.wantBindingKeys {
				if bindings[key] == nil {
					t.Errorf("expected binding for %q", key)
				}
			}
		})
	}
}
