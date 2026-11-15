package planner

import (
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
)

func (e *expander) expandRelation(
	bp *BlueprintDef,
	node *graph.Node,
	nodeID nodeIdentity,
	relationPath relationPath,
	rel RelationDef,
	opts *OptionSet,
	bindings map[string]boundParent,
) error {
	// A structural back-reference binds the child to the parent that created it
	// (e.g. an employee's belongs_to(department) inside department's has_many).
	// This FK must always be set, independent of When/Omit on the child relation,
	// otherwise the auto-generated child would be orphaned with a zero FK. A Use
	// or Ref on that relation would have to replace or rebuild the parent that
	// already exists, so it is rejected instead of silently dropped.
	if bound, ok := bindings[rel.Name]; ok {
		e.graph.AddEdgeBindings(bound.node, node, bound.edges)
		return boundRelationOverride(bp.Name, rel.Name, bound.node.BlueprintName, opts)
	}

	shouldExpand, err := relationExpansion(node.Value, rel, opts)
	if err != nil {
		return err
	}
	if !shouldExpand {
		return nil
	}

	switch rel.Kind {
	case BelongsTo:
		return e.expandBelongsTo(bp, node, nodeID, relationPath, rel, opts)
	case HasMany:
		return e.expandHasMany(node, nodeID, relationPath, rel, opts)
	case ManyToMany:
		return e.expandManyToMany(node, nodeID, relationPath, rel, opts)
	default:
		return fmt.Errorf("%w: unsupported relation kind %q on blueprint %q", errx.ErrInvalidOption, rel.Kind, bp.Name)
	}
}

// boundRelationOverride reports a Use or Ref given for a relation that is
// already bound to the parent that created the node.
func boundRelationOverride(blueprint, relation, parent string, opts *OptionSet) error {
	var option string
	switch {
	case relationUsed(opts, relation):
		option = "Use"
	case relationLinked(opts, relation):
		option = "Link"
	case relationReferenced(opts, relation):
		option = "Ref"
	default:
		return nil
	}
	return fmt.Errorf("validate relation %q options: %w", relation, errx.BoundRelationOverride(blueprint, relation, parent, option))
}

// relationExpansion reports whether a relation that is not a structural
// back-reference should expand a fresh subtree. Omit and a false When both
// suppress expansion; otherwise expansion follows Required/Use/Ref/predicate.
func relationExpansion(value any, rel RelationDef, opts *OptionSet) (bool, error) {
	if relationOmitted(opts, rel.Name) {
		return false, nil
	}

	enabled, err := relationWhen(value, rel, opts)
	if err != nil {
		return false, err
	}
	if !enabled {
		return false, nil
	}

	return relationEnabled(rel, opts), nil
}

func (e *expander) expandBelongsTo(
	bp *BlueprintDef,
	node *graph.Node,
	nodeID nodeIdentity,
	relationPath relationPath,
	rel RelationDef,
	opts *OptionSet,
) error {
	usedNode, ok, err := e.linkedBelongsToNode(rel, opts)
	if err == nil && !ok {
		usedNode, ok, err = e.providedBelongsToNode(rel, node, nodeID, opts)
	}
	if err != nil {
		return err
	}
	if ok {
		edgeBindings, err := buildLocalBindings(bp.Name, rel, usedNode.PrimaryKeyFields())
		if err != nil {
			return err
		}
		e.graph.AddEdgeBindings(usedNode, node, edgeBindings)
		return nil
	}

	parentBP, err := e.reg.LookupByName(rel.RefBlueprint)
	if err != nil {
		return fmt.Errorf("lookup belongs-to blueprint %q: %w", rel.RefBlueprint, err)
	}

	childPath := appendRelationPath(relationPath, rel.Name)
	origin := graph.Origin{Kind: graph.OriginBelongsTo, From: node, Relation: rel.Name}
	childNode, err := e.expandBlueprint(parentBP, e.belongsToNodeID(nodeID, childPath, rel, nestedRelationOpts(opts, rel.Name)), nestedRelationOpts(opts, rel.Name), nil, childPath, origin)
	if err != nil {
		return err
	}

	edgeBindings, err := buildLocalBindings(bp.Name, rel, childNode.PrimaryKeyFields())
	if err != nil {
		return err
	}
	e.graph.AddEdgeBindings(childNode, node, edgeBindings)
	return nil
}

