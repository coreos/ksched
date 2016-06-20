// Represents an arc in the scheduling flow graph.
// C++ file: https://github.com/camsas/firmament/blob/master/src/scheduling/flow/flow_graph_arc.h
package graph

//Enum for flow arc type
type FlowArcType int

const (
	Other FlowNodeType = iota
	Running
)

type FlowGraphArc struct {
	src           uint64
	dst           uint64
	capLowerBound uint64
	capUpperBound uint64
	cost          int64
	srcNode       *FlowGraphNode
	dstNode       *FlowGraphNode
	typ           FlowArcType
}

// Constructor equivalent in go
func NewArc(srcID, dstID uint64, srcNode, dstNode *FlowGraphNode) *FlowGraphArc {
	a := new(FlowGraphArc)
	a.src = srcID
	a.dst = dstID
	a.srcNode = srcNode
	a.dstNode = dstNode
	return a
}
