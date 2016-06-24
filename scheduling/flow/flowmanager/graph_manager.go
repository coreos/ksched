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
	"log"
	"sync"

	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/util"
	"github.com/coreos/ksched/pkg/util/queue"
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
	sinkNode    *flowgraph.Node
	costModeler costmodel.CostModeler
	mu          sync.Mutex

	// Resource and task mappings
	resourceToNode map[types.ResourceID]*flowgraph.Node
	taskToNode     map[types.TaskID]*flowgraph.Node
	// Mapping storing flow graph node for each task equivalence class.
	taskECToNode map[types.EquivClass]*flowgraph.Node
	// Mapping storing flow graph node for each unscheduled aggregator.
	jobUnschedToNode map[types.JobID]*flowgraph.Node
	// Mapping storing the running arc for every task that is running.
	taskToRunningArc map[types.TaskID]*flowgraph.Arc
	nodeToParentNode map[*flowgraph.Node]*flowgraph.Node
	// Set of leaf resource IDs, i.e connected to sink node in flowgraph
	leafResourceIDs map[types.ResourceID]struct{}
	// The "node ID" for the job is currently the ID of the job's unscheduled node
	leafNodes map[uint64]struct{}

	dimacsStats *dimacs.ChangeStats
	// Counter updated whenever we compute topology statistics. The counter is
	// used as a marker in the resource topology traversal. It helps us to avoid
	// having to reset the visited state before each traversal.
	curTraversalCounter uint32
}

// TaskOrNode used by private methods
// This struct is use to pair a Task with a Node in the flow graph.
// If a task is not RUNNABLE, RUNNING or ASSIGNED then it's Node field will be null
type taskOrNode struct {
	Node     *flowgraph.Node
	TaskDesc *pb.TaskDescriptor
}

func (gm *graphManager) AddOrUpdateJobNodes(jobs []pb.JobDescriptor) {
	// For each job:
	// 1. Add/Update unscheduled agg node
	// 2. Add its root task to the queue

	q := queue.NewFIFO()
	markedNodes := make(map[uint64]struct{})

	for _, j := range jobs {
		jid := util.MustJobIDFromString(j.Uuid)
		// First add an unscheduled aggregator node for this job if none exists already.
		unschedAggNode := gm.jobUnschedToNode[jid]
		if unschedAggNode == nil {
			unschedAggNode = gm.addUnscheduledAggNode(jid)
		}

		rootTD := j.RootTask
		rootTaskNode := gm.taskToNode[types.TaskID(rootTD.Uid)]
		if rootTaskNode != nil {
			q.Push(&taskOrNode{Node: rootTaskNode, TaskDesc: rootTD})
			markedNodes[rootTaskNode.ID] = struct{}{}
			continue
		}

		if taskMustHaveNode(rootTD) {
			rootTaskNode = gm.addTaskNode(jid, rootTD)
			// Increment capacity from unsched agg node to sink.
			gm.updateUnscheduledAggNode(unschedAggNode, 1)

			q.Push(&taskOrNode{Node: rootTaskNode, TaskDesc: rootTD})
			markedNodes[rootTaskNode.ID] = struct{}{}
		} else {
			// We don't have to add a new node for the task.
			q.Push(&taskOrNode{TaskDesc: rootTD})
			// We can't mark the task as visited because we don't have
			// a node id for it. However, this is fine in practice because the
			// tasks cannot be a DAG and so we will never visit them again.
		}
	}

	// UpdateFlowGraph is responsible for making sure that the node_queue is empty upon completion.
	gm.updateFlowGraph(q, markedNodes)
}

func (gm *graphManager) AddResourceTopology(rtnd *pb.ResourceTopologyNodeDescriptor) {
	rd := rtnd.ResourceDesc
	gm.addResourceTopologyDFS(rtnd)
	// Progapate the capacity increase to the root of the topology.
	if rtnd.ParentId != "" {
		// We start from rtnd's parent because in AddResourceTopologyDFS we
		// already added an arc between rtnd and its parent.
		rID, err := util.ResourceIDFromString(rtnd.ParentId)
		if err != nil {
			log.Panic(err)
		}
		currNode := gm.nodeForResourceID(rID)
		runningTasksDelta := rd.NumRunningTasksBelow
		capacityToParent := gm.capacityFromResNodeToParent(rd)
		gm.updateResourceStatsUpToRoot(currNode, int64(capacityToParent), int64(rd.NumSlotsBelow), int64(runningTasksDelta))
	}
}

