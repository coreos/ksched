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

// The FlowGraphChangeManager bridges FlowGraphManager and FlowGraph. Every
// graph change done by the FlowGraphManager should be conducted via
// FlowGraphChangeManager's methods.
// The class stores all the changes conducted in-between two scheduling rounds.
// Moreover, FlowGraphChangeManager applies various algorithms to reduce
// the number of changes (e.g., merges idempotent changes, removes superfluous
// changes).

package manage

import (
	cl "github.com/coreos/ksched/scheduling/flow/cluster"
	di "github.com/coreos/ksched/scheduling/flow/dimacs" // Mostly empty stubs
)

type ChangeManager interface {
	AddArcNew(src, dst *cl.FlowGraphNode,
		capLowerBound, capUpperBound uint64,
		cost int64,
		arcType cl.FlowArcType,
		changeType di.DimacsChangeType,
		comment string) *cl.FlowGraphArc

	AddArcExisting(srcNodeID, dstNodeID, capLowerBound, capUpperBound uint64,
		cost int64,
		arcType cl.FlowArcType,
		changeType di.DimacsChangeType,
		comment string) *cl.FlowGraphArc

	AddNode(nodeType cl.FlowNodeType,
		excess int64,
		changeType di.DimacsChangeType,
		comment string) *cl.FlowGraphNode

	ChangeArc(arc cl.FlowGraphArc, capLowerBound uint64,
		capUpperBound uint64, cost int64,
		changeType di.DimacsChangeType, comment string)

	ChangeArcCapacity(arc cl.FlowGraphArc, capacity uint64,
		changeType di.DimacsChangeType, comment string)

	ChangeArcCost(arc cl.FlowGraphArc, cost int64,
		changeType di.DimacsChangeType, comment string)

	DeleteArc(arc cl.FlowGraphArc, changeType di.DimacsChangeType, comment string)

	DeleteNode(arc cl.FlowGraphNode, changeType di.DimacsChangeType, comment string)

	// Smaller 2-3 line functions implemented in flow_graph_change_manager.h
	GetGraphChanges() []*di.DimacsChange
	GetOptimizedGraphChanges() []*di.DimacsChange
	ResetChanges()
	CheckNodeType(nodeID uint64, typ cl.FlowNodeType) bool
	// flow_graph() Returns flow graph instance for this manager
	GetFlowGraph() *cl.FlowGraph
	// No need for mutable_flow_graph(), treat as above
	GetNode(nodeID uint64) *cl.FlowGraphNode
}

// The change manager that should implement the ChangeMangerInterface
type changeManager struct {
	flowGraph *cl.FlowGraph
	// Vector storing the graph changes occured since the last scheduling round.
	graphChanges []*di.DimacsChange
	dimacsStats  *di.DimacsChangeStats
}

// Public Interface functions
func (cm *changeManager) AddArcNew(src, dst *cl.FlowGraphNode,
	capLowerBound, capUpperBound uint64,
	cost int64,
	arcType cl.FlowArcType,
	changeType di.DimacsChangeType,
	comment string) *cl.FlowGraphArc {
	return nil
}

func (cm *changeManager) AddArcExisting(srcNodeID, dstNodeID, capLowerBound, capUpperBound uint64,
	cost int64,
	arcType cl.FlowArcType,
	changeType di.DimacsChangeType,
	comment string) *cl.FlowGraphArc {
	return nil
}

func (cm *changeManager) AddNode(nodeType cl.FlowNodeType,
	excess int64,
	changeType di.DimacsChangeType,
	comment string) *cl.FlowGraphNode {
	return nil
}

func (cm *changeManager) ChangeArc(arc cl.FlowGraphArc, capLowerBound uint64,
	capUpperBound uint64, cost int64,
	changeType di.DimacsChangeType, comment string) {

}

func (cm *changeManager) ChangeArcCapacity(arc cl.FlowGraphArc, capacity uint64,
	changeType di.DimacsChangeType, comment string) {

}

func (cm *changeManager) ChangeArcCost(arc cl.FlowGraphArc, cost int64,
	changeType di.DimacsChangeType, comment string) {

}

func (cm *changeManager) DeleteArc(arc cl.FlowGraphArc, changeType di.DimacsChangeType, comment string) {

}

func (cm *changeManager) DeleteNode(arc cl.FlowGraphNode, changeType di.DimacsChangeType, comment string) {

}

// Smaller 2-3 line functions implemented in flow_graph_change_manager.h
func (cm *changeManager) GetGraphChanges() []*di.DimacsChange {
	return nil
}

func (cm *changeManager) GetOptimizedGraphChanges() []*di.DimacsChange {
	return nil
}

func (cm *changeManager) ResetChanges() {

}

func (cm *changeManager) CheckNodeType(nodeID uint64, typ cl.FlowNodeType) bool {
	return false
}

// flow_graph() Returns flow graph instance for this manager
// No need for mutable_flow_graph(), treat bothe the same
func (cm *changeManager) GetFlowGraph() *cl.FlowGraph {
	return nil
}

func (cm *changeManager) GetNode(nodeID uint64) *cl.FlowGraphNode {
	return nil
}

// Private helper methods for change_manager internal use
func (cm *changeManager) addGraphChange(change *di.DimacsChange) {

}

func (cm *changeManager) optimizeChanges() {

}

func (cm *changeManager) mergeChangesToSameArc() {

}

// Checks if there's already a change for the (src_id, dst_id) arc.
// If there's no change then it adds one to the state, otherwise
// it updates the existing change.
func (cm *changeManager) mergeChangesToSameArcHelper(
	srcID, dstID, capLowerBound, capUpperBound uint64,
	cost int64, typ cl.FlowArcType,
	change *di.DimacsChange, newGraphChanges []*di.DimacsChange,
	arcsSrcChanges map[uint64]map[uint64]*di.DimacsChange,
	arcsDstChanges map[uint64]map[uint64]*di.DimacsChange) {

}

func (cm *changeManager) purgeChangesBeforeNodeRemoval() {

}

func (cm *changeManager) removeDuplicateChanges() {

}

// Checks if there's already an identical change for the (src_id, dst_id) arc.
// If there's no change the it updates the state, otherwise it just ignores
// the change we're currently processing because it's duplicate.
func (cm *changeManager) removeDuplicateChangesHelper(
	srcID, dstID uint64, change *di.DimacsChange,
	newGraphChanges []*di.DimacsChange,
	node_to_change map[uint64]map[string]*di.DimacsChange) {

}

func (cm *changeManager) removeDuplicateChangesUpdateState(
	nodeID uint64, change *di.DimacsChange,
	node_to_change map[uint64]map[string]*di.DimacsChange) bool {
	return false
}

// Method to be called upon node addition. This method makes sure that the
// state is cleaned when we re-use a node id.
func (cm *changeManager) removeDuplicateCleanState(
	newNodeID, srcID, dstID uint64,
	changeDesc string,
	node_to_change map[uint64]map[string]*di.DimacsChange) {

}
