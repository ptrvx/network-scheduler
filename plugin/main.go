package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler"
	schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	schedframework "k8s.io/kubernetes/pkg/scheduler/framework"
	schedruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	"k8s.io/kubernetes/pkg/scheduler/profile"
)

const pluginName = "MyFilterPlugin"

type MyFilterPlugin struct {
	handle schedframework.Handle
}

var _ schedframework.FilterPlugin = &MyFilterPlugin{}

func (pl *MyFilterPlugin) Name() string {
	return pluginName
}

func (pl *MyFilterPlugin) Filter(ctx context.Context, state *schedframework.CycleState, pod *v1.Pod, nodeInfo *schedframework.NodeInfo) *schedframework.Status {
	if nodeInfo.Node().Name == "node1" {
		return schedframework.NewStatus(schedframework.Success)
	}
	return schedframework.NewStatus(schedframework.Unschedulable, fmt.Sprintf("node %s is not allowed", nodeInfo.Node().Name))
}

func NewMyFilterPlugin(ctx context.Context, _ runtime.Object, handle schedframework.Handle) (schedframework.Plugin, error) {
	return &MyFilterPlugin{
		handle: handle,
	}, nil
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	// Load kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.ExpandEnv("$HOME/.kube/config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %v", err)
	}

	// Create dynamic client
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error building dynamic client: %v", err)
	}

	// Create a plugin registry
	registry := schedruntime.Registry{
		pluginName: NewMyFilterPlugin,
	}

	// Create the plugin configuration
	profileConfig := schedconfig.KubeSchedulerProfile{
		SchedulerName: "network-scheduler",
		Plugins: &schedconfig.Plugins{
			Filter: schedconfig.PluginSet{
				Enabled: []schedconfig.Plugin{
					{Name: pluginName},
				},
			},
		},
	}

	// Initialize informer factories
	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	dynInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynClient, 0)

	// Create and start the scheduler using the framework option
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched, err := scheduler.New(
		ctx,
		clientset,
		informerFactory,
		dynInformerFactory,
		profile.NewRecorderFactory(nil),
		scheduler.WithProfiles(profileConfig),
		scheduler.WithFrameworkOutOfTreeRegistry(registry),
	)
	if err != nil {
		klog.Fatalf("Failed to create scheduler: %v", err)
	}

	// Start informers
	informerFactory.Start(ctx.Done())
	dynInformerFactory.Start(ctx.Done())

	klog.Info("Starting scheduler...")
	sched.Run(ctx)
	klog.Info("Scheduler stopped.")
}
