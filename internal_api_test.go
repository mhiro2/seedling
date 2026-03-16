package seedling

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/executor"
	"github.com/mhiro2/seedling/internal/graph"
)

type internalCompany struct {
	ID   int
	Name string
}

func TestFirstField(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   string
	}{
		{
			name:   "empty",
			fields: nil,
			want:   "",
		},
		{
			name:   "returns first value",
			fields: []string{"ID", "Name"},
			want:   "ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange

			// Act
			got := firstField(tt.fields)

			// Assert
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistryAdapter_LookupByName(t *testing.T) {
	// Arrange
	reg := NewRegistry()
	err := RegisterTo(reg, Blueprint[internalCompany]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() internalCompany {
			return internalCompany{Name: "test-company"}
		},
		Insert: func(_ context.Context, _ DBTX, v internalCompany) (internalCompany, error) {
			v.ID = 1
			return v, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	adapter := newRegistryAdapter(reg)

	// Act
	def, err := adapter.LookupByName("company")
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if def.Name != "company" {
		t.Fatalf("got %v, want %v", def.Name, "company")
	}
	if !reflect.DeepEqual(def.PKFields, []string{"ID"}) {
		t.Fatalf("got %v, want %v", def.PKFields, []string{"ID"})
	}
	if def.ModelType != reflect.TypeFor[internalCompany]() {
		t.Fatalf("got %v, want %v", def.ModelType, reflect.TypeFor[internalCompany]())
	}
}

func TestRegistryAdapter_LookupByName_Missing(t *testing.T) {
	// Arrange
	adapter := newRegistryAdapter(NewRegistry())

	// Act
	def, err := adapter.LookupByName("missing")

	// Assert
	if def != nil {
		t.Fatalf("expected nil, got %v", def)
	}
	if !errors.Is(err, errx.ErrBlueprintNotFound) {
		t.Fatalf("got %v, want %v", err, errx.ErrBlueprintNotFound)
	}
}

func TestResult_Nodes_ReturnsSortedMatches(t *testing.T) {
	// Arrange
	result := Result[internalCompany]{
		nodes: map[string]executor.NodeResult{
			"root.b": {Name: "company", Value: internalCompany{ID: 2, Name: "second"}},
			"root.a": {Name: "company", Value: internalCompany{ID: 1, Name: "first"}},
			"root.u": {Name: "user", Value: "ignored"},
		},
	}

	// Act
	nodes := result.Nodes("company")

	// Assert
	if len(nodes) != 2 {
		t.Fatalf("got len %d, want %d", len(nodes), 2)
	}
	if nodes[0].Name() != "company" {
		t.Fatalf("got %v, want %v", nodes[0].Name(), "company")
	}
	if !reflect.DeepEqual(nodes[0].Value(), internalCompany{ID: 1, Name: "first"}) {
		t.Fatalf("got %v, want %v", nodes[0].Value(), internalCompany{ID: 1, Name: "first"})
	}
	if !reflect.DeepEqual(nodes[1].Value(), internalCompany{ID: 2, Name: "second"}) {
		t.Fatalf("got %v, want %v", nodes[1].Value(), internalCompany{ID: 2, Name: "second"})
	}
}

func TestNodeAs_ReturnsTypedMatch(t *testing.T) {
	// Arrange
	result := Result[internalCompany]{
		nodes: map[string]executor.NodeResult{
			"root": {Name: "company", Value: internalCompany{ID: 7, Name: "root"}},
		},
	}

	// Act
	node, ok, err := NodeAs[internalCompany](result, "company")
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected true")
	}
	if !reflect.DeepEqual(node, internalCompany{ID: 7, Name: "root"}) {
		t.Fatalf("got %v, want %v", node, internalCompany{ID: 7, Name: "root"})
	}
}

func TestNodeAs_ReturnsTypeMismatch(t *testing.T) {
	// Arrange
	result := Result[internalCompany]{
		nodes: map[string]executor.NodeResult{
			"root": {Name: "company", Value: internalCompany{ID: 7, Name: "root"}},
		},
	}

	// Act
	_, ok, err := NodeAs[string](result, "company")

	// Assert
	if !ok {
		t.Fatal("expected true")
	}
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v, want %v", err, ErrTypeMismatch)
	}
}

func TestNodesAs_ReturnsTypedMatches(t *testing.T) {
	// Arrange
	result := Result[internalCompany]{
		nodes: map[string]executor.NodeResult{
			"root.b": {Name: "company", Value: internalCompany{ID: 2, Name: "second"}},
			"root.a": {Name: "company", Value: internalCompany{ID: 1, Name: "first"}},
		},
	}

	// Act
	nodes, err := NodesAs[internalCompany](result, "company")
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	want := []internalCompany{
		{ID: 1, Name: "first"},
		{ID: 2, Name: "second"},
	}
	if !reflect.DeepEqual(nodes, want) {
		t.Fatalf("mismatch:\ngot:  %+v\nwant: %+v", nodes, want)
	}
}

func TestResult_Nodes_ReturnsNilWhenMissing(t *testing.T) {
	// Arrange
	result := Result[internalCompany]{nodes: map[string]executor.NodeResult{}}

	// Act
	nodes := result.Nodes("company")

	// Assert
	if nodes != nil {
		t.Fatalf("expected nil, got %v", nodes)
	}
}

func TestResult_MustNode_ReturnsMatch(t *testing.T) {
	// Arrange
	result := Result[internalCompany]{
		nodes: map[string]executor.NodeResult{
			"root": {Name: "company", Value: internalCompany{ID: 7, Name: "root"}},
		},
	}

	// Act
	node := result.MustNode("company")

	// Assert
	if node.Name() != "company" {
		t.Fatalf("got %v, want %v", node.Name(), "company")
	}
	if !reflect.DeepEqual(node.Value(), internalCompany{ID: 7, Name: "root"}) {
		t.Fatalf("got %v, want %v", node.Value(), internalCompany{ID: 7, Name: "root"})
	}
}

func TestResult_DebugString_Empty(t *testing.T) {
	// Arrange
	result := Result[internalCompany]{}

	// Act
	out := result.DebugString()

	// Assert
	if out != "(empty)" {
		t.Fatalf("got %v, want %v", out, "(empty)")
	}
}

func TestSessionBuildE_ReturnsErrorForUnknownTrait(t *testing.T) {
	// Arrange
	reg := NewRegistry()
	err := RegisterTo(reg, Blueprint[internalCompany]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Insert: func(_ context.Context, _ DBTX, v internalCompany) (internalCompany, error) {
			return v, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Act
	plan, err := NewSession[internalCompany](reg).BuildE(BlueprintTrait("missing"))

	// Assert
	if plan != nil {
		t.Fatalf("expected nil, got %v", plan)
	}
	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, ErrInvalidOption)
	}
	if err == nil || !strings.Contains(err.Error(), `trait "missing" not defined`) {
		t.Fatalf("expected error containing %q, got %v", `trait "missing" not defined`, err)
	}
}

func TestRelationConstructors(t *testing.T) {
	// Arrange

	// Act
	belongsTo := BelongsToRelation("company", "company", false, "CompanyID")
	hasMany := HasManyRelation("users", "user", false, 2, "CompanyID")
	manyToMany := ManyToManyRelation("tags", "article_tag", "tag", false, 3, []string{"ArticleID"}, []string{"TagID"})

	// Assert
	wantBelongsTo := Relation{
		Name:         "company",
		Kind:         BelongsTo,
		LocalField:   "CompanyID",
		LocalFields:  []string{"CompanyID"},
		RefBlueprint: "company",
	}
	if !reflect.DeepEqual(belongsTo, wantBelongsTo) {
		t.Fatalf("mismatch:\ngot:  %+v\nwant: %+v", belongsTo, wantBelongsTo)
	}
	wantHasMany := Relation{
		Name:         "users",
		Kind:         HasMany,
		LocalField:   "CompanyID",
		LocalFields:  []string{"CompanyID"},
		RefBlueprint: "user",
		Count:        2,
	}
	if !reflect.DeepEqual(hasMany, wantHasMany) {
		t.Fatalf("mismatch:\ngot:  %+v\nwant: %+v", hasMany, wantHasMany)
	}
	wantManyToMany := Relation{
		Name:             "tags",
		Kind:             ManyToMany,
		LocalField:       "ArticleID",
		LocalFields:      []string{"ArticleID"},
		RefBlueprint:     "tag",
		ThroughBlueprint: "article_tag",
		RemoteField:      "TagID",
		RemoteFields:     []string{"TagID"},
		Count:            3,
	}
	if !reflect.DeepEqual(manyToMany, wantManyToMany) {
		t.Fatalf("mismatch:\ngot:  %+v\nwant: %+v", manyToMany, wantManyToMany)
	}
}

func TestNormalizeRelations_DefaultsToRequiredUnlessOptional(t *testing.T) {
	// Arrange
	rels := []Relation{
		{Name: "default", Kind: BelongsTo, RefBlueprint: "company"},
		{Name: "optional", Kind: BelongsTo, RefBlueprint: "company", Optional: true},
	}

	// Act
	got := normalizeRelations(rels)

	// Assert
	if !got[0].required {
		t.Fatal("expected true")
	}
	if got[1].required {
		t.Fatal("expected false")
	}
}

func TestPlanValidate_DetectsCycles(t *testing.T) {
	// Arrange
	g := graph.New()
	a := &graph.Node{ID: "a", BlueprintName: "a"}
	b := &graph.Node{ID: "b", BlueprintName: "b"}
	g.AddNode(a)
	g.AddNode(b)
	g.AddEdge(a, b, "AID")
	g.AddEdge(b, a, "BID")

	plan := &Plan[internalCompany]{graph: g}

	// Act
	err := plan.Validate()

	// Assert
	if !errors.Is(err, errx.ErrCycleDetected) {
		t.Fatalf("got %v, want %v", err, errx.ErrCycleDetected)
	}
}

func TestToOptionSet_Nil(t *testing.T) {
	// Arrange

	// Act
	got := toOptionSet(nil)

	// Assert
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}
