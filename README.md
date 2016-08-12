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

On the master node get the image
```
docker pull hasbro17/ksched:v0.6
```

Run the container on the host network, waiting in background mode
```
docker run --net="host" --name="ksched" -d hasbro17/ksched:v0.6 tail -f /dev/null
```


### Option 2: Run with Kubernetes API server: