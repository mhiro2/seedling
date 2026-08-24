package planner

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
)

type identityCollisionRoot struct {
	ID       int
	DirectID int
	MiddleID int
}

type identityCollisionDirect struct {
	ID int
}

type identityCollisionMiddle struct {
	ID       int
	NestedID int
}

type identityCollisionNested struct {
	ID int
}

type identityJoinRoot struct {
	ID int
}

type identityJoinChild struct {
	ID         int
	MetadataID int
}

type identityJoinMetadata struct {
	ID int
}

type identityJoinRow struct {
	RootID  int
	ChildID int
}

type identityCollisionRegistry struct {
	byName map[string]*BlueprintDef
	byType map[reflect.Type]*BlueprintDef
}

func (r identityCollisionRegistry) LookupByName(name string) (*BlueprintDef, error) {
	bp, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("lookup blueprint %q: %w", name, errx.BlueprintNotFound(name))
	}
	return bp, nil
}

func (r identityCollisionRegistry) LookupByType(modelType reflect.Type) (*BlueprintDef, error) {
	bp, ok := r.byType[modelType]
	if !ok {
		return nil, fmt.Errorf("lookup blueprint type %s: %w", modelType, errx.BlueprintNotFound(modelType.String()))
	}
	return bp, nil
}

func newIdentityPathCollisionRegistry() (identityCollisionRegistry, *BlueprintDef) {
	root := &BlueprintDef{
		Name:     "root",
		PKFields: []string{"ID"},
		Relations: []RelationDef{
			{Name: "a.b", Kind: BelongsTo, LocalFields: []string{"DirectID"}, RefBlueprint: "direct", Required: true},
			{Name: "a", Kind: BelongsTo, LocalFields: []string{"MiddleID"}, RefBlueprint: "middle", Required: true},
		},
		Defaults:  func() any { return identityCollisionRoot{} },
		ModelType: reflect.TypeFor[identityCollisionRoot](),
	}
	direct := &BlueprintDef{
		Name:      "direct",
		PKFields:  []string{"ID"},
		Defaults:  func() any { return identityCollisionDirect{} },
		ModelType: reflect.TypeFor[identityCollisionDirect](),
	}
	middle := &BlueprintDef{
		Name:     "middle",
		PKFields: []string{"ID"},
		Relations: []RelationDef{
			{Name: "b", Kind: BelongsTo, LocalFields: []string{"NestedID"}, RefBlueprint: "nested", Required: true},
		},
		Defaults:  func() any { return identityCollisionMiddle{} },
		ModelType: reflect.TypeFor[identityCollisionMiddle](),
	}
	nested := &BlueprintDef{
		Name:      "nested",
		PKFields:  []string{"ID"},
		Defaults:  func() any { return identityCollisionNested{} },
		ModelType: reflect.TypeFor[identityCollisionNested](),
	}
	return identityCollisionRegistry{
		byName: map[string]*BlueprintDef{
			root.Name:   root,
			direct.Name: direct,
			middle.Name: middle,
			nested.Name: nested,
		},
		byType: map[reflect.Type]*BlueprintDef{
			root.ModelType:   root,
			direct.ModelType: direct,
			middle.ModelType: middle,
			nested.ModelType: nested,
		},
	}, root
}

