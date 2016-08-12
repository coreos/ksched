# Ksched: A Firmament Prototype

[Firmament](http://firmament.io) is a new apporach to cluster scheduling that models the problem of scheduling as a network flow optimization problem.
Firmament aims on providing:
* Policy optimal scheduling
* Lower latency on scheduling decisions
* Flexibility through pluggable policies

Ksched is an experimental reimplementation of the scheduler from it's [C++ implementation](https://github.com/camsas/firmament) to Go. The goal of this project is to integrate Firmament in [Kubernetes](https://github.com/kubernetes/kubernetes) as an alternative scheduler. 


## Current State of Project:
The project so far is an early stage prototype with all the mechanisms for performing multiple scheduling iterations.

The scheduler layer has a minimal interface with the Kubernetes API allowing it to batch schedule pods. Currently the implementation has no support for sophisticated policies and will perform a trivial first-fit policy to assign pods to nodes in the cluster.

## Trying it Out:
To get the scheduler up and running there are two ways to currently test it out.

### Option 1: Running on a live cluster:
You can test the scheduler by running it inside of a container on the kubernetes master node. You can build the image from `build/Dockerfile` yourself or as described below use our hosted image.

On the master node pull the image.
```
docker pull hasbro17/ksched:v0.6
```

Run the container on the host network, waiting in background mode.
```
docker run --net="host" --name="ksched" -d hasbro17/ksched:v0.6 tail -f /dev/null
```

You will need to pause the pre-existing kubernetes scheduler's container before trying to run ksched.
```
docker pause <container-ID>
```

Get a shell into the ksched container.
```
docker exec -it ksched /bin/bash
```

Run the init script to clone and build the scheduler.
```
/root/init.sh
```

There should be two binaries present in the ksched project at `/root/go-workspace/src/github.com/coreos/ksched`

The first `k8sscheduler`, is the scheduler whose flags are specified in `cmd/k8sscheduler/scheduler.go`. Run this binary to start the scheduler.
```
k8sscheduler -fakeMachines=false
```

The scheduler should start up and wait for unscheduled pods at this point.

To generate a large number of pod requests you can use the binary `podgen`.
```
podgen -numPods=<number-of-pods> -image=nginx
```


### Option 2: Run with Kubernetes API server:
You can test the scheduler without the real cluster by only having the `kube-api` binary running. The setup is a little more involved for this case.

You will need to have the same environment set up as is for the ksched image described by `build/Dockerfile`. Use that as a guide for your setup.

* Setup the [Flowlessly](https://github.com/ICGog/Flowlessly) solver binary in the correct location: `/usr/local/bin/flowlessly/`.
* Setup the Kubernetes(v1.3) source at the following location in your go workspace: `$GOPATH/src/k8s.io`
* Get the ksched source: `go get github.com/coreos/ksched` (or from the mirror repo `github.com/hasbro17/ksched-mirror`)
* Generate the proto files by running `proto/genproto.sh` 


