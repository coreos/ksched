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

// UpdateArcChange implements the Change interface from dimacschange.go
type UpdateArcChange struct {
	commentChange
	Src, Dst                     flowgraph.NodeID
	CapLowerBound, CapUpperBound uint64
	Cost, OldCost                int64
	Typ                          flowgraph.ArcType
}

func NewUpdateArcChange(arc *flowgraph.Arc, oldCost int64) *UpdateArcChange {
	uac := &UpdateArcChange{
		Src:           arc.Src,
		Dst:           arc.Dst,
		CapLowerBound: arc.CapLowerBound,
		CapUpperBound: arc.CapUpperBound,
		Typ:           arc.Type,
		Cost:          arc.Cost,
		OldCost:       oldCost,
	}
	return uac
}

// Returns an update to the dimacs Arc Descriptor format
func (uac *UpdateArcChange) GenerateChange() string {
	return "x " + strconv.FormatUint(uint64(uac.Src), 10) +
		" " + strconv.FormatUint(uint64(uac.Dst), 10) +
		" " + strconv.FormatUint(uac.CapLowerBound, 10) +
		" " + strconv.FormatUint(uac.CapUpperBound, 10) +
		" " + strconv.FormatInt(uac.Cost, 10) +
		" " + strconv.Itoa(int(uac.Typ)) +
		" " + strconv.FormatInt(uac.OldCost, 10) + "\n"
}
