package graph

// Node represents a single entity in the dependency graph.
type Node struct {
	ID            string // unique identifier within the graph
	BlueprintName string // name of the blueprint this node was created from
	Table         string // database table name
	Value         any    // the struct value to be inserted
	IsProvided    bool   // true if the value was provided via Use (skip insert)

	// PKField is the Go struct field name of the primary key.
	PKField string

	// PKFields is the multi-column form of PKField.
	PKFields []string

	// SetFields tracks which fields were explicitly set via Set option.
	SetFields []string

	// edges
	dependencies []*Edge // edges to nodes this node depends on
	dependents   []*Edge // edges from nodes that depend on this node
}

// Edge represents a dependency between two nodes.
type Edge struct {
	Parent     *Node
	Child      *Node
	LocalField string // legacy single-column form of Bindings[0].ChildField
	Bindings   []FieldBinding
}

// FieldBinding maps one parent PK field onto one child FK field.
type FieldBinding struct {
	ParentField string
	ChildField  string
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