func (gm *graphManager) JobCompleted(id types.JobID) {
	// We don't have to do anything else here. The task nodes have already been
	// removed.
	gm.removeUnscheduledAggNode(id)
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
	taskNode.Type = flowgraph.NodeTypeUnscheduledTask

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

func (gm *graphManager) TaskKilled(id types.TaskID) {
	gm.TaskFailed(id)
}

func (gm *graphManager) TaskScheduled(id types.TaskID, rid types.ResourceID) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	taskNode := gm.taskToNode[id]
	taskNode.Type = flowgraph.NodeTypeScheduledTask

	resNode := gm.resourceToNode[rid]
	gm.updateArcsForScheduledTask(taskNode, resNode)
}

// Private Methods
func (gm *graphManager) addEquivClassNode(ec types.EquivClass) *flowgraph.Node {
	return nil
}

func (gm *graphManager) addResourceNode(rd *pb.ResourceDescriptor) *flowgraph.Node {
	comment := "AddResourceNode"
	if rd.FriendlyName != "" {
		comment = rd.FriendlyName
	}

	resourceNode := gm.cm.AddNode(flowgraph.TransformToResourceNodeType(rd),
		0, dimacs.AddResourceNode, comment)
	rID, err := util.ResourceIDFromString(rd.Uuid)
	if err != nil {
		panic(err)
	}
	resourceNode.ID = uint64(rID)
	resourceNode.ResourceDescriptor = rd
	// Insert mapping resource to node, must not already have mapping
	_, ok := gm.resourceToNode[rID]
	if ok {
		log.Panicf("gm:addResourceNode Mapping for rID:%v to resourceNode already present\n", rID)
	}
	gm.resourceToNode[rID] = resourceNode

	if resourceNode.Type == flowgraph.NodeTypePu {
		gm.leafNodes[uint64(rID)] = struct{}{}
		gm.leafResourceIDs[rID] = struct{}{}
	}
	return resourceNode
}

// Adds to the graph all the node from the subtree rooted at rtnd_ptr.
// The method also correctly computes statistics for every new node (e.g.,
// num slots, num running tasks)
// rtnd is the topology descriptor of the root node
func (gm *graphManager) addResourceTopologyDFS(rtnd *pb.ResourceTopologyNodeDescriptor) {

}

