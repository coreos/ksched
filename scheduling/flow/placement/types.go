package placement

import "github.com/coreos/ksched/scheduling/flow/flowgraph"

type flowPair struct {
	nodeID   flowgraph.NodeID
	capacity uint64
}

type flowPairList []*flowPair

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
