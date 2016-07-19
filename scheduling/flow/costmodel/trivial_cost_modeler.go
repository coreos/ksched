package costmodel

import (
	"fmt"

	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/util"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/flowgraph"
)

// make sure trivialCostModeler implements CostModeler
var _ CostModeler = new(trivialCostModeler)

// Note: not thread safe
type trivialCostModeler struct {
	// resourceMap is passed in and maintained by user.
	resourceMap *types.ResourceMap
	// taskMap is passed in and maintained by user.
	taskMap *types.TaskMap
	// leafResIDset is passed in and maintained by user.
	leafResIDset map[types.ResourceID]struct{}
	// Mapping betweeen machine res id and resource topology node descriptor.
	// It's updated in Add, Remove Machine methods.
	machineToResTopo map[types.ResourceID]*pb.ResourceTopologyNodeDescriptor
	//Flag set on initialization
	maxTasksPerPu uint64
}

func NewTrivial(resourceMap *types.ResourceMap, taskMap *types.TaskMap, leafResIDset map[types.ResourceID]struct{}, maxTasksPerPu uint64) *trivialCostModeler {
	return &trivialCostModeler{
		resourceMap:      resourceMap,
		taskMap:          taskMap,
		leafResIDset:     leafResIDset,
		machineToResTopo: make(map[types.ResourceID]*pb.ResourceTopologyNodeDescriptor),
		maxTasksPerPu:    maxTasksPerPu,
	}
}

func (t *trivialCostModeler) TaskToUnscheduledAggCost(types.TaskID) Cost {
	return 5
}

func (t *trivialCostModeler) UnscheduledAggToSinkCost(types.JobID) Cost {
	return 0
}

func (t *trivialCostModeler) TaskToResourceNodeCost(types.TaskID, types.ResourceID) Cost {
	return 0
}

func (t *trivialCostModeler) ResourceNodeToResourceNodeCost(source *pb.ResourceDescriptor, destination *pb.ResourceDescriptor) Cost {
	return 0
}

func (t *trivialCostModeler) LeafResourceNodeToSinkCost(types.ResourceID) Cost {
	return 0
}

func (t *trivialCostModeler) TaskContinuationCost(types.TaskID) Cost {
	return 0
}

func (t *trivialCostModeler) TaskPreemptionCost(types.TaskID) Cost {
	return 0
}

func (t *trivialCostModeler) TaskToEquivClassAggregator(id types.TaskID, ec types.EquivClass) Cost {
	if ec == ClusterAggregatorEC {
		return 2
	}
	return 0
}

func (t *trivialCostModeler) EquivClassToResourceNode(ec types.EquivClass, id types.ResourceID) (Cost, uint64) {
	rs := t.resourceMap.FindPtrOrNull(id)
	if rs == nil {
		panic(fmt.Errorf("couldn't find resource status for (%d)", id))
	}
	freeSlotNum := rs.Descriptor.NumSlotsBelow - rs.Descriptor.NumRunningTasksBelow
	return 0, freeSlotNum
}

func (t *trivialCostModeler) EquivClassToEquivClass(tec1 types.EquivClass, tec2 types.EquivClass) (Cost, uint64) {
	return 0, 0
}

func (t *trivialCostModeler) GetTaskEquivClasses(id types.TaskID) []types.EquivClass {
	task := t.taskMap.FindPtrOrNull(id)
	if task == nil {
		panic(fmt.Errorf("couldn't find task for (%d)", id))
	}
	// A level 0 Task EC is the hash of the task binary name.
	res := []types.EquivClass{} //util.HashBytesToEquivClass(task.Binary)}
	// All tasks also have an arc to the cluster aggregator.
	res = append(res, ClusterAggregatorEC)
	return res
}

func (t *trivialCostModeler) GetOutgoingEquivClassPrefArcs(ec types.EquivClass) []types.ResourceID {
	if ec != ClusterAggregatorEC {
		return nil
	}
	res := make([]types.ResourceID, 0, len(t.machineToResTopo))
	for m := range t.machineToResTopo {
		res = append(res, m)
	}
	return res
}

func (t *trivialCostModeler) GetTaskPreferenceArcs(types.TaskID) []types.ResourceID {
	/*
		for id := range t.leafResIDset {
			// Pick first one.
			// The original code picks randomly from the set. Too complicated.
			return []types.ResourceID{id}
		}
	*/
	// Return no preferences
	return []types.ResourceID{}
}

func (t *trivialCostModeler) GetEquivClassToEquivClassesArcs(types.EquivClass) []types.EquivClass {
	// The trivial cost model does not have any interconnected ECs.
	return []types.EquivClass{}
}

func (t *trivialCostModeler) AddMachine(r *pb.ResourceTopologyNodeDescriptor) {
	id, err := util.ResourceIDFromString(r.ResourceDesc.Uuid)
	if err != nil {
		panic(err)
	}
	if _, ok := t.machineToResTopo[id]; !ok {
		t.machineToResTopo[id] = r
	}
}

func (t *trivialCostModeler) AddTask(types.TaskID) {}

func (t *trivialCostModeler) RemoveMachine(id types.ResourceID) {
	delete(t.machineToResTopo, id)
}

func (t *trivialCostModeler) RemoveTask(types.TaskID) {}

func (t *trivialCostModeler) GatherStats(accumulator *flowgraph.Node, other *flowgraph.Node) *flowgraph.Node {
	if !accumulator.IsResourceNode() {
		return accumulator
	}
	if !other.IsResourceNode() {
		if other.Type == flowgraph.NodeTypeSink {
			accumulator.ResourceDescriptor.NumRunningTasksBelow = uint64(len(accumulator.ResourceDescriptor.CurrentRunningTasks))
			accumulator.ResourceDescriptor.NumSlotsBelow = t.maxTasksPerPu
		}
		return accumulator
	}
	if other.ResourceDescriptor == nil {
		panic(fmt.Errorf("the ResourceDescriptor of node (%d) is nil", other.ID))
	}

	accumulator.ResourceDescriptor.NumRunningTasksBelow += other.ResourceDescriptor.NumRunningTasksBelow
	accumulator.ResourceDescriptor.NumSlotsBelow += other.ResourceDescriptor.NumSlotsBelow
	return accumulator
}

func (t *trivialCostModeler) PrepareStats(accumulator *flowgraph.Node) {
	if !accumulator.IsResourceNode() {
		return
	}
	if accumulator.ResourceDescriptor == nil {
		panic(fmt.Errorf("the ResourceDescriptor of node (%d) is nil", accumulator.ID))
	}
	accumulator.ResourceDescriptor.NumRunningTasksBelow = 0
	accumulator.ResourceDescriptor.NumSlotsBelow = 0
}

func (t *trivialCostModeler) UpdateStats(accumulator *flowgraph.Node, other *flowgraph.Node) *flowgraph.Node {
	return accumulator
}

func (t *trivialCostModeler) DebugInfo() string {
	return ""
}

func (t *trivialCostModeler) DebugInfoCSV() string {
	return ""
}
