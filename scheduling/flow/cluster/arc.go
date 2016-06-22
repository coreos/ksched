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

// Represents an arc in the scheduling flow graph.
// C++ file: https://github.com/camsas/firmament/blob/master/src/scheduling/flow/flow_graph_arc.h
package cluster

//Enum for flow arc type
type FlowArcType int

const (
	Other FlowNodeType = iota + 1
	Running
)

type FlowGraphArc struct {
	src           uint64
	dst           uint64
	capLowerBound uint64
	capUpperBound uint64
	cost          int64
	srcNode       *FlowGraphNode
	dstNode       *FlowGraphNode
	typ           FlowArcType
}

// Constructor equivalent in go
func NewArc(srcID, dstID uint64, srcNode, dstNode *FlowGraphNode) *FlowGraphArc {
	a := new(FlowGraphArc)
	a.src = srcID
	a.dst = dstID
	a.srcNode = srcNode
	a.dstNode = dstNode
	return a
}
