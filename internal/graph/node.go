package graph

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mhiro2/seedling/internal/field"
)

// Node represents a single entity in the dependency graph.
type Node struct {
	ID            string // unique identifier within the graph
	BlueprintName string // name of the blueprint this node was created from
	Table         string // database table name
	Value         any    // the struct or pointer-to-struct value to be inserted
	IsProvided    bool   // true if the value was provided via Use (skip insert)

	// PKField is the Go struct field name of the primary key.
	PKField string

	// PKFields is the multi-column form of PKField.
	PKFields []string

	// SetFields tracks which fields were explicitly set via Set option.
	SetFields []string

	// Origin records why the planner created the node.
	Origin Origin

	// edges
	dependencies []*Edge // edges to nodes this node depends on
	dependents   []*Edge // edges from nodes that depend on this node
}

// OriginKind classifies how the planner reached a node.
type OriginKind string

const (
	OriginRoot       OriginKind = "root"
	OriginBelongsTo  OriginKind = "belongs_to"
	OriginHasMany    OriginKind = "has_many"
	OriginManyToMany OriginKind = "many_to_many"
	OriginThrough    OriginKind = "through"
)

// Origin records the relation that made the planner create a node. A node
// shared by several records keeps the origin of the first one that reached
// it.
type Origin struct {
	Kind OriginKind
	// From is the node whose relation created this one; nil for a root.
	From *Node
	// Relation is the relation of From that created the node. For a join
	// record it is the many-to-many relation the join belongs to.
	Relation string
	// Index is the element index of a has-many or many-to-many record, or the
	// position of a root among several roots, when Indexed is set.
	Index   int
	Indexed bool
}

// Path returns the relation path from the node's root to the node, such as
// task.project.company or company.users[1]. A join record ends in .through.
func (n *Node) Path() string {
	var segments []string
	for node := n; node != nil; node = node.Origin.From {
		segment := node.Origin.Relation
		switch {
		case node.Origin.Kind == OriginThrough:
			segment = "through"
		case node.Origin.From == nil:
			segment = node.BlueprintName
		}
		if node.Origin.Indexed {
			segment += "[" + strconv.Itoa(node.Origin.Index) + "]"
		}
		segments = append(segments, segment)
	}
	slices.Reverse(segments)
	return strings.Join(segments, ".")
}

// Edge represents a dependency between two nodes.
type Edge struct {
	Parent     *Node
	Child      *Node
	LocalField string // legacy single-column form of Bindings[0].ChildField
	Bindings   []FieldBinding
}

// FieldBinding maps one referenced parent field onto one child FK field.
type FieldBinding struct {
	ParentField string
	ChildField  string
	// Convert, when non-nil, turns the parent value into the child field's
	// type before assignment.
	Convert *field.Converter
}

// Dependencies returns the edges to nodes that this node depends on.
func (n *Node) Dependencies() []*Edge {
	return n.dependencies
}

// Dependents returns the edges from nodes that depend on this node.
func (n *Node) Dependents() []*Edge {
	return n.dependents
}

// PrimaryKeyFields returns the effective PK field list for the node.
func (n *Node) PrimaryKeyFields() []string {
	if len(n.PKFields) > 0 {
		out := make([]string, len(n.PKFields))
		copy(out, n.PKFields)
		return out
	}
	if n.PKField == "" {
		return nil
	}
	return []string{n.PKField}
}
