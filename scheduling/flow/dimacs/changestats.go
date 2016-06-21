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

type DimacsChangeType int

const NumChangeTypes = 36

const (
	AddTaskNode DimacsChangeType = iota
	AddResourceNode
	AddEquivClassNode
	// Add more types later
	// ...
)

type DimacsChangeStats struct {
	NodesAdded       uint64
	NodesRemoved     uint64
	ArcsAdded        uint64
	ArcsChanged      uint64
	ArcsRemoved      uint64
	NumChangesOfType [NumChangeTypes]uint64
}

// Needs these functions to be implemented later
/*
DIMACSChangeStats();
~DIMACSChangeStats();
string GetStatsString() const;
void ResetStats();
void UpdateStats(DIMACSChangeType change_type);
*/
