package flowscheduler

import (
	"log"

	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/util"
	"github.com/coreos/ksched/pkg/util/queue"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/dimacs"
	"github.com/coreos/ksched/scheduling/flow/flowmanager"
	"github.com/coreos/ksched/scheduling/flow/placement"
)

// Set of tasks
type TaskSet map[types.TaskID]struct{}

type scheduler struct {
	// Fields specific to every scheduler, originally present in the interface
	resourceMap      types.ResourceMap
	jobMap           types.JobMap
	taskMap          types.TaskMap
	resourceTopology *pb.ResourceTopologyNodeDescriptor

	// Flow scheduler specific fields
	gm          flowmanager.GraphManager
	solver      placement.Solver
	dimacsStats *dimacs.ChangeStats
	// Root nodes(presumably machines) of all the resources in the topology
	resourceRoots map[*pb.ResourceTopologyNodeDescriptor]struct{}

	// Event driven scheduler specific fields
	// Note: taskBindings tracks the old state of which task maps to which resource (before each iteration).
	taskBindings map[types.TaskID]types.ResourceID
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

}

func (s *scheduler) RegisterResource(rtnd *pb.ResourceTopologyNodeDescriptor) {
	// Event scheduler related work
	// Do a BFS traversal starting from rtnd root and set each PU in this topology as schedulable
	toVisit := queue.NewFIFO()
	toVisit.Push(rtnd)
	for !toVisit.IsEmpty() {
		currRD := toVisit.Pop().(*pb.ResourceTopologyNodeDescriptor).ResourceDesc
		if currRD.Type != pb.ResourceDescriptor_ResourcePu {
			continue
		}
		currRD.Schedulable = true
		if currRD.State == pb.ResourceDescriptor_ResourceUnknown {
			currRD.State = pb.ResourceDescriptor_ResourceIdle
		}
	}

	// Flow scheduler related work
	s.gm.AddResourceTopology(rtnd)
	if rtnd.ParentId == "" {
		s.resourceRoots[rtnd] = struct{}{}
	}
}

func (s *scheduler) DeregisterResource(*pb.ResourceTopologyNodeDescriptor) {

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

func (s *scheduler) BoundResourceForTask(taskID types.TaskID) *types.ResourceID {
	return nil
}

func (s *scheduler) BoundTasksForResource(resourceID types.ResourceID) []types.TaskID {
	return nil
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
	oldRID := s.taskBindings[taskID]
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

}

func (s *scheduler) KillRunningTask(taskID types.TaskID) {

}

func (s *scheduler) ComputeRunnableTasksForJob(jd *pb.JobDescriptor) map[types.TaskID]struct{} {
	return nil
}

// Flow scheduler method
func (s *scheduler) ScheduleAllJobs() (uint64, []pb.SchedulingDelta) {
	jds := make([]*pb.JobDescriptor, 0)
	for _, jobDesc := range s.jobsToSchedule {
		// If at least one task is runnable in the job, add it for scheduling
		if len(s.ComputeRunnableTasksForJob(jobDesc)) > 0 {
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
		// Note: Ignore those completed, removal check...

		d := s.gm.NodeBindingToSchedulingDelta(taskNodeID, resourceNodeID, s.taskBindings)
		deltas = append(deltas, d)
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
			jd := s.jobMap.FindPtrOrNull(util.MustJobIDFromString(td.JobID))
			if jd.State != pb.JobDescriptor_Running {
				jd.State = pb.JobDescriptor_Running
			}
			s.HandleTaskPlacement(td, rs.Descriptor())
			numScheduled++
		case pb.SchedulingDelta_PREEMPT:
			s.HandleTaskEviction(td, rs.Descriptor())
		case pb.SchedulingDelta_MIGRATE:
			s.HandleTaskMigration(td, rs.Descriptor())
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
	if _, ok := s.taskBindings[taskID]; ok {
		log.Panicf("scheduler/bindTaskToResource: mapping for taskID:%v in taskBindings must not already exist\n", taskID)
	}
	s.taskBindings[taskID] = rID
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
	rd := resourceStatus.Descriptor()
	// We don't have to remove the task from rd's running tasks because
	// we've already cleared the list in the scheduling iteration
	if len(rd.CurrentRunningTasks) == 0 {
		rd.State = pb.ResourceDescriptor_ResourceIdle
	}
	// Remove the task from the resource bindings, return false if not found in the mappings
	if _, ok := s.taskBindings[taskID]; !ok {
		return false
	}

	taskSet := s.resourceBindings[rID]
	if _, ok := taskSet[taskID]; !ok {
		return false
	}

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
