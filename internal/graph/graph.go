package graph

// Graph represents a directed acyclic graph of entity nodes.
type Graph struct {
	root  *Node
	nodes map[string]*Node // keyed by Node.ID
}

// New creates a new empty graph.
func New() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
	}
}

// AddNode adds a node to the graph. The first node added becomes the root.
func (g *Graph) AddNode(n *Node) {
	if g.root == nil {
		g.root = n
	}
	g.nodes[n.ID] = n
}

// AddEdge adds a dependency edge: child depends on parent.
// The edge records which FK field on the child should be populated with the parent's PK.
func (g *Graph) AddEdge(parent, child *Node, localField string) {
	bindings := []FieldBinding{{
		ParentField: firstPKField(parent),
		ChildField:  localField,
	}}
	g.AddEdgeBindings(parent, child, bindings)
}

// AddEdgeBindings adds a dependency edge with one or more PK→FK bindings.
func (*Graph) AddEdgeBindings(parent, child *Node, bindings []FieldBinding) {
	e := &Edge{
		Parent:   parent,
		Child:    child,
		Bindings: append([]FieldBinding(nil), bindings...),
	}
	if len(bindings) > 0 {
		e.LocalField = bindings[0].ChildField
	}
	child.dependencies = append(child.dependencies, e)
	parent.dependents = append(parent.dependents, e)
}

// Root returns the root node of the graph.
func (g *Graph) Root() *Node {
	return g.root
}

// Node returns the node with the given ID, or nil.
func (g *Graph) Node(id string) *Node {
	return g.nodes[id]
}

// Nodes returns all nodes in the graph.
func (g *Graph) Nodes() []*Node {
	out := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	return out
}

func firstPKField(n *Node) string {
	fields := n.PrimaryKeyFields()
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
