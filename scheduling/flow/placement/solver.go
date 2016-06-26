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

package placement

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"

	"github.com/coreos/ksched/scheduling/flow/dimacs"
	"github.com/coreos/ksched/scheduling/flow/flowgraph"
	"github.com/coreos/ksched/scheduling/flow/flowmanager"
)

var (
	FlowlesslyBinary                 = "bin/flowlessly/flow_scheduler"
	FlowlesslyAlgorithm              = "fast_cost_scaling"
	FlowlesslyInitialRunsAlgorithm   = ""
	FlowlesslyNumberInitialRuns      = 0
	OnlyReadTaskAssignmentChanges    = false
	FlowlesslyFlipAlgorithms         = false
	FlowlesslyBestAlgorithm          = false
	FlowlesslyRunCostScalingAndRelax = false
	FlowlesslyAlphaFactor            = 9
)

type Solver interface {
	Solve() flowmanager.TaskMapping
}

type flowlesslySolver struct {
	isSolverStarted bool
	gm              flowmanager.GraphManager
	toSolver        io.Writer
	fromSolver      io.Reader
}

// NOTE: assume we don't have debug flag
// NOTE: assume we only do incremental flow
// Note: assume we run Solve() iteratively and sequentially without concurrency.
func (fs *flowlesslySolver) Solve() flowmanager.TaskMapping {
	// Note: combine all the first time logic into this once function.
	// This is different from original cpp code.
	if !fs.isSolverStarted {
		fs.isSolverStarted = true

		fs.startSolver()
		// We must export graph and read from STDOUT/STDERR in parallel
		// Otherwise, the solver might block if STDOUT/STDERR buffer gets full.
		// (For example, if it outputs lots of warnings on STDERR.)
		go fs.exportGraph()
		tm := fs.readTaskMapping()
		// Exporter should have already finished writing because reading goroutine
		// have also finished.
		return tm
	}

	fs.gm.UpdateAllCostsToUnscheduledAggs()
	go fs.exportIncremental()
	tm := fs.readTaskMapping()
	return tm
}
func (fs *flowlesslySolver) startSolver() {
	binaryStr, args := fs.getBinConfig()
	cmd := exec.Command(binaryStr, args...)
	err := cmd.Start()
	if err != nil {
		panic(err)
	}
	fs.toSolver, err = cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	fs.fromSolver, err = cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
}
func (fs *flowlesslySolver) exportGraph() {
	dimacs.Export(fs.gm.GraphChangeManager().Graph(), fs.toSolver)
	fs.gm.GraphChangeManager().ResetChanges()
}

func (fs *flowlesslySolver) exportIncremental() {
	dimacs.ExportIncremental(fs.gm.GraphChangeManager().GetOptimizedGraphChanges(), fs.toSolver)
	fs.gm.GraphChangeManager().ResetChanges()
}

func (fs *flowlesslySolver) readTaskMapping() flowmanager.TaskMapping {
	extractedFlow := fs.readFlowGraph()
	return fs.parseFlowToMapping(extractedFlow)
}

// readFlowGraph returns a map of dst to a list of its corresponding src and flow capacity.
func (fs *flowlesslySolver) readFlowGraph() map[flowgraph.NodeID]flowPairList {
	adjList := map[flowgraph.NodeID]flowPairList{}
	scanner := bufio.NewScanner(fs.fromSolver)
	for scanner.Scan() {
		line := scanner.Text()
		switch line[0] {
		case 'f':
			var src, dst, flowCap uint64
			n, err := fmt.Sscanf(line, "%*c %d %d %d", &src, &dst, &flowCap)
			if err != nil {
				panic(err)
			}
			if n != 3 {
				panic("expected reading 3 items")
			}

			if flowCap > 0 {
				pair := &flowPair{flowgraph.NodeID(src), flowCap}
				adjList[flowgraph.NodeID(dst)].Insert(pair)
			}
		case 'c':
			if line == "c EOI" {
				return adjList
			} else if line == "c ALGORITHM TIME" {
				// Ignore. This is metrics of runtime.
			}
		case 's':
			// we don't care about cost
		default:
			panic("unknown: " + line)
		}
	}
	panic("wrong state")
}

