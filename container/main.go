package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/BGrewell/go-iperf"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	updateMetric()
}

func updateMetric() {
	_, dynamicClient, err := getKubernetesClient()
	if err != nil {
		log.Fatalf("failed to create k8s client: %v", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "raf.rs",
		Version:  "v1",
		Resource: "nodemetrics",
	}

	list, err := dynamicClient.Resource(gvr).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("failed to list NodeMetric CRDs: %v", err)
	}

	// Print all NodeMetrics
	fmt.Printf("Listing all NodeMetrics:\n")
	for _, nm := range list.Items {
		fmt.Printf("- %s\n", nm.GetName())
	}

	// Updating a NodeMetric
	nodeMetricName := "example-nodemetric"
	nodeMetric, err := dynamicClient.Resource(gvr).Namespace("").Get(context.TODO(), nodeMetricName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		log.Fatalf("NodeMetric %v does not exist", nodeMetricName)
	} else if err != nil {
		log.Fatalf("failed to get NodeMetric: %v", err)
	}

	metrics, found, err := unstructured.NestedSlice(nodeMetric.Object, "spec", "metrics")
	if err != nil {
		log.Fatalf("failed to retrieve metrics from NodeMetric: %v", err)
	}
	if !found || metrics == nil {
		log.Fatalf("failed to find metrics in NodeMetric or is nil")
	}

	if len(metrics) == 0 {
		log.Fatalf("failed to update bandwidth, metrics list is empty")
	}

	metricMap, ok := metrics[0].(map[string]any)
	if !ok {
		log.Fatalf("failed to update metric, first metric is not a map")
	}
	metricMap["bandwidth"] = "200Mbps"
	metrics[0] = metricMap

	err = unstructured.SetNestedSlice(nodeMetric.Object, metrics, "spec", "metrics")
	if err != nil {
		log.Fatalf("failed to set updated metrics in NodeMetric: %v", err)
	}

	// Push the updated NodeMetric back to the cluster
	updated, err := dynamicClient.Resource(gvr).Namespace("").Update(context.TODO(), nodeMetric, metav1.UpdateOptions{})
	if err != nil {
		log.Fatalf("failed to update NodeMetric: %v", err)
	}
	fmt.Printf("Updated NodeMetric %v successfully!\n", nodeMetricName)
	fmt.Printf("Updated NodeMetric: %v\n", updated.Object)

}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, World!")
}

func server() {
	s := iperf.NewServer()
	s.SetPort(5201)
	err := s.Start()
	if err != nil {
		fmt.Println("Failed to start server:", err)
		return
	}
	defer s.Stop()
	fmt.Println("Server is running...")

	http.HandleFunc("/", helloHandler)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
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
