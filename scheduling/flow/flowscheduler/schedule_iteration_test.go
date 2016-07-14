package flowscheduler

import (
	"fmt"
	"log"
	"strconv"
	"testing"

	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/types/resourcestatus"
	"github.com/coreos/ksched/pkg/util"
	"github.com/coreos/ksched/pkg/util/queue"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/flowscheduler"
)

func TestOneScheduleIteration(t *testing.T) {

	// Initialize empty resource, job and task maps.
	// Initialize a root ResourceTpoplogyNodeDescriptor of type Coordinator
	resourceMap := types.NewResourceMap()
	jobMap := types.NewJobMap()
	taskMap := types.NewTaskMap()
	rootNode := &pb.ResourceTopologyNodeDescriptor{
		ResourceDesc: getNewResourceDesc(pb.ResourceDescriptor_ResourceCoordinator, 0),
	}

	fmt.Printf("RootNode ID: %s\n", rootNode.ResourceDesc.Uuid)

	// Initialize the flow scheduler
	scheduler := flowscheduler.NewScheduler(resourceMap, jobMap, taskMap, rootNode)

	// Add 2 Machines to the topology, (2 cores per machine, 1 Pu per core, 1 Task per Pu)
	AddMachine(2, 1, 1, rootNode, resourceMap, scheduler)
	AddMachine(2, 1, 1, rootNode, resourceMap, scheduler)

	// Add 2 Jobs, with 2 Tasks each
	jobID1 := types.JobID(util.RandUint64())
	AddTask(jobID1, jobMap, taskMap)
	AddTask(jobID1, jobMap, taskMap)
	jobID2 := types.JobID(util.RandUint64())
	AddTask(jobID2, jobMap, taskMap)
	AddTask(jobID2, jobMap, taskMap)

	// Register the jobs with scheduler
	job1 := jobMap.FindPtrOrNull(jobID1)
	job2 := jobMap.FindPtrOrNull(jobID2)
	if job1 == nil || job2 == nil {
		log.Panicf("All jobs should exist\n")
	}
	scheduler.AddJob(job1)
	scheduler.AddJob(job2)
	// Don't need to worry about the resource usage or request vector since cost model is trivial
	// Check simulator_bridge.cc and simulator_bridge_test.cc to see how machines and tasks are added

	//Run one scheduling iteration
	numScheduled, _ := scheduler.ScheduleAllJobs()
	fmt.Printf("Number of tasks scheduled:%d\n", numScheduled)

	// Finally what is the output to observe after 1 scheduling iteration?
	// Print out the updated task bindings to see which task is placed on what resource
	for taskID, resourceID := range scheduler.GetTaskBindings() {
		taskDesc := taskMap.FindPtrOrNull(taskID)
		resourceNode := resourceMap.FindPtrOrNull(resourceID).TopologyNode
		resourceDesc := resourceMap.FindPtrOrNull(resourceID).Descriptor
		parentMachine := findParentMachine(resourceNode, resourceMap)
		fmt.Printf("Task:%v placed on resource:%v on machine:%v", taskDesc.Uid, resourceDesc.FriendlyName, parentMachine.FriendlyName)
	}
}

func findParentMachine(node *pb.ResourceTopologyNodeDescriptor, resourceMap *types.ResourceMap) *pb.ResourceDescriptor {
	for {
		if node.ResourceDesc.Type == pb.ResourceDescriptor_ResourceMachine {
			return node.ResourceDesc
		}
		// traverse to parent node
		id, err := strconv.ParseUint(node.ParentId, 10, 64)
		if err != nil {
			log.Panicf("Could not parse parentID\n")
		}
		parentID := types.ResourceID(id)
		parentNode := resourceMap.FindPtrOrNull(parentID)
		if parentNode == nil {
			// reached the root
			fmt.Printf("Machine not found\n")
			return nil
		}
		node = parentNode.TopologyNode
	}
}

// AddTask adds a new task to the specified jobID. If the jobID does not exist then
// a new job will be created for it. Both the taskMap and jobMap are updated with the new
// task and job.
// Returns the taskID of the task created
func AddTask(jobID types.JobID, jobMap *types.JobMap, taskMap *types.TaskMap) types.TaskID {
	// Create a new job descriptor if there isn't one in the jobMap already
	jobDesc := jobMap.FindPtrOrNull(jobID)
	jobUuid := strconv.FormatUint(uint64(jobID), 10)
	if jobDesc == nil {
		name := "Job " + jobUuid
		jobDesc = &pb.JobDescriptor{
			Uuid:  jobUuid,
			Name:  name,
			State: pb.JobDescriptor_Created,
		}
		jobMap.InsertIfNotPresent(jobID, jobDesc)
	}

	// Create a unique taskID
	duplicate := true
	taskID := types.TaskID(util.RandUint64())
	duplicate = taskMap.ContainsKey(taskID)
	for duplicate {
		taskID := types.TaskID(util.RandUint64())
		duplicate = taskMap.ContainsKey(taskID)
	}
	// Create the task descriptor and add it to the taskMap
	name := "Task " + strconv.FormatUint(uint64(taskID), 10)
	task := &pb.TaskDescriptor{
		Uid:   uint64(taskID),
		Name:  name,
		State: pb.TaskDescriptor_Created,
		JobID: jobUuid,
	}
	taskMap.InsertIfNotPresent(taskID, task)

	// If it is the first task then add it as the root task of this job
	if jobDesc.RootTask == nil {
		jobDesc.RootTask = task
	} else {
		// Add it as one of the children spawned by the root
		jobDesc.RootTask.Spawned = append(jobDesc.RootTask.Spawned, task)
	}

	return taskID
}

