package k8sclient

import (
	"fmt"
	"path"

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
	informer.AddEventHandler(framework.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*api.Pod)

			//DEBUGGING. Remove it afterwards.
			fmt.Printf("informer: addfunc, pod (%s/%s)\n", pod.Namespace, pod.Name)

			ourPod := &k8stype.Pod{
				ID: makePodID(pod.Namespace, pod.Name),
			}
			pch <- ourPod
		},
		UpdateFunc: func(oldObj, newObj interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	})
	stopCh := make(chan struct{})
	go informer.Run(stopCh)

	nch := make(chan *k8stype.Node, 100)

	_, nodeInformer := framework.NewInformer(
		cache.NewListWatchFromClient(c, "nodes", api.NamespaceAll, fields.ParseSelectorOrDie("spec.unschedulable!=true")),
		&api.Node{},
		0,
		framework.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				node := obj.(*api.Node)

				//DEBUGGING. Remove it afterwards.
				fmt.Printf("NodeInformer: addfunc, node (%s/%s)\n", node.Namespace, node.Name)

				ourNode := &k8stype.Node{
					ID: node.Name,
				}
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

func (c *Client) AssignBinding(bindings []*k8stype.Binding) error {
	for _, ob := range bindings {
		ns, name := parsePodID(ob.PodID)
		fmt.Printf("NS:%v podName:%v\n", ns, name)
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
