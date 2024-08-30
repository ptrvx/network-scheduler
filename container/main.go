package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	alpha = 0.4
)

var (
	Namespace = os.Getenv("POD_NAMESPACE")
	NodeName  = os.Getenv("NODE_NAME")
)

func main() {
	ctx := context.Background()
	go server()

	clientSet, dynamicClient, err := getKubernetesClient()
	if err != nil {
		log.Printf("failed to create k8s client: %v", err)
		os.Exit(1)
	}

	if Namespace == "" {
		Namespace = "default"
	}
	if NodeName == "" {
		log.Printf("failed to read NODE_NAME variable")
		os.Exit(1)
	}

	updater := &Updater{
		selfNode: NodeName,
		client:   dynamicClient,
		gvr: schema.GroupVersionResource{
			Group:    "raf.rs",
			Version:  "v1",
			Resource: "nodemetrics",
		},
		namespace: Namespace,
	}

	time.Sleep(15 * time.Second)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("context is done, program returning")
			return
		case <-ticker.C:
			nodes, err := getNodeItems(ctx, clientSet)
			if err != nil {
				log.Printf("failed to fetch k8s nodes: %v", err)
				continue
			}

			for _, node := range nodes {
				metric, err := measureNodeMetrics(node.Address)
				if err != nil {
					log.Printf("failed to measure node metrics for node %v(%v): %v", node.Name, node.Address, err)
					continue
				}
				err = updater.updateMetric(ctx, node.Name, metric)
				if err != nil {
					log.Printf("failed to update metrics for node %v(%v): %v", node.Name, node.Address, err)
				}
			}
		}
	}
}

type Updater struct {
	selfNode  string
	client    dynamic.Interface
	gvr       schema.GroupVersionResource
	namespace string
}

func (u *Updater) updateMetric(ctx context.Context, node string, metric Metric) error {
	nodeMetric, err := u.client.Resource(u.gvr).Namespace(u.namespace).Get(ctx, u.selfNode, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		nodeMetric = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "raf.rs/v1",
				"kind":       "NodeMetric",
				"metadata": map[string]interface{}{
					"name":      u.selfNode,
					"namespace": u.namespace,
				},
				"spec": map[string]interface{}{
					"metrics": map[string]interface{}{},
				},
			},
		}
		_, err = u.client.Resource(u.gvr).Namespace(u.namespace).Create(ctx, nodeMetric, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create NodeMetric: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get NodeMetric: %w", err)
	}

	metrics, found, err := unstructured.NestedMap(nodeMetric.Object, "spec", "metrics")
	if err != nil {
		return fmt.Errorf("failed to retreive metrics from NodeMetric %v: %w", node, err)
	}
	if !found || metrics == nil {
		metrics = make(map[string]interface{})
	}

	valuesRaw, ok := metrics[node]
	if ok {
		values, ok := valuesRaw.(map[string]interface{})
		if ok {
			upload, uploadOk := values["upload"].(float64)
			if uploadOk {
				metric.Bandwidth.Upload = metric.Bandwidth.Upload*alpha + (1-alpha)*upload
			}
			download, downloadOk := values["download"].(float64)
			if downloadOk {
				metric.Bandwidth.Download = metric.Bandwidth.Download*alpha + (1-alpha)*download
			}
			latency, latencyOk := values["latency"].(float64)
			if latencyOk {
				metric.Latency = metric.Latency*alpha + (1-alpha)*latency
			}
		}
	}

	metrics[node] = map[string]interface{}{
		"latency":  metric.Latency,
		"upload":   metric.Bandwidth.Upload,
		"download": metric.Bandwidth.Download,
	}

	err = unstructured.SetNestedMap(nodeMetric.Object, metrics, "spec", "metrics")
	if err != nil {
		return fmt.Errorf("failed to set updated metrics in NodeMetric: %v", err)
	}

	_, err = u.client.Resource(u.gvr).Namespace(u.namespace).Update(ctx, nodeMetric, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update NodeMetric %v: %w", node, err)
	}

	log.Printf("updated metrics for node %v", node)
	return nil
}

func measureNodeMetrics(node string) (Metric, error) {
	latency, err := ping(node)
	if err != nil {
		log.Printf("failed to measure latency to node %v: %v", node, err)
		return Metric{}, fmt.Errorf("failed to measure latency to node %v: %w", node, err)
	}
	bandwidth, err := client(node)
	if err != nil {
		log.Printf("failed to measure bandwidth to node %v: %v", node, err)
		return Metric{}, fmt.Errorf("failed to measrue bandwidth to node %v: %w", node, err)
	}
	return Metric{Latency: float64(latency.Microseconds()), Bandwidth: bandwidth}, nil
}
