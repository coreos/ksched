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

package queue

import "sync"

// Read Write thread safe FIFO queue data structure.
type FIFO interface {
	// Push to the end of a queue.
	Push(val interface{})
	// Pop from the front of a queue.
	Pop() interface{}
	// Same as Pop but doesn't remove the element from the queue.
	// The returned value should be read only.
	Front() interface{}
	// Get length of queue.
	Len() int
	// Check if queue is empty.
	IsEmpty() bool
}

func NewFIFO() FIFO {
	return &fifo{}
}

type fifo struct {
	rwMu  sync.RWMutex
	nodes []interface{}
}

func (f *fifo) Push(val interface{}) {
	f.rwMu.Lock()
	defer f.rwMu.Unlock()
	f.nodes = append(f.nodes, val)
}

func (f *fifo) Pop() interface{} {
	f.rwMu.Lock()
	defer f.rwMu.Unlock()
	n := f.nodes[0]
	f.nodes = f.nodes[1:]
	return n
}

func (f *fifo) Front() interface{} {
	f.rwMu.RLock()
	defer f.rwMu.RUnlock()
	return f.nodes[0]
}

func (f *fifo) Len() int {
	f.rwMu.RLock()
	defer f.rwMu.RUnlock()
	return len(f.nodes)
}

func (f *fifo) IsEmpty() bool {
	return f.Len() == 0
}
