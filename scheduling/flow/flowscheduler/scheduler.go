package flowscheduler

import (
	"log"

	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/util"
	"github.com/coreos/ksched/pkg/util/queue"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/costmodel"
	"github.com/coreos/ksched/scheduling/flow/dimacs"
	"github.com/coreos/ksched/scheduling/flow/flowmanager"
	"github.com/coreos/ksched/scheduling/flow/placement"
)

// Set of tasks
type TaskSet map[types.TaskID]struct{}

type scheduler struct {
	// Fields specific to every scheduler, originally present in the interface
	resourceMap      *types.ResourceMap
	jobMap           *types.JobMap
	taskMap          *types.TaskMap
	resourceTopology *pb.ResourceTopologyNodeDescriptor

	// Flow scheduler specific fields
	gm          flowmanager.GraphManager
	solver      placement.Solver
	dimacsStats *dimacs.ChangeStats
	// Root nodes(presumably machines) of all the resources in the topology
	resourceRoots map[*pb.ResourceTopologyNodeDescriptor]struct{}

	// Event driven scheduler specific fields
	// Note: taskBindings tracks the old state of which task maps to which resource (before each iteration).
	TaskBindings map[types.TaskID]types.ResourceID
	// Similar to taskBindings but tracks tasks binded to every resource. This is a multimap
	resourceBindings map[types.ResourceID]TaskSet
	// A vector holding descriptors of the jobs to be scheduled in the next scheduling round.
	jobsToSchedule map[types.JobID]*pb.JobDescriptor
	// Sets of runnable and blocked tasks in each job. Multimap
	// Originally maintained up by ComputeRunnableTasksForJob() and LazyGraphReduction()
	// by checking and resolving dependencies between tasks. We will avoid that for now
	// and simply declare all tasks as runnable
	runnableTasks map[types.JobID]TaskSet
}

// Initialize a new scheduler. All the input parameters will need to be kept updated by the caller
// of the scheduler functions, upon the addition of new machines, tasks
// resourceMap: The mappings of ResourceIDs to ResourceStatuses(wrappers around ResourceDescriptors and ResourceTopologyNodes)
// jobMap: The mappings of JobIDs to JobDescriptors
// taskMap: The mappings of TaskIDs to TaskDescriptors
// root: The root ResourceTopologyNodeDescriptor of all the resources in the cluster.
// Any new machines will need be added as children of this root before calling RegisterResource() on the scheduler
func NewScheduler(resourceMap *types.ResourceMap, jobMap *types.JobMap, taskMap *types.TaskMap,
	root *pb.ResourceTopologyNodeDescriptor, maxTasksPerPu uint64) Scheduler {
	// Initialize graph manager with trivial cost model
	leafResourceIDs := make(map[types.ResourceID]struct{})
	dimacsStats := &dimacs.ChangeStats{}
	costModeler := costmodel.NewTrivial(resourceMap, taskMap, leafResourceIDs, maxTasksPerPu)
	gm := flowmanager.NewGraphManager(costModeler, leafResourceIDs, dimacsStats, maxTasksPerPu)
	// Set up the initial flow graph
	gm.AddResourceTopology(root)
	// Set up the solver
	solver := placement.NewSolver(gm)

	// Initialize and return a new scheduler
	return &scheduler{
		resourceMap:      resourceMap,
		jobMap:           jobMap,
		taskMap:          taskMap,
		resourceTopology: root,
		gm:               gm,
		solver:           solver,
		dimacsStats:      dimacsStats,
		resourceRoots:    make(map[*pb.ResourceTopologyNodeDescriptor]struct{}),
		TaskBindings:     make(map[types.TaskID]types.ResourceID),
		resourceBindings: make(map[types.ResourceID]TaskSet),
		jobsToSchedule:   make(map[types.JobID]*pb.JobDescriptor),
		runnableTasks:    make(map[types.JobID]TaskSet),
	}
}

func (s *scheduler) GetTaskBindings() map[types.TaskID]types.ResourceID {
	return s.TaskBindings
}

// Event scheduler method
func (s *scheduler) AddJob(jd *pb.JobDescriptor) {
	s.jobsToSchedule[util.MustJobIDFromString(jd.Uuid)] = jd
}

