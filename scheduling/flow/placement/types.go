package placement

import "github.com/coreos/ksched/scheduling/flow/flowgraph"

// A flow pair associates the amount of flow that "originates" or flows through a node
type flowPair struct {
	srcNodeID flowgraph.NodeID
	flow      uint64
}

type flowPairMap map[flowgraph.NodeID]*flowPair

/*
func (l flowPairList) Insert(p *flowPair) {
	l = append(l, p)
}

func (l flowPairList) Find(nodeID flowgraph.NodeID) (*flowPair, bool) {
	for _, p := range l {
		if p.nodeID == nodeID {
			return p, true
		}
	}
	return nil, false
}
*/
