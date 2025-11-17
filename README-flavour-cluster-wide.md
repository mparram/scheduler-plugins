# Overview

This folder holds the FlavourClusterWide plugin implementation, a Kubernetes scheduler plugin that scores nodes based on the distribution of pods with specific "flavour" labels across the **entire cluster**. Unlike standard scheduling methods that may be namespace-scoped, this plugin performs **cluster-wide distribution** of pods, meaning it considers all pods with flavour labels across all namespaces when making scheduling decisions. The goal is to balance the number of pods with different flavours across all nodes in the cluster, regardless of which namespace they belong to.

## Maturity Level

<!-- Check one of the values: Sample, Alpha, Beta, GA -->

- [x] 💡 Sample (for demonstrating and inspiring purpose)
- [ ] 👶 Alpha (used in companies for pilot projects)
- [ ] 👦 Beta (used in companies and developed actively)
- [ ] 👨 Stable (used in companies for production workloads)

## FlavourClusterWide Plugin

The `FlavourClusterWide` plugin is a **Score** and **PostBind** plugin that helps distribute pods with different flavour labels evenly across cluster nodes. It implements the `framework.ScorePlugin` and `framework.PostBindPlugin` interfaces from the Kubernetes scheduler framework.

**Key Differentiator:** This plugin performs **cluster-wide distribution**, meaning it considers pods from **all namespaces** when calculating flavour distribution. This is different from standard scheduling methods that typically operate within namespace boundaries. The plugin queries pods across the entire cluster without namespace filtering, ensuring a truly global balance of flavours across all nodes.

### How It Works

The plugin maintains an in-memory cache that tracks the count of pods per flavour for each node in the cluster. The cache structure is: `map[nodeName]map[flavour]count`.

**Scoring Algorithm:**
- When scoring a node for a pod with a flavour label, the plugin:
  1. Finds the minimum number of pods with the same flavour across **all nodes in the entire cluster** (considering pods from all namespaces)
  2. If the current node has that minimum count, it scores the node with **100 points**
  3. Otherwise, it scores the node with **0 points**
- This approach favors nodes that have the least number of pods with the same flavour, promoting balanced distribution across the cluster
- **Important:** The distribution calculation is **cluster-wide** and **namespace-agnostic**. Pods from different namespaces with the same flavour are treated equally in the distribution algorithm

**Cache Management:**
- The cache is updated in two ways:
  1. **Periodic updates**: Every 1 minute, the plugin queries the Kubernetes API to refresh the cache with current pod distribution
  2. **PostBind updates**: Immediately after a pod is bound to a node, the cache is updated to reflect the new pod assignment
- The cache is protected by a read-write mutex to ensure thread safety in concurrent scheduling scenarios

**Dynamic Flavour Discovery:**
- The plugin dynamically discovers all flavour values from pod labels across the cluster
- No hardcoded flavour list - flavours are automatically discovered as pods are scheduled
- When a new flavour value is encountered, it's automatically added to the cache for all nodes

### Requirements

**Pod Requirements:**
- Pods must have the configured label (default: `flavour`) to be considered by the plugin
- Pods without the configured label will receive a score of 0 (scoring is not applied)
- **Namespace Independence:** Pods from any namespace are considered equally - the plugin does not differentiate between namespaces when calculating flavour distribution
- **Label Name Configuration:** The label name can be customized via plugin configuration (see Configuration section)

**Node Requirements:**
- Nodes must have the label `node-role.kubernetes.io/worker` to be included in the cache initialization
- Only worker nodes are considered for flavour distribution

### Configuration

The plugin is registered in the scheduler binary at `cmd/scheduler/main.go`. To use it, you need to enable it in your scheduler configuration.

#### Using OpenShift Secondary Scheduler Operator

This plugin is designed to work with the OpenShift Secondary Scheduler Operator. Configuration is done through:

1. **SecondaryScheduler CR**: Define the scheduler instance in `secondaryScheduler.yaml`:
```yaml
apiVersion: operator.openshift.io/v1
kind: SecondaryScheduler
metadata:
  name: cluster
  namespace: openshift-secondary-scheduler-operator
spec:
  logLevel: TraceAll
  managementState: Managed
  operatorLogLevel: TraceAll
  schedulerConfig: secondary-scheduler-config
  schedulerImage: 'registry.k8s.io/scheduler-plugins/kube-scheduler:v0.27.8'
```

2. **Scheduler ConfigMap**: Configure the plugin in the scheduler profile via `config.yaml`:

