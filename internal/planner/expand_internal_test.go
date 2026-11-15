package planner

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/field"
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
	parentNode := &graph.Node{BlueprintName: "department", PKFields: []string{"ID"}}
	rel := RelationDef{Name: "employees", Kind: HasMany, LocalFields: []string{"DepartmentID"}}

	// Act
	bindings, err := hasManyBindings(childBP, parentNode, rel, mustLocalBindings(t, rel, parentNode))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected exactly one matching binding: got len %d, want %d", len(bindings), 1)
	}
	if bindings["department"].node != parentNode {
		t.Errorf("expected department relation to be pre-bound to the parent node: got %v, want %v", bindings["department"].node, parentNode)
	}
}

func TestHasManyBindings_UsesTheHasManyRelationsReferencedFields(t *testing.T) {
	t.Parallel()

	converter := &field.Converter{From: reflect.TypeFor[string](), To: reflect.TypeFor[string]()}
	parentNode := &graph.Node{BlueprintName: "org", PKFields: []string{"ID"}}
	tests := []struct {
		name        string
		childRel    RelationDef
		wantParent  string
		wantConvert *field.Converter
		ownerRel    *RelationDef
		wantErr     bool
	}{
		{
			name:        "matching referenced fields take the has-many converter",
			childRel:    RelationDef{Name: "org", Kind: BelongsTo, LocalFields: []string{"OrgCode"}, RefFields: []string{"Code"}, RefBlueprint: "org"},
			wantParent:  "Code",
			wantConvert: converter,
		},
		{
			name: "composite fields in another order still match",
			childRel: RelationDef{
				Name: "org", Kind: BelongsTo, LocalFields: []string{"OrgCode", "TenantID"},
				RefFields: []string{"Code", "TenantID"}, RefBlueprint: "org",
			},
			wantParent: "Code",
			ownerRel: &RelationDef{
				Name: "members", Kind: HasMany, LocalFields: []string{"TenantID", "OrgCode"}, RefFields: []string{"TenantID", "Code"},
				Convert: map[string]*field.Converter{"OrgCode": converter},
			},
			wantConvert: converter,
		},
		{
			name:     "belongs-to referencing the primary key contradicts the has-many",
			childRel: RelationDef{Name: "org", Kind: BelongsTo, LocalFields: []string{"OrgCode"}, RefBlueprint: "org"},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			childBP := &BlueprintDef{Name: "member", Relations: []RelationDef{tt.childRel}}
			rel := RelationDef{
				Name: "members", Kind: HasMany, LocalFields: []string{"OrgCode"}, RefFields: []string{"Code"},
				Convert: map[string]*field.Converter{"OrgCode": converter},
			}
			if tt.ownerRel != nil {
				rel = *tt.ownerRel
			}

			// Act
			bindings, err := hasManyBindings(childBP, parentNode, rel, mustLocalBindings(t, rel, parentNode))

			// Assert
			if tt.wantErr {
				if !errors.Is(err, errx.ErrInvalidOption) {
					t.Fatalf("got %v, want %v", err, errx.ErrInvalidOption)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var orgCode *graph.FieldBinding
			for i, edge := range bindings["org"].edges {
				if edge.ChildField == "OrgCode" {
					orgCode = &bindings["org"].edges[i]
				}
			}
			if orgCode == nil || orgCode.ParentField != tt.wantParent || orgCode.Convert != tt.wantConvert {
				t.Fatalf("got edges %+v, want OrgCode from %s with the has-many converter", bindings["org"].edges, tt.wantParent)
			}
		})
	}
}

func mustLocalBindings(t *testing.T, rel RelationDef, parent *graph.Node) []graph.FieldBinding {
	t.Helper()
	edges, err := buildLocalBindings(parent.BlueprintName, rel, parent.PrimaryKeyFields())
	if err != nil {
		t.Fatal(err)
	}
	return edges
}

func TestManyToManyBindings(t *testing.T) {
	t.Parallel()

	// Arrange
	parentNode := &graph.Node{BlueprintName: "article", PKFields: []string{"ID"}}
	childNode := &graph.Node{BlueprintName: "tag", PKFields: []string{"ID"}}
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
			childEdges, err := buildRemoteBindings(parentNode.BlueprintName, rel, childNode.PrimaryKeyFields())
			if err != nil {
				t.Fatal(err)
			}
			bindings, parentBound, childBound, err := manyToManyBindings(joinBP, parentNode, childNode, rel, mustLocalBindings(t, rel, parentNode), childEdges)
			// Assert
			if err != nil {
				t.Fatal(err)
			}
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
				if bindings[key].node == nil {
					t.Errorf("expected binding for %q", key)
				}
			}
		})
	}
}
