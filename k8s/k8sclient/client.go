package k8sclient

import (
	"log"
	"path"

	"github.com/coreos/ksched/k8s/k8stype"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/client/restclient"
	kc "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/controller/framework"
	"k8s.io/kubernetes/pkg/fields"
)

type Config struct {
	Addr string
}

type Client struct {
	apisrvClient     *kc.Client
	unscheduledPodCh chan *k8stype.Pod
	nodeCh           chan *k8stype.Node
	idToNamespace    map[string]string
}

func New(cfg Config) (*Client, error) {
	restCfg := &restclient.Config{
		Host:  cfg.Addr,
		QPS:   1000,
		Burst: 1000,
	}
	c, err := kc.New(restCfg)
	if err != nil {
		return nil, err
	}

	pch := make(chan *k8stype.Pod, 100)
	nsMap := make(map[string]string)

	go func() {
		selector := fields.ParseSelectorOrDie("spec.nodeName==" + "" + ",status.phase!=" + string(api.PodSucceeded) + ",status.phase!=" + string(api.PodFailed))
		lw := cache.NewListWatchFromClient(c, "pods", api.NamespaceAll, selector)

		_, ctrl := framework.NewInformer(
			lw,
			&api.Pod{},
			0,
			framework.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					log.Printf("PodInformer/AddFunc: called")
					pod := obj.(*api.Pod)
					ourPod := &k8stype.Pod{
						ID: makePodID(pod.Namespace, pod.Name),
					}
					nsMap[ourPod.ID] = pod.Namespace
					pch <- ourPod
					log.Printf("PodInformer/AddFunc: Sent on pod:%v on the pod channel\n", ourPod.ID)

				},
				UpdateFunc: func(oldObj, newObj interface{}) {},
				DeleteFunc: func(obj interface{}) {},
			},
		)
		stopCh := make(chan struct{})
		ctrl.Run(stopCh)
	}()

	nch := make(chan *k8stype.Node, 100)

	go func() {
		_, ctrl := framework.NewInformer(
			cache.NewListWatchFromClient(c, "nodes", api.NamespaceAll, fields.ParseSelectorOrDie("")),
			&api.Node{},
			0,
			framework.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					node := obj.(*api.Node)
					ourNode := &k8stype.Node{
						ID: node.Name,
					}
					nch <- ourNode
				},
				UpdateFunc: func(oldObj, newObj interface{}) {},
				DeleteFunc: func(obj interface{}) {},
			},
		)
		stopCh := make(chan struct{})
		ctrl.Run(stopCh)
	}()

	return &Client{
		apisrvClient:     c,
		unscheduledPodCh: pch,
		nodeCh:           nch,
		idToNamespace:    nsMap,
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
		ctx := api.WithNamespace(api.NewContext(), c.idToNamespace[b.PodID])
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
