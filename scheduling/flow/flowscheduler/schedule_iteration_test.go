package flowscheduler

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/types/resourcestatus"
	"github.com/coreos/ksched/pkg/util"
	"github.com/coreos/ksched/pkg/util/queue"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/flowscheduler"
)

// Needed to specify
var MaxTasksPerPU int = 1

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

	// Add Machines
	// Need to prepare the ResourceTopologyNode(and ResourceDescriptors) for the entire machine
	// before adding it to the topology
	AddMachine(rootNode, resourceMap, scheduler)
	AddMachine(rootNode, resourceMap, scheduler)

	// TODO:
	// Add Tasks/Job
	// Don't need to worry about the resource usage or request vector since cost model is trivial
	// Check simulator_bridge.cc and simulator_bridge_test.cc to see how machines and tasks are added

	// Finally what is the output to observe after 1 scheduling iteration?
	// Can just print out the scheduling deltas to see where Task is placed

}

// AddMachine creates and adds a new machine topology descriptor to a root topology node
// It also updates the resourceMap and registers the resource with the scheduler
func AddMachine(root *pb.ResourceTopologyNodeDescriptor, resourceMap *types.ResourceMap, scheduler flowscheduler.Scheduler) {
	// Create a new machine topology descriptor and add it as the root's child
	// Hard code a machine topology for now, 2 cores, 1 PU each
	machineNode := getNewMachineRtnd(2, 1, MaxTasksPerPU)
	root.Children = append(root.Children, machineNode)
	// Link machine to root
	machineNode.ParentId = root.ResourceDesc.Uuid

	// Do a dfs from the rootNode and populate the resourceMap,
	// since the resourceMap is not maintained by the scheduler
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
