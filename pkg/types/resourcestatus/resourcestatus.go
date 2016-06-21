// Copyright 2016 The ksched Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Resource status representation.
// C++ file: https://github.com/camsas/firmament/blob/master/src/base/resource_status.h

package resourcestatus

import pb "github.com/coreos/ksched/proto"

type ResourceStatus struct {
	descriptor    *pb.ResourceDescriptor
	topologyNode  *pb.ResourceTopologyNodeDescriptor
	endpointUri   string
	lastHeartbeat uint64
}

// In C++:
// inline const ResourceDescriptor& descriptor() { return *descriptor_; }
// NOTE: mutable_descriptor() and descriptor() are the same thing in Go as const is not allowed in go
func (rs *ResourceStatus) Descriptor() *pb.ResourceDescriptor {
	return rs.descriptor
}

// In C++:
// inline const ResourceTopologyNodeDescriptor& topology_node() { return *topology_node_; }
// NOTE: topologyNode() and mutableTopologyNode() are the same thing in Go as const is not allowed in go
func (rs *ResourceStatus) TopologyNode() *pb.ResourceTopologyNodeDescriptor {
	return rs.topologyNode
}

func (rs *ResourceStatus) Location() string {
	return rs.endpointUri
}

func (rs *ResourceStatus) LastHeartbeat() uint64 {
	return rs.lastHeartbeat
}

func (rs *ResourceStatus) SetLastHeartbeat(hb uint64) {
	rs.lastHeartbeat = hb
}
