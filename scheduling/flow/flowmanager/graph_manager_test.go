package flowmanager

import (
	"strconv"
	"testing"

	"github.com/coreos/ksched/pkg/types"
	"github.com/coreos/ksched/pkg/util"
	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/costmodel"
	"github.com/coreos/ksched/scheduling/flow/dimacs"
)

var (
	resourceMap     types.ResourceMap
	taskMap         types.TaskMap
	leafResourceIDs map[types.ResourceID]struct{}
	dimacsStats     *dimacs.ChangeStats
)

func init() {
	resourceMap.Init()
	taskMap.Init()
	leafResourceIDs = make(map[types.ResourceID]struct{})
	dimacsStats = &dimacs.ChangeStats{}
}

func CreateGraphManagerUsingTrivialCost() *graphManager {
	costModeler := costmodel.NewTrivial(&resourceMap, &taskMap, leafResourceIDs)
	gm := NewGraphManager(costModeler, leafResourceIDs, dimacsStats)
	return gm
}

func CreateMachine(rtnd *pb.ResourceTopologyNodeDescriptor, machineName string) *pb.ResourceDescriptor {
	util.SeedStringRng(machineName)
	rID := util.GenerateResourceID()
	rd := rtnd.ResourceDesc
	rd.Uuid = strconv.FormatUint(uint64(rID), 10)
	rd.Type = pb.ResourceDescriptor_ResourceMachine
	return rd
}

func CreateTask(jd pb.JobDescriptor, jobIDSeed uint64) {
	util.SeedIntRng(int64(jobIDSeed))
	jobID := util.GenerateJobID()
	jd.Uuid = strconv.FormatUint(uint64(jobID), 10)
}

func TestAddResourceNode(t *testing.T) {
	//TODO
}
