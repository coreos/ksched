package dimacs

import (
	"fmt"
	"io"

	pb "github.com/coreos/ksched/proto"
	"github.com/coreos/ksched/scheduling/flow/flowgraph"
)

func Export(g *flowgraph.Graph, w io.Writer) {
	fmt.Fprint(w, "c ===========================\n")
	fmt.Fprintf(w, "p min %d %d\n", g.NumNodes(), g.NumArcs())
	fmt.Fprint(w, "c ===========================\n")

	fmt.Fprint(w, "c === ALL NODES FOLLOW ===\n")
	for _, n := range g.Nodes() {
		generateNode(n, w)
	}

	fmt.Fprint(w, "c === ALL ARCS FOLLOW ===\n")
	for r := range g.Arcs() {
		generateArc(r, w)
	}

	// Add end of iteration comment.
	fmt.Fprintf(w, "c EOI\n")
}

func ExportIncremental(changes []Change, w io.Writer) {
	for _, change := range changes {
		fmt.Fprint(w, change.GenerateChange())
	}

	// Add end of iteration comment.
	fmt.Fprintf(w, "c EOI\n")
}

func generateNode(n *flowgraph.Node, w io.Writer) {
	// dimacs comments
	switch {
	case n.ResourceDescriptor != nil:
		fmt.Fprintf(w, "c nd Res_%s %s\n", n.ResourceDescriptor.Uuid, pb.ResourceDescriptor_ResourceType_name[int32(n.ResourceDescriptor.Type)])
	case n.Task != nil:
		fmt.Fprintf(w, "c nd Task_%d\n", n.Task.Uid)
	case n.EquivClass != nil:
		fmt.Fprintf(w, "c nd EC_%d\n", *n.EquivClass)
	case n.Comment != "":
		fmt.Fprintf(w, "c nd %s\n", n.Comment)
	}

	// node type used by solver.
	// default is 0.
	// In the future, we should have common defs between both sides.
	nodeType := 0
	switch n.Type {
	case flowgraph.NodeTypePu:
		nodeType = 2
	case flowgraph.NodeTypeMachine:
		nodeType = 4
	case flowgraph.NodeTypeNuma, flowgraph.NodeTypeSocket, flowgraph.NodeTypeCache, flowgraph.NodeTypeCore:
		nodeType = 5
	case flowgraph.NodeTypeSink:
		nodeType = 3
	case flowgraph.NodeTypeUnscheduledTask, flowgraph.NodeTypeScheduledTask, flowgraph.NodeTypeRootTask:
		nodeType = 1
	}

	fmt.Fprintf(w, "n %d %d %d\n", n.ID, n.Excess, nodeType)
}

func generateArc(arc *flowgraph.Arc, w io.Writer) {
	fmt.Fprintf(w, "a %d %d %d %d %d\n",
		arc.Src, arc.Dst, arc.CapLowerBound, arc.CapUpperBound, arc.Cost)
}
