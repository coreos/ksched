package k8stype

type Pod struct {
	ID string
}

type Node struct {
	ID string
}

type Binding struct {
	PodID  string
	NodeID string
}
