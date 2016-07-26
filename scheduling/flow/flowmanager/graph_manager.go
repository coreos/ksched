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
	"strconv"
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
	LeafNodeIDs() map[flowgraph.NodeID]struct{}
	SinkNode() *flowgraph.Node
	GraphChangeManager() GraphChangeManager

	AddOrUpdateJobNodes(jobs []*pb.JobDescriptor)

	// TODO: do we really need this method? this is just a wrapper around AddOrUpdateJobNodes
	UpdateTimeDependentCosts(jobs []*pb.JobDescriptor)

	// AddResourceTopology adds the entire resource topology tree. The method
	// also updates the statistics of the nodes up to the root resource.
	AddResourceTopology(topo *pb.ResourceTopologyNodeDescriptor)

	UpdateResourceTopology(rtnd *pb.ResourceTopologyNodeDescriptor)

	// NOTE: The original interface passed in pointers to member functions of the costModeler
	// Now we just call the costModeler methods directly
	ComputeTopologyStatistics(node *flowgraph.Node)

	JobCompleted(id types.JobID)

	// Notes from xiang90: I modified the interface a little bit. Originally, the
	// interface would modify the passed in delta array by appending the scheduling delta.
	// This is not easy to be done in go. Rr it is not the common way to do it. We return
	// the delta instead. Users can just append it to the delta array themselves.
	NodeBindingToSchedulingDelta(taskNodeID, resourceNodeID flowgraph.NodeID,
		taskBindings map[types.TaskID]types.ResourceID) *pb.SchedulingDelta

	// NOTE(haseeb): Returns a slice of deltas for the user to append
	SchedulingDeltasForPreemptedTasks(taskMapping TaskMapping, rmap *types.ResourceMap) []pb.SchedulingDelta

	// As a result of task state change, preferences change or
	// resource removal we may end up with unconnected equivalence
	// class nodes. This method makes sure they are removed.
	// We cannot end up with unconnected unscheduled agg nodes,
	// task or resource nodes.
	PurgeUnconnectedEquivClassNodes()

	//  Removes the entire resource topology tree rooted at rd. The method also
	//  updates the statistics of the nodes up to the root resource.
	//  NOTE: Interface changed to return a slice of PUs to be removed by the caller
	RemoveResourceTopology(rd *pb.ResourceDescriptor) []flowgraph.NodeID

	TaskCompleted(id types.TaskID) flowgraph.NodeID
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
	// True if the preferences of a running task should be updated before each scheduling round
	UpdatePreferencesRunningTask bool
	Preemption                   bool
	MaxTasksPerPu                uint64
	flowSchedulingSolver         string

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
	leafNodeIDs map[flowgraph.NodeID]struct{}

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

func NewGraphManager(costModeler costmodel.CostModeler, leafResourceIDs map[types.ResourceID]struct{}, dimacsStats *dimacs.ChangeStats, maxTasksPerPu uint64) GraphManager {
	cm := NewChangeManager(dimacsStats)
	sinkNode := cm.AddNode(flowgraph.NodeTypeSink, 0, dimacs.AddSinkNode, "SINK")
	gm := &graphManager{dimacsStats: dimacsStats,
		leafResourceIDs:  leafResourceIDs,
		cm:               cm,
		costModeler:      costModeler,
		resourceToNode:   make(map[types.ResourceID]*flowgraph.Node),
		taskToNode:       make(map[types.TaskID]*flowgraph.Node),
		taskECToNode:     make(map[types.EquivClass]*flowgraph.Node),
		jobUnschedToNode: make(map[types.JobID]*flowgraph.Node),
		taskToRunningArc: make(map[types.TaskID]*flowgraph.Arc),
		nodeToParentNode: make(map[*flowgraph.Node]*flowgraph.Node),
		leafNodeIDs:      make(map[flowgraph.NodeID]struct{}),
		sinkNode:         sinkNode,
		MaxTasksPerPu:    maxTasksPerPu,
	}
	return gm
}

func (gm *graphManager) GraphChangeManager() GraphChangeManager {
	return gm.cm
}
func (gm *graphManager) SinkNode() *flowgraph.Node {
	return gm.sinkNode
}

func (gm *graphManager) LeafNodeIDs() map[flowgraph.NodeID]struct{} {
	return gm.leafNodeIDs
}

// AddOrUpdateJobNodes updates the flow graph by adding new unscheduled aggregator
// nodes for new jobs, and builds a queue of nodes(nodeQueue) in the graph
// that need to be updated(costs, capacities) via updateFlowGraph().
// For existing jobs it passes them on via the nodeQueue to be updated.
// jobs: The list of jobs that need updating
func (gm *graphManager) AddOrUpdateJobNodes(jobs []*pb.JobDescriptor) {

	nodeQueue := queue.NewFIFO()
	markedNodes := make(map[flowgraph.NodeID]struct{})
	// For each job:
	// 1. Add/Update its unscheduled agg node
	// 2. Add its root task to the nodeQueue
	for _, job := range jobs {
		//log.Printf("Graph Manager: AddOrUpdateJobNodes: job: %s\n", job.Name)
		jid := util.MustJobIDFromString(job.Uuid)
		// First add an unscheduled aggregator node for this job if none exists already.
		unschedAggNode := gm.jobUnschedToNode[jid]
		if unschedAggNode == nil {
			unschedAggNode = gm.addUnscheduledAggNode(jid)
		}

		rootTD := job.RootTask
		rootTaskNode := gm.nodeForTaskID(types.TaskID(rootTD.Uid))
		if rootTaskNode != nil {
			nodeQueue.Push(&taskOrNode{Node: rootTaskNode, TaskDesc: rootTD})
			markedNodes[rootTaskNode.ID] = struct{}{}
			continue
		}

		if taskNeedNode(rootTD) {
			//log.Printf("AddOrUpdateJobNode: task:%v needs node\n", rootTD.Name)
			rootTaskNode = gm.addTaskNode(jid, rootTD)
			// Increment capacity from unsched agg node to sink.
			gm.updateUnscheduledAggNode(unschedAggNode, 1)

			nodeQueue.Push(&taskOrNode{Node: rootTaskNode, TaskDesc: rootTD})
			markedNodes[rootTaskNode.ID] = struct{}{}
		} else {
			// We don't have to add a new node for the task.
			nodeQueue.Push(&taskOrNode{TaskDesc: rootTD})
			// We can't mark the task as visited because we don't have
			// a node id for it. However, this is fine in practice because the
			// tasks cannot be a DAG and so we will never visit them again.
		}
	}
	// UpdateFlowGraph is responsible for making sure that the node_queue is empty upon completion.
	gm.updateFlowGraph(nodeQueue, markedNodes)
}

// TODO: do we really need this method? this is just a wrapper around AddOrUpdateJobNodes
func (gm *graphManager) UpdateTimeDependentCosts(jobs []*pb.JobDescriptor) {
	gm.AddOrUpdateJobNodes(jobs)
}

