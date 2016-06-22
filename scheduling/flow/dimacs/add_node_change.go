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

package dimacs

import (
	"strconv"

	"github.com/coreos/ksched/scheduling/flow/flowgraph"
)

// Node type is used to construct the mapping of tasks to PUs in the solver.
// NOTE: Do not reorder types because it will affect the communication with
// the solver.
// Enum for Dimac node type
type NodeType int

const (
	Other NodeType = iota + 1
	Task
	Pu
	Sink
	Machine
	IntermediateResource
)

// AddNodeChange implements the Change interface from dimacschange.go
type AddNodeChange struct {
	comment      string
	ID           uint64
	Excess       int64
	Typ          flowgraph.NodeType
	ArcAdditions []CreateArcChange
}

// Get comment
func (an *AddNodeChange) Comment() string {
	return an.comment
}

// Set comment
func (an *AddNodeChange) SetComment(comment string) {
	an.comment = comment
}

// Generates the dimacs comment line for this change
func (an *AddNodeChange) GenerateChangeDescription() string {
	if an.comment != "" {
		return "c " + an.comment + "\n"
	}
	return ""
}

// Returns the dimacs Node Descriptor format
func (an *AddNodeChange) GenerateChange() string {
	return "n " + strconv.FormatUint(an.ID, 10) +
		" " + strconv.FormatInt(an.Excess, 10) +
		" " + strconv.Itoa(int(an.GetNodeType())) + "\n"
}

func (an *AddNodeChange) GetNodeType() NodeType {
	switch an.Typ {
	case flowgraph.Pu:
		return Pu
	case flowgraph.Machine:
		return Machine
	case flowgraph.Sink:
		return Sink
	case flowgraph.NumaNode, flowgraph.Socket, flowgraph.Cache, flowgraph.Core:
		return IntermediateResource
	case flowgraph.UnscheduledTask, flowgraph.ScheduledTask, flowgraph.RootTask:
		return Task
	default:
		return Other
	}
}
