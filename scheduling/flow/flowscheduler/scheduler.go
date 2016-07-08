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
	// A vector holding descriptors of the jobs to be scheduled in the next scheduling round.
	jobsToSchedule map[types.JobID]*pb.JobDescriptor
	runnableTasks  map[types.JobID]TaskSet
	// Sets of runnable and blocked tasks in each job.
	// Originally maintained up by ComputeRunnableTasksForJob() and LazyGraphReduction()
	// by checking and resolving dependencies between tasks. We will avoid that for now
	// and simply declare all tasks as runnable
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

}

func (s *scheduler) BoundResourceForTask(taskID types.TaskID) *types.ResourceID {

}

func (s *scheduler) BoundTasksForResource(resourceID types.ResourceID) []types.TaskID {

}

func (s *scheduler) HandleTaskEviction(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {

}

func (s *scheduler) HandleTaskMigration(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {

}

func (s *scheduler) HandleTaskFailure(td *pb.TaskDescriptor) {

}

func (s *scheduler) KillRunningTask(taskID types.TaskID) {

}

func (s *scheduler) ComputeRunnableTasksForJob(jd *pb.JobDescriptor) map[types.TaskID]struct{} {

}

// Flow scheduler method
func (s *scheduler) ScheduleAllJobs() (uint64, []pb.SchedulingDelta) {
	jds := make([]*pb.JobDescriptor, 0)
	for jobID, jobDesc := range s.jobsToSchedule {
		// If at least one task is runnable in the job, add it for scheduling
		if len(s.ComputeRunnableTasksForJob(jobDesc)) > 0 {
			jds = append(jds, jobDesc)
		}
	}
	return s.ScheduleJobs(jds)
}

// Flow scheduler method
func (s *scheduler) ScheduleJobs(jdsRunnable []*pb.JobDescriptor) (uint64, []pb.SchedulingDelta) {
	numScheduledTasks := 0
	if len(jdsRunnable) > 0 {
		s.updateCostModelResourceStats()
		s.gm.AddOrUpdateJobNodes(jdsRunnable)
		numScheduledTasks, deltas := s.runSchedulingIteration()
		log.Printf("Scheduling Iteration complete, placed %v tasks\n", numScheduledTasks)

		// We reset the DIMACS stats here because all the graph changes we make
		// from now on are going to be included in the next scheduler run.
		s.dimacsStats.ResetStats()
		// TODO
		// If the support for the trace generator is ever added then log the dimacs changes
		// for this iteration before resetting them
	}
}

func (s *scheduler) runSchedulingIteration() (int, []pb.SchedulingDelta) {
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

	numScheduled := s.ApplySchedulingDeltas(deltas)

	// TODO: update_resource_topology_capacities??
	for rtnd := range s.resourceRoots {
		s.gm.UpdateResourceTopology(rtnd)
	}

	return numScheduled, deltas
}

func (s *scheduler) applySchedulingDeltas(deltas []pb.SchedulingDelta) int {
	numScheduled := 0
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
	log.Printf("Updating resource statistics in flow graph\n")
	s.gm.ComputeTopologyStatistics(s.gm.SinkNode())
}
