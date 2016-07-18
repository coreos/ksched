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

// Cost model interface implemented by the cost model(s) implementations like coco
// and used by the flow graph

package costmodel

import (
	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/util"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/flowgraph"
)

type (
	Cost          int64
	CostModelType int64
)

//Enum for list of cost models supported
const (
	CostModelTrivial CostModelType = iota
	CostModelRandom
	CostModelSjf
	CostModelQuincy
	CostModelWhare
	CostModelCoco
	CostModelOctopus
	CostModelVoid
	CostModelNet
)

var (
	ClusterAggregatorEC = util.HashBytesToEquivClass([]byte("CLUSTER_AGG"))
)

// CostModeler provides APIs:
// - Tell the cost of arcs so that graph manager can apply them in graph.
// - Add, remove tasks, machines (resources) for the cost modeler to update
//   knowledge. It should be refactored out.
// - Stats related for cost calculation.
type CostModeler interface {

	// Get the cost from a task node to its unscheduled aggregator node.
	// The method should return a monotonically increasing value upon subsequent
	// calls. It is used to adjust the cost of leaving a task unscheduled after
	// each iteration.
	TaskToUnscheduledAggCost(types.TaskID) Cost
	UnscheduledAggToSinkCost(types.JobID) Cost

	// Get the cost of a preference arc from a task node to a resource node.

	TaskToResourceNodeCost(types.TaskID, types.ResourceID) Cost

	// Get the cost of an arc between two resource nodes.

	ResourceNodeToResourceNodeCost(source, destination *pb.ResourceDescriptor) Cost

	// Get the cost of an arc from a resource to the sink.
	LeafResourceNodeToSinkCost(types.ResourceID) Cost

	// Costs pertaining to preemption (i.e. already running tasks)
	TaskContinuationCost(types.TaskID) Cost
	TaskPreemptionCost(types.TaskID) Cost

	// Get the cost of an arc from a task node to an equivalence class node.
	TaskToEquivClassAggregator(types.TaskID, types.EquivClass) Cost

	// Get the cost of an arc from an equivalence class node to a resource node,
	// and free slots below this node in graph. The free slots information can be used to
	// optimize network flow algorithm.
	EquivClassToResourceNode(types.EquivClass, types.ResourceID) (Cost, uint64)

	// Get the cost and the capacity of an arc from an equivalence class node to
	// another equivalence class node.
	// @param tec1 the source equivalence class
	// @param tec2 the destination equivalence class
	EquivClassToEquivClass(tec1, tec2 types.EquivClass) (Cost, uint64)

	// Get the equivalence classes of a task.
	// @param task_id the task id for which to get the equivalence classes
	// @return a vector containing the task's equivalence classes
	GetTaskEquivClasses(types.TaskID) []types.EquivClass

	// Get the resource ids to which an equivalence class has arcs.
	// @param ec the equivalence class for which to get the resource ids
	GetOutgoingEquivClassPrefArcs(ec types.EquivClass) []types.ResourceID

	// Get the resource preference arcs of a task.
	// @param task_id the id of the task for which to get the preference arcs
	GetTaskPreferenceArcs(types.TaskID) []types.ResourceID

	// Get equivalence classes to which the outgoing arcs of an equivalence class
	// are pointing to.
	// @return a vectors of equivalence classes to which we have an outgoing arc.
	GetEquivClassToEquivClassesArcs(types.EquivClass) []types.EquivClass

	// Called by the flow_graph when a machine is added.
	AddMachine(*pb.ResourceTopologyNodeDescriptor)

	// Called by the flow graph when a task is submitted.
	AddTask(types.TaskID)

	// Called by the flow_graph when a machine is removed.
	RemoveMachine(types.ResourceID)

	RemoveTask(types.TaskID)

	// Gathers statistics during reverse traversal of resource topology (from
	// sink upwards). Called on pairs of connected nodes.
	GatherStats(accumulator, other *flowgraph.Node) *flowgraph.Node

	// The default Prepare action is a no-op. Cost models can override this if
	// they need to perform preparation actions before GatherStats is invoked.
	PrepareStats(accumulator *flowgraph.Node)

	// Generates updates for arc costs in the resource topology.
	UpdateStats(accumulator, other *flowgraph.Node) *flowgraph.Node

	// Handle to pull debug information from cost model; return string.
	DebugInfo() string

	DebugInfoCSV() string
}