func (e *expander) expandHasMany(
	parentNode *graph.Node,
	parentNodeID nodeIdentity,
	relationPath relationPath,
	rel RelationDef,
	opts *OptionSet,
) error {
	parentEdgeBindings, err := buildLocalBindings(parentNode.BlueprintName, rel, parentNode.PrimaryKeyFields())
	if err != nil {
		return err
	}

	childBP, err := e.reg.LookupByName(rel.RefBlueprint)
	if err != nil {
		return fmt.Errorf("lookup has-many blueprint %q: %w", rel.RefBlueprint, err)
	}

	if err := e.reserve(parentNode, rel.Name, relationCount(rel, opts)); err != nil {
		return err
	}
	elements := relationElements(rel, opts)
	if len(elements) == 0 {
		return nil
	}
	childBindings, err := hasManyBindings(childBP, parentNode, rel, parentEdgeBindings)
	if err != nil {
		return err
	}
	count := len(elements)

	for i, elem := range elements {
		childPath := appendRelationPath(relationPath, rel.Name)
		origin := graph.Origin{Kind: graph.OriginHasMany, From: parentNode, Relation: rel.Name, Index: i, Indexed: true}
		childNode, err := e.expandBlueprint(childBP, relationNodeIdentity(parentNodeID, rel.Name, i, count), elem.Opts, childBindings, childPath, origin)
		if err != nil {
			return err
		}
		if len(childBindings) == 0 {
			e.graph.AddEdgeBindings(parentNode, childNode, parentEdgeBindings)
		}
	}

	return nil
}

// boundParent is the parent a structural back-reference binds to, with the
// edge bindings that fill the child's foreign key.
type boundParent struct {
	node  *graph.Node
	edges []graph.FieldBinding
}

// hasManyBindings finds the belongs-to relations of a has-many child that
// point back at parentNode over the foreign key the has-many relation fills.
func hasManyBindings(childBP *BlueprintDef, parentNode *graph.Node, rel RelationDef, parentEdges []graph.FieldBinding) (map[string]boundParent, error) {
	bindings := make(map[string]boundParent)
	for _, childRel := range childBP.Relations {
		if childRel.Kind != BelongsTo || childRel.RefBlueprint != parentNode.BlueprintName {
			continue
		}
		edges, ok, err := backReferenceBindings(childBP.Name, childRel, parentNode, parentNode.BlueprintName, rel, parentEdges)
		if err != nil {
			return nil, err
		}
		if ok {
			bindings[childRel.Name] = boundParent{node: parentNode, edges: edges}
		}
	}
	return bindings, nil
}

// backReferenceBindings returns the edge bindings of childRel, a belongs-to
// relation on a record that the owner relation of ownerBlueprint creates,
// when childRel covers the foreign key fields that ownerEdges fill, in any
// order. Both relations then describe one foreign key: each field must
// reference the same parent field, or the two definitions contradict each
// other and the plan fails. Each field takes childRel's converter, or else the
// owner relation's.
func backReferenceBindings(
	childBlueprint string,
	childRel RelationDef,
	parent *graph.Node,
	ownerBlueprint string,
	owner RelationDef,
	ownerEdges []graph.FieldBinding,
) ([]graph.FieldBinding, bool, error) {
	ownerByField := make(map[string]graph.FieldBinding, len(ownerEdges))
	for _, edge := range ownerEdges {
		ownerByField[edge.ChildField] = edge
	}
	localFields := localFieldsForRelation(childRel)
	if len(localFields) != len(ownerByField) {
		return nil, false, nil
	}
	for _, f := range localFields {
		if _, ok := ownerByField[f]; !ok {
			return nil, false, nil
		}
	}
	edges, err := buildLocalBindings(childBlueprint, childRel, parent.PrimaryKeyFields())
	if err != nil {
		return nil, false, err
	}
	for i, edge := range edges {
		ownerEdge := ownerByField[edge.ChildField]
		if edge.ParentField != ownerEdge.ParentField {
			return nil, false, fmt.Errorf(
				"%w: relation %q on blueprint %q fills %s from %s.%s, but relation %q on blueprint %q fills it from %s.%s",
				errx.ErrInvalidOption,
				childRel.Name, childBlueprint, edge.ChildField, parent.BlueprintName, edge.ParentField,
				owner.Name, ownerBlueprint, parent.BlueprintName, ownerEdge.ParentField,
			)
		}
		if edge.Convert == nil {
			edges[i].Convert = ownerEdge.Convert
		}
	}
	return edges, true, nil
}

