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

// CreateArcChange implements the Change interface from dimacschange.go
type CreateArcChange struct {
	commentChange
	Src, Dst, CapLowerBound, CapUpperBound uint64
	Cost                                   int64
	Typ                                    flowgraph.ArcType
}

func NewCreateArcChange(a *flowgraph.Arc) *CreateArcChange {
	cac := &CreateArcChange{
		Src:           uint64(a.Src),
		Dst:           uint64(a.Dst),
		CapLowerBound: a.CapLowerBound,
		CapUpperBound: a.CapUpperBound,
		Cost:          a.Cost,
		Typ:           a.Type,
	}
	return cac
}

// Returns the dimacs Arc Descriptor format
func (cac *CreateArcChange) GenerateChange() string {
	return "a " + strconv.FormatUint(cac.Src, 10) +
		" " + strconv.FormatUint(cac.Dst, 10) +
		" " + strconv.FormatUint(cac.CapLowerBound, 10) +
		" " + strconv.FormatUint(cac.CapUpperBound, 10) +
		" " + strconv.FormatInt(cac.Cost, 10) +
		" " + strconv.Itoa(int(cac.Typ)) + "\n"
}
