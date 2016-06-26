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

import "strconv"

// RemoveNodeChange implements the Change interface from dimacschange.go
type RemoveNodeChange struct {
	commentChange
	ID uint64
}

// Returns an update to the dimacs Node Descriptor format
func (rn *RemoveNodeChange) GenerateChange() string {
	return "r " + strconv.FormatUint(rn.ID, 10) + "\n"
}