// UpdateResourceTopology first updates(capacity, num running tasks) of the resource tree rooted at rtnd
// and then propagates those changes up to the root.
func (gm *graphManager) UpdateResourceTopology(rtnd *pb.ResourceTopologyNodeDescriptor) {
	// TODO(ionel): We don't currently update the arc costs. Moreover, we should
	// handle the case when a resource's parent changes.
	rd := rtnd.ResourceDesc
	oldCapacity := int64(gm.capacityFromResNodeToParent(rd))
	oldNumSlots := int64(rd.NumSlotsBelow)
	oldNumRunningTasks := int64(rd.NumRunningTasksBelow)
	gm.updateResourceTopologyDFS(rtnd)

	// Update towards the parent
	if rtnd.ParentId != "" {
		// We start from rtnd's parent because in UpdateResourceTopologyDFS
		// we already update the arc between rtnd and its parent.
		curNode := gm.nodeForResourceID(util.MustResourceIDFromString(rtnd.ParentId))
		capDelta := int64(gm.capacityFromResNodeToParent(rd)) - oldCapacity
		slotsDelta := int64(rd.NumSlotsBelow) - oldNumSlots
		runningTasksDelta := int64(rd.NumRunningTasksBelow) - oldNumRunningTasks
		gm.updateResourceStatsUpToRoot(curNode, capDelta, slotsDelta, runningTasksDelta)
	}
}

func (gm *graphManager) AddResourceTopology(rtnd *pb.ResourceTopologyNodeDescriptor) {
	rd := rtnd.ResourceDesc
	gm.addResourceTopologyDFS(rtnd)
	// Progapate the capacity increase to the root of the topology.
	if rtnd.ParentId != "" {
		// We start from rtnd's parent because in AddResourceTopologyDFS we
		// already added an arc between rtnd and its parent.
		rID := util.MustResourceIDFromString(rtnd.ParentId)
		currNode := gm.nodeForResourceID(rID)
		runningTasksDelta := rd.NumRunningTasksBelow
		capacityToParent := gm.capacityFromResNodeToParent(rd)
		gm.updateResourceStatsUpToRoot(currNode, int64(capacityToParent), int64(rd.NumSlotsBelow), int64(runningTasksDelta))
	}
}

func (gm *graphManager) NodeBindingToSchedulingDelta(tid, rid flowgraph.NodeID, tb map[types.TaskID]types.ResourceID) *pb.SchedulingDelta {
	taskNode := gm.cm.Graph().Node(tid)
	if !taskNode.IsTaskNode() {
		log.Panicf("unexpected non-task node %d\n", tid)
	}
	// Destination must be a PU node
	resNode := gm.cm.Graph().Node(rid)
	if resNode.Type != flowgraph.NodeTypePu {
		log.Panicf("unexpected non-pu node %d\n", rid)
	}

	task := taskNode.Task
	res := resNode.ResourceDescriptor

	// Is the source (task) already placed elsewhere?
	boundRes, ok := tb[types.TaskID(task.Uid)]
	if !ok {
		// Place the task.
		////log.Printf("flowmanager: place %v on %v", task.Uid, res.Uuid)
		sd := &pb.SchedulingDelta{
			Type:       pb.SchedulingDelta_PLACE,
			TaskId:     task.Uid,
			ResourceId: res.Uuid,
		}
		return sd
	}

	// Task already running somewhere.
	if boundRes != util.MustResourceIDFromString(res.Uuid) {
		////log.Printf("flowmanager: migrate %v from %v to %v", task.Uid, boundRes, res.Uuid)
		sd := &pb.SchedulingDelta{
			Type:       pb.SchedulingDelta_MIGRATE,
			TaskId:     task.Uid,
			ResourceId: res.Uuid,
		}
		return sd
	}

	// We were already scheduled here. Add back the task_id to the resource's running tasks list.
	res.CurrentRunningTasks = append(res.CurrentRunningTasks, task.Uid)
	return nil
}

func (gm *graphManager) SchedulingDeltasForPreemptedTasks(taskMappings TaskMapping, rmap *types.ResourceMap) []pb.SchedulingDelta {
	deltas := make([]pb.SchedulingDelta, 0)
	// Need to lock the map before iterating over it
	rmap.RLock()
	defer rmap.RUnlock()

	for _, resourceStatus := range rmap.UnsafeGet() {
		rd := resourceStatus.Descriptor
		runningTasks := rd.CurrentRunningTasks
		for _, taskID := range runningTasks {
			taskNode := gm.nodeForTaskID(types.TaskID(taskID))
			if taskNode == nil {
				// There's no node for the task => we don't need to generate
				// a PREEMPT delta because the task has finished.
				continue
			}

			_, ok := taskMappings[taskNode.ID]
			if !ok {
				// The task doesn't exist in the mappings => the task has been
				// preempted.
				////log.Printf("PREEMPTION: take %v off %v\n", taskID, resourceID)
				preemptDelta := pb.SchedulingDelta{
					TaskId:     uint64(taskID),
					ResourceId: rd.Uuid,
					Type:       pb.SchedulingDelta_PREEMPT,
				}
				deltas = append(deltas, preemptDelta)
			}
		}
		// We clear all the running tasks on the machine. The list is going to be
		// populated again in NodeBindingToSchedulingDeltas and
		// EventDrivenScheduler.
		// It is easier and less expensive to clear it and populate it back again
		// than making sure the preempted tasks are removed.
		rd.CurrentRunningTasks = make([]uint64, 0)

		// NOTE(haseeb): NodeBindingToSchedulingDeltas has been changed so,
		// the CurrentRunningTasks have to be repopulated by whoever calls
		// NodeBindingToSchedulingDeltas
	}
	return deltas
}

func (gm *graphManager) JobCompleted(id types.JobID) {
	// We don't have to do anything else here. The task nodes have already been
	// removed.
	gm.removeUnscheduledAggNode(id)
}

func (gm *graphManager) PurgeUnconnectedEquivClassNodes() {
	// NOTE: we could have a subgraph consisting of equiv class nodes.
	// They would likely not end up being removed in a single
	// PurgeUnconnectedEquivClassNodes call. However, this is fine
	// because we will finish removing all of them in future calls.
	for _, node := range gm.taskECToNode {
		if len(node.IncomingArcMap) == 0 {
			gm.removeEquivClassNode(node)
		}
	}
}

// Removes the resource, and all of it's children from the flowgraph
// Updates the capcaities, numRunningTasks and numSlotsBelow all the way
// from this node up to the root of the flow graph
func (gm *graphManager) RemoveResourceTopology(rd *pb.ResourceDescriptor) []flowgraph.NodeID {
	rID := util.MustResourceIDFromString(rd.Uuid)
	rNode := gm.nodeForResourceID(rID)
	if rNode == nil {
		log.Panic("gm/RemoveResourceTopology: resourceNode cannot be nil\n")
	}
	removedPUs := make([]flowgraph.NodeID, 0)
	capDelta := int64(0)
	// Delete the children nodes.
	for _, arc := range rNode.OutgoingArcMap {
		capDelta -= int64(arc.CapUpperBound)
		if arc.DstNode.ResourceID != 0 {
			removedPUs = append(removedPUs, gm.traverseAndRemoveTopology(arc.DstNode)...)
		}
	}
	// Propagate the stats update up to the root resource.
	gm.updateResourceStatsUpToRoot(rNode, capDelta, -int64(rNode.ResourceDescriptor.NumSlotsBelow), -int64(rNode.ResourceDescriptor.NumRunningTasksBelow))
	// Delete the node.
	if rNode.Type == flowgraph.NodeTypePu {
		removedPUs = append(removedPUs, rNode.ID)
	} else if rNode.Type == flowgraph.NodeTypeMachine {
		gm.costModeler.RemoveMachine(rNode.ResourceID)
	}
	gm.removeResourceNode(rNode)
	return removedPUs
}

