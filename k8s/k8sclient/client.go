package k8sclient

import (
	"fmt"
	"path"
	"time"

	"github.com/coreos/ksched/k8s/k8stype"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/client/restclient"
	kc "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/controller/framework"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/watch"
)

type Config struct {
	Addr string
}

type Client struct {
	apisrvClient     *kc.Client
	unscheduledPodCh chan *k8stype.Pod
	nodeCh           chan *k8stype.Node
}

func New(cfg Config, podChanSize int) (*Client, error) {
	restCfg := &restclient.Config{
		Host:  fmt.Sprintf("http://%s", cfg.Addr),
		QPS:   1000,
		Burst: 1000,
	}
	c, err := kc.New(restCfg)
	if err != nil {
		return nil, err
	}

	pch := make(chan *k8stype.Pod, podChanSize)

	// Create informer to watch on unscheduled Pods (non-failed non-succeeded pods with an empty node binding)
	sel := fields.ParseSelectorOrDie("spec.nodeName==" + "" + ",status.phase!=" + string(api.PodSucceeded) + ",status.phase!=" + string(api.PodFailed))
	informer := framework.NewSharedInformer(
		&cache.ListWatch{
			ListFunc: func(options api.ListOptions) (runtime.Object, error) {
				options.FieldSelector = sel
				return c.Pods(api.NamespaceAll).List(options)
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				options.FieldSelector = sel
				return c.Pods(api.NamespaceAll).Watch(options)
			},
		},
		&api.Pod{},
		0,
	)
	// Add event handlers for the addition, update and deletion of the pods watched by the above informer
	informer.AddEventHandler(framework.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*api.Pod)
			ourPod := &k8stype.Pod{
				ID: makePodID(pod.Namespace, pod.Name),
			}
			// For now every new pod is just added to the channel being polled by the batch mechanism
			pch <- ourPod
		},
		UpdateFunc: func(oldObj, newObj interface{}) {},
		DeleteFunc: func(obj interface{}) {
		},
	})
	stopCh := make(chan struct{})
	go informer.Run(stopCh)

	nch := make(chan *k8stype.Node, 100)
	// Informer for watching the addition and removal of nodes in the cluster
	_, nodeInformer := framework.NewInformer(
		cache.NewListWatchFromClient(c, "nodes", api.NamespaceAll, fields.ParseSelectorOrDie("")),
		&api.Node{},
		0,
		framework.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				node := obj.(*api.Node)
				// Skip the master node
				if node.Spec.Unschedulable {
					return
				}
				ourNode := &k8stype.Node{
					ID: node.Name,
				}
				// Add every new node seen to the channel being polled by the
				// resource topology initializer mechanism
				nch <- ourNode
			},
			UpdateFunc: func(oldObj, newObj interface{}) {},
			DeleteFunc: func(obj interface{}) {},
		},
	)
	stopCh2 := make(chan struct{})
	go nodeInformer.Run(stopCh2)

	return &Client{
		apisrvClient:     c,
		unscheduledPodCh: pch,
		nodeCh:           nch,
	}, nil
}

type PodChan <-chan *k8stype.Pod

func (c *Client) GetUnscheduledPodChan() PodChan {
	return c.unscheduledPodCh
}

type NodeChan <-chan *k8stype.Node

func (c *Client) GetNodeChan() NodeChan {
	return c.nodeCh
}

// Write out node bindings
func (c *Client) AssignBinding(bindings []*k8stype.Binding) error {
	for _, ob := range bindings {
		ns, name := parsePodID(ob.PodID)
		b := &api.Binding{
			ObjectMeta: api.ObjectMeta{Namespace: ns, Name: name},
			Target: api.ObjectReference{
				Kind: "Node",
				Name: parseNodeID(ob.NodeID),
			},
		}
		ctx := api.WithNamespace(api.NewContext(), ns)
		err := c.apisrvClient.Post().Namespace(api.NamespaceValue(ctx)).Resource("bindings").Body(b).Do().Error()
		if err != nil {
			panic(err)
		}
	}
	return nil
}

// Returns a batch of pods or blocks until there is at least on pod creation call back
// The timeout specifies how long to wait for another pod on the pod channel before returning
// the batch of pods that need to be scheduled
func (c *Client) GetPodBatch(timeout time.Duration) []*k8stype.Pod {
	batchedPods := make([]*k8stype.Pod, 0)

	fmt.Printf("Waiting for a pod scheduling request\n")

	// Check for first pod, block until at least 1 is available
	pod := <-c.unscheduledPodCh
	batchedPods = append(batchedPods, pod)

	// Set timer for timeout between successive pods
	timer := time.NewTimer(timeout)
	done := make(chan bool)
	go func() {
		<-timer.C
		done <- true
	}()

	fmt.Printf("Batching pod scheduling requests\n")
	numPods := 1
	//fmt.Printf("Number of pods requests: %d", numPods)
	// Poll until done from timeout
	// TODO: Put a cap on the batch size since this could go on forever
	finish := false
	for !finish {
		select {
		case pod = <-c.unscheduledPodCh:
			numPods++
			fmt.Printf("\rNumber of pods requests: %d", numPods)
			batchedPods = append(batchedPods, pod)
			// Refresh the timeout for next pod
			timer.Reset(timeout)
		case <-done:
			finish = true
			fmt.Printf("\n")
		default:
			// Do nothing and keep polling until timeout
		}
	}
	return batchedPods
}

func makePodID(namespace, name string) string {
	return path.Join(namespace, name)
}

func parsePodID(id string) (string, string) {
	ns, podName := path.Split(id)
	// Get rid of the / at the end
	ns = ns[:len(ns)-1]
	return ns, podName
}

func parseNodeID(id string) string {
	return id
}