// Not needed for testing
func (s *scheduler) HandleJobCompletion(jobID types.JobID) {
	// Job completed, so remove its nodes
	s.gm.JobCompleted(jobID)
	// Event scheduler related work
	jd := s.jobMap.FindPtrOrNull(jobID)
	if jd == nil {
		log.Panicf("Job for id:%v must exist\n", jobID)
	}
	delete(s.jobsToSchedule, jobID)
	delete(s.runnableTasks, jobID)
	jd.State = pb.JobDescriptor_Completed
}

func (s *scheduler) HandleTaskCompletion(td *pb.TaskDescriptor) {
	// Event scheduler related work
	// Find resource binded for this task
	rID, ok := s.TaskBindings[types.TaskID(td.Uid)]
	if !ok {
		log.Panicf("Task:%v must be bounded to a resource\n", td.Uid)
	}
	resourceStatus := s.resourceMap.FindPtrOrNull(rID)
	if resourceStatus == nil {
		log.Panicf("Resource:%v must have a resource status in the resourceMap\n", rID)
	}
	// Free the resource
	if !s.unbindTaskFromResource(td, rID) {
		log.Panicf("Could not unbind task:%v from resource:%v for eviction\n", td.Uid, rID)
	}
	// Set task state as completed
	td.State = pb.TaskDescriptor_Completed
	// TODO: Not important but the executor would then set the finish time
	// and total run time for the task descriptor for reporting purposes

	// Flow scheduler related work
	s.gm.TaskCompleted(types.TaskID(td.Uid))
	// TODO: If the scheduler is ever made asynchronous or event based then
	// we need to take care of the fact that the task may have completed during the
	// the solver run. See original

}

func (s *scheduler) RegisterResource(rtnd *pb.ResourceTopologyNodeDescriptor) {
	// Event scheduler related work
	// Do a BFS traversal starting from rtnd root and set each PU in this topology as schedulable
	toVisit := queue.NewFIFO()
	toVisit.Push(rtnd)
	for !toVisit.IsEmpty() {
		currNode := toVisit.Pop().(*pb.ResourceTopologyNodeDescriptor)
		currRD := currNode.ResourceDesc
		if currRD.Type != pb.ResourceDescriptor_ResourcePu {
			continue
		}
		currRD.Schedulable = true
		if currRD.State == pb.ResourceDescriptor_ResourceUnknown {
			currRD.State = pb.ResourceDescriptor_ResourceIdle
		}
		// Add children to queue
		for _, child := range currNode.Children {
			toVisit.Push(child)
		}
	}

	// Flow scheduler related work
	s.gm.AddResourceTopology(rtnd)
	if rtnd.ParentId == "" {
		s.resourceRoots[rtnd] = struct{}{}
	}
}

func (s *scheduler) DeregisterResource(rtnd *pb.ResourceTopologyNodeDescriptor) {
	// Flow scheduler related work
	// Traverse the resource topology tree in order to evict tasks.
	// Do a dfs post order traversal to evict all tasks from the resource topology
	s.dfsEvictTasks(rtnd)

	// TODO: The scheduler is not an event based scheduler right now and so is not concurrent.
	// If it is implemented in an event based fashion then we need to implement locks
	// as well as make sure we don't place any tasks on the PUs that were removed
	// while the solver was running. In a non event based setting this won't be an issue.
	// pusRemovedDuringSolverRun := s.gm.RemoveResourceTopology(rtnd.ResourceDesc)
	s.gm.RemoveResourceTopology(rtnd.ResourceDesc)

	// If it is an entire machine that was removed
	if rtnd.ParentId != "" {
		delete(s.resourceRoots, rtnd)
	}

	// Event scheduler related work
	s.dfsCleanUpResource(rtnd)
	// We've finished using ResourceTopologyNodeDescriptor. We can now deconnect it from its parent.
	if rtnd.ParentId != "" {
		// Remove resource node from its parent's children list
		parentID := util.MustResourceIDFromString(rtnd.ParentId)
		parentResourceStatus := s.resourceMap.FindPtrOrNull(parentID)
		if parentResourceStatus == nil {
			log.Panicf("Parent resource status for node:%v must exist", rtnd.ResourceDesc.Uuid)
		}
		parentNode := parentResourceStatus.TopologyNode
		children := parentNode.Children
		index := -1
		//Find the index of the child in the parent
		for i, childNode := range children {
			if childNode.ResourceDesc.Uuid == rtnd.ResourceDesc.Uuid {
				index = i
				break
			}
		}
		// Remove the node from the parent's slice
		if index == -1 {
			log.Panicf("Resource node:%v not found as child of its parent:%v\n", rtnd.ResourceDesc.Uuid, parentID)
		} else if index == 0 {
			parentNode.Children = children[1:]
		} else {
			parentNode.Children = append(children[:index-1], children[index+1:]...)
		}
	}

}

