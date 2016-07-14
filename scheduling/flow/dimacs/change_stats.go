// Copyright 2016 The ksched Authors
//
// Licensed under the Apache License Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing software
// distributed under the License is distributed on an "AS IS" BASIS
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dimacs

import "strconv"

type ChangeType int

const NumChangeTypes = 36

const (
	AddTaskNode ChangeType = iota
	AddResourceNode
	AddEquivClassNode
	AddUnschedJobNode
	AddSinkNode
	AddArcTaskToEquivClass
	AddArcTaskToRes
	AddArcEquivClassToRes
	AddArcBetweenEquivClass
	AddArcBetweenRes
	AddArcToUnsched
	AddArcFromUnsched
	AddArcRunningTask
	AddArcResToSink
	DelUnschedJobNode
	DelTaskNode
	DelResourceNode
	DelEquivClassNode
	DelArcEquivClassToRes
	DelArcRunningTask
	DelArcEvictedTask
	DelArcBetweenEquivClass
	DelArcBetweenRes
	DelArcTaskToEquivClass
	DelArcTaskToRes
	DelArcResToSink
	ChgArcEvictedTask
	ChgArcToUnsched
	ChgArcFromUnsched
	ChgArcTaskToEquivClass
	ChgArcEquivClassToRes
	ChgArcBetweenEquivClass
	ChgArcBetweenRes
	ChgArcRunningTask
	ChgArcTaskToRes
	ChgArcResToSink
)

type ChangeStats struct {
	NodesAdded       uint64
	NodesRemoved     uint64
	ArcsAdded        uint64
	ArcsChanged      uint64
	ArcsRemoved      uint64
	NumChangesOfType [NumChangeTypes]uint64
}

func (cs *ChangeStats) GetStatsString() string {
	stats := strconv.FormatUint(cs.NodesAdded, 10) +
		"," + strconv.FormatUint(cs.NodesRemoved, 10) +
		"," + strconv.FormatUint(cs.ArcsAdded, 10) +
		"," + strconv.FormatUint(cs.ArcsChanged, 10) +
		"," + strconv.FormatUint(cs.ArcsRemoved, 10)

	for i := 0; i < NumChangeTypes; i++ {
		stats += "," + strconv.FormatUint(cs.NumChangesOfType[i], 10)
	}
	return stats
}

// Any need for zeroing out a ChangeStats struct, or just get a new one
func (cs *ChangeStats) ResetStats() {
	cs.NodesAdded = 0
	cs.NodesRemoved = 0
	cs.ArcsAdded = 0
	cs.ArcsChanged = 0
	cs.ArcsRemoved = 0
	for i := 0; i < NumChangeTypes; i++ {
		cs.NumChangesOfType[i] = 0
	}
}

func (cs *ChangeStats) UpdateStats(changeType ChangeType) {
	// TODO
}
