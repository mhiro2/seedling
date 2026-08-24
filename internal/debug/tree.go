package debug

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mhiro2/seedling/internal/field"
	"github.com/mhiro2/seedling/internal/graph"
)

// TreeString returns a human-readable tree representation of the graph,
// starting from the root node.
func TreeString(g *graph.Graph) string {
	return renderGraph(g, writeNode)
}

// renderGraph walks the root first and then any node the root could not reach,
// so a partial result whose root never completed still lists every node it has.
func renderGraph(g *graph.Graph, write func(*strings.Builder, *graph.Node, string, string, map[string]bool)) string {
	if g == nil {
		return "(empty)"
	}
	entries := entryNodes(g)
	if len(entries) == 0 {
		return "(empty)"
	}

	var b strings.Builder
	seen := make(map[string]bool)
	for _, entry := range entries {
		if seen[entry.ID] {
			continue
		}
		write(&b, entry, "", "", seen)
	}
	return b.String()
}

// entryNodes returns the root, when the graph has one, followed by every other
// node ordered by ID so the rendering is deterministic.
func entryNodes(g *graph.Graph) []*graph.Node {
	nodes := g.Nodes()
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	root := g.Root()
	if root == nil {
		return nodes
	}
	entries := make([]*graph.Node, 0, len(nodes))
	entries = append(entries, root)
	for _, node := range nodes {
		if node.ID != root.ID {
			entries = append(entries, node)
		}
	}
	return entries
}

func writeNode(b *strings.Builder, node *graph.Node, prefix, prevID string, seen map[string]bool) {
	// Write this node's name.
	name := node.BlueprintName
	switch {
	case seen[node.ID]:
		name += " [reused]"
	case node.IsProvided:
		name += " [provided]"
	}
	if len(node.SetFields) > 0 {
		sorted := make([]string, len(node.SetFields))
		copy(sorted, node.SetFields)
		sort.Strings(sorted)
		name += fmt.Sprintf(" (Set: %s)", strings.Join(sorted, ", "))
	}
	b.WriteString(name)
	b.WriteByte('\n')

	if seen[node.ID] {
		return
	}
	seen[node.ID] = true

	next := adjacentNodes(node, prevID)
	for i, child := range next {
		isLastChild := i == len(next)-1

		if isLastChild {
			b.WriteString(prefix + "└─ ")
		} else {
			b.WriteString(prefix + "├─ ")
		}

		childPrefix := prefix
		if isLastChild {
			childPrefix += "   "
		} else {
			childPrefix += "│  "
		}

		writeNode(b, child, childPrefix, node.ID, seen)
	}
}

// ResultString returns a human-readable tree representation of execution results,
// showing each node with its PK value.
func ResultString(g *graph.Graph) string {
	return renderGraph(g, writeResultNode)
}

func writeResultNode(b *strings.Builder, node *graph.Node, prefix, prevID string, seen map[string]bool) {
	var name strings.Builder
	name.WriteString(node.BlueprintName)
	switch {
	case seen[node.ID]:
		name.WriteString(" [reused]")
	case node.IsProvided:
		name.WriteString(" [provided]")
	default:
		name.WriteString(" [inserted]")
	}

	// Show PK value if available.
	if node.Value != nil {
		for _, pkField := range node.PrimaryKeyFields() {
			if pkVal, err := field.GetField(node.Value, pkField); err == nil {
				fmt.Fprintf(&name, " %s=%v", pkField, pkVal)
			}
		}
	}

	b.WriteString(name.String())
	b.WriteByte('\n')

	if seen[node.ID] {
		return
	}
	seen[node.ID] = true

	next := adjacentNodes(node, prevID)
	for i, child := range next {
		isLastChild := i == len(next)-1

		if isLastChild {
			b.WriteString(prefix + "└─ ")
		} else {
			b.WriteString(prefix + "├─ ")
		}

		childPrefix := prefix
		if isLastChild {
			childPrefix += "   "
		} else {
			childPrefix += "│  "
		}

		writeResultNode(b, child, childPrefix, node.ID, seen)
	}
}

// DryRunString returns the planned INSERT execution order with FK assignments.
// Each step shows which table will be inserted and how FK fields are populated
// from referenced parent field values.
func DryRunString(g *graph.Graph) string {
	if g == nil {
		return "(empty)"
	}
	order, err := g.TopoSort()
	if err != nil {
		return fmt.Sprintf("(error: %v)", err)
	}
	if len(order) == 0 {
		return "(empty)"
	}

	var b strings.Builder
	for i, node := range order {
		table := node.Table
		if table == "" {
			table = node.BlueprintName
		}

		action := "INSERT INTO " + table
		if node.IsProvided {
			action = "SKIP " + table + " (provided)"
		}

		fmt.Fprintf(&b, "Step %d: %s (blueprint: %s)\n", i+1, action, node.BlueprintName)

		for _, edge := range node.Dependencies() {
			parentTable := edge.Parent.Table
			if parentTable == "" {
				parentTable = edge.Parent.BlueprintName
			}
			for _, binding := range edge.Bindings {
				fmt.Fprintf(&b, "        SET %s ← %s.%s\n", binding.ChildField, parentTable, binding.ParentField)
			}
		}
	}
	return b.String()
}

func adjacentNodes(node *graph.Node, prevID string) []*graph.Node {
	adjacent := make(map[string]*graph.Node, len(node.Dependencies())+len(node.Dependents()))

	for _, edge := range node.Dependencies() {
		if edge.Parent.ID == prevID {
			continue
		}
		adjacent[edge.Parent.ID] = edge.Parent
	}

	for _, edge := range node.Dependents() {
		if edge.Child.ID == prevID {
			continue
		}
		adjacent[edge.Child.ID] = edge.Child
	}

	nodes := make([]*graph.Node, 0, len(adjacent))
	for _, n := range adjacent {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}