**Example with default label name ("flavour"):**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: secondary-scheduler-config
  namespace: openshift-secondary-scheduler-operator
data:
  config.yaml: |
    apiVersion: kubescheduler.config.k8s.io/v1
    kind: KubeSchedulerConfiguration
    enableContentionProfiling: false
    enableProfiling: false
    percentageOfNodesToScore: 100
    profiles:
      - schedulerName: secondary-scheduler
        plugins:
          score:
            enabled:
              - name: FlavourClusterWide
              - name: NodeResourcesFit
              - name: NodeResourcesBalancedAllocation
        pluginConfig:
          - name: FlavourClusterWide
    leaderElection:
      leaderElect: true
      leaseDuration: 137s
      renewDeadline: 107s
      resourceLock: leases
      resourceNamespace: openshift-secondary-scheduler-operator
      retryPeriod: 26s
```

**Example with custom label name:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: secondary-scheduler-config
  namespace: openshift-secondary-scheduler-operator
data:
  config.yaml: |
    apiVersion: kubescheduler.config.k8s.io/v1
    kind: KubeSchedulerConfiguration
    enableContentionProfiling: false
    enableProfiling: false
    percentageOfNodesToScore: 100
    profiles:
      - schedulerName: secondary-scheduler
        plugins:
          score:
            enabled:
              - name: FlavourClusterWide
              - name: NodeResourcesFit
              - name: NodeResourcesBalancedAllocation
        pluginConfig:
          - name: FlavourClusterWide
            args:
              labelName: "tier"  # Custom label name instead of default "flavour"
    leaderElection:
      leaderElect: true
      leaseDuration: 137s
      renewDeadline: 107s
      resourceLock: leases
      resourceNamespace: openshift-secondary-scheduler-operator
      retryPeriod: 26s
```

**Plugin Configuration Parameters:**
- `labelName` (optional, string): The label key to use for identifying pod flavours. Defaults to `"flavour"` if not specified.

### Usage Examples

#### Example Deployments with Flavour Labels

To use the FlavourClusterWide plugin, create deployments with pods that have the configured label (default: `flavour`). Here are three example deployments with different flavours:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gold-workload
spec:
  replicas: 3
  selector:
    matchLabels:
      app: gold-workload
  template:
    metadata:
      labels:
        app: gold-workload
        flavour: gold
    spec:
      schedulerName: secondary-scheduler
      containers:
      - name: app
        image: nginx:latest
        resources:
          requests:
            memory: "64Mi"
            cpu: "250m"
          limits:
            memory: "128Mi"
            cpu: "500m"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: silver-workload
spec:
  replicas: 3
  selector:
    matchLabels:
      app: silver-workload
  template:
    metadata:
      labels:
        app: silver-workload
        flavour: silver
    spec:
      schedulerName: secondary-scheduler
      containers:
      - name: app
        image: nginx:latest
        resources:
          requests:
            memory: "64Mi"
            cpu: "250m"
          limits:
            memory: "128Mi"
            cpu: "500m"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bronze-workload
spec:
  replicas: 3
  selector:
    matchLabels:
      app: bronze-workload
  template:
    metadata:
      labels:
        app: bronze-workload
        flavour: bronze
    spec:
      schedulerName: secondary-scheduler
      containers:
      - name: app
        image: nginx:latest
        resources:
          requests:
            memory: "64Mi"
            cpu: "250m"
          limits:
            memory: "128Mi"
            cpu: "500m"
```

### Technical Details

**Cache Structure:**
```
map[string]map[string]int
  └─ nodeName: map[string]int
       └─ flavour: count
```

**Cache Update Frequency:**
- Minimum interval: 1 minute (cache TTL)
- Immediate updates on pod binding via PostBind hook

**API Queries:**
- Nodes: Queried with label selector `node-role.kubernetes.io/worker`
- Pods: Queried with the configured label name (default: `flavour`) across **all namespaces** (empty namespace string `""` in the API call)
  - This ensures cluster-wide visibility: pods from `default`, `kube-system`, `production`, `staging`, or any other namespace are all considered equally
  - Unlike namespace-scoped scheduling methods, this plugin does not filter or differentiate pods based on their namespace
  - The label name used for queries is configurable via the `labelName` parameter in plugin configuration

**Thread Safety:**
- Cache operations are protected by `sync.RWMutex`
- Read locks for scoring operations
- Write locks for cache updates

**Plugin Name:**
- The plugin is registered with the name `FlavourClusterWide`

### Future Enhancements

- Configurable cache TTL
- Additional scoring strategies beyond minimum count
- Metrics and observability improvements