func (e *expander) expandManyToMany(
	parentNode *graph.Node,
	parentNodeID nodeIdentity,
	relationPath relationPath,
	rel RelationDef,
	opts *OptionSet,
) error {
	if rel.ThroughBlueprint == "" {
		return fmt.Errorf("%w: relation %q on blueprint %q requires ThroughBlueprint for many_to_many", errx.ErrInvalidOption, rel.Name, parentNode.BlueprintName)
	}

	childBP, err := e.reg.LookupByName(rel.RefBlueprint)
	if err != nil {
		return fmt.Errorf("lookup many-to-many child blueprint %q: %w", rel.RefBlueprint, err)
	}
	joinBP, err := e.reg.LookupByName(rel.ThroughBlueprint)
	if err != nil {
		return fmt.Errorf("lookup many-to-many join blueprint %q: %w", rel.ThroughBlueprint, err)
	}

	parentEdgeBindings, err := buildLocalBindings(parentNode.BlueprintName, rel, parentNode.PrimaryKeyFields())
	if err != nil {
		return err
	}
	childEdgeBindings, err := buildRemoteBindings(parentNode.BlueprintName, rel, pkFieldsForBlueprint(childBP))
	if err != nil {
		return err
	}

	// Each element adds its related record and a join record.
	if err := e.reserve(parentNode, rel.Name, 2*relationCount(rel, opts)); err != nil {
		return err
	}
	elements := relationElements(rel, opts)
	count := len(elements)

	for i, elem := range elements {
		childPath := appendRelationPath(relationPath, rel.Name)
		childNodeID := relationNodeIdentity(parentNodeID, rel.Name, i, count)
		origin := graph.Origin{Kind: graph.OriginManyToMany, From: parentNode, Relation: rel.Name, Index: i, Indexed: true}
		var childNode *graph.Node
		if elem.Use != nil {
			childNode, err = e.addProvidedNode(childBP, childNodeID, elem.Use, origin)
		} else {
			childNode, err = e.expandBlueprint(childBP, childNodeID, elem.Opts, nil, childPath, origin)
		}
		if err != nil {
			return err
		}

		joinBindings, parentBound, childBound, err := manyToManyBindings(joinBP, parentNode, childNode, rel, parentEdgeBindings, childEdgeBindings)
		if err != nil {
			return err
		}
		joinOrigin := graph.Origin{Kind: graph.OriginThrough, From: childNode, Relation: rel.Name}
		joinNode, err := e.expandBlueprint(joinBP, joinNodeIdentity(childNodeID, rel.ThroughBlueprint), elem.Through, joinBindings, appendJoinPath(childPath, rel.ThroughBlueprint), joinOrigin)
		if err != nil {
			return err
		}

		if !parentBound {
			e.graph.AddEdgeBindings(parentNode, joinNode, parentEdgeBindings)
		}
		if !childBound {
			e.graph.AddEdgeBindings(childNode, joinNode, childEdgeBindings)
		}
	}

	return nil
}

// manyToManyBindings finds the belongs-to relations of a join record that
// point at the parent or the related record over the foreign keys the
// many-to-many relation fills.
func manyToManyBindings(
	joinBP *BlueprintDef,
	parentNode, childNode *graph.Node,
	rel RelationDef,
	parentEdges, childEdges []graph.FieldBinding,
) (bindings map[string]boundParent, parentBound, childBound bool, err error) {
	bindings = make(map[string]boundParent)
	bind := func(joinRel RelationDef, node *graph.Node, edges []graph.FieldBinding) (bool, error) {
		if joinRel.RefBlueprint != node.BlueprintName {
			return false, nil
		}
		bound, ok, err := backReferenceBindings(joinBP.Name, joinRel, node, parentNode.BlueprintName, rel, edges)
		if err != nil || !ok {
			return false, err
		}
		bindings[joinRel.Name] = boundParent{node: node, edges: bound}
		return true, nil
	}

	for _, joinRel := range joinBP.Relations {
		if joinRel.Kind != BelongsTo {
			continue
		}
		ok, err := bind(joinRel, parentNode, parentEdges)
		if err != nil {
			return nil, false, false, err
		}
		if ok {
			parentBound = true
			continue
		}
		ok, err = bind(joinRel, childNode, childEdges)
		if err != nil {
			return nil, false, false, err
		}
		childBound = childBound || ok
	}
	return bindings, parentBound, childBound, nil
}

func relationOmitted(opts *OptionSet, relation string) bool {
	return opts != nil && opts.Omits[relation]
}

func relationEnabled(rel RelationDef, opts *OptionSet) bool {
	if rel.Required {
		return true
	}
	return relationUsed(opts, rel.Name) || relationLinked(opts, rel.Name) || relationReferenced(opts, rel.Name) || relationHasPredicate(rel, opts)
}

func relationLinked(opts *OptionSet, relation string) bool {
	if opts == nil {
		return false
	}
	_, ok := opts.Links[relation]
	return ok
}

