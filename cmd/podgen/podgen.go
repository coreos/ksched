package main

import (
	"flag"
	"fmt"
	"strconv"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/restclient"
	kc "k8s.io/kubernetes/pkg/client/unversioned"
)

var (
	address string
	numPods int
)

func init() {
	flag.StringVar(&address, "addr", "130.211.169.84:8080", "APIServer addr")
	flag.IntVar(&numPods, "numPods", 1, "Number of pods to create")
	flag.Parse()
}

func main() {
	// Initialize the kubernetes client
	restCfg := &restclient.Config{
		Host:  fmt.Sprintf("http://%s", address),
		QPS:   1000,
		Burst: 1000,
	}
	c, err := kc.New(restCfg)
	if err != nil {
		panic(err.Error())
	}

	// Create one pod
	ns := "default"

	for i := 0; i < numPods; i++ {
		podName := "Pod-" + strconv.Itoa(i)
		_, err := c.Pods(ns).Create(&api.Pod{
			TypeMeta: unversioned.TypeMeta{
				Kind: "Pod",
			},
			ObjectMeta: api.ObjectMeta{
				Name: podName,
			},
			Spec: api.PodSpec{
				Containers: []api.Container{
					{
						Name:  podName,
						Image: "none",
					},
				},
			},
		})

		if err != nil {
			fmt.Printf("Failed to create pod:%s\n", podName)
			fmt.Printf("Error:%s", err.Error())
		}

	}

}
