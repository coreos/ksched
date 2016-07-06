package flowgraph

import "testing"

func TestNodeAddArc(t *testing.T) {
	g := NewGraph(false)
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
	if n0.OutgoingArcMap[n1.ID] != arc {
		t.Errorf("n0->n1 = %+v, want %+v", n0.OutgoingArcMap[n1.ID], arc)
	}
}

func TestArcChange(t *testing.T) {
	g := NewGraph(false)

	n0 := g.AddNode()
	n1 := g.AddNode()
	arc := g.AddArc(n0, n1)
	g.ChangeArc(arc, 0, 100, 42)

	if arc.CapLowerBound != 0 || arc.CapUpperBound != 100 || arc.Cost != 42 {
		t.Errorf("got %d, %d, %d, want %d, %d, %d",
			arc.CapLowerBound, arc.CapUpperBound, arc.Cost,
			0, 100, 42)
	}

	na := g.NumArcs()
	g.ChangeArc(arc, 0, 0, 42)
	if g.NumArcs() != na-1 {
		t.Errorf("numArcs = %d, want %d", g.NumArcs(), na-1)
	}
}
