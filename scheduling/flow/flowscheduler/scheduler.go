package flowscheduler

import (
	"log"

	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/util"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/flowmanager"
	"github.com/coreos/ksched/scheduling/flow/placement"
)

type scheduler struct {
	gm      flowmanager.GraphManager
	solver  placement.Solver
	rmap    types.ResourceMap
	jobMap  types.JobMap
	taskMap types.TaskMap
	// Note: taskBindings records which task maps to which resource (before each iteration).
	taskBindings  map[types.TaskID]types.ResourceID
	resourceRoots map[*pb.ResourceTopologyNodeDescriptor]struct{}
}

func (s *scheduler) RunSchedulingIteration() ([]pb.SchedulingDelta, int) {
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
	deltas := s.gm.SchedulingDeltasForPreemptedTasks(taskMappings, s.rmap)

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

	return deltas, numScheduled
}

func (s *scheduler) ApplySchedulingDeltas(deltas []pb.SchedulingDelta) int {
	numScheduled := 0
	for _, d := range deltas {
		td := s.taskMap.FindPtrOrNull(types.TaskID(d.TaskId))
		if td == nil {
			panic("")
		}
		resID := util.MustResourceIDFromString(d.ResourceId)
		rs := s.rmap.FindPtrOrNull(resID)
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
		default:
			log.Println("Unhandled Delta type:", d.Type)
		}
	}
	return numScheduled
}

func (s *scheduler) HandleTaskPlacement(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {
}

func (s *scheduler) HandleTaskEviction(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {
}

func (s *scheduler) HandleTaskMigration(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor) {
}