func (gm *graphManager) addTaskNode(jobID types.JobID, td *pb.TaskDescriptor) *flowgraph.Node {
	// TODO:
	// trace.traceGenerator.TaskSubmitted(td)
	gm.costModeler.AddTask(types.TaskID(td.Uid))
	taskNode := gm.cm.AddNode(flowgraph.NodeTypeUnscheduledTask, 1, dimacs.AddTaskNode, "AddTaskNode")
	taskNode.Task = td
	taskNode.JobID = jobID
	gm.sinkNode.Excess--
	// Insert mapping tast to node, must not already have mapping
	_, ok := gm.taskToNode[types.TaskID(td.Uid)]
	if ok {
		log.Panicf("gm:addTaskNode Mapping for taskID:%v to node already present\n", td.Uid)
	}
	gm.taskToNode[types.TaskID(td.Uid)] = taskNode
	return taskNode
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
// node the node for which to remove its invalid peference arcs
// to equivalence classes
// prefEcs is the node's current preferred equivalence classes
// changeType is the type of the change
func (gm *graphManager) removeInvalidECPrefArcs(node *flowgraph.Node, prefEcs []types.EquivClass,
	changeType dimacs.ChangeType) {

}

// Remove invalid preference arcs from node to resource nodes.
// node the node for which to remove its invalid preference arcs to
// resources
// prefResources is the node's current preferred resources
// changeType is the type of the change
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

// Remove the resource topology rooted at resourceNode.
// resNode the root of the topology tree to remove
// pusRemoved is the set that gets updated whenever we remove a PU
func (gm *graphManager) traverseAndRemoveTopology(resNode *flowgraph.Node,
	pusRemoved map[uint64]struct{}) {

}

// Updates the arc of a newly scheduled task.
// If we're running with preemption enabled then the method just adds/changes
// an arc to the resource node and updates the arc to the unscheduled agg to
// have the premeption cost.
// If we're not running with preemption enabled then the method deletes the
// task's arcs and only adds a running arc.
// taskNode is the node of the task recently scheduled
// resourceNode is the node of the resource to which the task has been
// scheduled
func (gm *graphManager) updateArcsForScheduledTask(taskNode, resourceNode *flowgraph.Node) {

}

// Adds the children tasks of the nodeless current task to the node queue.
// If a child task doesn't need to have a graph node (e.g., task is not
// RUNNABLE, RUNNING or ASSIGNED) then its TDOrNodeWrapper will only contain
// a pointer to its task descriptor.
func (gm *graphManager) updateChildrenTasks(td *pb.TaskDescriptor,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

func (gm *graphManager) updateEquivClassNode(ecNode *flowgraph.Node,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

// Updates an EC's outgoing arcs to other ECs. If the EC has new outgoing arcs
// to new EC nodes then the method appends them to the node_queue. Similarly,
// EC nodes that have not yet been marked are appended to the queue.

func (gm *graphManager) updateEquivToEquivArcs(ecNode *flowgraph.Node,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

// Updates the resource preference arcs an equivalence class has.
// ecNode is that node for which to update its preferences
func (gm *graphManager) updateEquivToResArcs(ecNode *flowgraph.Node,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

func (gm *graphManager) updateFlowGraph(nodeQueue queue.FIFO, markedNodes map[uint64]struct{}) {
}

func (gm *graphManager) updateResourceNode(resNode *flowgraph.Node,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

// Update resource related stats (e.g., arc capacities, num slots,
// num running tasks) on every arc/node up to the root resource.
func (gm *graphManager) updateResourceStatsUpToRoot(currNode *flowgraph.Node,
	capDelta, slotsDelta, runningTasksDelta int64) {
}

func (gm *graphManager) updateResourceTopologyDFS(rtnd *pb.ResourceTopologyNodeDescriptor) {
}

func (gm *graphManager) updateResOutgoingArcs(resNode *flowgraph.Node,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

// Updates the arc connecting a resource to the sink. It requires the resource
// to be a PU.
// resourceNode is the resource node for which to update its arc to the sink
func (gm *graphManager) updateResToSinkArc(resNode *flowgraph.Node) {
}

// Updates the cost on running arc of the task. If preemption is enabled then
// the method also updates the preemption cost on the arc to the unscheduled
// aggregator.
// NOTE: nodeQueue and markedNodes can be NULL as long as updatePreferences
// is false.
// taskNode is the node for which to update the arcs
// updatePreferences is true if the method should update the resource and
// equivalence preferences

func (gm *graphManager) updateRunningTaskNode(taskNode *flowgraph.Node,
	updatePreferences bool,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

// Updates the cost of the arc connecting a running task with its unscheduled
// aggregator.
// NOTE: This method should only be called when preemption is enabled.
// taskNode is the node for which to update the arc
func (gm *graphManager) updateRunningTaskToUnscheduledAggArc(taskNode *flowgraph.Node) {
}

func (gm *graphManager) updateTaskNode(taskNode *flowgraph.Node,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

// Updates a task's outgoing arcs to ECs. If the task has new outgoing arcs
// to new EC nodes then the method appends them to the nodeQueue. Similarly,
// EC nodes that have not yet been marked are appended to the queue.
func (gm *graphManager) updateTaskToEquivArcs(taskNode *flowgraph.Node,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

// Updates a task's preferences to resources.
func (gm *graphManager) updateTaskToResArcs(taskNode *flowgraph.Node,
	nodeQueue queue.FIFO,
	markedNodes map[uint64]struct{}) {
}

// Updates the arc from a task to its unscheduled aggregator. The method
// adds the unscheduled if it doesn't already exist.
// returns the unscheduled aggregator node
func (gm *graphManager) updateTaskToUnscheduledAggArc(taskNode *flowgraph.Node) *flowgraph.Node {
	return nil
}

// Adjusts the capacity of the arc connecting the unscheduled agg to the sink
// by cap_delta. The method also updates the cost if need be.
// unschedAggNode is the unscheduled aggregator node
// capDelta is the delta by which to change the capacity
func (gm *graphManager) updateUnscheduledAggNode(unschedAggNode *flowgraph.Node, capDelta int64) {
}

func (gm *graphManager) visitTopologyChildren(rtnd *pb.ResourceTopologyNodeDescriptor) {
}

// Small helper functions, might not really be needed
func (gm *graphManager) nodeForEquivClass(ec types.EquivClass) *flowgraph.Node {
	return nil
}

func (gm *graphManager) nodeForResourceID(resourceID types.ResourceID) *flowgraph.Node {
	return nil
}

func (gm *graphManager) nodeForTaskID(taskID types.TaskID) *flowgraph.Node {
	return nil
}

func (gm *graphManager) unschedAggNodeForJobID(jobID types.JobID) *flowgraph.Node {
	return nil
}

func taskMustHaveNode(td *pb.TaskDescriptor) bool {
	return td.State == pb.TaskDescriptor_Runnable ||
		td.State == pb.TaskDescriptor_Running ||
		td.State == pb.TaskDescriptor_Assigned
}
