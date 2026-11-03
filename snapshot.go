package seedling

import (
	"slices"
	"strings"

	"github.com/mhiro2/seedling/internal/graph"
)

// PlanReason tells why a plan holds a record.
type PlanReason string

const (
	// ReasonRoot marks a record the caller asked for.
	ReasonRoot PlanReason = "root"
	// ReasonBelongsTo marks a parent created or passed with [Use] for a
	// belongs-to relation.
	ReasonBelongsTo PlanReason = "belongs_to"
	// ReasonHasMany marks a child created for a has-many relation.
	ReasonHasMany PlanReason = "has_many"
	// ReasonManyToMany marks a related record created or passed with [Use]
	// for a many-to-many relation.
	ReasonManyToMany PlanReason = "many_to_many"
	// ReasonThrough marks the join record of a many-to-many relation.
	ReasonThrough PlanReason = "through"
)

// PlanNode describes one record of a plan.
type PlanNode struct {
	// ID is the record's opaque node ID, as in [Result.All].
	ID        string
	Blueprint string
	Table     string
	// Path is the relation path from the record's root, such as
	// task.project.company or company.users[1]. A record shared by several
	// records keeps the path of the first one that reached it.
	Path   string
	Reason PlanReason
	// Relation is the relation that created the record; empty for a root.
	Relation string
	// Provided marks an existing record passed with [Use], which the plan
	// binds but never inserts.
	Provided bool
	// Shared marks a record more than one other record depends on, such as a
	// belongs-to parent InsertMany shares or a [Scenario] record bound with
	// [Link] more than once.
	Shared bool
	// DependsOn lists the IDs of the records whose keys this record's
	// foreign keys take.
	DependsOn []string
}

// PlanSnapshot is a read-only description of a built plan, for asserting how
// many and which records a fixture creates:
//
//	plan := seedling.Build[Task](t)
//	if got := plan.Snapshot().Inserts(); got != 4 {
//	    t.Fatalf("task fixture inserts %d records, want 4:\n%s", got, plan.DebugString())
//	}
//
// Counts are of planned records and Insert callback calls, not of SQL
// statements, which only the callbacks know.
type PlanSnapshot struct {
	nodes []PlanNode
}

func newPlanSnapshot(g *graph.Graph) PlanSnapshot {
	if g == nil {
		return PlanSnapshot{}
	}
	order, err := g.TopoSort()
	if err != nil {
		order = g.Nodes()
		slices.SortFunc(order, func(a, b *graph.Node) int { return strings.Compare(a.ID, b.ID) })
	}
	nodes := make([]PlanNode, len(order))
	for i, node := range order {
		nodes[i] = PlanNode{
			ID:        node.ID,
			Blueprint: node.BlueprintName,
			Table:     node.Table,
			Path:      node.Path(),
			Reason:    PlanReason(node.Origin.Kind),
			Relation:  node.Origin.Relation,
			Provided:  node.IsProvided,
			Shared:    sharedNode(node),
			DependsOn: dependencyIDs(node),
		}
	}
	return PlanSnapshot{nodes: nodes}
}

// sharedNode reports whether more than one record depends on node, other
// than the records planned under it, such as its has-many children and the
// join records of its many-to-many relations. A record that binds several of
// its relations to node counts once.
func sharedNode(node *graph.Node) bool {
	dependents := make(map[*graph.Node]struct{})
	for _, edge := range node.Dependents() {
		if !plannedUnder(edge.Child, node) {
			dependents[edge.Child] = struct{}{}
		}
	}
	return len(dependents) > 1
}

// plannedUnder reports whether ancestor is on child's origin chain.
func plannedUnder(child, ancestor *graph.Node) bool {
	for n := child.Origin.From; n != nil; n = n.Origin.From {
		if n == ancestor {
			return true
		}
	}
	return false
}

func dependencyIDs(node *graph.Node) []string {
	edges := node.Dependencies()
	if len(edges) == 0 {
		return nil
	}
	ids := make([]string, len(edges))
	for i, edge := range edges {
		ids[i] = edge.Parent.ID
	}
	return ids
}

// Nodes returns every record of the plan in insert order.
func (s PlanSnapshot) Nodes() []PlanNode {
	nodes := slices.Clone(s.nodes)
	for i := range nodes {
		nodes[i].DependsOn = slices.Clone(nodes[i].DependsOn)
	}
	return nodes
}

// Len returns the number of records the plan holds, including records passed
// with [Use].
func (s PlanSnapshot) Len() int {
	return len(s.nodes)
}

// Inserts returns the number of Insert callback calls the plan makes, one per
// record it creates.
func (s PlanSnapshot) Inserts() int {
	n := 0
	for _, node := range s.nodes {
		if !node.Provided {
			n++
		}
	}
	return n
}

// InsertsByBlueprint returns the number of records the plan creates per
// blueprint name.
func (s PlanSnapshot) InsertsByBlueprint() map[string]int {
	return s.countInserts(func(node PlanNode) string { return node.Blueprint })
}

// InsertsByTable returns the number of records the plan creates per table.
// Records of a blueprint without a Table are counted under "".
func (s PlanSnapshot) InsertsByTable() map[string]int {
	return s.countInserts(func(node PlanNode) string { return node.Table })
}

func (s PlanSnapshot) countInserts(key func(PlanNode) string) map[string]int {
	counts := make(map[string]int)
	for _, node := range s.nodes {
		if !node.Provided {
			counts[key(node)]++
		}
	}
	return counts
}

// Shared returns the records more than one other record depends on.
func (s PlanSnapshot) Shared() []PlanNode {
	return s.filter(func(node PlanNode) bool { return node.Shared })
}

// Provided returns the existing records passed with [Use].
func (s PlanSnapshot) Provided() []PlanNode {
	return s.filter(func(node PlanNode) bool { return node.Provided })
}

func (s PlanSnapshot) filter(keep func(PlanNode) bool) []PlanNode {
	var out []PlanNode
	for _, node := range s.Nodes() {
		if keep(node) {
			out = append(out, node)
		}
	}
	return out
}
