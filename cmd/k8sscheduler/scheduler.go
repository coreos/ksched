package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/coreos/ksched/k8s/k8sclient"
	"github.com/coreos/ksched/k8s/k8stype"
	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/types/resourcestatus"
	"github.com/coreos/ksched/pkg/util"
	"github.com/coreos/ksched/pkg/util/idgenerator"
	"github.com/coreos/ksched/pkg/util/queue"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/flowscheduler"
)

type k8scheduler struct {
	// TODO: Abstract the two maps into a wrapper
	// Two maps for bidirectional mapping
	// Internal mapping for k8s nodeID(string) to the machine resource's Uid(string) number
	nodeToMachineID map[string]string
	machineToNodeID map[string]string
	// Internal mapping for k8s nodeID(string) to the task's Uid(uint64) number
	podToTaskID   map[string]uint64
	taskToPodID   map[uint64]string
	resourceMap   *types.ResourceMap
	jobMap        *types.JobMap
	taskMap       *types.TaskMap
	rootNode      *pb.ResourceTopologyNodeDescriptor
	flowScheduler flowscheduler.Scheduler
	client        *k8sclient.Client
	maxTasksPerPu int
	// Capacity on number of tasks per PU(or node in this case since 1 node: 1 PU)
}

func New(client *k8sclient.Client, maxTasksPerPu int) *k8scheduler {
	resourceMap := types.NewResourceMap()
	jobMap := types.NewJobMap()
	taskMap := types.NewTaskMap()
	rootNode := &pb.ResourceTopologyNodeDescriptor{
		ResourceDesc: createResourceDesc(pb.ResourceDescriptor_ResourceCoordinator, 0),
	}
	flowScheduler := flowscheduler.NewScheduler(resourceMap, jobMap, taskMap, rootNode, uint64(maxTasksPerPu))

	return &k8scheduler{
		nodeToMachineID: make(map[string]string),
		machineToNodeID: make(map[string]string),
		podToTaskID:     make(map[string]uint64),
		taskToPodID:     make(map[uint64]string),
		resourceMap:     resourceMap,
		jobMap:          jobMap,
		taskMap:         taskMap,
		rootNode:        rootNode,
		client:          client,
		flowScheduler:   flowScheduler,
		maxTasksPerPu:   maxTasksPerPu,
	}
}

func main() {
	args := os.Args[1:]
	if len(args) != 3 {
		fmt.Printf("Usage: ./scheduler <API-Server-Address> <Number of Machines/Nodes> <Max-Number-Of-Pods-Per-Node>\n")
		os.Exit(1)
	}
	address := args[0]
	// Number of nodes in topology
	numMachines, err := strconv.Atoi(args[1])
	if err != nil {
		log.Panicf(err.Error())
	}
	// Max pods per node
	maxTasksPerPu, err := strconv.Atoi(args[2])
	if err != nil {
		log.Panicf(err.Error())
	}
	// Initialize the kubernetes client
	config := k8sclient.Config{Addr: address}
	client, err := k8sclient.New(config)
	if err != nil {
		log.Panicf(err.Error())
	}

	// Initialize the scheduler
	scheduler := New(client, maxTasksPerPu)

	// Fake the topology
	scheduler.fakeResourceTopology(numMachines)

	// Start the scheduler
	scheduler.Run()
}

