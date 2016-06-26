package flowmanager

import "github.com/coreos/ksched/scheduling/flow/flowgraph"

type NodeSet map[flowgraph.NodeID]struct{}

// TaskMapping
type TaskMapping map[flowgraph.NodeID]NodeSet

func (tm TaskMapping) Insert(task, pu flowgraph.NodeID) {
	ns, ok := tm[task]
	if !ok {
		ns = NodeSet{}
		tm[task] = ns
	}
	ns[pu] = struct{}{}
}
