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

package flowmanager

import (
	"github.com/coreos/ksched/scheduling/flow/dimacs"
	"github.com/coreos/ksched/scheduling/flow/flowgraph"
)

// The GraphChangeManager bridges GraphManager and Graph. Every
// graph change done by the GraphManager should be conducted via
// FlowGraphChangeManager's methods.
// The class stores all the changes conducted in-between two scheduling rounds.
// Moreover, FlowGraphChangeManager applies various algorithms to reduce
// the number of changes (e.g., merges idempotent changes, removes superfluous
// changes).
type GraphChangeManager interface {
	AddArc(src, dst *flowgraph.Node,
		capLowerBound, capUpperBound uint64,
		cost int64,
		arcType flowgraph.ArcType,
		changeType dimacs.ChangeType,
		comment string) *flowgraph.Arc

	AddNode(nodeType flowgraph.NodeType,
		excess int64,
		changeType dimacs.ChangeType,
		comment string) *flowgraph.Node

	ChangeArc(arc *flowgraph.Arc, capLowerBound uint64,
		capUpperBound uint64, cost int64,
		changeType dimacs.ChangeType, comment string)

	ChangeArcCapacity(arc *flowgraph.Arc, capacity uint64,
		changeType dimacs.ChangeType, comment string)

	ChangeArcCost(arc *flowgraph.Arc, cost int64,
		changeType dimacs.ChangeType, comment string)

	DeleteArc(arc *flowgraph.Arc, changeType dimacs.ChangeType, comment string)

	DeleteNode(arc *flowgraph.Node, changeType dimacs.ChangeType, comment string)

	GetGraphChanges() []dimacs.Change

	GetOptimizedGraphChanges() []dimacs.Change

	// ResetChanges resets the incremental changes that the manager keeps.
	// This method should be called after consuming all the recent changes.
	ResetChanges()

	// Graph returns flow graph instance for this manager.
	Graph() *flowgraph.Graph

	CheckNodeType(flowgraph.NodeID, flowgraph.NodeType) bool
}

// The change manager that should implement the ChangeMangerInterface
type changeManager struct {
	// Optimization options
	RemoveDuplicate        bool
	MergeToSameArc         bool
	PurgeBeforeNodeRemoval bool

	flowGraph *flowgraph.Graph
	// Vector storing the graph changes occured since the last scheduling round.
	graphChanges []dimacs.Change
	dimacsStats  *dimacs.ChangeStats
}

func NewChangeManager(dimacsStats *dimacs.ChangeStats) *changeManager {
	cm := changeManager{flowGraph: flowgraph.NewGraph(false), dimacsStats: dimacsStats}
	return &cm
}

func (cm *changeManager) CheckNodeType(id flowgraph.NodeID, typ flowgraph.NodeType) bool {
	return cm.flowGraph.Node(id).Type == typ
}

// Public Interface functions
func (cm *changeManager) AddArc(src, dst *flowgraph.Node,
	capLowerBound, capUpperBound uint64,
	cost int64,
	arcType flowgraph.ArcType,
	changeType dimacs.ChangeType,
	comment string) *flowgraph.Arc {

	//log.Printf("Change:%s - Added arc from src(%s):%d to dst(%s):%d\n", comment, src.Type, src.ID, dst.Type, dst.ID)

	arc := cm.flowGraph.AddArc(src, dst)
	arc.CapLowerBound = capLowerBound
	arc.CapUpperBound = capUpperBound
	arc.Cost = cost
	arc.Type = arcType

	change := dimacs.NewCreateArcChange(arc)
	change.SetComment(comment)
	cm.addGraphChange(change)
	cm.dimacsStats.UpdateStats(changeType)
	return arc
}

func (cm *changeManager) AddNode(t flowgraph.NodeType, excess int64, changeType dimacs.ChangeType, comment string) *flowgraph.Node {
	n := cm.flowGraph.AddNode()
	n.Type = t
	n.Excess = excess
	n.Comment = comment

	//log.Printf("Change:%s - Added node(%s):%d\n", comment, n.Type, n.ID)

	change := dimacs.NewAddNodeChange(n)
	change.SetComment(comment)
	cm.addGraphChange(change)
	cm.dimacsStats.UpdateStats(changeType)
	return n
}

func (cm *changeManager) DeleteNode(n *flowgraph.Node, changeType dimacs.ChangeType, comment string) {
	//log.Printf("Change:%s - Deleted node(%s):%d\n", comment, n.Type, n.ID)

	change := &dimacs.RemoveNodeChange{
		ID: uint64(n.ID),
	}
	change.SetComment(comment)
	cm.addGraphChange(change)
	cm.dimacsStats.UpdateStats(changeType)
	cm.flowGraph.DeleteNode(n)

}

