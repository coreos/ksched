// Represents the scheduling flow graph.
// C++ file: https://github.com/camsas/firmament/blob/master/src/scheduling/flow/flow_graph.h

package graph

import (
	"log"
	"math/rand"
	"time"

	q "github.com/coreos/ksched/misc/queue"
)

// If true the the flow graph will not generate node ids in order
var RandomizeNodeIds bool

type FlowGraph struct {
	// Next node id to use
	currentID uint64
	// Unordered set of arcs in graph
	arcSet map[*FlowGraphArc]interface{}
	// Map of nodes keyed by nodeID
	nodeMap map[uint64]*FlowGraphNode
	// Queue storing the ids of the nodes we've previously removed.
	unusedIDs q.Queue
}

// Constructor equivalent in Go
func NewFlowGraph() *FlowGraph {
	g := new(FlowGraph)
	g.currentID = 1
	if RandomizeNodeIds {
		g.PopulateUnusedIds(50)
	}
	return g
}

func (fg *FlowGraph) AddArcNew(src *FlowGraphNode, dst *FlowGraphNode) *FlowGraphArc {
	arc := NewFlowGraphArc(src.id, dst.id, src, dst)
	return arc
}

func (fg *FlowGraph) AddArcExisting(srcID uint64, dstID uint64) *FlowGraphArc {
	srcNode := fg.nodeMap[srcID]
	if srcNode == nil {
		log.Fatalf("FlowGraph::AddArc error, src node with id:%d not found\n", srcID)
	}
	dstNode := fg.nodeMap[dstID]
	if dstNode == nil {
		log.Fatalf("FlowGraph::AddArc error, dst node with id:%d not found\n", dstID)
	}
	arc := NewFlowGraphArc(srcID, dstID, srcNode, dstNode)
	fg.arcSet[arc] = true
	srcNode.AddArc(arc)
	return arc
}

func (fg *FlowGraph) AddNode() *FlowGraphNode {
	id := fg.NextId()
	node := new(FlowGraphNode)
	if node == nil {
		log.Fatalf("FlowGraph::AddNode error, memory for node struct not allocated\n")
	}
	node.id = id
	// Insert into nodeMap, must not already be present
	_, ok := fg.nodeMap[id]
	if ok {
		log.Fatalf("FlowGraph::AddNode error, node with id:%d already present in nodeMap\n", id)
	}
	fg.nodeMap[id] = node
	return node
}

func (fg *FlowGraph) ChangeArc(arc *FlowGraphArc, capLowerBound uint64, capUpperBound uint64, cost int64) {
	arc.capLowerBound = capLowerBound
	arc.capUpperBound = capUpperBound
	arc.cost = cost
}

func (fg *FlowGraph) ChangeArcCost(arc *FlowGraphArc, cost int64) {
	arc.cost = cost
}

func (fg *FlowGraph) DeleteArc(arc *FlowGraphArc) {
	delete(arc.srcNode.outgoingArcMap, arc.dstNode.id)
	delete(arc.dstNode.incomingArcMap, arc.srcNode.id)
	delete(fg.arcSet, arc)
}

func (fg *FlowGraph) DeleteNode(node *FlowGraphNode) {
	fg.unusedIDs.Push(&q.Node{Value: node.id})
	// First remove all outgoing arcs
	for dstID, arc := range node.outgoingArcMap {
		checkEquals(dstID, arc.dst)
		checkEquals(node.id, arc.src)
		delete(arc.dstNode.incomingArcMap, arc.src)
		fg.DeleteArc(arc)
	}
	// Remove all incoming arcs
	for srcID, arc := range node.incomingArcMap {
		checkEquals(srcID, arc.src)
		checkEquals(node.id, arc.dst)
		delete(arc.srcNode.outgoingArcMap, arc.dst)
		fg.DeleteArc(arc)
	}
	// Remove node from nodeMap
	delete(fg.nodeMap, node.id)
}

// Returns nil if arc not found
func (fg *FlowGraph) GetArc(src *FlowGraphNode, dst *FlowGraphNode) *FlowGraphArc {
	if src == nil || dst == nil {
		log.Fatalf("FlowGraph::GetArc error, src:%v and dst:%v cannot be nil\n", src, dst)
	}
	arc := src.outgoingArcMap[dst.id]
	return arc
}

// Returns the nextID to assign to a node
func (fg *FlowGraph) NextId() uint64 {
	if RandomizeNodeIds {
		if fg.unusedIDs.IsEmpty() {
			fg.PopulateUnusedIds(fg.currentID * 2)
		}
		newID := fg.unusedIDs.Pop().Value.(uint64)
		return newID
	} else {
		if fg.unusedIDs.IsEmpty() {
			newID := fg.currentID
			fg.currentID++
			return newID
		} else {
			newID := fg.unusedIDs.Pop().Value.(uint64)
			return newID
		}

	}
}

// Called if RandomizeNodeIds is true to generate a random shuffle of ids
func (fg *FlowGraph) PopulateUnusedIds(newCurrentID uint64) {
	t := time.Now().UnixNano()
	r := rand.New(rand.NewSource(t))
	ids := make([]uint64, 0)
	for i := fg.currentID; i < newCurrentID; i++ {
		ids = append(ids, i)
	}
	// Fisher-Yates shuffle
	for i := range ids {
		j := r.Intn(i + 1)
		ids[i], ids[j] = ids[j], ids[i]
	}
	for i, _ := range ids {
		fg.unusedIDs.Push(&q.Node{Value: ids[i]})
	}
	fg.currentID = newCurrentID
}

// Macro to error check equivalency
func checkEquals(a interface{}, b interface{}) {
	if a != b {
		log.Fatalf("Error:Not Equal %v != %v\n", a, b)
	}
}