func (gm *graphManager) TaskCompleted(id types.TaskID) flowgraph.NodeID {
	taskNode := gm.taskToNode[id]

	if gm.Preemption {
		// When we pin the task we reduce the capacity from the unscheduled
		// aggrator to the sink. Hence, we only have to reduce the capacity
		// when we support preemption.
		gm.updateUnscheduledAggNode(gm.unschedAggNodeForJobID(taskNode.JobID), -1)
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

func (gm *graphManager) TaskEvicted(taskID types.TaskID, rid types.ResourceID) {
	taskNode := gm.nodeForTaskID(taskID)
	taskNode.Type = flowgraph.NodeTypeUnscheduledTask

	arc, ok := gm.taskToRunningArc[taskID]
	if !ok {
		log.Panicf("gb/TaskEvicted: running arc mapping for taskID:%d must exist\n", taskID)
	}
	delete(gm.taskToRunningArc, taskID)
	gm.cm.DeleteArc(arc, dimacs.DelArcEvictedTask, "TaskEvicted: delete running arc")

	if !gm.Preemption {
		// If we're running with preemption disabled then increase the capacity from
		// the unscheduled aggregator to the sink because the task can now stay
		// unscheduled.
		jobID := util.MustJobIDFromString(taskNode.Task.JobID)
		unschedAggNode := gm.unschedAggNodeForJobID(jobID)
		// Increment capacity from unsched agg node to sink.
		gm.updateUnscheduledAggNode(unschedAggNode, 1)
	}
	// The task's arcs will be updated just before the next solver run.
}

func (gm *graphManager) TaskFailed(id types.TaskID) {
	taskNode := gm.taskToNode[id]
	if gm.Preemption {
		// When we pin the task we reduce the capacity from the unscheduled
		// aggrator to the sink. Hence, we only have to reduce the capacity
		// when we support preemption.
		unschedAggNode := gm.unschedAggNodeForJobID(taskNode.JobID)
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
	taskNode := gm.nodeForTaskID(id)
	taskNode.Type = flowgraph.NodeTypeScheduledTask

	resNode := gm.nodeForResourceID(rid)
	gm.updateArcsForScheduledTask(taskNode, resNode)
}

func (gm *graphManager) UpdateAllCostsToUnscheduledAggs() {
	for _, jobNode := range gm.jobUnschedToNode {
		if jobNode == nil {
			log.Panicf("gm/UpdateAllCostsToUnscheduledAggs: node for jobID:%v cannot be nil", jobNode)
		}
		for _, arc := range jobNode.IncomingArcMap {
			if arc.SrcNode.IsTaskAssignedOrRunning() {
				gm.updateRunningTaskNode(arc.SrcNode, false, nil, nil)
			} else {
				gm.updateTaskToUnscheduledAggArc(arc.SrcNode)
			}
		}
	}
}

// ComputeTopologyStatistics does a BFS traversal starting from the sink
// to gather and update the usage statistics for the resource topology
func (gm *graphManager) ComputeTopologyStatistics(node *flowgraph.Node) {
	////log.Printf("Updating resource statistics in flow graph\n")
	// XXX(ionel): The function only works correctly as long as the topology is a
	// tree. If the topology is a DAG then it does not work correctly! It does
	// not work in the DAG case because the function implements BFS. Hence,
	// we may pop a node of the queue and propagate its statistics via its incoming
	// arcs before we've received all the statistics at the node.
	toVisit := queue.NewFIFO()
	// We maintain a value that is used to mark visited nodes. Before each
	// visit we increment the mark to make sure that nodes visited in previous
	// traversal are not going to be treated as marked. By using the mark
	// variable we avoid having to reset the visited state of each node before
	// of a traversal.
	gm.curTraversalCounter++
	toVisit.Push(node)
	node.Visited = gm.curTraversalCounter
	for !toVisit.IsEmpty() {
		curNode := toVisit.Pop().(*flowgraph.Node)
		for _, incomingArc := range curNode.IncomingArcMap {
			if incomingArc.SrcNode.Visited != gm.curTraversalCounter {
				gm.costModeler.PrepareStats(incomingArc.SrcNode)
				toVisit.Push(incomingArc.SrcNode)
				incomingArc.SrcNode.Visited = gm.curTraversalCounter
			}
			incomingArc.SrcNode = gm.costModeler.GatherStats(incomingArc.SrcNode, curNode)
			// The update part might not be needed since that functionality has been moved
			// to the graph manager itself.
			incomingArc.SrcNode = gm.costModeler.UpdateStats(incomingArc.SrcNode, curNode)
		}
	}
}

// Private Methods
func (gm *graphManager) addEquivClassNode(ec types.EquivClass) *flowgraph.Node {
	ecNode := gm.cm.AddNode(flowgraph.NodeTypeEquivClass, 0, dimacs.AddEquivClassNode, "AddEquivClassNode")
	ecNode.EquivClass = &ec
	//log.Printf("Graph Manager: addEquivClassNode(%d) ec:%v\n", ecNode.ID, ec)
	// Insert mapping taskEquivalenceClass to node, must not already exist
	_, ok := gm.taskECToNode[ec]
	if ok {
		log.Panicf("gm:addEquivClassNode Mapping for ec:%v to node already present\n", ec)
	}
	gm.taskECToNode[ec] = ecNode
	return ecNode

}

func (gm *graphManager) addResourceNode(rd *pb.ResourceDescriptor) *flowgraph.Node {
	comment := "AddResourceNode"
	if rd.FriendlyName != "" {
		comment = rd.FriendlyName
	}

	resourceNode := gm.cm.AddNode(flowgraph.TransformToResourceNodeType(rd),
		0, dimacs.AddResourceNode, comment)
	rID := util.MustResourceIDFromString(rd.Uuid)
	resourceNode.ResourceID = rID
	resourceNode.ResourceDescriptor = rd
	// Insert mapping resource to node, must not already have mapping
	_, ok := gm.resourceToNode[rID]
	if ok {
		log.Panicf("gm:addResourceNode Mapping for rID:%v to resourceNode already present\n", rID)
	}
	gm.resourceToNode[rID] = resourceNode

	if resourceNode.Type == flowgraph.NodeTypePu {
		gm.leafNodeIDs[resourceNode.ID] = struct{}{}
		gm.leafResourceIDs[rID] = struct{}{}
	}
	return resourceNode
}

// Adds to the graph all the node from the subtree rooted at rtnd_ptr.
// The method also correctly computes statistics for every new node (e.g.,
// num slots, num running tasks)
// rtnd is the topology descriptor of the root node
func (gm *graphManager) addResourceTopologyDFS(rtnd *pb.ResourceTopologyNodeDescriptor) {
	// Steps:
	// 1) Add new resource node and connect it to the sink if the new node is a
	// PU node.
	// 2) Add the node's subtree.
	// 3) Connect the node to its parent.

	// Not doing any nil checks. Will just panic
	rd := rtnd.ResourceDesc
	rID := util.MustResourceIDFromString(rd.Uuid)
	resourceNode := gm.nodeForResourceID(rID)

	addedNewResNode := false
	if resourceNode == nil {
		addedNewResNode = true
		resourceNode = gm.addResourceNode(rd)
		if resourceNode.Type == flowgraph.NodeTypePu {
			gm.updateResToSinkArc(resourceNode)
			if rd.NumSlotsBelow == 0 {
				rd.NumSlotsBelow = uint64(gm.MaxTasksPerPu)
				if rd.NumRunningTasksBelow == 0 {
					rd.NumRunningTasksBelow = uint64(len(rd.CurrentRunningTasks))
				}
			}
		} else {
			if resourceNode.Type == flowgraph.NodeTypeMachine {
				// TODO: gm.traceGenerator.AddMachine(rd);
				gm.costModeler.AddMachine(rtnd)
			}
			rd.NumSlotsBelow = 0
			rd.NumRunningTasksBelow = 0
		}
	} else {
		rd.NumSlotsBelow = 0
		rd.NumRunningTasksBelow = 0
		// NOTE: This comment seems to be an issue with their coordinator implementation
		// maybe not relevant to our purposes.
		// TODO(ionel): The method continues even if we already had a node for the
		// "new" resources. This is because the coordinator ends up calling twice
		// RegisterResource for the same resource. Uncomment the LOG(FATAL) once
		// the coordinator is fixed.
		// (see https://github.com/ms705/firmament/issues/41)
		// LOG(FATAL) << "Resource node for resource: " << res_id
		//            << " already exists";
	}

	gm.visitTopologyChildren(rtnd)
	// If we reach the root
	if rtnd.ParentId == "" {
		if rd.Type != pb.ResourceDescriptor_ResourceCoordinator {
			log.Panicf("A resource node that is not a coordinator must have a parent")
		}
		return
	}

	if addedNewResNode {
		// Connect the node to the parent
		pID := util.MustResourceIDFromString(rtnd.ParentId)
		parentNode := gm.nodeForResourceID(pID)

		// Insert mapping to parentNode, must not already have a parent
		_, ok := gm.nodeToParentNode[resourceNode]
		if ok {
			log.Panicf("gm:AddResourceTopologyDFS Mapping for resourceNode:%v to parent already present\n", rd.Uuid)
		}
		gm.nodeToParentNode[resourceNode] = parentNode

		gm.cm.AddArc(parentNode, resourceNode,
			0, gm.capacityFromResNodeToParent(rd),
			int64(gm.costModeler.ResourceNodeToResourceNodeCost(parentNode.ResourceDescriptor, rd)),
			flowgraph.ArcTypeOther, dimacs.AddArcBetweenRes, "AddResourceTopologyDFS")
	}

}

func (gm *graphManager) addTaskNode(jobID types.JobID, td *pb.TaskDescriptor) *flowgraph.Node {
	// TODO:
	// trace.traceGenerator.TaskSubmitted(td)
	gm.costModeler.AddTask(types.TaskID(td.Uid))
	taskNode := gm.cm.AddNode(flowgraph.NodeTypeUnscheduledTask, 1, dimacs.AddTaskNode, "AddTaskNode")
	//log.Printf("Graph Manager: addTaskNode: name (%s)\n", td.Name)
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
	comment := "UNSCHED_AGG_for_" + strconv.FormatInt(int64(jobID), 10)
	unschedAggNode := gm.cm.AddNode(flowgraph.NodeTypeJobAggregator, 0, dimacs.AddUnschedJobNode, comment)
	// Insert mapping jobUnscheduled to node, must not already have mapping
	_, ok := gm.jobUnschedToNode[jobID]
	if ok {
		log.Panicf("gm:addUnscheduledAggNode Mapping for unscheduled jobID:%v to node already present\n", jobID)
	}
	gm.jobUnschedToNode[jobID] = unschedAggNode
	return unschedAggNode
}

func (gm *graphManager) capacityFromResNodeToParent(rd *pb.ResourceDescriptor) uint64 {
	if gm.Preemption {
		return rd.NumSlotsBelow
	}
	return rd.NumSlotsBelow - rd.NumRunningTasksBelow
}

// Pins the task (taskNode) to the resource (resourceNode).
// This ensures that the task can only be scheduled on that particular resource(machine,core etc).
// It does this by removing all arcs from this task node that do not point to the desired resource node.
// If an arc from the task node to the resource node does not already exist then a new arc will be added.
// This arc is the running arc, indicating where this particular will run and it's cost is assigned as a TaskContinuationCost
// from the cost model.
func (gm *graphManager) pinTaskToNode(taskNode, resourceNode *flowgraph.Node) {
	addedRunningArc := false
	lowBoundCapacity := uint64(1)
	// TODO: Address the lower capacity issue on custom solvers, see original

	for dstNodeID, arc := range taskNode.OutgoingArcMap {
		// Delete any arc not pointing to the desired resource node
		if dstNodeID != resourceNode.ID {
			// TODO(ionel): This doesn't correctly compute the type of changes. The
			// arcs we are deleting can point to unscheduled or equiv classes as well.
			gm.cm.DeleteArc(arc, dimacs.DelArcTaskToEquivClass, "PinTaskNode")
			continue
		}

		// This preference arc connects the same nodes as the running arc. Hence,
		// we just transform it into the running arc.
		addedRunningArc = true
		newCost := int64(gm.costModeler.TaskContinuationCost(types.TaskID(taskNode.Task.Uid)))
		arc.Type = flowgraph.ArcTypeRunning
		gm.cm.ChangeArc(arc, lowBoundCapacity, 1, newCost, dimacs.ChgArcRunningTask, "PinTaskToNode: transform to running arc")

		// Insert mapping for Task to RunningArc, must not already exist
		_, ok := gm.taskToRunningArc[types.TaskID(taskNode.Task.Uid)]
		if ok {
			log.Panicf("gm:pintTaskToNode Mapping for taskID:%v to running arc already present\n", taskNode.Task.Uid)
		}
		gm.taskToRunningArc[types.TaskID(taskNode.Task.Uid)] = arc
	}

	// Decrement capacity from unsched agg node to sink.
	gm.updateUnscheduledAggNode(gm.unschedAggNodeForJobID(taskNode.JobID), -1)
	if !addedRunningArc {
		// Add a single arc from the task to the resource node
		newCost := int64(gm.costModeler.TaskContinuationCost(types.TaskID(taskNode.Task.Uid)))
		newArc := gm.cm.AddArc(taskNode, resourceNode, lowBoundCapacity, 1, newCost, flowgraph.ArcTypeRunning, dimacs.AddArcRunningTask, "PinTaskToNode: add running arc")

		// Insert mapping for Task to RunningArc, must not already exist
		_, ok := gm.taskToRunningArc[types.TaskID(taskNode.Task.Uid)]
		if ok {
			log.Panicf("gm:pintTaskToNode Mapping for taskID:%v to running arc already present\n", taskNode.Task.Uid)
		}
		gm.taskToRunningArc[types.TaskID(taskNode.Task.Uid)] = newArc
	}

}

func (gm *graphManager) removeEquivClassNode(ecNode *flowgraph.Node) {
	delete(gm.taskECToNode, *ecNode.EquivClass)
	gm.cm.DeleteNode(ecNode, dimacs.DelEquivClassNode, "RemoveEquivClassNode")
}

// Remove invalid preference arcs from node to equivalence class nodes.
// node: the node for which to remove its invalid peference arcs
// to equivalence classes
// prefEcs: is the node's current preferred equivalence classes
// changeType: is the type of the change
func (gm *graphManager) removeInvalidECPrefArcs(node *flowgraph.Node, prefEcs []types.EquivClass, changeType dimacs.ChangeType) {
	// Make a set of the preferred equivalence classes
	prefECSet := make(map[types.EquivClass]struct{})
	for _, ec := range prefEcs {
		prefECSet[types.EquivClass(ec)] = struct{}{}
	}
	var toDelete []*flowgraph.Arc

	//log.Printf("Graph Manager: removeInvalidECPrefArcs: prefECSet:%v\n", prefECSet)

	// For each arc, check if the preferredEC is actually an EC node and that it's not in the preferences slice(prefEC)
	// If yes, remove that arc
	for _, arc := range node.OutgoingArcMap {
		ecPtr := arc.DstNode.EquivClass
		if ecPtr == nil {
			continue
		}
		prefEC := *ecPtr
		if _, ok := prefECSet[prefEC]; ok {
			continue
		}
		////log.Printf("Deleting no-longer-current arc to EC:%v", prefEC)
		toDelete = append(toDelete, arc)
	}

	for _, arc := range toDelete {
		gm.cm.DeleteArc(arc, changeType, "RemoveInvalidECPrefArcs")
	}
}

// Remove invalid preference arcs from node to resource nodes.
// node the node for which to remove its invalid preference arcs to resources
// prefResources: is the node's current preferred resources
// changeType: is the type of the change
func (gm *graphManager) removeInvalidPrefResArcs(node *flowgraph.Node, prefResources []types.ResourceID, changeType dimacs.ChangeType) {
	// Make a set of the preferred resources
	prefResSet := make(map[types.ResourceID]struct{})
	for _, rID := range prefResources {
		prefResSet[types.ResourceID(rID)] = struct{}{}
	}
	toDelete := make([]*flowgraph.Arc, 0)

	// For each arc, check if the dst node is actually a preferred resource node and that it's not in the preferred resource slice
	// If yes, remove that arc
	for _, arc := range node.OutgoingArcMap {
		rID := arc.DstNode.ResourceID
		if rID == 0 {
			continue
		}
		if _, ok := prefResSet[rID]; !ok {
			////log.Printf("Deleting no-longer-current arc to resource:%v", rID)
			toDelete = append(toDelete, arc)
		}
	}

	for _, arc := range toDelete {
		gm.cm.DeleteArc(arc, changeType, "RemoveInvalidResPrefArcs")
	}
}

func (gm *graphManager) removeResourceNode(resNode *flowgraph.Node) {
	if _, ok := gm.nodeToParentNode[resNode]; !ok {
		////log.Printf("Warning: Removing root resource node\n")
	}
	delete(gm.nodeToParentNode, resNode)
	delete(gm.leafNodeIDs, resNode.ID)
	delete(gm.leafResourceIDs, resNode.ResourceID)
	delete(gm.resourceToNode, resNode.ResourceID)
	gm.cm.DeleteNode(resNode, dimacs.DelResourceNode, "RemoveResourceNode")
}

func (gm *graphManager) removeTaskNode(n *flowgraph.Node) flowgraph.NodeID {
	taskNodeID := n.ID

	// Increase the sink's excess and set this node's excess to zero.
	n.Excess = 0
	gm.sinkNode.Excess++
	delete(gm.taskToNode, types.TaskID(n.Task.Uid))
	gm.cm.DeleteNode(n, dimacs.DelTaskNode, "RemoveTaskNode")

	return taskNodeID
}

func (gm *graphManager) removeUnscheduledAggNode(jobID types.JobID) {
	unschedAggNode := gm.unschedAggNodeForJobID(jobID)
	if unschedAggNode == nil {
		log.Panicf("gm/removeUnscheduledAggNode: unschedAggNode for jobID:%v cannot be nil\n", jobID)
	}
	if _, ok := gm.jobUnschedToNode[jobID]; !ok {
		log.Panicf("gm/removeUnscheduledAggNode: unscheduled jobID:%v has no aggregator node\n", jobID)
	}
	gm.cm.DeleteNode(unschedAggNode, dimacs.DelUnschedJobNode, "RemoveUnscheduledAggNode")
}

// Remove the resource topology rooted at resourceNode.
// resNode: The root of the topology tree to remove
// returns: The set of PUs that need to be removed by the caller of this function
func (gm *graphManager) traverseAndRemoveTopology(resNode *flowgraph.Node) []flowgraph.NodeID {
	removedPUs := make([]flowgraph.NodeID, 0)
	for _, arc := range resNode.OutgoingArcMap {
		if arc.DstNode.ResourceID != 0 {
			// The arc is pointing to a resource node.
			removedPUs = append(removedPUs, gm.traverseAndRemoveTopology(arc.DstNode)...)
		}
	}
	if resNode.Type == flowgraph.NodeTypePu {
		removedPUs = append(removedPUs, resNode.ID)
	} else if resNode.Type == flowgraph.NodeTypeMachine {
		gm.costModeler.RemoveMachine(resNode.ResourceID)
	}
	gm.removeResourceNode(resNode)
	return removedPUs
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
	if !gm.Preemption {
		gm.pinTaskToNode(taskNode, resourceNode)
		return
	}

	// With preemption we do not remove any old arcs. We only add/change a running arc to
	// the resource.
	taskID := types.TaskID(taskNode.Task.Uid)
	newCost := int64(gm.costModeler.TaskContinuationCost(taskID))
	runningArc := gm.taskToRunningArc[taskID]

	if runningArc != nil {
		// The running arc points to the same destination as a preference arc.
		// We just modify the preference arc because the graph doesn't currently
		// support multi-arcs.
		runningArc.Type = flowgraph.ArcTypeRunning
		gm.cm.ChangeArc(runningArc, 0, 1, newCost, dimacs.ChgArcRunningTask, "UpdateArcsForScheduledTask: transform to running arc")
		gm.updateRunningTaskToUnscheduledAggArc(taskNode)
		return
	}

	// No running arc was found
	runningArc = gm.cm.AddArc(taskNode, resourceNode, 0, 1, newCost,
		flowgraph.ArcTypeRunning, dimacs.AddArcRunningTask, "UpdateArcsForScheduledTask: add running arc")
	// Insert mapping for task to running arc, must not already exist
	_, ok := gm.taskToRunningArc[taskID]
	if ok {
		log.Panicf("gm:updateArcsForScheduledTask Mapping for tID:%v to running arc already present\n", taskID)
	}
	gm.taskToRunningArc[taskID] = runningArc
	gm.updateRunningTaskToUnscheduledAggArc(taskNode)
}

// NOTE(haseeb): This functions modifies the input queue and map parameters, which is contrary to Go style
// Adds the children tasks of the nodeless current task to the node queue.
// If a child task doesn't need to have a graph node (e.g., task is not
// RUNNABLE, RUNNING or ASSIGNED) then its taskOrNode struct will only contain
// a pointer to its task descriptor.
func (gm *graphManager) updateChildrenTasks(td *pb.TaskDescriptor, nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	// We do actually need to push tasks even if they are already completed,
	// failed or running, since they may have children eligible for
	// scheduling.
	//log.Printf("Updating children of task:%v\n", td.Name)
	//log.Printf("Length of children:%v\n", len(td.Spawned))
	for _, childTask := range td.Spawned {
		childTaskNode := gm.nodeForTaskID(types.TaskID(childTask.Uid))
		//log.Printf("Updating child task:%v\n", childTask.Name)

		// If childTaskNode does not have a marked node
		if childTaskNode != nil {
			if _, ok := markedNodes[childTaskNode.ID]; !ok {
				nodeQueue.Push(&taskOrNode{Node: childTaskNode, TaskDesc: childTask})
				markedNodes[childTaskNode.ID] = struct{}{}
			}
			continue
		}

		// ChildTask has no node
		if !taskNeedNode(childTask) {
			//log.Printf("Child task:%v does not need node\n", childTask.Name)
			nodeQueue.Push(&taskOrNode{Node: nil, TaskDesc: childTask})
			continue
		}

		// ChildTask needs a node
		jobID := util.MustJobIDFromString(childTask.JobID)
		childTaskNode = gm.addTaskNode(jobID, childTask)
		// Increment capacity from unsched agg node to sink.
		gm.updateUnscheduledAggNode(gm.unschedAggNodeForJobID(jobID), 1)
		nodeQueue.Push(&taskOrNode{Node: childTaskNode, TaskDesc: childTask})
		markedNodes[childTaskNode.ID] = struct{}{}
	}
}

func (gm *graphManager) updateEquivClassNode(ecNode *flowgraph.Node, nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	gm.updateEquivToEquivArcs(ecNode, nodeQueue, markedNodes)
	gm.updateEquivToResArcs(ecNode, nodeQueue, markedNodes)
}

// Updates an EC's outgoing arcs to other ECs. If the EC has new outgoing arcs
// to new EC nodes then the method appends them to the node_queue. Similarly,
// EC nodes that have not yet been marked are appended to the queue.
func (gm *graphManager) updateEquivToEquivArcs(ecNode *flowgraph.Node, nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	prefECs := gm.costModeler.GetEquivClassToEquivClassesArcs(*ecNode.EquivClass)
	// Empty slice means no preferences
	if len(prefECs) == 0 {
		gm.removeInvalidECPrefArcs(ecNode, prefECs, dimacs.DelArcBetweenEquivClass)
		return
	}

	for _, prefEC := range prefECs {
		prefECNode := gm.nodeForEquivClass(prefEC)
		if prefECNode == nil {
			prefECNode = gm.addEquivClassNode(prefEC)
		}

		cost, capUpper := gm.costModeler.EquivClassToEquivClass(*ecNode.EquivClass, prefEC)
		prefECArc := gm.cm.Graph().GetArc(ecNode, prefECNode)

		if prefECArc == nil {
			// Create arc if it doesn't exist
			gm.cm.AddArc(ecNode, prefECNode, 0, capUpper, int64(cost), flowgraph.ArcTypeOther, dimacs.AddArcBetweenEquivClass, "UpdateEquivClassNode")
		} else {
			gm.cm.ChangeArc(prefECArc, prefECArc.CapLowerBound, capUpper, int64(cost), dimacs.ChgArcBetweenEquivClass, "UpdateEquivClassNode")
		}

		if _, ok := markedNodes[prefECNode.ID]; !ok {
			// Add the EC node to the queue if it hasn't been marked yet.
			markedNodes[prefECNode.ID] = struct{}{}
			nodeQueue.Push(&taskOrNode{Node: prefECNode, TaskDesc: prefECNode.Task})
		}
	}
	gm.removeInvalidECPrefArcs(ecNode, prefECs, dimacs.DelArcBetweenEquivClass)
}

// Updates the resource preference arcs an equivalence class has.
// ecNode is that node for which to update its preferences
func (gm *graphManager) updateEquivToResArcs(ecNode *flowgraph.Node,
	nodeQueue queue.FIFO,
	markedNodes map[flowgraph.NodeID]struct{}) {

	prefResources := gm.costModeler.GetOutgoingEquivClassPrefArcs(*ecNode.EquivClass)
	// Empty slice means no preferences
	if len(prefResources) == 0 {
		gm.removeInvalidPrefResArcs(ecNode, prefResources, dimacs.DelArcEquivClassToRes)
		return
	}

	for _, prefRID := range prefResources {
		prefResNode := gm.nodeForResourceID(prefRID)
		// The resource node should already exist because the cost models cannot
		// prefer a resource before it is added to the graph.
		if prefResNode == nil {
			log.Panicf("gm/updateEquivToResArcs: preferred resource node cannot be nil")
		}

		cost, capUpper := gm.costModeler.EquivClassToResourceNode(*ecNode.EquivClass, prefRID)
		prefResArc := gm.cm.Graph().GetArc(ecNode, prefResNode)

		if prefResArc == nil {
			// Create arc if it doesn't exist
			gm.cm.AddArc(ecNode, prefResNode, 0, capUpper, int64(cost), flowgraph.ArcTypeOther, dimacs.AddArcEquivClassToRes, "UpdateEquivToResArcs")
		} else {
			gm.cm.ChangeArc(prefResArc, prefResArc.CapLowerBound, capUpper, int64(cost), dimacs.ChgArcEquivClassToRes, "UpdateEquivToResArcs")
		}

		if _, ok := markedNodes[prefResNode.ID]; !ok {
			// Add the res node to the queue if it hasn't been marked yet.
			markedNodes[prefResNode.ID] = struct{}{}
			nodeQueue.Push(&taskOrNode{Node: prefResNode, TaskDesc: prefResNode.Task})
		}
	}
	gm.removeInvalidPrefResArcs(ecNode, prefResources, dimacs.DelArcEquivClassToRes)
}

func (gm *graphManager) updateFlowGraph(nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	for !nodeQueue.IsEmpty() {
		taskOrNode := nodeQueue.Pop().(*taskOrNode)
		node := taskOrNode.Node
		task := taskOrNode.TaskDesc
		switch {
		case node == nil:
			// We're handling a task that doesn't have an associated flow graph node.
			gm.updateChildrenTasks(task, nodeQueue, markedNodes)
		case node.IsTaskNode():
			//log.Printf("Updating taskNode:%v task:%v\n", node.ID, node.Task.Name)
			gm.updateTaskNode(node, nodeQueue, markedNodes)
			gm.updateChildrenTasks(task, nodeQueue, markedNodes)
		case node.IsEquivalenceClassNode():
			gm.updateEquivClassNode(node, nodeQueue, markedNodes)
		case node.IsResourceNode():
			gm.updateResourceNode(node, nodeQueue, markedNodes)
		default:
			log.Panicf("gm/updateFlowGraph: Unexpected node type: %v", node.Type)
		}
	}
}

func (gm *graphManager) updateResourceNode(resNode *flowgraph.Node, nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	gm.updateResOutgoingArcs(resNode, nodeQueue, markedNodes)
}

// Update resource related stats (e.g., arc capacities, num slots,
// num running tasks) on every arc/node up to the root resource.
func (gm *graphManager) updateResourceStatsUpToRoot(currNode *flowgraph.Node, capDelta, slotsDelta, runningTasksDelta int64) {
	for {
		parentNode := gm.nodeToParentNode[currNode]
		if parentNode == nil {
			// The node is the root of the topology.
			return
		}

		parentArc := gm.cm.Graph().GetArc(parentNode, currNode)
		if parentArc == nil {
			log.Panicf("gm/updateResourceStatsUpToRoot: parent:%v to currNode:%v arc cannot be nil", parentNode.ID, currNode.ID)
		}

		newCapacity := uint64(int64(parentArc.CapUpperBound) + capDelta)
		gm.cm.ChangeArcCapacity(parentArc, newCapacity, dimacs.ChgArcBetweenRes, "UpdateCapacityUpToRoot")
		parentNode.ResourceDescriptor.NumSlotsBelow = uint64(int64(parentNode.ResourceDescriptor.NumSlotsBelow) + slotsDelta)
		parentNode.ResourceDescriptor.NumRunningTasksBelow = uint64(int64(parentNode.ResourceDescriptor.NumRunningTasksBelow) + runningTasksDelta)

		currNode = parentNode
	}
}

func (gm *graphManager) updateResourceTopologyDFS(rtnd *pb.ResourceTopologyNodeDescriptor) {
	rd := rtnd.ResourceDesc
	rd.NumSlotsBelow = 0
	rd.NumRunningTasksBelow = 0
	if rd.Type == pb.ResourceDescriptor_ResourcePu {
		// Base case
		rd.NumSlotsBelow = gm.MaxTasksPerPu
		rd.NumRunningTasksBelow = uint64(len(rd.CurrentRunningTasks))
	}

	for _, rtndChild := range rtnd.Children {
		gm.updateResourceTopologyDFS(rtndChild)
		rd.NumSlotsBelow += rtndChild.ResourceDesc.NumSlotsBelow
		rd.NumRunningTasksBelow += rtndChild.ResourceDesc.NumRunningTasksBelow
	}

	if rtnd.ParentId != "" {
		// Update the arc to the parent.
		currNode := gm.nodeForResourceID(util.MustResourceIDFromString(rd.Uuid))
		if currNode == nil {
			log.Panicf("gm/updateResourceTopologyDFS: node for resource.Uuid:%v cannot be nil\n", rd.Uuid)
		}
		parentNode := gm.nodeToParentNode[currNode]
		if parentNode == nil {
			log.Panicf("gm/updateResourceTopologyDFS: parentNode for node.ID:%v cannot be nil\n", currNode.ID)
		}
		parentArc := gm.cm.Graph().GetArc(parentNode, currNode)
		gm.cm.ChangeArcCapacity(parentArc, gm.capacityFromResNodeToParent(rd), dimacs.ChgArcBetweenRes, "UpdateResourceTopologyDFS")
	}
}

func (gm *graphManager) updateResOutgoingArcs(resNode *flowgraph.Node, nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	for _, arc := range resNode.OutgoingArcMap {
		if arc.DstNode.ResourceID == 0 {
			// Connected to sink
			gm.updateResToSinkArc(resNode)
			continue
		}

		cost := int64(gm.costModeler.ResourceNodeToResourceNodeCost(resNode.ResourceDescriptor, arc.DstNode.ResourceDescriptor))
		gm.cm.ChangeArcCost(arc, cost, dimacs.ChgArcBetweenRes, "UpdateResOutgoingArcs")
		if _, ok := markedNodes[arc.DstNode.ID]; !ok {
			// Add the dst node to the queue if it hasn't been marked yet.
			markedNodes[arc.DstNode.ID] = struct{}{}
			nodeQueue.Push(&taskOrNode{Node: arc.DstNode, TaskDesc: arc.DstNode.Task})
		}
	}
}

// Updates the arc connecting a resource to the sink. It requires the resource
// to be a PU.
// resourceNode is the resource node for which to update its arc to the sink
func (gm *graphManager) updateResToSinkArc(resNode *flowgraph.Node) {
	if resNode.Type != flowgraph.NodeTypePu {
		log.Panicf("gm:updateResToSinkArc: Updating an arc from a non-PU to the sink")
	}

	resArcSink := gm.cm.Graph().GetArc(resNode, gm.sinkNode)
	cost := int64(gm.costModeler.LeafResourceNodeToSinkCost(resNode.ResourceID))
	if resArcSink == nil {
		gm.cm.AddArc(resNode, gm.sinkNode, 0, gm.MaxTasksPerPu, cost, flowgraph.ArcTypeOther, dimacs.AddArcResToSink, "UpdateResToSinkArc")
	} else {
		gm.cm.ChangeArcCost(resArcSink, cost, dimacs.ChgArcResToSink, "UpdateResToSinkArc")
	}

}

// Updates the cost on running arc of the task. If preemption is enabled then
// the method also updates the preemption cost on the arc to the unscheduled
// aggregator.
// NOTE: nodeQueue and markedNodes can be NULL as long as updatePreferences
// is false.
// taskNode is the node for which to update the arcs
// updatePreferences is true if the method should update the resource and
// equivalence preferences
func (gm *graphManager) updateRunningTaskNode(taskNode *flowgraph.Node, updatePreferences bool, nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	taskID := types.TaskID(taskNode.Task.Uid)
	runningArc := gm.taskToRunningArc[taskID]
	if runningArc == nil {
		log.Panicf("gm/updateRunningTaskNode: running arc for taskNode.Task.Uid:%v must exist\n", taskNode.Task.Uid)
	}
	newCost := int64(gm.costModeler.TaskContinuationCost(taskID))
	gm.cm.ChangeArcCost(runningArc, newCost, dimacs.ChgArcTaskToRes, "UpdateRunningTaskNode: continuation cost")
	if !gm.Preemption {
		return
	}

	gm.updateRunningTaskToUnscheduledAggArc(taskNode)
	if updatePreferences {
		// nodeQueue and markedNodes must not be nil at this point
		gm.updateTaskToResArcs(taskNode, nodeQueue, markedNodes)
		gm.updateTaskToEquivArcs(taskNode, nodeQueue, markedNodes)
	}
}

// Updates the cost of the arc connecting a running task with its unscheduled
// aggregator.
// NOTE: This method should only be called when preemption is enabled.
// taskNode is the node for which to update the arc
func (gm *graphManager) updateRunningTaskToUnscheduledAggArc(taskNode *flowgraph.Node) {
	if !gm.Preemption {
		log.Panicf("Arc to unscheduled doesn't exist for running task when preemption is not enabled")
	}

	unschedAggNode := gm.unschedAggNodeForJobID(taskNode.JobID)
	if unschedAggNode == nil {
		log.Panicf("gm/updateRunningTaskToUnscheduledAggArc: unscheduledAggNode must exist for taskNode.JobID:%v\n", taskNode.JobID)
	}

	unschedArc := gm.cm.Graph().GetArc(taskNode, unschedAggNode)
	if unschedArc == nil {
		log.Panicf("gm/updateRunningTaskToUnscheduledAggArc: unscheduledArc must exist for unschedAggNode.ID:%v\n", unschedAggNode.ID)
	}

	cost := int64(gm.costModeler.TaskPreemptionCost(types.TaskID(taskNode.Task.Uid)))
	gm.cm.ChangeArcCost(unschedArc, cost, dimacs.ChgArcToUnsched, "UpdateRunningTaskToUnscheduledAggArc")
}

func (gm *graphManager) updateTaskNode(taskNode *flowgraph.Node, nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	if taskNode.IsTaskAssignedOrRunning() {
		gm.updateRunningTaskNode(taskNode, gm.UpdatePreferencesRunningTask, nodeQueue, markedNodes)
		return
	}
	//log.Printf("Graph Manager: updateTaskNode: id (%s)\n", taskNode.Task.Name)
	gm.updateTaskToUnscheduledAggArc(taskNode)
	gm.updateTaskToEquivArcs(taskNode, nodeQueue, markedNodes)
	gm.updateTaskToResArcs(taskNode, nodeQueue, markedNodes)
}

// Updates a task's outgoing arcs to ECs. If the task has new outgoing arcs
// to new EC nodes then the method appends them to the nodeQueue. Similarly,
// EC nodes that have not yet been marked are appended to the queue.
func (gm *graphManager) updateTaskToEquivArcs(taskNode *flowgraph.Node, nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	prefECs := gm.costModeler.GetTaskEquivClasses(types.TaskID(taskNode.Task.Uid))
	// Empty slice means no preferences
	if len(prefECs) == 0 {
		gm.removeInvalidECPrefArcs(taskNode, prefECs, dimacs.DelArcTaskToEquivClass)
		return
	}

	for _, prefEC := range prefECs {
		prefECNode := gm.nodeForEquivClass(prefEC)
		if prefECNode == nil {
			prefECNode = gm.addEquivClassNode(prefEC)
		}
		newCost := int64(gm.costModeler.TaskToEquivClassAggregator(types.TaskID(taskNode.Task.Uid), prefEC))
		prefECArc := gm.cm.Graph().GetArc(taskNode, prefECNode)

		if prefECArc == nil {
			gm.cm.AddArc(taskNode, prefECNode, 0, 1, newCost, flowgraph.ArcTypeOther, dimacs.AddArcTaskToEquivClass, "UpdateTaskToEquivArcs")
		} else {
			gm.cm.ChangeArc(prefECArc, prefECArc.CapLowerBound, prefECArc.CapUpperBound, newCost, dimacs.ChgArcTaskToEquivClass, "UpdateTaskToEquivArcs")
		}

		if _, ok := markedNodes[prefECNode.ID]; !ok {
			// Add the EC node to the queue if it hasn't been marked yet.
			markedNodes[prefECNode.ID] = struct{}{}
			nodeQueue.Push(&taskOrNode{Node: prefECNode, TaskDesc: prefECNode.Task})
		}
	}
	gm.removeInvalidECPrefArcs(taskNode, prefECs, dimacs.DelArcTaskToEquivClass)
}

// Updates a task's preferences to resources.
func (gm *graphManager) updateTaskToResArcs(taskNode *flowgraph.Node, nodeQueue queue.FIFO, markedNodes map[flowgraph.NodeID]struct{}) {
	prefRIDs := gm.costModeler.GetTaskPreferenceArcs(types.TaskID(taskNode.Task.Uid))
	// Empty slice means no preferences
	if len(prefRIDs) == 0 {
		gm.removeInvalidPrefResArcs(taskNode, prefRIDs, dimacs.DelArcTaskToRes)
		return
	}

	for _, prefRID := range prefRIDs {
		prefResNode := gm.nodeForResourceID(prefRID)
		// The resource node should already exist because the cost models cannot
		// prefer a resource before it is added to the graph.
		if prefResNode == nil {
			log.Panicf("gm/updateTaskToResArcs: preferred resource node cannot be nil")
		}
		newCost := int64(gm.costModeler.TaskToResourceNodeCost(types.TaskID(taskNode.Task.Uid), prefRID))
		prefResArc := gm.cm.Graph().GetArc(taskNode, prefResNode)

		if prefResArc == nil {
			gm.cm.AddArc(taskNode, prefResNode, 0, 1, newCost, flowgraph.ArcTypeOther, dimacs.AddArcTaskToRes, "UpdateTaskToResArcs")
		} else if prefResArc.Type != flowgraph.ArcTypeRunning {
			// We don't change the cost of the arc if it's a running arc because
			// the arc is updated somewhere else. Moreover, the cost of running
			// arcs is returned by TaskContinuationCost.
			gm.cm.ChangeArcCost(prefResArc, int64(newCost), dimacs.ChgArcTaskToRes, "UpdateTaskToResArcs")
		}

		if _, ok := markedNodes[prefResNode.ID]; !ok {
			// Add the res node to the queue if it hasn't been marked yet.
			markedNodes[prefResNode.ID] = struct{}{}
			nodeQueue.Push(&taskOrNode{Node: prefResNode, TaskDesc: prefResNode.Task})
		}
	}
	gm.removeInvalidPrefResArcs(taskNode, prefRIDs, dimacs.DelArcTaskToRes)
}

// Updates the arc from a task to its unscheduled aggregator. The method
// adds the unscheduled if it doesn't already exist.
// returns the unscheduled aggregator node
func (gm *graphManager) updateTaskToUnscheduledAggArc(taskNode *flowgraph.Node) *flowgraph.Node {
	unschedAggNode := gm.unschedAggNodeForJobID(taskNode.JobID)
	if unschedAggNode == nil {
		unschedAggNode = gm.addUnscheduledAggNode(taskNode.JobID)
	}
	newCost := int64(gm.costModeler.TaskToUnscheduledAggCost(types.TaskID(taskNode.Task.Uid)))
	toUnschedArc := gm.cm.Graph().GetArc(taskNode, unschedAggNode)

	if toUnschedArc == nil {
		gm.cm.AddArc(taskNode, unschedAggNode, 0, 1, newCost, flowgraph.ArcTypeOther, dimacs.AddArcToUnsched, "UpdateTaskToUnscheduledAggArc")
	} else {
		gm.cm.ChangeArcCost(toUnschedArc, newCost, dimacs.ChgArcToUnsched, "UpdateTaskToUnscheduledAggArc")
	}
	return unschedAggNode
}

// Adjusts the capacity of the arc connecting the unscheduled agg to the sink
// by cap_delta. The method also updates the cost if need be.
// unschedAggNode is the unscheduled aggregator node
// capDelta is the delta by which to change the capacity
func (gm *graphManager) updateUnscheduledAggNode(unschedAggNode *flowgraph.Node, capDelta int64) {
	unschedAggSinkArc := gm.cm.Graph().GetArc(unschedAggNode, gm.sinkNode)
	newCost := int64(gm.costModeler.UnscheduledAggToSinkCost(unschedAggNode.JobID))
	if unschedAggSinkArc != nil {
		newCapacity := uint64(int64(unschedAggSinkArc.CapUpperBound) + capDelta)
		gm.cm.ChangeArc(unschedAggSinkArc, unschedAggSinkArc.CapLowerBound, newCapacity, newCost, dimacs.ChgArcFromUnsched, "UpdateUnscheduledAggNode")
		return
	}

	if capDelta < 1 {
		log.Panicf("gb/updateUnscheduledAggNode: capDelta:%v must be >= 1\n", capDelta)
	}

	gm.cm.AddArc(unschedAggNode, gm.sinkNode, 0, uint64(capDelta), newCost, flowgraph.ArcTypeOther, dimacs.AddArcFromUnsched, "UpdateUnscheduledAggNode")
}

func (gm *graphManager) visitTopologyChildren(rtnd *pb.ResourceTopologyNodeDescriptor) {
	rd := rtnd.ResourceDesc
	for _, rtndChild := range rtnd.Children {
		gm.addResourceTopologyDFS(rtndChild)
		rd.NumSlotsBelow += rtndChild.ResourceDesc.NumSlotsBelow
		rd.NumRunningTasksBelow += rtndChild.ResourceDesc.NumRunningTasksBelow
	}
}

// Small helper functions
func (gm *graphManager) nodeForEquivClass(ec types.EquivClass) *flowgraph.Node {
	return gm.taskECToNode[ec]
}

func (gm *graphManager) nodeForResourceID(resourceID types.ResourceID) *flowgraph.Node {
	return gm.resourceToNode[resourceID]
}

func (gm *graphManager) nodeForTaskID(taskID types.TaskID) *flowgraph.Node {
	return gm.taskToNode[taskID]
}

func (gm *graphManager) unschedAggNodeForJobID(jobID types.JobID) *flowgraph.Node {
	return gm.jobUnschedToNode[jobID]
}

// check to see for this task it should be schedulable.
func taskNeedNode(td *pb.TaskDescriptor) bool {
	return td.State == pb.TaskDescriptor_Runnable ||
		td.State == pb.TaskDescriptor_Running ||
		td.State == pb.TaskDescriptor_Assigned
}