func TestPlan_RelationNamesCannotCollideWithNestedPaths(t *testing.T) {
	t.Parallel()

	reg, root := newIdentityPathCollisionRegistry()

	result, err := Plan(reg, root.ModelType, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := len(result.Graph.Nodes()); got != 4 {
		t.Fatalf("got %d graph nodes, want 4", got)
	}
	directNode := result.Graph.Node("root.a%2Eb")
	if directNode == nil || directNode.BlueprintName != "direct" {
		t.Fatalf("direct relation node = %#v, want blueprint %q", directNode, "direct")
	}
	nestedNode := result.Graph.Node("root.a.b")
	if nestedNode == nil || nestedNode.BlueprintName != "nested" {
		t.Fatalf("nested relation node = %#v, want blueprint %q", nestedNode, "nested")
	}
}

func TestPlanMany_RelationPathsRemainDistinctWhenShared(t *testing.T) {
	t.Parallel()

	reg, root := newIdentityPathCollisionRegistry()
	result, err := PlanMany(reg, root.ModelType, []*OptionSet{{}, {}})
	if err != nil {
		t.Fatal(err)
	}

	if got := len(result.Graph.Nodes()); got != 5 {
		t.Fatalf("got %d graph nodes, want 5", got)
	}
	directNode := result.Graph.Node("shared.a%2Eb")
	if directNode == nil || directNode.BlueprintName != "direct" {
		t.Fatalf("shared direct relation node = %#v, want blueprint %q", directNode, "direct")
	}
	nestedNode := result.Graph.Node("shared.a.b")
	if nestedNode == nil || nestedNode.BlueprintName != "nested" {
		t.Fatalf("shared nested relation node = %#v, want blueprint %q", nestedNode, "nested")
	}
}

func TestNodeIdentity_EscapesReservedCharactersInjectively(t *testing.T) {
	t.Parallel()

	segments := []string{"a", "a.b", "a%2Eb", "a[0]", "a#1"}
	identities := make(map[nodeIdentity]string, len(segments))
	for _, segment := range segments {
		identity := relationNodeIdentity(rootNodeIdentity("root"), segment, 0, 1)
		if previous, exists := identities[identity]; exists {
			t.Fatalf("segments %q and %q produced the same identity %q", previous, segment, identity)
		}
		identities[identity] = segment
	}

	if rootNodeIdentity("root.a") == relationNodeIdentity(rootNodeIdentity("root"), "a", 0, 1) {
		t.Fatal("blueprint root identity collided with a relation path")
	}
	root := rootNodeIdentity("root")
	if relationNodeIdentity(root, "a[0]", 0, 1) == relationNodeIdentity(root, "a", 0, 2) {
		t.Fatal("literal relation brackets collided with a collection index")
	}
	if sharedRelationNodeIdentity(appendRelationPath("", "a#1"), 0) == sharedRelationNodeIdentity(appendRelationPath("", "a"), 1) {
		t.Fatal("literal relation suffix collided with a shared candidate index")
	}
	if relationNodeIdentity(root, "%join:link", 0, 1) == joinNodeIdentity(root, "link") {
		t.Fatal("literal relation name collided with a synthetic join identity")
	}
}

func TestPlan_JoinIdentityCannotCollideWithChildRelation(t *testing.T) {
	t.Parallel()

	root := &BlueprintDef{
		Name:     "join_root",
		PKFields: []string{"ID"},
		Relations: []RelationDef{{
			Name:             "children",
			Kind:             ManyToMany,
			LocalFields:      []string{"RootID"},
			RefBlueprint:     "join_child",
			ThroughBlueprint: "link",
			RemoteFields:     []string{"ChildID"},
			Required:         true,
			Count:            1,
		}},
		Defaults:  func() any { return identityJoinRoot{} },
		ModelType: reflect.TypeFor[identityJoinRoot](),
	}
	child := &BlueprintDef{
		Name:     "join_child",
		PKFields: []string{"ID"},
		Relations: []RelationDef{{
			Name:         "link",
			Kind:         BelongsTo,
			LocalFields:  []string{"MetadataID"},
			RefBlueprint: "join_metadata",
			Required:     true,
		}},
		Defaults:  func() any { return identityJoinChild{} },
		ModelType: reflect.TypeFor[identityJoinChild](),
	}
	metadata := &BlueprintDef{
		Name:      "join_metadata",
		PKFields:  []string{"ID"},
		Defaults:  func() any { return identityJoinMetadata{} },
		ModelType: reflect.TypeFor[identityJoinMetadata](),
	}
	join := &BlueprintDef{
		Name:      "link",
		PKFields:  []string{"RootID", "ChildID"},
		Defaults:  func() any { return identityJoinRow{} },
		ModelType: reflect.TypeFor[identityJoinRow](),
	}
	reg := identityCollisionRegistry{
		byName: map[string]*BlueprintDef{
			root.Name:     root,
			child.Name:    child,
			metadata.Name: metadata,
			join.Name:     join,
		},
		byType: map[reflect.Type]*BlueprintDef{
			root.ModelType:     root,
			child.ModelType:    child,
			metadata.ModelType: metadata,
			join.ModelType:     join,
		},
	}

	result, err := Plan(reg, root.ModelType, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := len(result.Graph.Nodes()); got != 4 {
		t.Fatalf("got %d graph nodes, want 4", got)
	}
	metadataNode := result.Graph.Node("join_root.children.link")
	if metadataNode == nil || metadataNode.BlueprintName != metadata.Name {
		t.Fatalf("child relation node = %#v, want blueprint %q", metadataNode, metadata.Name)
	}
	joinNode := result.Graph.Node("join_root.children.%join:link")
	if joinNode == nil || joinNode.BlueprintName != join.Name {
		t.Fatalf("join node = %#v, want blueprint %q", joinNode, join.Name)
	}
}
