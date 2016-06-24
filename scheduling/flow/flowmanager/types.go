package flowmanager

import "github.com/coreos/ksched/scheduling/flow/flowgraph"

type TaskMappings map[flowgraph.NodeID][]flowgraph.NodeID
