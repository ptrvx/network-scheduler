package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/BGrewell/go-iperf"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	iperfPort = 5201
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Server is running")
}

func server() {
	s := iperf.NewServer()
	s.SetPort(iperfPort)
	err := s.Start()
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}
	defer s.Stop()
	log.Println("iperf server listening on port", iperfPort)

	http.HandleFunc("/", healthHandler)
	if err := http.ListenAndServe(":30080", nil); err != nil {
		log.Println("health server failed:", err)
	}
}

func client(target string) {
	c := iperf.NewClient(target)
	c.SetStreams(4)
	c.SetTimeSec(30)
	c.SetInterval(1)
	c.SetPort(iperfPort)
	liveReports := c.SetModeLive()

	go func() {
		for report := range liveReports {
			log.Println(report.String())
		}
	}()
	err := c.Start()
	if err != nil {
		log.Fatalf("Watching live reports from %v:%v\n", c.Host(), c.Port())
	}
	<-c.Done
}

func getKubernetesClient() (*kubernetes.Clientset, dynamic.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return clientset, dynamicClient, nil
}

func getNodeIPs(ctx context.Context, clientset *kubernetes.Clientset) ([]string, error) {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var nodeIPs []string
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodeIPs = append(nodeIPs, addr.Address)
				// break
			}
		}
	}
	return nodeIPs, nil
}
