package planner

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
)

func (e *expander) expandRelation(
	bp *BlueprintDef,
	node *graph.Node,
	nodeID string,
	relationPath string,
	rel RelationDef,
	opts *OptionSet,
	bindings map[string]*graph.Node,
) error {
	if relationOmitted(opts, rel.Name) {
		return nil
	}

	expand, err := relationWhen(node.Value, rel, opts)
	if err != nil {
		return err
	}
	if !expand {
		return nil
	}

	bound, err := e.bindRelationNode(bp.Name, node, rel, bindings)
	if err != nil || bound {
		return err
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
	nodeID string,
	relationPath string,
	rel RelationDef,
	opts *OptionSet,
) error {
	usedNode, ok, err := e.providedBelongsToNode(rel, nodeID, opts)
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

	if !rel.Required {
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
	parentNodeID string,
	relationPath string,
	rel RelationDef,
	opts *OptionSet,
) error {
	if !rel.Required {
		return nil
	}

	parentEdgeBindings, err := buildLocalBindings(parentNode.BlueprintName, rel, parentNode.PrimaryKeyFields())
	if err != nil {
		return err
	}

	childBP, err := e.reg.LookupByName(rel.RefBlueprint)
	if err != nil {
		return fmt.Errorf("lookup has-many blueprint %q: %w", rel.RefBlueprint, err)
	}

	childOpts := nestedRelationOpts(opts, rel.Name)
	childBindings := hasManyBindings(childBP, parentNode, rel)
	count := relationCount(rel)

	for i := range count {
		childPath := appendRelationPath(relationPath, rel.Name)
		childNode, err := e.expandBlueprint(childBP, relationNodeID(parentNodeID, rel.Name, i, count), childOpts, childBindings, childPath)
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
	parentNodeID string,
	relationPath string,
	rel RelationDef,
	opts *OptionSet,
) error {
	if !rel.Required {
		return nil
	}
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

	childOpts := nestedRelationOpts(opts, rel.Name)
	count := relationCount(rel)

	for i := range count {
		childPath := appendRelationPath(relationPath, rel.Name)
		childNodeID := relationNodeID(parentNodeID, rel.Name, i, count)
		childNode, err := e.expandBlueprint(childBP, childNodeID, childOpts, nil, childPath)
		if err != nil {
			return err
		}

		joinBindings, parentBound, childBound := manyToManyBindings(joinBP, parentNode, childNode, rel)
		joinNode, err := e.expandBlueprint(joinBP, relationNodeID(childNodeID, rel.ThroughBlueprint, 0, 1), nil, joinBindings, appendRelationPath(childPath, rel.ThroughBlueprint))
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

func usedRelationValue(opts *OptionSet, relation string) (any, bool) {
	if opts == nil {
		return nil, false
	}
	value, ok := opts.Uses[relation]
	return value, ok
}

func relationCount(rel RelationDef) int {
	if rel.Count > 0 {
		return rel.Count
	}
	return 1
}

func relationNodeID(parentNodeID, relation string, index, count int) string {
	nodeID := fmt.Sprintf("%s.%s", parentNodeID, relation)
	if count <= 1 {
		return nodeID
	}
	return fmt.Sprintf("%s[%d]", nodeID, index)
}

func appendRelationPath(path, relation string) string {
	if path == "" {
		return relation
	}
	return path + "." + relation
}

func (e *expander) belongsToNodeID(parentNodeID, relationPath string, rel RelationDef, opts *OptionSet) string {
	if e.share == nil {
		return relationNodeID(parentNodeID, rel.Name, 0, 1)
	}
	return e.share.belongsToNodeID(parentNodeID, relationPath, rel, opts)
}

type batchShareState struct {
	candidates map[string][]sharedCandidate
}

type sharedCandidate struct {
	nodeID string
	opts   *OptionSet
}

func newBatchShareState() *batchShareState {
	return &batchShareState{
		candidates: make(map[string][]sharedCandidate),
	}
}

func (s *batchShareState) belongsToNodeID(parentNodeID, relationPath string, rel RelationDef, opts *OptionSet) string {
	if !shareableOptionSet(opts) {
		return relationNodeID(parentNodeID, rel.Name, 0, 1)
	}

	candidates := s.candidates[relationPath]
	for _, candidate := range candidates {
		if reflect.DeepEqual(candidate.opts, opts) {
			return candidate.nodeID
		}
	}

	nodeID := sharedRelationNodeID(relationPath, len(candidates))
	s.candidates[relationPath] = append(candidates, sharedCandidate{
		nodeID: nodeID,
		opts:   opts,
	})
	return nodeID
}

func sharedRelationNodeID(path string, index int) string {
	replacer := strings.NewReplacer(".", "__", "[", "_", "]", "")
	base := "shared." + replacer.Replace(path)
	if index == 0 {
		return base
	}
	return fmt.Sprintf("%s#%d", base, index)
}

func shareableOptionSet(opts *OptionSet) bool {
	if opts == nil {
		return true
	}
	if len(opts.Uses) > 0 || len(opts.WithFns) > 0 || len(opts.GenFns) > 0 || len(opts.Whens) > 0 || opts.Rand != nil {
		return false
	}
	for _, value := range opts.Sets {
		if !shareableValue(value) {
			return false
		}
	}
	for _, refOpts := range opts.Refs {
		if !shareableOptionSet(refOpts) {
			return false
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
