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
