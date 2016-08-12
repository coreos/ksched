#!/bin/bash

# Clone the ksched repo, (for now it's a duplicate repo because of access issues with private repos)
cd /root/go-workspace/src/github.com/coreos && git clone https://github.com/hasbro17/ksched-mirror ksched
cd ksched/proto
./genproto.sh
cd ..
go build ./cmd/k8sscheduler
go build ./cmd/podgen