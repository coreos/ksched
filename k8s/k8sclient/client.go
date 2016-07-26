package k8sclient

import (
	"fmt"
	"path"
	"sync"

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
	idToNSMu         *sync.Mutex
	idToNamespace    map[string]string
}

func New(cfg Config) (*Client, error) {
	restCfg := &restclient.Config{
		Host:  fmt.Sprintf("http://%s", cfg.Addr),
		QPS:   1000,
		Burst: 1000,
	}
	c, err := kc.New(restCfg)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Created K8S CLIENT (%s)\n", cfg.Addr)

	pch := make(chan *k8stype.Pod, 100)
	nsMap := make(map[string]string)

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
	var idToNSMu sync.Mutex
	informer.AddEventHandler(framework.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*api.Pod)

			//DEBUGGING. Remove it afterwards.
			fmt.Printf("informer: addfunc, pod (%s/%s)\n", pod.Namespace, pod.Name)

			ourPod := &k8stype.Pod{
				ID: makePodID(pod.Namespace, pod.Name),
			}
			// TODO: not a good idea to block in handler..
			idToNSMu.Lock()
			nsMap[ourPod.ID] = pod.Namespace
			idToNSMu.Unlock()
			pch <- ourPod
		},
		UpdateFunc: func(oldObj, newObj interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	})
	stopCh := make(chan struct{})
	go informer.Run(stopCh)

	// nch := make(chan *k8stype.Node, 100)

	// // go func() {
	// _, ctrl2 := framework.NewInformer(
	// 	cache.NewListWatchFromClient(c, "nodes", api.NamespaceAll, fields.ParseSelectorOrDie("")),
	// 	&api.Node{},
	// 	0,
	// 	framework.ResourceEventHandlerFuncs{
	// 		AddFunc: func(obj interface{}) {
	// 			node := obj.(*api.Node)
	// 			ourNode := &k8stype.Node{
	// 				ID: node.Name,
	// 			}
	// 			nch <- ourNode
	// 		},
	// 		UpdateFunc: func(oldObj, newObj interface{}) {},
	// 		DeleteFunc: func(obj interface{}) {},
	// 	},
	// )
	// stopCh2 := make(chan struct{})
	// go ctrl2.Run(stopCh2)
	// }()

	return &Client{
		apisrvClient:     c,
		unscheduledPodCh: pch,
		// nodeCh:           nch,
		idToNSMu:      &idToNSMu,
		idToNamespace: nsMap,
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

func (c *Client) AssignBinding(bindings []*k8stype.Binding) error {
	for _, b := range bindings {
		c.idToNSMu.Lock()
		ns := c.idToNamespace[b.PodID]
		c.idToNSMu.Unlock()

		ctx := api.WithNamespace(api.NewContext(), ns)
		err := c.apisrvClient.Post().Namespace(api.NamespaceValue(ctx)).Resource("bindings").Body(b).Do().Error()
		if err != nil {
			panic(err)
		}
	}
	return nil
}

func makePodID(namespace, name string) string {
	return path.Join(namespace, name)
}