// relationWhen evaluates the When predicate for a relation. An option-level When
// (from the When[T] option) takes precedence over a blueprint-level When (from
// the Relation.When field). If no When predicate is set, the function returns
// true (meaning expansion proceeds as normal based on Required/Optional).
func relationWhen(value any, rel RelationDef, opts *OptionSet) (bool, error) {
	if opts != nil && len(opts.Whens) > 0 {
		if fn, ok := opts.Whens[rel.Name]; ok {
			return fn(value)
		}
	}
	if rel.When != nil {
		return rel.When(value)
	}
	return true, nil
}

func nestedRelationOpts(opts *OptionSet, relation string) *OptionSet {
	if opts == nil {
		return nil
	}
	return opts.Refs[relation]
}

func relationReferenced(opts *OptionSet, relation string) bool {
	if opts == nil {
		return false
	}
	if _, ok := opts.Refs[relation]; ok {
		return true
	}
	_, ok := opts.Collections[relation]
	return ok
}

func usedRelationValue(opts *OptionSet, relation string) (any, bool) {
	if opts == nil {
		return nil, false
	}
	value, ok := opts.Uses[relation]
	return value, ok
}

func relationUsed(opts *OptionSet, relation string) bool {
	if opts == nil {
		return false
	}
	_, ok := opts.Uses[relation]
	return ok
}

func relationHasPredicate(rel RelationDef, opts *OptionSet) bool {
	if rel.When != nil {
		return true
	}
	if opts == nil {
		return false
	}
	_, ok := opts.Whens[rel.Name]
	return ok
}

// relationElements returns one entry per HasMany/ManyToMany child to expand.
// Per-element options given for the relation decide the number of children;
// otherwise the relation's Count does, each child built from its defaults. A
// zero Count (including the unset zero value) expands none; negative counts are
// rejected when the blueprint is registered.
func relationElements(rel RelationDef, opts *OptionSet) []ElementOptions {
	if opts != nil {
		if elements, ok := opts.Collections[rel.Name]; ok {
			return elements
		}
	}
	return make([]ElementOptions, rel.Count)
}

// relationCount returns the number of records relationElements expands,
// without allocating them.
func relationCount(rel RelationDef, opts *OptionSet) int {
	if opts != nil {
		if elements, ok := opts.Collections[rel.Name]; ok {
			return len(elements)
		}
	}
	return rel.Count
}

func (e *expander) belongsToNodeID(parentNodeID nodeIdentity, path relationPath, rel RelationDef, opts *OptionSet) nodeIdentity {
	if e.share == nil {
		return relationNodeIdentity(parentNodeID, rel.Name, 0, 1)
	}
	return e.share.belongsToNodeID(parentNodeID, path, rel, opts)
}

type batchShareState struct {
	candidates map[relationPath][]sharedCandidate
}

type sharedCandidate struct {
	nodeID nodeIdentity
	opts   *OptionSet
}

func newBatchShareState() *batchShareState {
	return &batchShareState{
		candidates: make(map[relationPath][]sharedCandidate),
	}
}

func (s *batchShareState) belongsToNodeID(parentNodeID nodeIdentity, path relationPath, rel RelationDef, opts *OptionSet) nodeIdentity {
	if !shareableOptionSet(opts) {
		return relationNodeIdentity(parentNodeID, rel.Name, 0, 1)
	}

	candidates := s.candidates[path]
	for _, candidate := range candidates {
		if reflect.DeepEqual(candidate.opts, opts) {
			return candidate.nodeID
		}
	}

	nodeID := sharedRelationNodeIdentity(path, len(candidates))
	s.candidates[path] = append(candidates, sharedCandidate{
		nodeID: nodeID,
		opts:   opts,
	})
	return nodeID
}

func sharedRelationNodeIdentity(path relationPath, index int) nodeIdentity {
	base := "shared." + string(path)
	if index == 0 {
		return nodeIdentity(base)
	}
	return nodeIdentity(fmt.Sprintf("%s#%d", base, index))
}

func shareableOptionSet(opts *OptionSet) bool {
	if opts == nil {
		return true
	}
	if len(opts.Uses) > 0 || len(opts.Links) > 0 || len(opts.WithFns) > 0 || len(opts.GenFns) > 0 || len(opts.Whens) > 0 || opts.Rand != nil {
		return false
	}
	for _, set := range opts.Sets {
		if !shareableValue(set.Value) {
			return false
		}
	}
	for _, refOpts := range opts.Refs {
		if !shareableOptionSet(refOpts) {
			return false
		}
	}
	for _, elements := range opts.Collections {
		for _, elem := range elements {
			if elem.Use != nil || !shareableOptionSet(elem.Opts) || !shareableOptionSet(elem.Through) {
				return false
			}
		}
	}
	return true
}

func shareableValue(value any) bool {
	if value == nil {
		return true
	}
	return reflect.ValueOf(value).Kind() != reflect.Func
}
