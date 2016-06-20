// Simple FIFO queue data structure using slices
// The type has to be asserted on retrieval of value

package queue

type Node struct {
	Value interface{}
}

type Queue []*Node

// Push to the end of a queue
func (q *Queue) Push(n *Node) {
	*q = append(*q, n)
}

// Pop from the front of a queue
func (q *Queue) Pop() *Node {
	n := (*q)[0]
	*q = (*q)[1:]
	return n
}

// Same as Pop but doesn't remove the element from the queue
func (q *Queue) Front() (n *Node) {
	n = (*q)[0]
	return n
}

// Get length of queue
func (q *Queue) Len() int {
	return len(*q)
}

// Check if queue is empty
func (q *Queue) IsEmpty() bool {
	return q.Len() == 0
}
