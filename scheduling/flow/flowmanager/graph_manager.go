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
	"github.com/coreos/ksched/scheduling/flow/costmodel"
	"github.com/coreos/ksched/scheduling/flow/dimacs"
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

	cm          GraphChangeManager
	costModeler costmodel.CostModeler

	mu sync.Mutex

	// Resource and task mappings
	resourceToNode map[types.ResourceID]*flowgraph.Node
	taskToNode     map[types.TaskID]*flowgraph.Node
	// Map storing the running arc for every task that is running.
	taskToRunningArc map[types.TaskID]*flowgraph.Arc

	// Mapping storing flow graph node for each unscheduled aggregator.
	jobUnschedToNode map[types.JobID]*flowgraph.Node

	sinkNode *flowgraph.Node
}

func (gm *graphManager) JobCompleted(id types.JobID) {
	// We don't have to do anything else here. The task nodes have already been
	// removed.
	gm.removeUnscheduledAggNode(jobID)
}

func (gm *graphManager) TaskCompleted(id types.TaskID) uint64 {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	taskNode := gm.taskToNode[id]

	if gm.Preemption {
		// When we pin the task we reduce the capacity from the unscheduled
		// aggrator to the sink. Hence, we only have to reduce the capacity
		// when we support preemption.
		unschedAggNode := gm.jobUnschedToNode[taskNode.JobID]
		gm.updateUnscheduledAggNode(unschedAggNode, -1)
	}

	delete(gm.taskToRunningArc, id)
	nodeID := gm.removeTaskNode(taskNode)

	// NOTE: We do not remove the task from the cost_model because
	// HandleTaskFinalReport still needs to get the task's  equivalence classes.
	return nodeID
}

func (gm *graphManager) TaskMigrated(id types.TaskID, from, to types.ResourceID) {
	gm.TaskEvicted(id, from)
	gm.TaskScheduled(id, to)
}

func (gm *graphManager) TaskEvicted(id types.TaskID, rid types.ResourceID) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	taskNode := gm.taskToNode[id]
	taskNode.Type = flowgraph.UnscheduledTask

	arc := gm.taskToRunningArc[id]
	delete(gm.taskToRunningArc, id)
	gm.cm.DeleteArc(arc, dimacs.DelArcEvictedTask, "TaskEvicted: delete running arc")

	if !gm.Preemption {
		// If we're running with preemption disabled then increase the capacity from
		// the unscheduled aggregator to the sink because the task can now stay
		// unscheduled.
		unschedAggNode := gm.jobUnschedToNode[taskNode.JobID]
		// Increment capacity from unsched agg node to sink.
		gm.updateUnscheduledAggNode(unschedAggNode, 1)
	}
	// The task's arcs will be updated just before the next solver run.
}

func (gm *graphManager) TaskFailed(id types.TaskID) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	taskNode := gm.taskToNode[id]
	if gm.Preemption {
		// When we pin the task we reduce the capacity from the unscheduled
		// aggrator to the sink. Hence, we only have to reduce the capacity
		// when we support preemption.
		unschedAggNode := gm.jobUnschedToNode[taskNode.JobID]
		gm.updateUnscheduledAggNode(unschedAggNode, -1)
	}

	delete(gm.taskToRunningArc, id)
	gm.removeTaskNode(taskNode)
	gm.costModeler.RemoveTask(id)
}

func (gm *graphManager) TaskScheduled(id types.TaskID, rid types.ResourceID) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	taskNode := gm.taskToNode[id]
	taskNode.Type = flowgraph.ScheduledTask

	resNode := gm.resourceToNode[rid]
	gm.updateArcsForScheduledTask(taskNode, resNode)
}

// Private Methods
func (gm *graphManager) updateUnscheduledAggNode(unschedAggNode *flowgraph.Node, capDelta int64) {
	panic("not implemented")
}

func (gm *graphManager) addEquivClassNode(ec types.EquivClass) *flowgraph.Node {
	return nil
}

func (gm *graphManager) addResourceNode(rd *pb.ResourceDescriptor) *flowgraph.Node {
	return nil
}

// Updates the arc of a newly scheduled task.
// If we're running with preemption enabled then the method just adds/changes
// an arc to the resource node and updates the arc to the unscheduled agg to
// ave the premeption cost.
// If we're not running with preemption enabled then the method deletes the
// task's arcs and only adds a running arc.
//
// tn is the node of the task recently scheduled
// rn is the node of the resource to which the task has been scheduled
func (gm *graphManager) updateArcsForScheduledTask(tn *flowgraph.Node, rn *flowgraph.Node) {

}

// Adds to the graph all the node from the subtree rooted at rtnd_ptr.
// The method also correctly computes statistics for every new node (e.g.,
// num slots, num running tasks)
// @param rtnd the topology descriptor of the root node
func (gm *graphManager) addResourceTopologyDFS(rtnd *pb.ResourceTopologyNodeDescriptor) {

}

func (gm *graphManager) addTaskNode(jobID types.JobID, td *pb.TaskDescriptor) *flowgraph.Node {
	return nil
}

func (gm *graphManager) addUnscheduledAggNode(jobID types.JobID) *flowgraph.Node {
	return nil
}

func (gm *graphManager) capacityFromResNodeToParent(rd *pb.ResourceDescriptor) uint64 {
	return 0
}

func (gm *graphManager) pinTaskToNode(taskNode, resourceNode *flowgraph.Node) {

}

func (gm *graphManager) removeEquivClassNode(ecNode *flowgraph.Node) {

}

// Remove invalid preference arcs from node to equivalence class nodes.
// @param node the node for which to remove its invalid peference arcs
// to equivalence classes
// @param prefEcs node's current preferred equivalence classes
// @param changeType the type of the change
func (gm *graphManager) removeInvalidECPrefArcs(node *flowgraph.Node, prefEcs []types.EquivClass,
	changeType dimacs.ChangeType) {

}

// Remove invalid preference arcs from node to resource nodes.
// @param node the node for which to remove its invalid preference arcs to
// resources
// @param prefResources node's current preferred resources
// @param changeType the type of the change
func (gm *graphManager) removeInvalidPrefResArcs(node *flowgraph.Node,
	prefResources []types.ResourceID,
	changeType dimacs.ChangeType) {

}

func (gm *graphManager) removeResourceNode(resNode *flowgraph.Node) {

}

func (gm *graphManager) removeTaskNode(n *flowgraph.Node) uint64 {
	taskNodeID := n.ID

	// Increase the sink's excess and set this node's excess to zero.
	n.Excess = 0
	gm.sinkNode.Excess++
	delete(gm.taskToNode, types.TaskID(n.Task.Uid))
	gm.cm.DeleteNode(n, dimacs.DelTaskNode, "RemoveTaskNode")

	return taskNodeID
}

func (gm *graphManager) removeUnscheduledAggNode(jobID types.JobID) {

}

// Remove the resource topology rooted at res_node.
// @param resNode the root of the topology tree to remove
// @param pusRemoved set that gets updated whenever we remove a PU
func (gm *graphManager) traverseAndRemoveTopology(resNode *flowgraph.Node,
	pusRemoved map[uint64]struct{}) {

}