// AddMachine creates and adds a new machine topology node to the root topology node
// It then traverses the machine node topology and updates the resourceMap and finally registers the machine node with the scheduler
func AddMachine(numCores int, pusPerCore int, tasksPerPu int,
	root *pb.ResourceTopologyNodeDescriptor, resourceMap *types.ResourceMap, scheduler flowscheduler.Scheduler) {
	// Create a new machine topology descriptor and add it as the root's child
	machineNode := getNewMachineRtnd(numCores, pusPerCore, tasksPerPu)
	root.Children = append(root.Children, machineNode)
	// Link machine to root
	machineNode.ParentId = root.ResourceDesc.Uuid

	// Do a dfs from the rootNode and populate the resourceMap,
	// since the resourceMap is supposed to be updated outside of the scheduler
	nodes := queue.NewFIFO()
	nodes.Push(machineNode)
	for !nodes.IsEmpty() {
		currNode := nodes.Pop().(*pb.ResourceTopologyNodeDescriptor)
		resourceStatus := &resourcestatus.ResourceStatus{
			Descriptor:   currNode.ResourceDesc,
			TopologyNode: currNode,
		}
		// Add the resource node to the resourceMap
		resourceMap.InsertIfNotPresent(util.MustResourceIDFromString(currNode.ResourceDesc.Uuid), resourceStatus)
	}

	// Register the resource with the scheduler
	scheduler.RegisterResource(machineNode)
}

// getNewMachineRtnd returns an initialized and fully populated resource topology of type Machine
// that looks like machine->cores->PUs
// numCores: Total number of cores in the machine
// pusPerCore: Number of processing units(hardware threads)PUs per core
// tasksPerPu: The task capacity of each processing unit in the machine.
// The total machine capacity = tasksPerPu * numCores * pusPerCore
func getNewMachineRtnd(numCores int, pusPerCore int, tasksPerPu int) *pb.ResourceTopologyNodeDescriptor {
	totalCap := numCores * pusPerCore * tasksPerPu
	machineNode := &pb.ResourceTopologyNodeDescriptor{
		ResourceDesc: getNewResourceDesc(pb.ResourceDescriptor_ResourceMachine, totalCap),
	}
	// Add cores(and PUs by extension)
	for i := 0; i < numCores; i++ {
		coreNode := getNewCoreRtnd(pusPerCore, tasksPerPu)
		// Link to parent
		coreNode.ParentId = machineNode.ResourceDesc.Uuid
		machineNode.Children = append(machineNode.Children, coreNode)
	}

	// Set the resource capacity of the machine
	// NOTE: The number of cores in the resource vector is actually the number of PUs in the machine
	machineNode.ResourceDesc.ResourceCapacity = &pb.ResourceVector{
		CpuCores: float32(numCores * pusPerCore),
		RamCap:   1024, // Just some placeholder number for now
	}
	return machineNode
}

// getNewCoreRtnd returns an initialized and fully populated resource topology node
// of type Core with the specified number of PUs as its children
func getNewCoreRtnd(numPUs int, tasksPerPu int) *pb.ResourceTopologyNodeDescriptor {
	totalCap := numPUs * tasksPerPu
	coreNode := &pb.ResourceTopologyNodeDescriptor{
		ResourceDesc: getNewResourceDesc(pb.ResourceDescriptor_ResourceCore, totalCap),
	}
	// Add PUs
	for i := 0; i < numPUs; i++ {
		puNode := getNewPuRtnd(tasksPerPu)
		// Link to parent
		puNode.ParentId = coreNode.ResourceDesc.Uuid
		coreNode.Children = append(coreNode.Children, puNode)
	}
	return coreNode
}

// getNewPuRtnd returns a resource topology node of type PU with the sepcified task capacity
func getNewPuRtnd(taskCap int) *pb.ResourceTopologyNodeDescriptor {
	return &pb.ResourceTopologyNodeDescriptor{
		ResourceDesc: getNewResourceDesc(pb.ResourceDescriptor_ResourcePu, taskCap),
	}
}

// getNewResourceDesc returns an initialized Resource Descriptor of the specified type
func getNewResourceDesc(resourceType pb.ResourceDescriptor_ResourceType, taskCap int) *pb.ResourceDescriptor {
	// TODO: This isn't a good way to generate unique IDs.
	IDString := strconv.FormatUint(util.RandUint64(), 10)
	// Resource name = type + IDString
	name := pb.ResourceDescriptor_ResourceType_name[int32(resourceType)] + IDString
	fmt.Printf("Created Resource: %s\n", name)
	return &pb.ResourceDescriptor{
		Uuid:         IDString,
		FriendlyName: name,
		TaskCapacity: uint64(taskCap),
		Type:         resourceType,
		// Default state and type
		State:       pb.ResourceDescriptor_ResourceIdle,
		Schedulable: true,
	}
}
