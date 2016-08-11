# This image will setup the container for compiling and running the ksched binary
# The flowlessly solver is already set up in the location needed by ksched

# Prepare base image
FROM ubuntu:14.04
MAINTAINER Haseeb Tariq <haseeb.tariq@coreos.com>
RUN apt-get update
RUN apt-get --yes --force-yes install git-all build-essential cmake libgflags2 libgflags-dev libgoogle-glog0 libgoogle-glog-dev libboost-all-dev curl wget autoconf automake libtool curl make g++ unzip

# Set up the solver binary in the location "/usr/local/bin/flowlessly/" hardcoded in the placement/solver
RUN cd /root && git clone https://github.com/hasbro17/Flowlessly.git && cd /root/Flowlessly && cmake . && make
RUN mkdir -p /usr/local/bin/flowlessly/ && cp /root/Flowlessly/build/flow_scheduler /usr/local/bin/flowlessly/

# This part not really needed if we just get the ksched binary directly but putting in to compile it within the container

# Enivronment variables for Go
RUN echo "export PATH=$PATH:/usr/local/go/bin:/root/go-workspace/bin" >> ~/.bashrc
RUN echo "export GOPATH=/root/go-workspace" >> ~/.bashrc

# Install Go and set up workspace for compiling binary
RUN cd /root && curl -O https://storage.googleapis.com/golang/go1.6.linux-amd64.tar.gz && tar -xvf go1.6.linux-amd64.tar.gz && mv go /usr/local
RUN mkdir -p /root/go-workspace/src/github.com/coreos

# Set up kubernetes in go workspace
RUN mkdir /root/go-workspace/src/k8s.io && cd /root/go-workspace/src/github.com/ && git clone https://github.com/kubernetes/kubernetes.git && mv /root/go-workspace/src/github.com/kubernetes /root/go-workspace/src/k8s.io/

# Set up proto stuff
RUN cd /root && mkdir proto-bin && cd proto-bin && wget "https://github.com/google/protobuf/releases/download/v3.0.0/protoc-3.0.0-linux-x86_64.zip" && unzip protoc-3.0.0-linux-x86_64.zip && cp bin/protoc /usr/local/bin/protoc

# After this point, you would still need to 
# 1. Run the container on the k8s master node in host networking mode
# 2. Clone the ksched repo inside it
# 3. Build and run the k8sscheduler binary inside the container

# TODO:
# Add an init file to clone the ksched repo on startup using ssh auth