// The main workflow of the scheduler happens here
func (ks *k8scheduler) Run() {
	// Initialize the resource topology by polling the node channel for 5 seconds
	// ks.initResourceTopology()

	// Get the pod channel from the client
	podChan := ks.client.GetUnscheduledPodChan()

	// Add one Job to the graph, under which all incoming pods will be added as tasks
	jobID := addNewJob(ks.jobMap, ks.flowScheduler)

	log.Printf("Starting scheduling loop\n")
	// Loop: Read pods, Schedule, and Assign Bindings
	for {

		// Poll on the channel
		if len(podChan) == 0 {
			continue
		}

		// Process new Pod updates
		newPods := make([]*k8stype.Pod, 0)
		// Read all outstanding pods in the channel
		for len(podChan) > 0 {
			select {
			case pod := <-podChan:
				log.Printf("Got Pod request id:%v", pod.ID)
				// Skip addition if duplicate podID
				if _, ok := ks.podToTaskID[pod.ID]; ok {
					continue
				}
				newPods = append(newPods, pod)
			default:
				// Do nothing to do a non blocking read
			}
		}

		// No need to schedule or assign task bindings if no new pods
		if len(newPods) == 0 {
			continue
		}

		log.Printf("Adding Pods as tasks to scheduler\n")
		// Add every new pod as a new task in the flowgraph
		// TODO: Need to rethink later if a Pod should be a Task or a Job
		for _, pod := range newPods {
			// Add the task to the job
			taskID := addTaskToJob(jobID, ks.jobMap, ks.taskMap)
			// Insert mapping for task to pod
			ks.podToTaskID[pod.ID] = uint64(taskID)
			ks.taskToPodID[uint64(taskID)] = pod.ID
		}

		log.Printf("\nPerforming scheduling iteration\n\n")
		// Peform a scheduling iteration
		ks.flowScheduler.ScheduleAllJobs()
		log.Printf("\nScheduling iteration done\n\n")

		// Prepare the Pod to Node bindings
		podToNodeBindings := make([]*k8stype.Binding, 0)
		// Collect scheduling decisions/bindings only for the newly scheduled pods
		// taskBindings will contain old placements as well
		taskBindings := ks.flowScheduler.GetTaskBindings()
		for taskID, resourceID := range taskBindings {
			// The resourceID is for the PU, so we get it's machineID first
			puNode := ks.resourceMap.FindPtrOrNull(resourceID).TopologyNode
			machineID := findParentMachine(puNode, ks.resourceMap).Uuid

			// Get the nodeID corresponding to the machineID
			nodeID := ks.machineToNodeID[machineID]
			// Get the podID corresponding to the taskID
			podID := ks.taskToPodID[uint64(taskID)]
			log.Printf("Binding: pod:%v to node:%v\n", podID, nodeID)
			// Create binding
			binding := &k8stype.Binding{
				PodID:  podID,
				NodeID: nodeID,
			}
			podToNodeBindings = append(podToNodeBindings, binding)
		}

		// Report the bindings for the newly scheduled pods
		ks.client.AssignBinding(podToNodeBindings)
		log.Printf("TaskBindings assigned\n")
	}
}

func (ks *k8scheduler) fakeResourceTopology(numMachines int) {
	nodeIDGen := idgenerator.New(false)
	// Add machines
	for i := 0; i < numMachines; i++ {
		nextID := strconv.FormatUint(nodeIDGen.NextID(), 10)
		// Add the node as a machine to the root ResourceTopologyNodeDescriptor
		machineNode := addMachine(nextID, ks.maxTasksPerPu, ks.rootNode, ks.resourceMap, ks.flowScheduler)
		// Insert mapping for node to machine resource in both maps
		ks.nodeToMachineID[nextID] = machineNode.ResourceDesc.Uuid
		ks.machineToNodeID[machineNode.ResourceDesc.Uuid] = nextID
	}
}

// InitTopology initializes the resourceMap and the resource topology
// by polling the node channel for a while(5 seconds) to get all the nodes
func (ks *k8scheduler) initResourceTopology() {
	nodeChan := ks.client.GetNodeChan()

	done := make(chan bool)
	// Send a done signal after 5 seconds
	go func() {
		<-time.After(5 * time.Second)
		done <- true
	}()

	// Poll until done
	finish := false
	for !finish {
		//Poll for nodes from the node channel
		select {
		case node := <-nodeChan:
			// Skip addition if duplicate nodeID
			if _, ok := ks.nodeToMachineID[node.ID]; ok {
				log.Printf("Duplicate nodeID%v recieved from node channel\n", node.ID)
				continue
			}
			// Add the node as a machine to the root ResourceTopologyNodeDescriptor
			machineNode := addMachine(node.ID, ks.maxTasksPerPu, ks.rootNode, ks.resourceMap, ks.flowScheduler)
			// Insert mapping for node to machine resource in both maps
			ks.nodeToMachineID[node.ID] = machineNode.ResourceDesc.Uuid
			ks.machineToNodeID[machineNode.ResourceDesc.Uuid] = node.ID
		case <-done:
			finish = true
		default:
			// Do nothing to do a non blocking read from the timer and node channels
		}
	}
}

