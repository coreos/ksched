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

package flowgraph

//Enum for flow arc type
type ArcType int

const (
	Other NodeType = iota + 1
	Running
)

// Represents an arc in the scheduling flow graph.
type Arc struct {
	src           uint64
	dst           uint64
	capLowerBound uint64
	capUpperBound uint64
	cost          int64
	srcNode       *Node
	dstNode       *Node
	typ           ArcType
}

// Constructor equivalent in go
func NewArc(srcNode, dstNode *Node) *Arc {
	a := new(Arc)
	a.src = srcNode.id
	a.dst = dstNode.id
	a.srcNode = srcNode
	a.dstNode = dstNode
	return a
}

func (a *Arc) Change(capLowerBound, capUpperBound uint64, cost int64) {
	a.capLowerBound = capLowerBound
	a.capUpperBound = capUpperBound
	a.cost = cost
}

func (a *Arc) ChangeCost(cost int64) {
	a.cost = cost
}
