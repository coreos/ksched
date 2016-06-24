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
	"sync"

	"github.com/coreos/ksched/scheduling/flow/dimacs"
	"github.com/coreos/ksched/scheduling/flow/flowgraph"
	"github.com/coreos/ksched/scheduling/flow/flowmanager"
)

var (
	FlowlesslyBinary                 = "bin/flowlessly/flow_scheduler"
	FlowlesslyAlgorithm              = "fast_cost_scaling"
	FlowlesslyInitialRunsAlgorithm   = ""
	FlowlesslyNumberInitialRuns      = 0
	OnlyReadAssignmentChanges        = false
	FlowlesslyFlipAlgorithms         = false
	FlowlesslyBestAlgorithm          = false
	FlowlesslyRunCostScalingAndRelax = false
	FlowlesslyAlphaFactor            = 9
)

type Solver interface {
	Solve() flowmanager.TaskMappings
}

type flowlesslySolver struct {
	once sync.Once
	gm   flowmanager.GraphManager

	toSolver   io.Writer
	fromSolver io.Reader
}

// NOTE: assume we don't have debug flag
// NOTE: assume we only do incremental flow
func (fs *flowlesslySolver) Solve() flowmanager.TaskMappings {
	var tm flowmanager.TaskMappings
	// Note: combine all the first time logic into this once function.
	// This is different from original cpp code.
	firstTime := false
	fs.once.Do(func() {
		firstTime = true

		binaryStr, args := solverConfig()
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

		// We must export graph and read from STDOUT/STDERR in parallel
		// Otherwise, the solver might block if STDOUT/STDERR buffer gets full.
		// (For example, if it outputs lots of warnings on STDERR.)
		done := fs.goExportGraph()
		tm = fs.readOutput()
		// Wait for exporter to complete. (Should already have happened when we
		// get here, given we've finished reading the output.)
		<-done
	})
	if firstTime {
		return tm
	}

	fs.gm.UpdateAllCostsToUnscheduledAggs()
	done := fs.goExportIncremental
	tm = fs.readOutput()
	<-done
	return tm
}

func (fs *flowlesslySolver) goExportGraph() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		dimacs.Export(fs.gm.GraphChangeManager().Graph(), fs.toSolver)
		fs.gm.GraphChangeManager().ResetChanges()
		close(done)
	}()
	return done
}

func (fs *flowlesslySolver) goExportIncremental() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		dimacs.ExportIncremental(fs.gm.GraphChangeManager().GetOptimizedGraphChanges(), fs.toSolver)
		fs.gm.GraphChangeManager().ResetChanges()
		close(done)
	}()
	return done
}

func (fs *flowlesslySolver) readOutput() flowmanager.TaskMappings {
	extractedFlow := fs.readFlowGraph()
	return fs.getMappings(extractedFlow)
}

func (fs *flowlesslySolver) readFlowGraph() map[flowgraph.NodeID]flowPairList {
	adjList := map[flowgraph.NodeID]flowPairList{}
	scanner := bufio.NewScanner(fs.fromSolver)
	for scanner.Scan() {
		line := scanner.Text()
		switch line[0] {
		case 'f':
			var src, dst, flow uint64
			n, err := fmt.Sscanf(line, "%*c %d %d %d", &src, &dst, &flow)
			if err != nil {
				panic(err)
			}
			if n != 3 {
				panic("")
			}
			if flow > 0 {
				adjList[flowgraph.NodeID(dst)].Insert(
					&flowPair{flowgraph.NodeID(src), flow})
			}
		case 'c':
			if line != "c EOI" {
				break
			} else if line != "c ALGORITHM TIME" {
				// Ignore. This is metrics of runtime.
			}
		case 's':
			// we don't care about cost
		default:
			panic("unknown: " + line)
		}
	}
	return adjList
}

// Maps worker|root tasks to leaves. It expects a extracted_flow containing
// only the arcs with positive flow (i.e. what ReadFlowGraph returns).
func (fs *flowlesslySolver) getMappings(extractedFlow map[flowgraph.NodeID]flowPairList) flowmanager.TaskMappings {
	taskToPu := make(map[flowgraph.NodeID][]flowgraph.NodeID)
	puIDs := make(map[flowgraph.NodeID][]flowgraph.NodeID)
	graph := fs.gm.GraphChangeManager().Graph()
	visited := make([]bool, graph.NumNodes()+1) // assuming node ID is 1 to N.
	toVisit := make([]flowgraph.NodeID, 0)
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

	for len(toVisit) != 0 {
		nodeID := toVisit[0]
		toVisit = toVisit[1:]
		visited[nodeID] = true
		if fs.gm.GraphChangeManager().CheckNodeType(nodeID, flowgraph.NodeTypeRootTask) ||
			fs.gm.GraphChangeManager().CheckNodeType(nodeID, flowgraph.NodeTypeUnscheduledTask) ||
			fs.gm.GraphChangeManager().CheckNodeType(nodeID, flowgraph.NodeTypeScheduledTask) {
			// It's a task node.
			for _, puID := range puIDs[nodeID] {
				taskToPu[nodeID] = append(taskToPu[nodeID], puID)
			}
		} else {
			iter := 0
			for _, srcFlow := range extractedFlow[nodeID] {
				assignedAllPus := false
				for srcFlow.capacity > 0 {
					if iter == len(puIDs[nodeID]) {
						assignedAllPus = true
						break
					}
					puIDs[srcFlow.nodeID] = append(puIDs[srcFlow.nodeID], puIDs[nodeID][iter])
					iter++
					srcFlow.capacity--
				}
				if !visited[srcFlow.nodeID] {
					toVisit = append(toVisit, srcFlow.nodeID)
					visited[srcFlow.nodeID] = true
				}
				if assignedAllPus {
					break
				}
			}
		}
	}

	return flowmanager.TaskMappings(taskToPu)
}

// Note: assume we only use flowlessly solver
func solverConfig() (string, []string) {
	args := []string{"--graph_has_node_types=true"}
	args = append(args, fmt.Sprintf("--algorithm=%s", FlowlesslyAlgorithm))
	if OnlyReadAssignmentChanges {
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
