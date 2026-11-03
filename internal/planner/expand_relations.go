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
	bindings map[string]*graph.Node,
) error {
	// A structural back-reference binds the child to the parent that created it
	// (e.g. an employee's belongs_to(department) inside department's has_many).
	// This FK must always be set, independent of When/Omit on the child relation,
	// otherwise the auto-generated child would be orphaned with a zero FK. A Use
	// or Ref on that relation would have to replace or rebuild the parent that
	// already exists, so it is rejected instead of silently dropped.
	bound, err := e.bindRelationNode(bp.Name, node, rel, bindings)
	if err != nil {
		return err
	}
	if bound {
		return boundRelationOverride(bp.Name, rel.Name, bindings[rel.Name].BlueprintName, opts)
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

func (e *expander) bindRelationNode(
	blueprint string,
	node *graph.Node,
	rel RelationDef,
	bindings map[string]*graph.Node,
) (bool, error) {
	if len(bindings) == 0 {
		return false, nil
	}

	boundNode, ok := bindings[rel.Name]
	if !ok {
		return false, nil
	}

	edgeBindings, err := buildLocalBindings(blueprint, rel, boundNode.PrimaryKeyFields())
	if err != nil {
		return false, err
	}
	e.graph.AddEdgeBindings(boundNode, node, edgeBindings)
	return true, nil
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
		usedNode, ok, err = e.providedBelongsToNode(rel, nodeID, opts)
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
	childNode, err := e.expandBlueprint(parentBP, e.belongsToNodeID(nodeID, childPath, rel, nestedRelationOpts(opts, rel.Name)), nestedRelationOpts(opts, rel.Name), nil, childPath)
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

	elements := relationElements(rel, opts)
	childBindings := hasManyBindings(childBP, parentNode, rel)
	count := len(elements)

	for i, elem := range elements {
		childPath := appendRelationPath(relationPath, rel.Name)
		childNode, err := e.expandBlueprint(childBP, relationNodeIdentity(parentNodeID, rel.Name, i, count), elem.Opts, childBindings, childPath)
		if err != nil {
			return err
		}
		if len(childBindings) == 0 {
			e.graph.AddEdgeBindings(parentNode, childNode, parentEdgeBindings)
		}
	}

	return nil
}

func hasManyBindings(childBP *BlueprintDef, parentNode *graph.Node, rel RelationDef) map[string]*graph.Node {
	parentFields := localFieldsForRelation(rel)
	bindings := make(map[string]*graph.Node)

	for _, childRel := range childBP.Relations {
		if childRel.Kind != BelongsTo {
			continue
		}
		if childRel.RefBlueprint != parentNode.BlueprintName {
			continue
		}
		if !sameFields(localFieldsForRelation(childRel), parentFields) {
			continue
		}
		bindings[childRel.Name] = parentNode
	}

	if len(bindings) == 0 {
		return nil
	}
	return bindings
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

	elements := relationElements(rel, opts)
	count := len(elements)

	for i, elem := range elements {
		childPath := appendRelationPath(relationPath, rel.Name)
		childNodeID := relationNodeIdentity(parentNodeID, rel.Name, i, count)
		var childNode *graph.Node
		if elem.Use != nil {
			childNode = e.addProvidedNode(childBP, childNodeID, elem.Use)
		} else {
			childNode, err = e.expandBlueprint(childBP, childNodeID, elem.Opts, nil, childPath)
			if err != nil {
				return err
			}
		}

		joinBindings, parentBound, childBound := manyToManyBindings(joinBP, parentNode, childNode, rel)
		joinNode, err := e.expandBlueprint(joinBP, joinNodeIdentity(childNodeID, rel.ThroughBlueprint), elem.Through, joinBindings, appendJoinPath(childPath, rel.ThroughBlueprint))
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

func manyToManyBindings(joinBP *BlueprintDef, parentNode, childNode *graph.Node, rel RelationDef) (map[string]*graph.Node, bool, bool) {
	parentFields := localFieldsForRelation(rel)
	childFields := remoteFieldsForRelation(rel)
	bindings := make(map[string]*graph.Node)
	var parentBound bool
	var childBound bool

	for _, joinRel := range joinBP.Relations {
		if joinRel.Kind != BelongsTo {
			continue
		}

		switch {
		case joinRel.RefBlueprint == parentNode.BlueprintName && sameFields(localFieldsForRelation(joinRel), parentFields):
			bindings[joinRel.Name] = parentNode
			parentBound = true
		case joinRel.RefBlueprint == childNode.BlueprintName && sameFields(localFieldsForRelation(joinRel), childFields):
			bindings[joinRel.Name] = childNode
			childBound = true
		}
	}

	if len(bindings) == 0 {
		return nil, parentBound, childBound
	}
	return bindings, parentBound, childBound
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
		return rel.When(value), nil
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
