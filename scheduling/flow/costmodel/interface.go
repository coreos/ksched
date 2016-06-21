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
	t "github.com/coreos/ksched/pkg/types"
	pb "github.com/coreos/ksched/proto"
	n "github.com/coreos/ksched/scheduling/flow/cluster"
	//"scheduling/common" // Not yet added
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

type CostModel interface {
	/**
	 * Get the cost from a task node to its unscheduled aggregator node.
	 * The method should return a monotonically increasing value upon subsequent
	 * calls. It is used to adjust the cost of leaving a task unscheduled after
	 * each iteration.
	 */
	TaskToUnscheduledAggCost(t.TaskID) Cost
	UnscheduledAggToSinkCost(t.JobID) Cost

	/**
	 * Get the cost of a preference arc from a task node to a resource node.
	 */
	TaskToResourceNodeCost(t.TaskID, t.ResourceID) Cost

	/**
	 * Get the cost of an arc between two resource nodes.
	 */
	ResourceNodeToResourceNodeCost(source, destination *pb.ResourceDescriptor) Cost

	/**
	 * Get the cost of an arc from a resource to the sink.
	 **/
	LeafResourceNodeToSinkCost(t.ResourceID) Cost

	// Costs pertaining to preemption (i.e. already running tasks)
	TaskContinuationCost(t.TaskID) Cost
	TaskPreemptionCost(t.TaskID) Cost

	/**
	 * Get the cost of an arc from a task node to an equivalence class node.
	 */
	TaskToEquivClassAggregator(t.TaskID) Cost

	/**
	 * Get the cost of an arc from an equivalence class node to a resource node.
	 */
	EquivClassToResourceNode(t.EquivClass, t.ResourceID) (Cost, uint64)

	/**
	 * Get the cost and the capacity of an arc from an equivalence class node to
	 * another equivalence class node.
	 * @param tec1 the source equivalence class
	 * @param tec2 the destination equivalence class
	 */
	EquivClassToEquivClass(tec1, tec2 t.EquivClass) (Cost, uint64)

	/**
	 * Get the equivalence classes of a task.
	 * @param task_id the task id for which to get the equivalence classes
	 * @return a vector containing the task's equivalence classes
	 */
	GetTaskEquivClasses(t.TaskID) []t.EquivClass

	/**
	 * Get the resource ids to which an equivalence class has arcs.
	 * @param tec the equivalence class for which to get the resource ids
	 */
	GetOutgoingEquivClassPrefArcs(t.ResourceID) []t.ResourceID

	/**
	 * Get the resource preference arcs of a task.
	 * @param task_id the id of the task for which to get the preference arcs
	 */
	GetTaskPreferenceArcs(t.TaskID) []t.ResourceID

	/**
	 * Get equivalence classes to which the outgoing arcs of an equivalence class
	 * are pointing to.
	 * @return a vectors of equivalence classes to which we have an outgoing arc.
	 */
	GetEquivClassToEquivClassesArcs(t.EquivClass) []t.EquivClass

	/**
	 * Called by the flow_graph when a machine is added.
	 */
	AddMachine(*pb.ResourceTopologyNodeDescriptor)

	/**
	 * Called by the flow graph when a task is submitted.
	 */
	AddTask(t.TaskID)

	/**
	 * Called by the flow_graph when a machine is removed.
	 */
	RemoveMachine(t.ResourceID)

	RemoveTask(t.TaskID)

	/**
	 * Gathers statistics during reverse traversal of resource topology (from
	 * sink upwards). Called on pairs of connected nodes.
	 */
	GatherStats(accumulator, other *n.FlowGraphNode)

	/**
	 * The default Prepare action is a no-op. Cost models can override this if
	 * they need to perform preparation actions before GatherStats is invoked.
	 */
	PrepareStats(accumulator *n.FlowGraphNode)

	/**
	 * Generates updates for arc costs in the resource topology.
	 */
	UpdateStats(accumulator, other *n.FlowGraphNode) *n.FlowGraphNode

	/**
	 * Handle to pull debug information from cost model; return string.
	 */
	DebugInfo() string

	DebugInfoCSV() string
}
