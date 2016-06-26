package flowmanager

import "github.com/coreos/ksched/scheduling/flow/flowgraph"

// TaskMappings
type TaskMappings map[flowgraph.NodeID][]flowgraph.NodeID
