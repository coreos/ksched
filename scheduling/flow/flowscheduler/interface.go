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

// The interface implemented for all types of schedulers
// NOTE: some extra methods related to task reports and template dictionaries not included
package flowscheduler

import (
	"github.com/coreos/ksched/pkg/types"
	pb "github.com/coreos/ksched/proto"
)

type Scheduler interface {
	GetTaskBindings() map[types.TaskID]types.ResourceID

	// AddJob adds a new job. The job will be scheduled on the next run of the scheduler
	// if it has any runnable tasks.
	// jd: JobDescriptor of the job to add
	// NOTE: This method was originally implemented only by event_scheduler and not flow_scheduler
	AddJob(jd *pb.JobDescriptor)

	// RegisterResource registers a resource with the scheduler, who may subsequently assign
	// work to this resource.
	// rtnd: the resource topology node descriptor
	// local: boolean to indicate if the resource is local or not
	// NOTE: We don't distinguish between local(cpu), remote(storage) or simulated resources
	// Original interface modified to not take inputs for local or simulated flags
	RegisterResource(rtnd *pb.ResourceTopologyNodeDescriptor)

	// DeregisterResource unregisters a resource ID from the scheduler. No-op if the resource ID is
	// not actually registered with it.
	// rtnd: pointer to the resource topology node descriptor of the resource to deregister
	DeregisterResource(*pb.ResourceTopologyNodeDescriptor)

	// ScheduleAllJobs runs a scheduling iteration for all active jobs. Computes runnable jobs and then calls ScheduleJobs()
	// Returns the number of tasks scheduled, and the scheduling deltas
	// NOTE: Modified from original interface to return deltas rather than passing in and modifying the deltas
	// Also removed the schedulerStats from the input arguments
	ScheduleAllJobs() (uint64, []pb.SchedulingDelta)

	// ScheduleJobs schedules the given jobs. This is called by ScheduleAllJobs()
	// jds: a slice of job descriptors
	// Returns the number of tasks scheduled, and the scheduling deltas
	// NOTE: Modified from original interface to return deltas rather than passing in and modifying the deltas
	// Also removed the schedulerStats from the input arguments
	ScheduleJobs(jds []*pb.JobDescriptor) (uint64, []pb.SchedulingDelta)

	// HandleJobCompletion handles the completion of a job (all tasks are completed, failed or
	// aborted). May clean up scheduler-specific state.
	// jobID: the id of the completed job
	HandleJobCompletion(jobID types.JobID)

	// HandleTaskCompletion handles the completion of a task. This usually involves freeing up its
	// resource by setting it idle, and recording any bookkeeping data required.
	// td: the task descriptor of the completed task
	// report: the task report to be populated with statistics
	// (e.g., finish time).
	// NOTE: Modified to not include processing the TaskFinalReport
	// originally: HandleTaskCompletion(td *TaskDescriptor, report *TaskFinalReport)
	HandleTaskCompletion(td *pb.TaskDescriptor)

	// HandleTaskPlacement places a task to a resource, i.e. effects a scheduling assignment.
	// This will modify various bits of meta-data tracking assignments. It will
	// then delegate the actual execution of the task binary to the appropriate
	// local execution handler.
	// td: the descriptor of the task to bind
	// rd: the descriptor of the resource to bind to
	// This method is called by the flow scheduler for every PLACE scheduling delta
	// indicating a successful placement of a task on a resource
	HandleTaskPlacement(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor)

	// HandleTaskEviction handles the eviction of a task.
	// td: The task descriptor of the evicted task
	// rd: The resource descriptor of the resource from which the task was evicted
	HandleTaskEviction(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor)

	// HandleTaskMigration handles the migration of a task.
	// td: the descriptor of the migrated task
	// rd: the descriptor of the resource to which the task was migrated
	HandleTaskMigration(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor)

	// HandleTaskFailure handles the failure of a task. This usually involves freeing up its
	// resource by setting it idle, and kicking off the necessary fault tolerance
	// handling procedures.
	// td: the task descriptor of the failed task
	HandleTaskFailure(td *pb.TaskDescriptor)

	// KillRunningTask kills a running task.
	// task_id: the id of the task to kill
	// NOTE: modified to not include kill message
	KillRunningTask(taskID types.TaskID)
}
