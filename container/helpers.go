package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/BGrewell/go-iperf"
	probing "github.com/prometheus-community/pro-bing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	iperfPort = 5201
)

type Metric struct {
	Latency   float64
	Bandwidth Bandwidth
}
type Bandwidth struct {
	Upload, Download float64
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Server is running")
}

func ping(target string) (time.Duration, error) {
	pinger, err := probing.NewPinger(target)
	if err != nil {
		log.Println("failed to start new pinger:", err)
		return 0, fmt.Errorf("failed to start new pinger: %w", err)
	}
	pinger.Count = 3
	err = pinger.Run() // Blocks until finished
	if err != nil {
		log.Println("failed to run ping:", err)
		return 0, fmt.Errorf("failed to run ping: %w", err)
	}
	stats := pinger.Statistics() // get send/receive/duplicate/rtt stats
	log.Printf("ping to %v, latency %v\n:", target, stats.AvgRtt)
	return stats.AvgRtt, nil
}

// Sends iperf requests to target returning download and upload bandwidth values
func client(target string) (Bandwidth, error) {
	c := iperf.NewClient(target)
	c.SetStreams(3)
	c.SetTimeSec(5)
	c.SetInterval(1)
	c.SetPort(iperfPort)

	err := c.Start()
	if err != nil {
		log.Printf("failed to run iperf client to %v: %v", target, err)
		return Bandwidth{}, fmt.Errorf("failed to run iperf client: %w", err)
	}
	<-c.Done
	report := c.Report()
	if report == nil {
		log.Println("failed to measure iperf report")
		return Bandwidth{}, fmt.Errorf("failed to measure iperf report")
	}
	download := report.End.SumReceived.BitsPerSecond
	upload := report.End.SumSent.BitsPerSecond
	log.Printf("iperf to %v, download: %v bps, upload: %v bps\n", target, download, upload)
	return Bandwidth{Download: download, Upload: upload}, nil
}

// Starts iperf and healthcheck servers
func server() {
	s := iperf.NewServer()
	s.SetPort(iperfPort)
	err := s.Start()
	if err != nil {
		log.Fatal("failed to start server:", err)
	}
	defer s.Stop()
	log.Println("iperf server listening on port", iperfPort)

	http.HandleFunc("/", healthHandler)
	if err := http.ListenAndServe(":30080", nil); err != nil {
		log.Println("health server failed:", err)
	}
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

func getNodeItems(ctx context.Context, clientset *kubernetes.Clientset) ([]NodeItem, error) {
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var nodes []NodeItem
	for _, node := range nodeList.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodes = append(nodes, NodeItem{Name: node.Name, Address: addr.Address})
				break
			}
		}
	}
	return nodes, nil
}

type NodeItem struct {
	Address string
	Name    string
}