func (cm *changeManager) ChangeArc(arc *flowgraph.Arc, lower, upper uint64, cost int64, changeType dimacs.ChangeType, comment string) {
	oldCost := arc.Cost
	if arc.CapLowerBound == lower && arc.CapUpperBound == upper && oldCost == cost {
		return
	}

	arc.CapLowerBound = lower
	arc.CapUpperBound = upper
	arc.Cost = cost

	change := dimacs.NewUpdateArcChange(arc, oldCost)
	change.SetComment(comment)
	cm.addGraphChange(change)
	cm.dimacsStats.UpdateStats(changeType)
}

func (cm *changeManager) ChangeArcCapacity(arc *flowgraph.Arc, capacity uint64, changeType dimacs.ChangeType, comment string) {
	oldcap := arc.CapUpperBound
	if oldcap == capacity {
		return
	}
	arc.CapUpperBound = capacity

	change := dimacs.NewUpdateArcChange(arc, arc.Cost)
	change.SetComment(comment)
	cm.addGraphChange(change)
	cm.dimacsStats.UpdateStats(changeType)
}

func (cm *changeManager) ChangeArcCost(arc *flowgraph.Arc, cost int64, changeType dimacs.ChangeType, comment string) {
	oldCost := arc.Cost
	if oldCost == cost {
		return
	}
	arc.Cost = cost

	change := dimacs.NewUpdateArcChange(arc, oldCost)
	change.SetComment(comment)
	cm.addGraphChange(change)
	cm.dimacsStats.UpdateStats(changeType)
}

func (cm *changeManager) DeleteArc(arc *flowgraph.Arc, changeType dimacs.ChangeType, comment string) {
	arc.CapUpperBound = 0
	arc.CapLowerBound = 0
	//log.Printf("Change:%s - Deleted arc from src(%s):%d to dst(%s):%d\n", comment, arc.SrcNode.Type, arc.SrcNode.ID, arc.DstNode.Type, arc.DstNode.ID)
	change := dimacs.NewUpdateArcChange(arc, arc.Cost)
	change.SetComment(comment)
	cm.addGraphChange(change)
	cm.dimacsStats.UpdateStats(changeType)
	cm.flowGraph.DeleteArc(arc)
}

func (cm *changeManager) GetGraphChanges() []dimacs.Change {
	return cm.graphChanges
}

func (cm *changeManager) GetOptimizedGraphChanges() []dimacs.Change {
	// cm.optimizeChanges()
	return cm.graphChanges
}

func (cm *changeManager) ResetChanges() {
	cm.graphChanges = make([]dimacs.Change, 0)
}

func (cm *changeManager) Graph() *flowgraph.Graph {
	return cm.flowGraph
}

// Private helper methods for change_manager internal use
func (cm *changeManager) addGraphChange(change dimacs.Change) {
	if change.Comment() == "" {
		change.SetComment("addGraphChange: anonymous caller")
	}
	cm.graphChanges = append(cm.graphChanges, change)
}

func (cm *changeManager) optimizeChanges() {
	if cm.RemoveDuplicate {
		cm.removeDuplicateChanges()
	}
	if cm.MergeToSameArc {
		cm.mergeChangesToSameArc()
	}
	if cm.PurgeBeforeNodeRemoval {
		cm.purgeChangesBeforeNodeRemoval()
	}
}

func (cm *changeManager) mergeChangesToSameArc() {
	panic("optimization not implemented ")
}

// Checks if there's already a change for the (src_id, dst_id) arc.
// If there's no change then it adds one to the state, otherwise
// it updates the existing change.
func (cm *changeManager) mergeChangesToSameArcHelper(
	srcID, dstID, capLowerBound, capUpperBound uint64,
	cost int64, typ flowgraph.ArcType,
	change *dimacs.Change, newGraphChanges []*dimacs.Change,
	arcsSrcChanges map[uint64]map[uint64]*dimacs.Change,
	arcsDstChanges map[uint64]map[uint64]*dimacs.Change) {
	panic("optimization not implemented ")
}

func (cm *changeManager) purgeChangesBeforeNodeRemoval() {
	panic("optimization not implemented ")
}

func (cm *changeManager) removeDuplicateChanges() {
	panic("optimization not implemented ")
}

// Checks if there's already an identical change for the (src_id, dst_id) arc.
// If there's no change the it updates the state, otherwise it just ignores
// the change we're currently processing because it's duplicate.
func (cm *changeManager) removeDuplicateChangesHelper(
	srcID, dstID uint64, change *dimacs.Change,
	newGraphChanges []*dimacs.Change,
	node_to_change map[uint64]map[string]*dimacs.Change) {
	panic("optimization not implemented ")
}

func (cm *changeManager) removeDuplicateChangesUpdateState(
	nodeID uint64, change *dimacs.Change,
	node_to_change map[uint64]map[string]*dimacs.Change) bool {
	return false
}

// Method to be called upon node addition. This method makes sure that the
// state is cleaned when we re-use a node id.
func (cm *changeManager) removeDuplicateCleanState(
	newNodeID, srcID, dstID uint64,
	changeDesc string,
	node_to_change map[uint64]map[string]*dimacs.Change) {
	panic("optimization not implemented ")
}
