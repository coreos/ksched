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

	// Adds a new job. The job will be scheduled on the next run of the scheduler
	// if it has any runnable tasks.
	// jd: JobDescriptor of the job to add
	AddJob(jd *pb.JobDescriptor)

	// Finds the resource to which a particular task ID is currently bound.
	// taskID: the id of the task for which to do the lookup
	// Returns nil if the task does not exist or is not currently bound
	// Otherwise, it returns its resource id
	BoundResourceForTask(taskID types.TaskID) *types.ResourceID

	// Finds the tasks which are bound to a particular resource ID
	// resourceID: the id of the resource for which to do the lookup
	// Returns a slice of task ids
	BoundTasksForResource(resourceID types.ResourceID) []types.TaskID

	// Checks if all running tasks managed by this scheduler are healthy. It
	// invokes failure handlers if any failures are detected.
	CheckRunningTasksHealth()

	// Unregisters a resource ID from the scheduler. No-op if the resource ID is
	// not actually registered with it.
	// param1: rtnd pointer to the resource topology node descriptor of the resource to deregister
	DeregisterResource(*pb.ResourceTopologyNodeDescriptor)

	// Handles the completion of a job (all tasks are completed, failed or
	// aborted). May clean up scheduler-specific state.
	// jobID: the id of the completed job
	HandleJobCompletion(jobID types.JobID)

	// Handles the completion of a task. This usually involves freeing up its
	// resource by setting it idle, and recording any bookkeeping data required.
	// @td: the task descriptor of the completed task
	// @report: the task report to be populated with statistics
	// (e.g., finish time).
	// NOTE: Modified to not include processing the TaskFinalReport
	// originally: HandleTaskCompletion(td *TaskDescriptor, report *TaskFinalReport)
	HandleTaskCompletion(td *pb.TaskDescriptor)

	// Handles the failure of an attempt to delegate a task to a subordinate
	// coordinator. This can happen because the resource is no longer there (it
	// failed) or it is no longer idle (someone else put a task there).
	// td: the descriptor of the task that could not be delegated
	HandleTaskDelegationFailure(td *pb.TaskDescriptor)
	HandleTaskDelegationSuccess(td *pb.TaskDescriptor)

	// Handles the eviction of a task.
	// @td: The task descriptor of the evicted task
	// rd: The resource descriptor of the resource from which the task was evicted
	HandleTaskEviction(td *pb.TaskDescriptor, rd *pb.ResourceDescriptor)

	// Handles the failure of a task. This usually involves freeing up its
	// resource by setting it idle, and kicking off the necessary fault tolerance
	// handling procedures.
	// td: the task descriptor of the failed task
	HandleTaskFailure(td *TaskDescriptor)

	// Kills a running task.
	// @param task_id the id of the task to kill
	// NOTE: modified to not include kill message
	KillRunningTask(taskID types.TaskID)

	// Places a task delegated from a superior coordinator to a resource managed
	// by this scheduler.
	// @td the task descriptor of the delegated task
	// targetRID the resource ID on which to place the task
	PlaceDelegatedTask(td *TaskDescriptor, targetRID types.ResourceID) bool

	// Registers a resource with the scheduler, who may subsequently assign
	// work to this resource.
	// rtnd: the resource topology node descriptor
	// local: boolean to indicate if the resource is local or not
	RegisterResource(ResourceTopologyNodeDescriptor *rtnd, local bool, simulated bool)

	// Runs a scheduling iteration for all active jobs.
	// Returns the number of tasks scheduled, and the scheduling deltas
	// NOTE: Modified from original interface to return deltas rather than passing in and modifying the deltas
	ScheduleAllJobs(schedulerStats *SchedulerStats) (uint64, []pb.SchedulingDelta)
}
