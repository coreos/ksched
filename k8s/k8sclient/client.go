package k8sclient

import (
	"fmt"
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
	KeepAround       interface{}
	idToNamespace    map[string]string
}

func New(cfg Config) (*Client, error) {
	log.Printf("\n\nCreating CLIENT (%s) for scheduler\n\n", cfg.Addr)
	restCfg := &restclient.Config{
		Host:  fmt.Sprintf("http://%s", cfg.Addr),
		QPS:   1000,
		Burst: 1000,
	}
	c, err := kc.New(restCfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.Pods(api.NamespaceAll).List(api.ListOptions{}); err != nil {
		return nil, err
	}

	pch := make(chan *k8stype.Pod, 100)
	nsMap := make(map[string]string)

	log.Printf("\n\nHere 1\n\n")

	// go func() {
	//selector := fields.ParseSelectorOrDie( /* "spec.nodeName==" + "" + */ "status.phase!=" + string(api.PodSucceeded) + ",status.phase!=" + string(api.PodFailed))
	lw := cache.NewListWatchFromClient(c, "pods", api.NamespaceAll, fields.Everything())

	store, ctrl1 := framework.NewInformer(
		lw,
		&api.Pod{},
		0,
		framework.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				fmt.Printf("\n\nPodInformer/AddFunc: called\n\n")
				// pod := obj.(*api.Pod)
				// ourPod := &k8stype.Pod{
				// 	ID: makePodID(pod.Namespace, pod.Name),
				// }
				// nsMap[ourPod.ID] = pod.Namespace
				// pch <- ourPod
				// fmt.Printf("PodInformer/AddFunc: Sent on pod:%v on the pod channel\n", ourPod.ID)

			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				fmt.Printf("PodInformer/UpdateFunc: called\n")
			},
			DeleteFunc: func(obj interface{}) {
				fmt.Printf("PodInformer/DeleteFunc: called\n")
			},
		},
	)
	stopCh1 := make(chan struct{})
	log.Printf("informer: Informer running\n")
	go ctrl1.Run(stopCh1)
	// )}

	// go func() {
	// 	for {
	// 		l, err := c.Pods("default").List(api.ListOptions{})
	// 		if err != nil {
	// 			panic(err)
	// 		}
	// 		for i := range l.Items {
	// 			pod := &l.Items[i]
	// 			fmt.Println(pod.Name)
	// 		}
	// 	}
	// }()

	// informer := framework.NewSharedInformer(lw, &api.Pod{}, 0)
	// informer.AddEventHandler(framework.ResourceEventHandlerFuncs{
	// 	AddFunc: func(obj interface{}) {
	// 		fmt.Printf("\n\nPodInformer/AddFunc: called\n\n")
	// 		// pod := obj.(*api.Pod)
	// 		// ourPod := &k8stype.Pod{
	// 		// 	ID: makePodID(pod.Namespace, pod.Name),
	// 		// }
	// 		// nsMap[ourPod.ID] = pod.Namespace
	// 		// pch <- ourPod
	// 		// fmt.Printf("PodInformer/AddFunc: Sent on pod:%v on the pod channel\n", ourPod.ID)

	// 	},
	// 	UpdateFunc: func(oldObj, newObj interface{}) {
	// 		fmt.Printf("PodInformer/UpdateFunc: called\n")
	// 	},
	// 	DeleteFunc: func(obj interface{}) {
	// 		fmt.Printf("PodInformer/DeleteFunc: called\n")
	// 	},
	// })

	// stopCh1 := make(chan struct{})
	// log.Printf("informer: Informer running\n")
	// go informer.Run(stopCh1)
	// go ctrl1.Run(stopCh1)

	// store.List()
	/*
		go func() {
			for {
				time.Sleep(1 * time.Second)
				fmt.Println("Go Func Loop: items num:", len(store.List()))
			}
		}()
	*/
	/*
		for i := 0; i < 20; i++ {
			time.Sleep(1 * time.Second)
			fmt.Println("Normal: items num:", len(store.List()))
		}
	*/
	// }()

	// log.Printf("\n\nHere 2\n\n")

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

	log.Printf("Returning Client\n")
	return &Client{
		apisrvClient:     c,
		unscheduledPodCh: pch,
		// nodeCh:           nch,
		KeepAround:    store,
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
