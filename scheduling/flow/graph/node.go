// Represents a node in the scheduling flow graph.
// C++ file: https://github.com/camsas/firmament/blob/master/src/scheduling/flow/flow_graph_node.h

package graph

import (
	"log"

	pb "github.com/coreos/ksched/base/protofiles"
	t "github.com/coreos/ksched/base/types"
)

//Enum for flow node type
type FlowNodeType int

const (
	RootTask FlowNodeType = iota
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

type FlowGraphNode struct {
	id       uint64
	excess   int64
	nodeType FlowNodeType
	// TODO(malte): Not sure if these should be here, but they've got to go
	// somewhere.
	// The ID of the job that this task belongs to (if task node).
	jobID t.JobID
	// The ID of the resource that this node represents.
	resourceID t.ResourceID
	// The descriptor of the resource that this node represents.
	rdPtr *pb.ResourceDescriptor
	// The descriptor of the task represented by this node.
	tdPtr *pb.TaskDescriptor
	// the ID of the equivalence class represented by this node.
	ecID t.EquivClass
	// Free-form comment for debugging purposes (used to label special nodes)
	comment string
	// Outgoing arcs from this node, keyed by destination node
	outgoingArcMap map[uint64]*FlowGraphArc
	// Incoming arcs to this node, keyed by source node
	incomingArcMap map[uint64]*FlowGraphArc
	// Field use to mark if the node has been visited in a graph traversal.
	// TODO: Why is this a uint32 in the original code
	visited uint32
}

// True indicates that an insert took place,
// False indicates the key was already present.
func insertIfNotPresent(m map[uint64]*FlowGraphArc, k uint64, val *FlowGraphArc) bool {
	_, ok := m[k]
	if !ok {
		m[k] = val
	}
	return !ok
}

func (n *FlowGraphNode) AddArc(arc *FlowGraphArc) {
	//Arc must be outgoing from this node
	if arc.src != n.id {
		log.Fatalf("AddArc Error: arc.src:%v != node:%v\n", arc.src, n.id)
	}
	//Add arc to outgoing arc map from current node, must not already be present
	if !insertIfNotPresent(n.outgoingArcMap, arc.dst, arc) {
		log.Fatalf("AddArc Error: arc:%v already present in node:%v outgoingArcMap\n", arc, n.id)
	}
	//Add arc to incoming arc map at dst node, must not already be present
	if !insertIfNotPresent(arc.dstNode.incomingArcMap, arc.src, arc) {
		log.Fatalf("AddArc Error: arc:%v already present in node:%v incomingArcMap\n", arc, arc.dstNode.id)
	}
}

func (n *FlowGraphNode) IsEquivalenceClassNode() bool {
	return n.nodeType == EquivalenceClass
}

func (n *FlowGraphNode) IsResourceNode() bool {
	return n.nodeType == Coordinator ||
		n.nodeType == Machine ||
		n.nodeType == NumaNode ||
		n.nodeType == Socket ||
		n.nodeType == Cache ||
		n.nodeType == Core ||
		n.nodeType == Pu
}

func (n *FlowGraphNode) IsTaskNode() bool {
	return n.nodeType == RootTask ||
		n.nodeType == ScheduledTask ||
		n.nodeType == UnscheduledTask
}

func (n *FlowGraphNode) IsTaskAssignedOrRunning() bool {
	if n.tdPtr == nil {
		log.Fatalf("TaskDescriptor pointer for node:%v is nil\n", n.id)
	}
	return n.tdPtr.State == pb.TaskDescriptor_Assigned || n.tdPtr.State == pb.TaskDescriptor_Running
}

func (n *FlowGraphNode) TransformToResourceNodeType(rdPtr *pb.ResourceDescriptor) FlowNodeType {
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