func (s *scheduler) HandleTaskPlacement(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {
	// Flow scheduler related work
	td.ScheduledToResource = rd.Uuid
	taskID := types.TaskID(td.Uid)
	s.gm.TaskScheduled(taskID, util.MustResourceIDFromString(rd.Uuid))

	// Event scheduler related work
	s.bindTaskToResource(td, rd)
	// Remove the task from the runnable_tasks
	jobID := util.MustJobIDFromString(td.JobID)
	runnablesForJob := s.runnableTasks[jobID]
	if runnablesForJob != nil {
		delete(runnablesForJob, taskID)
	}

	// Execute the task on the resource
	s.executeTask(td, rd)
}

func (s *scheduler) HandleTaskEviction(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {
	rID := util.MustResourceIDFromString(rd.Uuid)
	taskID := types.TaskID(td.Uid)
	jobID := util.MustJobIDFromString(td.JobID)
	// Flow scheduler related work
	s.gm.TaskEvicted(taskID, rID)

	// Event scheudler related work
	if !s.unbindTaskFromResource(td, rID) {
		log.Panicf("Could not unbind task:%v from resource:%v for eviction\n", taskID, rID)
	}
	td.State = pb.TaskDescriptor_Runnable
	s.insertTaskIntoRunnables(jobID, taskID)
	// Some work is then done by the executor to handle the task eviction(update finish/running times)
	// but we don't need to account for that right now
}

func (s *scheduler) HandleTaskMigration(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {
	taskID := types.TaskID(td.Uid)
	oldRID := s.TaskBindings[taskID]
	newRID := util.MustResourceIDFromString(rd.Uuid)

	// Flow scheduler related work
	// XXX(ionel): HACK! We update scheduledToResource field here
	// and in the EventDrivenScheduler. We update it here because
	// TaskMigrated first calls TaskEvict and then TaskSchedule.
	// TaskSchedule requires scheduledToResource to be up to date.
	// Hence, we have to set it before we call the method.
	td.ScheduledToResource = rd.Uuid
	s.gm.TaskMigrated(taskID, oldRID, newRID)

	// Event scheudler related work
	// Unbind task from old resource and bind to new one
	rd.State = pb.ResourceDescriptor_ResourceBusy
	td.State = pb.TaskDescriptor_Running
	if !s.unbindTaskFromResource(td, oldRID) {
		log.Panicf("Task/Resource binding for taskID:%v to rID:%v must exist\n", taskID, oldRID)
	}
	s.bindTaskToResource(td, rd)
}

func (s *scheduler) HandleTaskFailure(td *pb.TaskDescriptor) {
	taskID := types.TaskID(td.Uid)
	// Flow scheduler related work
	s.gm.TaskFailed(taskID)

	// Event scheduler related work
	// Find resource for task
	rID, ok := s.TaskBindings[taskID]
	if !ok {
		log.Panicf("No resource found for task:%v that failed/should have been running\n", taskID)
	}
	// Remove the task's resource binding (as it is no longer currently bound)
	s.unbindTaskFromResource(td, rID)
	// Set the task to "failed" state and deal with the consequences
	td.State = pb.TaskDescriptor_Failed
}

func (s *scheduler) KillRunningTask(taskID types.TaskID) {
	// Flow scheduler related work
	s.gm.TaskKilled(taskID)

	// Event scheduler related work
	td := s.taskMap.FindPtrOrNull(taskID)
	if td == nil {
		// TODO: This could just be an error instead of a panic
		log.Panicf("Tried to kill unknown task:%v, not present in taskMap\n", taskID)
	}
	// Check if we have a bound resource for the task and if it is marked as running
	_, ok := s.TaskBindings[taskID]
	if td.State != pb.TaskDescriptor_Running || !ok {
		// TODO: This could just be an error instead of a panic
		log.Panicf("Task:%v not bound or running on any resource", taskID)
	}
	td.State = pb.TaskDescriptor_Aborted
}

// Flow scheduler method
func (s *scheduler) ScheduleAllJobs() (uint64, []pb.SchedulingDelta) {
	jds := make([]*pb.JobDescriptor, 0)
	for _, jobDesc := range s.jobsToSchedule {
		// If at least one task is runnable in the job, add it for scheduling
		if len(s.computeRunnableTasksForJob(jobDesc)) > 0 {
			jds = append(jds, jobDesc)
		}
	}
	return s.ScheduleJobs(jds)
}

// Flow scheduler method
func (s *scheduler) ScheduleJobs(jdsRunnable []*pb.JobDescriptor) (uint64, []pb.SchedulingDelta) {
	numScheduledTasks := uint64(0)
	deltas := make([]pb.SchedulingDelta, 0)
	if len(jdsRunnable) > 0 {
		s.updateCostModelResourceStats()
		s.gm.AddOrUpdateJobNodes(jdsRunnable)
		numScheduledTasks, deltas = s.runSchedulingIteration()
		log.Printf("Scheduling Iteration complete, placed %v tasks\n", numScheduledTasks)

		// We reset the DIMACS stats here because all the graph changes we make
		// from now on are going to be included in the next scheduler run.
		s.dimacsStats.ResetStats()
		// TODO
		// If the support for the trace generator is ever added then log the dimacs changes
		// for this iteration before resetting them
	}
	return numScheduledTasks, deltas
}

func (s *scheduler) runSchedulingIteration() (uint64, []pb.SchedulingDelta) {
	// Steps:
	// - run solver and get task mapping
	// - update graph manager
	// - output delta.

	// Note:
	// - In original code, it also handles time dependent cost updating. Ignored here.
	// - No purging of unconnected EC.

	taskMappings := s.solver.Solve()

	// We first generate the deltas for the preempted tasks in a separate step.
	// Otherwise, we would have to maintain for every ResourceDescriptor the
	// current_running_tasks field which would be expensive because
	// RepeatedFields don't have any efficient remove element method.
	deltas := s.gm.SchedulingDeltasForPreemptedTasks(taskMappings, s.resourceMap)

	for taskNodeID, resourceNodeID := range taskMappings {
		// fmt.Printf("taskNode:%d going to resourceNode:%d\n", taskNodeID, resourceNodeID)
		// Note: Ignore those completed, removal check...
		delta := s.gm.NodeBindingToSchedulingDelta(taskNodeID, resourceNodeID, s.TaskBindings)
		if delta != nil {
			deltas = append(deltas, *delta)
		}
	}

	numScheduled := s.applySchedulingDeltas(deltas)

	// TODO: update_resource_topology_capacities??
	for rtnd := range s.resourceRoots {
		s.gm.UpdateResourceTopology(rtnd)
	}

	return numScheduled, deltas
}

func (s *scheduler) applySchedulingDeltas(deltas []pb.SchedulingDelta) uint64 {
	numScheduled := uint64(0)
	for _, d := range deltas {
		td := s.taskMap.FindPtrOrNull(types.TaskID(d.TaskId))
		if td == nil {
			panic("")
		}
		resID := util.MustResourceIDFromString(d.ResourceId)
		rs := s.resourceMap.FindPtrOrNull(resID)
		if rs == nil {
			panic("")
		}

		switch d.Type {
		case pb.SchedulingDelta_PLACE:
			// log.Printf("TASK PLACEMENT: task:%v on resource:%v\n", td.Uid, rs.Descriptor.FriendlyName)
			jd := s.jobMap.FindPtrOrNull(util.MustJobIDFromString(td.JobID))
			if jd.State != pb.JobDescriptor_Running {
				jd.State = pb.JobDescriptor_Running
			}
			s.HandleTaskPlacement(td, rs.Descriptor)
			numScheduled++
		case pb.SchedulingDelta_PREEMPT:
			log.Printf("TASK PREEMPTION: task:%v from resource:%v\n", td.Uid, rs.Descriptor.FriendlyName)
			s.HandleTaskEviction(td, rs.Descriptor)
		case pb.SchedulingDelta_MIGRATE:
			log.Printf("TASK MIGRATION: task:%v to resource:%v\n", td.Uid, rs.Descriptor.FriendlyName)
			s.HandleTaskMigration(td, rs.Descriptor)
		case pb.SchedulingDelta_NOOP:
			log.Println("NOOP Delta type:", d.Type)
		default:
			log.Fatalf("Unknown delta type: %v", d.Type)
		}
	}
	return numScheduled
}

func (s *scheduler) updateCostModelResourceStats() {
	s.gm.ComputeTopologyStatistics(s.gm.SinkNode())
}

// BindTaskToResource is used to update metadata anytime a task is placed on a some resource by the scheduler
// either through a placement or migration
// Event driven scheduler specific method
func (s *scheduler) bindTaskToResource(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {
	taskID := types.TaskID(td.Uid)
	rID := util.MustResourceIDFromString(rd.Uuid)
	// Mark resource as busy and record task binding
	rd.State = pb.ResourceDescriptor_ResourceBusy
	rd.CurrentRunningTasks = append(rd.CurrentRunningTasks, uint64(taskID))
	// Insert mapping into task bindings, must not already exist
	if _, ok := s.TaskBindings[taskID]; ok {
		log.Panicf("scheduler/bindTaskToResource: mapping for taskID:%v in taskBindings must not already exist\n", taskID)
	}
	s.TaskBindings[taskID] = rID
	// Update resource bindings, create a binding set if it doesn't exist already
	if _, ok := s.resourceBindings[rID]; !ok {
		s.resourceBindings[rID] = make(TaskSet)
	}
	s.resourceBindings[rID][taskID] = struct{}{}
}

// UnbindTaskFromResource is similar to BindTaskToResource, in that it just updates the metadata for a task being removed from a resource
// It is called in the event of a task failure, migration or eviction.
// Returns false in case the task was not already bound to the resource in the taskMappings or resourceMappings
// Event driven scheduler specific method
func (s *scheduler) unbindTaskFromResource(td *pb.TaskDescriptor, rID types.ResourceID) bool {
	taskID := types.TaskID(td.Uid)
	resourceStatus := s.resourceMap.FindPtrOrNull(rID)
	rd := resourceStatus.Descriptor
	// We don't have to remove the task from rd's running tasks because
	// we've already cleared the list in the scheduling iteration
	if len(rd.CurrentRunningTasks) == 0 {
		rd.State = pb.ResourceDescriptor_ResourceIdle
	}
	// Remove the task from the resource bindings, return false if not found in the mappings
	if _, ok := s.TaskBindings[taskID]; !ok {
		return false
	}

	taskSet := s.resourceBindings[rID]
	if _, ok := taskSet[taskID]; !ok {
		return false
	}
	delete(s.TaskBindings, taskID)
	delete(taskSet, taskID)
	return true
}

// ExecuteTask is used to actually execute the task on a resource via an excution handler
// For our purposes we skip that and only update the meta data to mark the task as running
// Event driven scheduler specific method
func (s *scheduler) executeTask(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {
	// This function actually executes the task asynchronously on that resource via an executor
	// but we don't need that
	td.State = pb.TaskDescriptor_Running
	td.ScheduledToResource = rd.Uuid
}

// InsertTaskIntoRunnables is a helper method used to update the runnable tasks set for the specified job by adding the new task
// Event driven scheduler specific method
func (s *scheduler) insertTaskIntoRunnables(jobID types.JobID, taskID types.TaskID) {
	// Create a task set for this job if it doesn't already exist
	if _, ok := s.runnableTasks[jobID]; !ok {
		s.runnableTasks[jobID] = make(TaskSet)
	}
	// Insert task into runnable set for this job
	s.runnableTasks[jobID][taskID] = struct{}{}
}

// NOTE: This method is not implemented by the flow_scheduler but by the event_driven_sched
// Our implementation should be to ignore dependencies and mark all runnable tasks as runnable
// ComputeRunnableTasksForJob finds runnable tasks for the job in the argument and adds them to the
// global runnable set.
// jd: the descriptor of the job for which to find tasks
// Returns the set of tasks that are runnable for this job
func (s *scheduler) computeRunnableTasksForJob(jd *pb.JobDescriptor) TaskSet {
	// The "lazy graph reduction" performed in the original method
	// places all tasks that are newly created or blocking in the runnableTasks set
	// only if their dependencies are fulfilled. We disregards dependencies and
	// place all tasks that are in the Created or Blocking state into the runnableTasks set
	jobID := util.MustJobIDFromString(jd.Uuid)
	rootTask := jd.RootTask

	newlyActiveTasks := queue.NewFIFO()
	// Only add the root task if it is not already scheduled, running, done
	// or failed.
	if rootTask.State == pb.TaskDescriptor_Created ||
		rootTask.State == pb.TaskDescriptor_Running ||
		rootTask.State == pb.TaskDescriptor_Runnable ||
		rootTask.State == pb.TaskDescriptor_Completed {
		newlyActiveTasks.Push(rootTask)
	}
	for !newlyActiveTasks.IsEmpty() {
		currentTask := newlyActiveTasks.Pop().(*pb.TaskDescriptor)
		// Put all child tasks in queue
		for _, childTask := range currentTask.Spawned {
			newlyActiveTasks.Push(childTask)
		}
		if currentTask.State == pb.TaskDescriptor_Created || currentTask.State == pb.TaskDescriptor_Blocking {
			currentTask.State = pb.TaskDescriptor_Runnable
			s.insertTaskIntoRunnables(util.MustJobIDFromString(currentTask.JobID), types.TaskID(currentTask.Uid))
		}
	}

	if runnableTasksForJob, ok := s.runnableTasks[jobID]; ok {
		return runnableTasksForJob
	}
	s.runnableTasks[jobID] = make(TaskSet)
	return s.runnableTasks[jobID]
}

// DfsEvictTasks traverses the given root of a resource topology in a post order fashion
// and evicts all tasks from every resource in the topology
func (s *scheduler) dfsEvictTasks(rtnd *pb.ResourceTopologyNodeDescriptor) {
	for _, childNode := range rtnd.Children {
		s.dfsEvictTasks(childNode)
	}
	s.evictTasksFromResource(rtnd)
}

// DfsCleanUpResource traverses the given root of a resource topology in a post order fashion
// and cleans up the meta data for every resource node in the topology
func (s *scheduler) dfsCleanUpResource(rtnd *pb.ResourceTopologyNodeDescriptor) {
	for _, childNode := range rtnd.Children {
		s.dfsCleanUpResource(childNode)
	}
	s.cleanStateForDeregisteredResource(rtnd)
}

// EvictTasksFromResource updates the required metadata in order to evict all tasks bound to the resource
func (s *scheduler) evictTasksFromResource(rtnd *pb.ResourceTopologyNodeDescriptor) {
	resourceDesc := rtnd.ResourceDesc
	rID := util.MustResourceIDFromString(resourceDesc.Uuid)
	// Get the tasks bound to this resource
	tasks, ok := s.resourceBindings[rID]
	if !ok {
		return
	}
	// Evict every task
	for taskID, _ := range tasks {
		taskDesc := s.taskMap.FindPtrOrNull(taskID)
		if taskDesc == nil {
			log.Panicf("Descriptor for task:%v must exist in taskMap\n", taskID)
		}
		s.HandleTaskEviction(taskDesc, resourceDesc)
	}
}

// EvictTasksFromResource updates the resource bindings and resource maps on the removal of this resource
func (s *scheduler) cleanStateForDeregisteredResource(rtnd *pb.ResourceTopologyNodeDescriptor) {
	rID := util.MustResourceIDFromString(rtnd.ResourceDesc.Uuid)
	// Originally had cleanups related to the executors and the trace generators but we don't need that
	delete(s.resourceBindings, rID)
	delete(s.resourceMap.UnsafeGet(), rID)
}
