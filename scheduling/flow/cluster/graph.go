// Copyright 2016 The ksched Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Represents the scheduling flow graph.
// C++ file: https://github.com/camsas/firmament/blob/master/src/scheduling/flow/flow_graph.h

package cluster

import (
	"log"
	"math/rand"
	"time"

	"github.com/coreos/ksched/pkg/util/queue"
)

type FlowGraph struct {
	// Next node id to use
	nextID uint64
	// Unordered set of arcs in graph
	arcSet map[*FlowGraphArc]struct{}
	// Map of nodes keyed by nodeID
	nodeMap map[uint64]*FlowGraphNode
	// Queue storing the ids of the nodes we've previously removed.
	unusedIDs queue.FIFO

	// Behaviour flag - set as struct field rather than global static variable
	//                  since we will have only one instance of the FlowGraph.
	// If true the the flow graph will not generate node ids in order
	RandomizeNodeIDs bool
}

// Constructor equivalent in Go
// Must specify RandomizeNodeIDs flag
func NewFlowGraph(randomizeNodeIDs bool) *FlowGraph {
	fg := new(FlowGraph)
	fg.nextID = 1
	fg.unusedIDs = queue.NewFIFO()
	if randomizeNodeIDs {
		fg.RandomizeNodeIDs = true
		fg.PopulateUnusedIds(50)
	}
	return fg
}

// Adds an arc based on references to the src and dst nodes
func (fg *FlowGraph) AddArcNew(src, dst *FlowGraphNode) *FlowGraphArc {
	arc := NewArc(src.id, dst.id, src, dst)
	return arc
}

// Adds an arc based on ids of existing src and dst nodes in the graph
func (fg *FlowGraph) AddArcExisting(srcID, dstID uint64) *FlowGraphArc {
	srcNode := fg.nodeMap[srcID]
	if srcNode == nil {
		log.Fatalf("graph: AddArc error, src node with id:%d not found\n", srcID)
	}
	dstNode := fg.nodeMap[dstID]
	if dstNode == nil {
		log.Fatalf("graph: AddArc error, dst node with id:%d not found\n", dstID)
	}
	arc := NewArc(srcID, dstID, srcNode, dstNode)
	var s struct{}
	fg.arcSet[arc] = s
	srcNode.AddArc(arc)
	return arc
}

func (fg *FlowGraph) AddNode() *FlowGraphNode {
	id := fg.NextId()
	node := &FlowGraphNode{}
	if node == nil {
		log.Fatalf("graph: AddNode error, memory for node struct not allocated\n")
	}
	node.id = id
	// Insert into nodeMap, must not already be present
	_, ok := fg.nodeMap[id]
	if ok {
		log.Fatalf("graph: AddNode error, node with id:%d already present in nodeMap\n", id)
	}
	fg.nodeMap[id] = node
	return node
}

func (fg *FlowGraph) ChangeArc(arc *FlowGraphArc, capLowerBound, capUpperBound uint64, cost int64) {
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
	// Reuse this ID for later
	fg.unusedIDs.Push(&queue.Node{Value: node.id})
	// First remove all outgoing arcs
	for dstID, arc := range node.outgoingArcMap {
		if dstID != arc.dst {
			log.Fatalf("graph: DeleteNode error, dstID:%d != arc.dst:%d\n", dstID, arc.dst)
		}
		if node.id != arc.src {
			log.Fatalf("graph: DeleteNode error, node.id:%d != arc.src:%d\n", node.id, arc.src)
		}
		delete(arc.dstNode.incomingArcMap, arc.src)
		fg.DeleteArc(arc)
	}
	// Remove all incoming arcs
	for srcID, arc := range node.incomingArcMap {
		if srcID != arc.dst {
			log.Fatalf("graph: DeleteNode error, srcID:%d != arc.src:%d\n", srcID, arc.src)
		}
		if node.id != arc.dst {
			log.Fatalf("graph: DeleteNode error, node.id:%d != arc.dst:%d\n", node.id, arc.dst)
		}
		delete(arc.srcNode.outgoingArcMap, arc.dst)
		fg.DeleteArc(arc)
	}
	// Remove node from nodeMap
	delete(fg.nodeMap, node.id)
}

// Returns nil if arc not found
func (fg *FlowGraph) GetArc(src, dst *FlowGraphNode) *FlowGraphArc {
	if src == nil || dst == nil {
		log.Fatalf("graph: GetArc error, src:%v and dst:%v cannot be nil\n", src, dst)
	}
	return src.outgoingArcMap[dst.id]
}

// Returns the nextID to assign to a node
func (fg *FlowGraph) NextId() uint64 {
	if fg.RandomizeNodeIDs {
		if fg.unusedIDs.IsEmpty() {
			fg.PopulateUnusedIds(fg.nextID * 2)
		}
		newID := fg.unusedIDs.Pop().Value.(uint64)
		return newID
	}
	if fg.unusedIDs.IsEmpty() {
		newID := fg.nextID
		fg.nextID++
		return newID
	}
	newID := fg.unusedIDs.Pop().Value.(uint64)
	return newID
}

// Called if fg.RandomizeNodeIDs is true to generate a random shuffle of ids
func (fg *FlowGraph) PopulateUnusedIds(newNextID uint64) {
	t := time.Now().UnixNano()
	r := rand.New(rand.NewSource(t))
	ids := make([]uint64, 0)
	for i := fg.nextID; i < newNextID; i++ {
		ids = append(ids, i)
	}
	// Fisher-Yates shuffle
	for i := range ids {
		j := r.Intn(i + 1)
		ids[i], ids[j] = ids[j], ids[i]
	}
	for i := range ids {
		fg.unusedIDs.Push(&queue.Node{Value: ids[i]})
	}
	fg.nextID = newNextID
}