// Creates a new job with no tasks and registers it with the scheduler
func addNewJob(jobMap *types.JobMap, scheduler flowscheduler.Scheduler) types.JobID {
	// Make sure jobID is unique
	jobID := types.JobID(util.RandUint64())
	for jobMap.ContainsKey(jobID) {
		jobID = types.JobID(util.RandUint64())
	}
	jobUuid := strconv.FormatUint(uint64(jobID), 10)
	name := "Job " + jobUuid
	jobDesc := &pb.JobDescriptor{
		Uuid:  jobUuid,
		Name:  name,
		State: pb.JobDescriptor_Created,
	}
	jobMap.InsertIfNotPresent(jobID, jobDesc)
	scheduler.AddJob(jobDesc)
	return jobID
}

// addTaskToJob adds a new task to job with the specified jobID. The job must already exist or it will panic.
// The taskMap is updated for the new task
// Returns the taskID of the task created
func addTaskToJob(jobID types.JobID, jobMap *types.JobMap, taskMap *types.TaskMap) types.TaskID {
	jobDesc := jobMap.FindPtrOrNull(jobID)
	jobUuid := strconv.FormatUint(uint64(jobID), 10)
	if jobDesc == nil {
		log.Panicf("No job for jobID:%v exists in the jobMap\n", jobID)
	}

	// Create a unique taskID
	taskID := types.TaskID(util.RandUint64())
	for taskMap.ContainsKey(taskID) {
		taskID = types.TaskID(util.RandUint64())
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
// It then traverses the machine node topology and updates the resourceMap and finally
// registers the machine node with the scheduler.
func addMachine(nodeID string, tasksPerPu int, root *pb.ResourceTopologyNodeDescriptor, resourceMap *types.ResourceMap, scheduler flowscheduler.Scheduler) *pb.ResourceTopologyNodeDescriptor {
	// Create a new machine topology descriptor and add it as the root's child
	machineNode := createMachineNode(tasksPerPu)
	root.Children = append(root.Children, machineNode)
	// Link machine to root
	machineNode.ParentId = root.ResourceDesc.Uuid

	// Do a bfs from the rootNode and populate the resourceMap.
	// The resourceMap is supposed to be updated outside of the scheduler
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
		// We don't need a visited map because it's tree structure.
		for _, childDesc := range currNode.GetChildren() {
			nodes.Push(childDesc)
		}
	}

	// Register the resource with the scheduler
	scheduler.RegisterResource(machineNode)
	return machineNode

}

// createMachineNode returns an initialized and fully populated resource topology of type Machine
// that looks like machine->PU
// tasksPerPu: The task capacity of each processing unit in the machine.
// The total machine capacity = tasksPerPu
func createMachineNode(tasksPerPu int) *pb.ResourceTopologyNodeDescriptor {
	totalCap := tasksPerPu
	machineNode := &pb.ResourceTopologyNodeDescriptor{
		ResourceDesc: createResourceDesc(pb.ResourceDescriptor_ResourceMachine, totalCap),
	}

	// Add 1 PU for the machine
	puNode := getNewPuRtnd(tasksPerPu)
	// Link to parent
	puNode.ParentId = machineNode.ResourceDesc.Uuid
	machineNode.Children = append(machineNode.Children, puNode)

	// Set the resource capacity of the machine
	machineNode.ResourceDesc.ResourceCapacity = &pb.ResourceVector{
		CpuCores: float32(1), // Hardcoded place holders for now, don't matter without the coco cost model
		RamCap:   1024,
	}
	return machineNode
}

// getNewPuRtnd returns a resource topology node of type PU with the sepcified task capacity
func getNewPuRtnd(taskCap int) *pb.ResourceTopologyNodeDescriptor {
	return &pb.ResourceTopologyNodeDescriptor{
		ResourceDesc: createResourceDesc(pb.ResourceDescriptor_ResourcePu, taskCap),
	}
}

// createResourceDesc returns an initialized Resource Descriptor of the specified type and with the desired ID
func createResourceDesc(resourceType pb.ResourceDescriptor_ResourceType, taskCap int) *pb.ResourceDescriptor {
	// TODO: This isn't a good way to generate unique IDs.
	IDString := strconv.FormatUint(util.RandUint64(), 10)
	// Resource name = type + IDString
	name := fmt.Sprintf("%s-%s", pb.ResourceDescriptor_ResourceType_name[int32(resourceType)], IDString)
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

// FindParentMachine returns the machine descriptor if given any descendant node of the machine.
// If it's not a descendant or the machine node it self then it returns nil.
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
