package main

import (
	"flag"
	"fmt"
	"strconv"

	"github.com/coreos/ksched/pkg/util"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/restclient"
	kc "k8s.io/kubernetes/pkg/client/unversioned"
)

var (
	address string
	numPods int
	image   string
	ns      string
)

func init() {
	flag.StringVar(&address, "endpoint", "localhost:8080", "API server address")
	flag.IntVar(&numPods, "numPods", 1, "Number of pods to create")
	flag.StringVar(&image, "image", "nginx", "The image for the container in the pod(s)")
	flag.StringVar(&ns, "ns", "default", "Namespace for the new pod(s)")
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

	// Generate the specified number of pods
	for i := 0; i < numPods; i++ {
		id := util.RandUint64()
		podName := image + strconv.FormatUint(id, 10)
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
						Image: image,
					},
				},
			},
		})

		if err != nil {
			fmt.Printf("Failed to create pod:%s\n", podName)
			fmt.Printf("Error:%s\n", err.Error())
			i--
		}

	}

}
