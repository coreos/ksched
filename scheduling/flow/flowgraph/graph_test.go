package flowgraph

import "testing"

func TestNodeAddArc(t *testing.T) {
	g := New(false)
	nc := g.NumNodes()

	n0 := g.AddNode()
	n1 := g.AddNode()
	arc := g.AddArc(n0, n1)

	if g.NumNodes() != nc+2 {
		t.Errorf("number of nodes = %d, want %d", g.NumNodes(), nc+2)
	}
	if g.NumArcs() != 1 {
		t.Errorf("number of arcs = %d, want %d", g.NumNodes(), 1)
	}
	if n0.outgoingArcMap[n1.ID] != arc {
		t.Errorf("n0->n1 = %p, want %p", n0.outgoingArcMap[n1.ID] != arc)
	}
}
