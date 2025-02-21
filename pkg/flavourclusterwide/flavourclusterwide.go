// Package flavourclusterwide provides a Kubernetes scheduler plugin that scores nodes based on the distribution
// of pods with specific "flavour" labels across the cluster. The goal is to balance the number of pods with
// different flavours (gold, silver, bronze) across all nodes.
//
// The FlavourClusterWide plugin implements the framework.ScorePlugin and framework.PostBindPlugin interfaces.
// It maintains a cache of pod counts per flavour for each node, which is periodically updated by querying the
// Kubernetes API. The cache is protected by a mutex to ensure thread safety.
//
// The plugin provides the following methods:
// - New: Initializes a new instance of the FlavourClusterWide plugin.
// - Name: Returns the name of the plugin.
// - updateCacheIfNeeded: Checks if the cache needs to be updated based on the last update time and updates it if necessary.
// - PostBind: Updates the cache when a pod is bound to a node.
// - Score: Scores a node based on the number of pods with the same flavour already running on the node.
// - ScoreExtensions: Returns the ScoreExtensions interface for the plugin.
// - NormalizeScore: Normalizes the scores of nodes (not implemented in this example).
package flavourclusterwide

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const Name = "FlavourClusterWide"

type FlavourClusterWide struct {
	handle      framework.Handle
	client      *kubernetes.Clientset
	cache       map[string]map[string]int
	cacheMutex  sync.RWMutex
	lastUpdated time.Time
}

var _ = framework.ScorePlugin(&FlavourClusterWide{})
var _ = framework.PostBindPlugin(&FlavourClusterWide{})

func New(_ context.Context, obj runtime.Object, h framework.Handle) (framework.Plugin, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("error getting cluster configuration: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating Kubernetes client: %v", err)
	}

	return &FlavourClusterWide{
		handle:      h,
		client:      clientset,
		cache:       make(map[string]map[string]int),
		cacheMutex:  sync.RWMutex{},
		lastUpdated: time.Time{},
	}, nil
}

func (f *FlavourClusterWide) Name() string {
	return Name
}

// updateCacheIfNeeded checks if the cache needs to be updated based on the last update time.
// If the cache is still valid (updated within the last minute), returns without updating.
// Otherwise, it fetches the list of nodes and pods from the Kubernetes API, filters them based on specific labels,
// and updates the cache with the count of pods per flavour (gold, silver, bronze) for each node.
// The cache is protected by a mutex to ensure thread safety.
func (f *FlavourClusterWide) updateCacheIfNeeded() {
	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	if time.Since(f.lastUpdated) < 1*time.Minute {
		log.Printf("Cache is still valid, not updating")
		return
	}

	ctx := context.TODO()

	nodes, err := f.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: "node-role.kubernetes.io/worker",
	})
	if err != nil {
		log.Printf("Error listing nodes: %v", err)
		return
	}

	pods, err := f.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: "flavour",
	})
	if err != nil {
		log.Printf("Error listing pods: %v", err)
		return
	}

	newCache := make(map[string]map[string]int)

	for _, node := range nodes.Items {
		newCache[node.Name] = map[string]int{
			"gold":   0,
			"silver": 0,
			"bronze": 0,
		}
	}

	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" {
			continue
		}
		node := pod.Spec.NodeName
		flavour := pod.Labels["flavour"]

		if _, exists := newCache[node]; !exists {
			newCache[node] = map[string]int{
				"gold":   0,
				"silver": 0,
				"bronze": 0,
			}
		}

		newCache[node][flavour]++
	}

	f.cache = newCache
	f.lastUpdated = time.Now()
	log.Printf("Cache recreated from API: %v", f.cache)
}

// PostBind is a method of the FlavourClusterWide struct that is called after a pod is bound to a node.
// It updates the cache with the count of pods of each flavour (gold, silver, bronze) per node.
// If the pod does not have a "flavour" label, the method returns immediately.
// The cache is protected by a mutex to ensure thread safety.
func (f *FlavourClusterWide) PostBind(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {

	flavour := pod.Labels["flavour"]
	if flavour == "" {
		return
	}

	f.cacheMutex.Lock()
	defer f.cacheMutex.Unlock()

	if _, exists := f.cache[nodeName]; !exists {
		f.cache[nodeName] = map[string]int{
			"gold":   0,
			"silver": 0,
			"bronze": 0,
		}
	}
	// log all the cache for debugging purposes

	f.cache[nodeName][flavour]++
	log.Printf("Cache: %v", f.cache)
}

// Score evaluates a given pod and node to determine a score based on the distribution of pods with the same "flavour" label across the cluster.
// It returns a score of 100 if the pod's flavour is the least common on the specified node, otherwise it returns 0.
// If the pod does not have a "flavour" label, scoring is not applied and a status message is returned.
func (f *FlavourClusterWide) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {

	flavour := pod.Labels["flavour"]
	if flavour == "" {
		return 0, framework.NewStatus(framework.Success, "Pod does not have the 'flavour' label, scoring is not applied")
	}

	f.updateCacheIfNeeded()

	f.cacheMutex.RLock()
	defer f.cacheMutex.RUnlock()

	minPods := -1
	for _, nodeCounts := range f.cache {
		if count, exists := nodeCounts[flavour]; exists {
			if minPods == -1 || count < minPods {
				minPods = count
			}
		}
	}

	podCount := f.cache[nodeName][flavour]

	if podCount == minPods {
		log.Printf("Pod %s with flavour %s is the least common in node %s", pod.Name, flavour, nodeName)
		return 100, framework.NewStatus(framework.Success, "")
	}

	return 0, framework.NewStatus(framework.Success, "")
}

func (f *FlavourClusterWide) ScoreExtensions() framework.ScoreExtensions {
	return f
}

func (f *FlavourClusterWide) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	return nil
}
