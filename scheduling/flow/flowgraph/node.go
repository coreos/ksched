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

	"github.com/coreos/ksched/pkg/types"
	pb "github.com/coreos/ksched/proto"
)

//Enum for flow node type
type NodeType int

const (
	RootTask NodeType = iota + 1
	ScheduledTask
	UnscheduledTask
	JobAggregator
	Sink
	EquivalenceClass
	Coordinator
	Machine
	NumaNode
	Socket
	Cache
	Core
	Pu
)

// Represents a node in the scheduling flow graph.
type Node struct {
	ID uint64
	// The supply of excess flow at this node. 0 for non-source/sink nodes
	Excess int64
	Type   NodeType
	// Comment for debugging purposes (used to label special nodes)
	Comment string

	// The descriptor of the task represented by this node.
	Task *pb.TaskDescriptor
	// TODO(malte): Not sure if these should be here, but they've got to go
	// somewhere.
	// The ID of the job that this task belongs to (if task node).
	JobID types.JobID

	// The ID of the resource that this node represents.
	ResourceID types.ResourceID
	// The descriptor of the resource that this node represents.
	ResourceDescriptor *pb.ResourceDescriptor
	// the ID of the equivalence class represented by this node.
	ecID types.EquivClass

	// Outgoing arcs from this node, keyed by destination node
	outgoingArcMap map[uint64]*Arc
	// Incoming arcs to this node, keyed by source node
	incomingArcMap map[uint64]*Arc
	// Field use to mark if the node has been visited in a graph traversal.
	// TODO: Why is this a uint32 in the original code
	visited uint32
}

// True indicates that an insert took place,
// False indicates the key was already present.
func insertIfNotPresent(m map[uint64]*Arc, k uint64, val *Arc) bool {
	_, ok := m[k]
	if !ok {
		m[k] = val
	}
	return !ok
}

func (n *Node) AddArc(arc *Arc) {
	//Arc must be outgoing from this node
	if arc.Src != n.ID {
		log.Fatalf("AddArc Error: arc.Src:%v != node:%v\n", arc.Src, n.ID)
	}
	//Add arc to outgoing arc map from current node, must not already be present
	if !insertIfNotPresent(n.outgoingArcMap, arc.Dst, arc) {
		log.Fatalf("AddArc Error: arc:%v already present in node:%v outgoingArcMap\n", arc, n.ID)
	}
	//Add arc to incoming arc map at dst node, must not already be present
	if !insertIfNotPresent(arc.DstNode.incomingArcMap, arc.Src, arc) {
		log.Fatalf("AddArc Error: arc:%v already present in node:%v incomingArcMap\n", arc, arc.DstNode.ID)
	}
}

func (n *Node) IsEquivalenceClassNode() bool {
	return n.Type == EquivalenceClass
}

func (n *Node) IsResourceNode() bool {
	return n.Type == Coordinator ||
		n.Type == Machine ||
		n.Type == NumaNode ||
		n.Type == Socket ||
		n.Type == Cache ||
		n.Type == Core ||
		n.Type == Pu
}

func (n *Node) IsTaskNode() bool {
	return n.Type == RootTask ||
		n.Type == ScheduledTask ||
		n.Type == UnscheduledTask
}

func (n *Node) IsTaskAssignedOrRunning() bool {
	t := n.Task
	if t == nil {
		log.Fatalf("TaskDescriptor pointer for node:%v is nil\n", n.ID)
	}
	return t.State == pb.TaskDescriptor_Assigned || t.State == pb.TaskDescriptor_Running
}

func TransformToResourceNodeType(rdPtr *pb.ResourceDescriptor) NodeType {
	// Using proto3 syntax
	resourceType := rdPtr.Type
	switch resourceType {
	case pb.ResourceDescriptor_ResourcePu:
		return Pu
	case pb.ResourceDescriptor_ResourceCore:
		return Core
	case pb.ResourceDescriptor_ResourceCache:
		return Cache
	case pb.ResourceDescriptor_ResourceNic:
		log.Fatalf("Node type not supported yet: %v", resourceType)
	case pb.ResourceDescriptor_ResourceDisk:
		log.Fatalf("Node type not supported yet: %v", resourceType)
	case pb.ResourceDescriptor_ResourceSsd:
		log.Fatalf("Node type not supported yet: %v", resourceType)
	case pb.ResourceDescriptor_ResourceMachine:
		return Machine
	case pb.ResourceDescriptor_ResourceLogical:
		log.Fatalf("Node type not supported yet: %v", resourceType)
	case pb.ResourceDescriptor_ResourceNumaNode:
		return NumaNode
	case pb.ResourceDescriptor_ResourceSocket:
		return Socket
	case pb.ResourceDescriptor_ResourceCoordinator:
		return Coordinator
	default:
		log.Fatalf("Unknown node type: %v", resourceType)
	}
	return -1
}
