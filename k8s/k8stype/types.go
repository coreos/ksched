package k8stype

type Pod struct {
	PodID string
}

type Node struct {
	NodeID string
}

type Binding struct {
	PodID  string
	NodeID string
}
