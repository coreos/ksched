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

// Read Write thread safe FIFO queue data structure using slices
// The type has to be asserted on retrieval of value

package queue

import "sync"

type Node struct {
	Value interface{}
}

type FIFO interface {
	// Push to the end of a queue
	Push(*Node)
	// Pop from the front of a queue
	Pop() *Node
	// Same as Pop but doesn't remove the element from the queue
	Front() *Node
	// Get length of queue
	Len() int
	// Check if queue is empty
	IsEmpty() bool
}

func NewFIFO() FIFO {
	return &fifo{}
}

type fifo struct {
	rwMu  sync.RWMutex
	nodes []*Node
}

func (f *fifo) Push(n *Node) {
	f.rwMu.Lock()
	defer f.rwMu.Unlock()
	f.nodes = append(f.nodes, n)
}

func (f *fifo) Pop() *Node {
	f.rwMu.Lock()
	defer f.rwMu.Unlock()
	n := f.nodes[0]
	f.nodes = f.nodes[1:]
	return n
}

func (f *fifo) Front() (n *Node) {
	f.rwMu.RLock()
	defer f.rwMu.RUnlock()
	n = f.nodes[0]
	return n
}

func (f *fifo) Len() int {
	f.rwMu.RLock()
	defer f.rwMu.RUnlock()
	return len(f.nodes)
}

func (f *fifo) IsEmpty() bool {
	f.rwMu.RLock()
	defer f.rwMu.RUnlock()
	return f.Len() == 0
}
