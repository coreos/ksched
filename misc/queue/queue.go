// Read Write thread safe FIFO queue data structure using slices
// The type has to be asserted on retrieval of value

package queue

import "sync"

type Node struct {
	Value interface{}
}

type FIFO interface {
	Push(*Node)
	Pop() *Node
	Front() *Node
	Len() int
	IsEmpty() bool
}

func NewFIFO() FIFO {
	return &fifo{}
}

type fifo struct {
	rwMu  sync.RWMutex
	array []*Node
}

// Push to the end of a queue
func (f *fifo) Push(n *Node) {
	f.rwMu.Lock()
	defer f.rwMu.Unlock()
	f.array = append(f.array, n)
}

// Pop from the front of a queue
func (f *fifo) Pop() *Node {
	f.rwMu.Lock()
	defer f.rwMu.Unlock()
	n := f.array[0]
	f.array = f.array[1:]
	return n
}

// Same as Pop but doesn't remove the element from the queue
func (f *fifo) Front() (n *Node) {
	f.rwMu.RLock()
	defer f.rwMu.RUnlock()
	n = f.array[0]
	return n
}

// Get length of queue
func (f *fifo) Len() int {
	f.rwMu.RLock()
	defer f.rwMu.RUnlock()
	return len(f.array)
}

// Check if queue is empty
func (f *fifo) IsEmpty() bool {
	f.rwMu.RLock()
	defer f.rwMu.RUnlock()
	return f.Len() == 0
}
