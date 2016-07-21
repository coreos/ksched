package k8sclient

import "github.com/coreos/ksched/k8s/k8stype"

type Config struct {
}

type Client struct {
}

func New(Config) (*Client, error) {
	panic("")
}

type PodChan <-chan *k8stype.Pod

func (*Client) GetUnscheduledPodChan() PodChan {
	panic("")
}

type NodeChan <-chan *k8stype.Node

func (*Client) GetNodeChan() NodeChan {
	panic("")
}

func (*Client) AssignBinding([]*k8stype.Binding) error {
	panic("")
}
