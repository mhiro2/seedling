package planner

import (
	"fmt"
	"strings"
)

var identitySegmentEscaper = strings.NewReplacer(
	"%", "%25",
	".", "%2E",
	"[", "%5B",
	"]", "%5D",
	"#", "%23",
)

// nodeIdentity is the planner's opaque, collision-free graph node key.
type nodeIdentity string

func (id nodeIdentity) String() string {
	return string(id)
}

type relationPath string

func rootNodeIdentity(blueprint string) nodeIdentity {
	return nodeIdentity(escapeIdentitySegment(blueprint))
}

func batchRootNodeIdentity(index int) nodeIdentity {
	return nodeIdentity(fmt.Sprintf("root[%d]", index))
}

func relationNodeIdentity(parent nodeIdentity, relation string, index, count int) nodeIdentity {
	nodeID := appendIdentitySegment(parent.String(), escapeIdentitySegment(relation))
	if count <= 1 {
		return nodeIdentity(nodeID)
	}
	return nodeIdentity(fmt.Sprintf("%s[%d]", nodeID, index))
}

func joinNodeIdentity(parent nodeIdentity, throughBlueprint string) nodeIdentity {
	return nodeIdentity(appendIdentitySegment(parent.String(), "%join:"+escapeIdentitySegment(throughBlueprint)))
}

func appendRelationPath(path relationPath, relation string) relationPath {
	return relationPath(appendIdentitySegment(string(path), escapeIdentitySegment(relation)))
}

func appendJoinPath(path relationPath, throughBlueprint string) relationPath {
	return relationPath(appendIdentitySegment(string(path), "%join:"+escapeIdentitySegment(throughBlueprint)))
}

func appendIdentitySegment(path, segment string) string {
	if path == "" {
		return segment
	}
	return path + "." + segment
}

func escapeIdentitySegment(segment string) string {
	return identitySegmentEscaper.Replace(segment)
}
