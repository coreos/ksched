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

package flowmanager

import (
	"sync"

	"github.com/coreos/ksched/pkg/types"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/flowgraph"
)

// NOTE: GraphManager uses GraphChangeManager to change the graph.
type GraphManager interface {
	AddOrUpdateJobNodes(jobs []pb.JobDescriptor)

	UpdateTimeDependentCosts(jobs []pb.JobDescriptor)

	// AddResourceTopology adds the entire resource topology tree. The method
	// also updates the statistics of the nodes up to the root resource.
	AddResourceTopology(topo pb.ResourceTopologyNodeDescriptor)

	UpdateResourceTopology(topo pb.ResourceTopologyNodeDescriptor)

	// TODO: ComputeTopologyStatistics(...)

	JobCompleted(id types.JobID)

	NodeBindingToSchedulingDeltas(
		taskNodeID, resourceNodeID uint64,
		taskBindings map[types.TaskID]types.ResourceID,
		deltas []pb.SchedulingDelta)

	SchedulingDeltasForPreemptedTasks(taskMapping map[uint64]uint64, rmap types.ResourceMap, deltas []pb.SchedulingDelta)

	// As a result of task state change, preferences change or
	// resource removal we may end up with unconnected equivalence
	// class nodes. This method makes sure they are removed.
	// We cannot end up with unconnected unscheduled agg nodes,
	// task or resource nodes.
	PurgeUnconnectedEquivClassNodes()

	//  Removes the entire resource topology tree rooted at rd. The method also
	//  updates the statistics of the nodes up to the root resource.
	RemoveResourceTopology(rd pb.ResourceDescriptor) (removedPUs []uint64)

	TaskCompleted(id types.TaskID) uint64
	TaskEvicted(id types.TaskID, rid types.ResourceID)
	TaskFailed(id types.TaskID)
	TaskKilled(id types.TaskID)
	TaskMigrated(id types.TaskID, from, to types.ResourceID)
	TaskScheduled(id types.TaskID, rid types.ResourceID)

	// Update each task's arc to its unscheduled aggregator. Moreover, for
	// running tasks we update their continuation costs.
	UpdateAllCostsToUnscheduledAggs()
}

type graphManager struct {
	Preemption bool

	mu         sync.Mutex
	taskToNode map[types.TaskID]*flowgraph.Node
	// Map storing the running arc for every task that is running.
	taskToRunningArc map[types.TaskID]*flowgraph.Arc
}

func (gm *graphManager) TaskCompleted(id types.TaskID) uint64 {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	taskNode := gm.taskToNode[id]

	if gm.Preemption {
		// When we pin the task we reduce the capacity from the unscheduled
		// aggrator to the sink. Hence, we only have to reduce the capacity
		// when we support preemption.
		gm.updateUnscheduledAggNode()
	}

	delete(gm.taskToRunningArc, id)
	nodeID := gm.removeTaskNode(taskNode)

	// NOTE: We do not remove the task from the cost_model because
	// HandleTaskFinalReport still needs to get the task's  equivalence classes.
	return nodeID
}

func (gm *graphManager) updateUnscheduledAggNode() {
	panic("not implemented")
}

func (gm *graphManager) removeTaskNode(n *flowgraph.Node) uint64 {
	panic("not implemented")
}