// Maps worker|root tasks to leaves. It expects a extracted_flow containing
// only the arcs with positive flow (i.e. what ReadFlowGraph returns).
func (fs *flowlesslySolver) parseFlowToMapping(extractedFlow map[flowgraph.NodeID]flowPairList) flowmanager.TaskMapping {
	taskToPU := flowmanager.TaskMapping{}
	// Note:
	puIDs := make(map[flowgraph.NodeID][]flowgraph.NodeID)
	graph := fs.gm.GraphChangeManager().Graph()
	visited := make([]bool, graph.NumNodes()+1) // assuming node ID range is 1 to N.
	toVisit := make([]flowgraph.NodeID, 0)      // fifo queue
	leafIDs := fs.gm.LeafNodeIDs()
	sink := fs.gm.SinkNode()

	for leafID := range leafIDs {
		visited[leafID] = true
		flow, ok := extractedFlow[sink.ID].Find(leafID)
		if !ok {
			continue
		}
		for i := uint64(0); i < flow.capacity; i++ { // capacity of flow
			puIDs[leafID] = append(puIDs[leafID], leafID)
		}
		toVisit = append(toVisit, leafID)
	}

	// a variant of breath-frist search
	for len(toVisit) != 0 {
		nodeID := toVisit[0]
		toVisit = toVisit[1:]
		visited[nodeID] = true

		if fs.isTaskNode(nodeID) {
			// It's a task node.
			for _, puID := range puIDs[nodeID] {
				taskToPU.Insert(nodeID, puID)
			}
			continue
		}

		addPUToSourceNodes(extractedFlow, puIDs, nodeID, visited, toVisit)
	}

	return taskToPU
}

func addPUToSourceNodes(extractedFlow map[flowgraph.NodeID]flowPairList, puIDs map[flowgraph.NodeID][]flowgraph.NodeID, nodeID flowgraph.NodeID, visited []bool, toVisit []flowgraph.NodeID) {
	iter := 0
	// search each source and assign all its downstream PUs to them.
	for _, srcFlow := range extractedFlow[nodeID] {
		// Populate the PUs vector at the source of the arc with as many PU
		// entries from the incoming set of PU IDs as there's flow on the arc.
		for ; srcFlow.capacity > 0; srcFlow.capacity-- {
			if iter == len(puIDs[nodeID]) {
				break
			}
			// It's an incoming arc with flow on it.
			// Add the PU to the PUs vector of the source node.
			puIDs[srcFlow.nodeID] = append(puIDs[srcFlow.nodeID], puIDs[nodeID][iter])
			iter++
		}
		if !visited[srcFlow.nodeID] {
			toVisit = append(toVisit, srcFlow.nodeID)
			visited[srcFlow.nodeID] = true
		}

		if iter == len(puIDs[nodeID]) {
			// No more PUs left to assign
			break
		}
	}
}

func (fs *flowlesslySolver) isTaskNode(nodeID flowgraph.NodeID) bool {
	// TODO: We need to make sure locking on graph change manager.
	return fs.gm.GraphChangeManager().CheckNodeType(nodeID, flowgraph.NodeTypeRootTask) ||
		fs.gm.GraphChangeManager().CheckNodeType(nodeID, flowgraph.NodeTypeUnscheduledTask) ||
		fs.gm.GraphChangeManager().CheckNodeType(nodeID, flowgraph.NodeTypeScheduledTask)
}

// TODO: We can definitely make it cleaner. But currently we just copy the code.
func (fs *flowlesslySolver) getBinConfig() (string, []string) {
	args := []string{"--graph_has_node_types=true"}

	args = append(args, fmt.Sprintf("--algorithm=%s", FlowlesslyAlgorithm))

	if OnlyReadTaskAssignmentChanges {
		args = append(args, "--print_assignments=true")
	} else {
		args = append(args, "--print_assignments=false")
	}

	if FlowlesslyInitialRunsAlgorithm == "" {
		args = append(args, fmt.Sprintf("--algorithm_initial_solver_runs=%s", FlowlesslyInitialRunsAlgorithm))
		args = append(args, fmt.Sprintf("--algorithm_number_initial_runs=%d", FlowlesslyNumberInitialRuns))
	}

	if FlowlesslyFlipAlgorithms {
		args = append(args, "--flip_algorithms")
	}

	if FlowlesslyBestAlgorithm {
		args = append(args, "--best_flowlessly_algorithm")
	}

	if FlowlesslyRunCostScalingAndRelax {
		args = append(args, "--run_cost_scaling_and_relax")
	}

	args = append(args, fmt.Sprintf("--alpha_scaling_factor=%d", FlowlesslyAlphaFactor))
	return FlowlesslyBinary, args
}
