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

package flowgraph

import (
	"log"
	"math/rand"
	"time"

	"github.com/coreos/ksched/pkg/util/queue"
)

type NodeID uint64

type Graph struct {
	// Next node id to use
	nextID NodeID
	// Unordered set of arcs in graph
	arcSet map[*Arc]struct{}
	// Map of nodes keyed by nodeID
	nodeMap map[NodeID]*Node
	// Queue storing the ids of the nodes we've previously removed.
	unusedIDs queue.FIFO

	// Behaviour flag - set as struct field rather than global static variable
	//                  since we will have only one instance of the FlowGraph.
	// If true the the flow graph will not generate node ids in order
	RandomizeNodeIDs bool
}

// Constructor equivalent in Go
// Must specify RandomizeNodeIDs flag
func NewGraph(randomizeNodeIDs bool) *Graph {
	fg := &Graph{
		arcSet:  make(map[*Arc]struct{}),
		nodeMap: make(map[NodeID]*Node),
	}
	fg.nextID = 1
	fg.unusedIDs = queue.NewFIFO()
	if randomizeNodeIDs {
		fg.RandomizeNodeIDs = true
		fg.PopulateUnusedIds(50)
	}
	return fg
}

// Adds an arc based on references to the src and dst nodes
func (fg *Graph) AddArc(src, dst *Node) *Arc {
	srcID, dstID := src.ID, dst.ID

	srcNode := fg.nodeMap[srcID]
	if srcNode == nil {
		log.Fatalf("graph: AddArc error, src node with id:%d not found\n", srcID)
	}
	dstNode := fg.nodeMap[dstID]
	if dstNode == nil {
		log.Fatalf("graph: AddArc error, dst node with id:%d not found\n", dstID)
	}
	arc := NewArc(srcNode, dstNode)
	fg.arcSet[arc] = struct{}{}
	srcNode.AddArc(arc)
	return arc
}

func (fg *Graph) ChangeArc(arc *Arc, l, u uint64, c int64) {
	if l == 0 && u == 0 {
		delete(fg.arcSet, arc)
	}
	arc.CapLowerBound = l
	arc.CapUpperBound = u
	arc.Cost = c
}

func (fg *Graph) AddNode() *Node {
	id := fg.NextId()
	// fmt.Printf("AddNode called for id:%v\n", id)
	node := &Node{
		ID:             id,
		IncomingArcMap: make(map[NodeID]*Arc),
		OutgoingArcMap: make(map[NodeID]*Arc),
	}
	// Insert into nodeMap, must not already be present
	_, ok := fg.nodeMap[id]
	if ok {
		log.Fatalf("graph: AddNode error, node with id:%d already present in nodeMap\n", id)
	}
	fg.nodeMap[id] = node
	return node
}

func (fg *Graph) DeleteArc(arc *Arc) {
	delete(arc.SrcNode.OutgoingArcMap, arc.DstNode.ID)
	delete(arc.DstNode.IncomingArcMap, arc.SrcNode.ID)
	delete(fg.arcSet, arc)
}

func (fg *Graph) NumArcs() int {
	return len(fg.arcSet)
}

func (fg *Graph) Arcs() map[*Arc]struct{} {
	// TODO: we should return a copy? Only after concurrency pattern is known.
	return fg.arcSet
}

func (fg *Graph) Node(id NodeID) *Node {
	return fg.nodeMap[id]
}

func (fg *Graph) NumNodes() int {
	return len(fg.nodeMap)
}

func (fg *Graph) Nodes() map[NodeID]*Node {
	// TODO: we should return a copy? Only after concurrency pattern is known.
	return fg.nodeMap
}

func (fg *Graph) DeleteNode(node *Node) {
	// Reuse this ID for later
	fg.unusedIDs.Push(node.ID)
	// fmt.Printf("DeleteNode called for id:%v\n", node.ID)
	// First remove all outgoing arcs
	for dstID, arc := range node.OutgoingArcMap {
		if dstID != arc.Dst {
			log.Fatalf("graph: DeleteNode error, dstID:%d != arc.Dst:%d\n", dstID, arc.Dst)
		}
		if node.ID != arc.Src {
			log.Fatalf("graph: DeleteNode error, node.ID:%d != arc.Src:%d\n", node.ID, arc.Src)
		}
		delete(arc.DstNode.IncomingArcMap, arc.Src)
		fg.DeleteArc(arc)
	}
	// Remove all incoming arcs
	for srcID, arc := range node.IncomingArcMap {
		if srcID != arc.Dst {
			log.Fatalf("graph: DeleteNode error, srcID:%d != arc.Src:%d\n", srcID, arc.Src)
		}
		if node.ID != arc.Dst {
			log.Fatalf("graph: DeleteNode error, node.ID:%d != arc.Dst:%d\n", node.ID, arc.Dst)
		}
		delete(arc.SrcNode.OutgoingArcMap, arc.Dst)
		fg.DeleteArc(arc)
	}
	// Remove node from nodeMap
	// log.Printf("Deleting nodeID:%v from nodeMap\n", node.ID)
	delete(fg.nodeMap, node.ID)

}

// Returns nil if arc not found
func (fg *Graph) GetArc(src, dst *Node) *Arc {
	return src.OutgoingArcMap[dst.ID]
}

// Returns the nextID to assign to a node
func (fg *Graph) NextId() NodeID {
	if fg.RandomizeNodeIDs {
		if fg.unusedIDs.IsEmpty() {
			fg.PopulateUnusedIds(fg.nextID * 2)
		}
		return fg.unusedIDs.Pop().(NodeID)
	}
	if fg.unusedIDs.IsEmpty() {
		newID := fg.nextID
		fg.nextID++
		return newID
	}
	return fg.unusedIDs.Pop().(NodeID)
}

// Called if fg.RandomizeNodeIDs is true to generate a random shuffle of ids
func (fg *Graph) PopulateUnusedIds(newNextID NodeID) {
	t := time.Now().UnixNano()
	r := rand.New(rand.NewSource(t))
	ids := make([]NodeID, 0)
	for i := fg.nextID; i < newNextID; i++ {
		ids = append(ids, i)
	}
	// Fisher-Yates shuffle
	for i := range ids {
		j := r.Intn(i + 1)
		ids[i], ids[j] = ids[j], ids[i]
	}
	for i := range ids {
		fg.unusedIDs.Push(ids[i])
	}
	fg.nextID = newNextID
}
