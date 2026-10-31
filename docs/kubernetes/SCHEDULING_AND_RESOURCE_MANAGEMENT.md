# KubeCheck — Scheduling and Resource Management

**Document ID:** K8S-SCHED-001  
**File:** `docs/kubernetes/SCHEDULING_AND_RESOURCE_MANAGEMENT.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Kubernetes Scheduling and Capacity Diagnostics  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`
- `docs/kubernetes/WORKLOADS_AND_PODS.md`
- `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md`
- `docs/kubernetes/CONFIGURATION_AND_STORAGE.md`
- `docs/kubernetes/SECURITY_AND_RBAC.md`
- `docs/kubernetes/KUBERNETES_SCHEMA_VALIDATION.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/testing/DETERMINISM_TESTS.md`

---

## 1. Purpose and scope

This document defines KubeCheck's **offline analysis of resource allocation and scheduling configuration** in Kubernetes manifests. It covers CPU, memory, ephemeral storage, and extended-resource quantities; requests, limits, and QoS-related configuration; node selectors, affinity, anti-affinity, and topology spread; taints, tolerations, and priority classes; PodDisruptionBudgets; HorizontalPodAutoscalers; ResourceQuota and LimitRange; storage-scheduling interactions; architecture and OS constraints; and version-specific scheduling fields.

**Governing distinction:**

> **K8S-SCHED-REQ-001 — KubeCheck MUST distinguish impossible or internally inconsistent declared configurations from merely restrictive configurations whose feasibility depends on actual cluster nodes and capacity.** A request that exceeds its limit is impossible and is rejected by the API server. A nodeSelector that matches no node is restrictive; its feasibility depends on the cluster's node inventory, which KubeCheck cannot observe offline. The first is an `error`; the second is `info` or `warning` with a clear statement that cluster state is required.

**In scope:** quantity parsing and comparison; QoS derivation; scheduling constraints (node selectors, affinity, topology spread, taints/tolerations); priority classes; PodDisruptionBudgets; HPA configuration; ResourceQuota and LimitRange; storage-scheduling interactions; architecture/OS constraints; pending-Pod and OOMKilled pattern attribution; version-specific fields.

**Out of scope:** live node capacity, live scheduling decisions, node conditions as runtime evidence, and actual resource usage. These are referenced but not claimed as KubeCheck capabilities.

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **Request** | The amount of a resource the scheduler reserves for a container. |
| **Limit** | The maximum amount of a resource the container may use at runtime. |
| **QoS class** | The quality-of-service classification (`Guaranteed`, `Burstable`, `BestEffort`) derived from requests and limits. |
| **Extended resource** | A non-CPU, non-memory, non-ephemeral-storage resource (e.g., `nvidia.com/gpu`, `hugepages-2Mi`). |
| **Impossible configuration** | A configuration the API server rejects or that cannot be satisfied under any cluster state. |
| **Restrictive configuration** | A configuration that is syntactically valid and may be feasible in some clusters but not in others. |
| **Node selector** | A simple key-value constraint on node labels. |
| **Node affinity** | A richer constraint with `requiredDuringSchedulingIgnoredDuringExecution` and `preferredDuringSchedulingIgnoredDuringExecution`. |
| **Pod affinity/anti-affinity** | Constraints relating a Pod to other Pods in the same or different topology domains. |
| **Topology spread constraint** | A declarative constraint on how Pods are distributed across topology domains. |
| **Taint** | A node property that repels Pods that do not tolerate it. |
| **Toleration** | A Pod property that allows scheduling onto a tainted node. |
| **PriorityClass** | A cluster-scoped object that assigns a scheduling priority to Pods. |
| **PDB** | PodDisruptionBudget, a policy that limits voluntary disruptions. |
| **HPA** | HorizontalPodAutoscaler, an autoscaling object that adjusts replica count based on metrics. |

**Requirement IDs** use `K8S-SCHED-REQ-NNN`. Finding IDs use `SCHED-QTY-*`, `SCHED-QOS-*`, `SCHED-NODE-*`, `SCHED-AFF-*`, `SCHED-TAINT-*`, `SCHED-PRIO-*`, `SCHED-PDB-*`, `SCHED-HPA-*`, `SCHED-QUOTA-*`, `SCHED-LIMIT-*`, `SCHED-STOR-*`, and `SCHED-ARCH-*`.

---

## 3. Resource quantities: units, parsing, overflow, comparison

### 3.1 CPU quantities

CPU is measured in **cores**. The quantity expression `1` means one core; `0.5` means half a core; `500m` means 500 millicores, which is equivalent to `0.5` cores. The smallest representable CPU unit is `1m` (one millicore). To specify a value less than `1.0` or `1000m`, the millicore form must be used: `5m`, not `0.005` CPU.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-QTY-001** | A CPU quantity uses a fractional value below `1m` (e.g., `0.0005`) | `error` | The value is not representable; the API server may round or reject it. |
| **SCHED-QTY-002** | A CPU quantity uses a suffix that is not `m` or a valid SI suffix (e.g., `500mcpu`) | `error` | Invalid quantity syntax. |
| **SCHED-QTY-003** | A CPU request or limit is negative | `error` | Negative quantities are invalid. |

### 3.2 Memory quantities

Memory is measured in **bytes**. Quantity suffixes include decimal SI (`k`, `M`, `G`, `T`, `P`, `E`) and binary SI (`Ki`, `Mi`, `Gi`, `Ti`, `Pi`, `Ei`). The binary suffixes are powers of 1024: `1Ki` = 1024 bytes, `1Mi` = 1024² bytes, `1Gi` = 1024³ bytes.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-QTY-004** | A memory quantity uses an invalid suffix (e.g., `512MB` is valid, but `512Mib` is not) | `error` | Invalid quantity syntax. |
| **SCHED-QTY-005** | A memory quantity is negative | `error` | Negative quantities are invalid. |

### 3.3 Ephemeral storage quantities

Ephemeral storage is measured in **bytes** and uses the same suffixes as memory. It covers writable container layers, container logs, and `emptyDir` volumes (except `tmpfs`-backed `emptyDir` volumes). Container images and `tmpfs` `emptyDir` volumes do **not** count against ephemeral storage limits.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-QTY-006** | An `ephemeral-storage` quantity uses an invalid suffix | `error` | Invalid quantity syntax. |
| **SCHED-QTY-007** | An `emptyDir.sizeLimit` exceeds the container's `ephemeral-storage` limit | `warning` | The volume may exhaust the container's ephemeral storage budget. |

**K8S-SCHED-REQ-002:** An `emptyDir` with `medium: Memory` is backed by tmpfs and does **not** count against ephemeral storage. KubeCheck MUST NOT count tmpfs-backed `emptyDir` against `ephemeral-storage` limits.

### 3.4 Extended resources

Extended resources are non-CPU, non-memory, non-ephemeral-storage resources that are advertised by nodes. They include `nvidia.com/gpu`, `amd64`/`arm64` architecture hints, and `hugepages-<size>` for pre-allocated huge pages. Extended resources must be specified as **integers**; fractional values are invalid.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-QTY-008** | An extended resource request is fractional | `error` | Extended resources must be integers. |
| **SCHED-QTY-009** | A `hugepages-<size>` resource uses a size not supported by the node (heuristic) | `info` | Node hugepage support is cluster-dependent. |

### 3.5 Quantity comparison and overflow

Quantity comparison must use the Kubernetes quantity semantics: values are parsed into a canonical form and compared numerically. A quantity may overflow the implementation's integer range; such a value must be rejected rather than wrapped.

**K8S-SCHED-REQ-003:** KubeCheck MUST compare quantities using canonical numeric values, not string comparison. `1Gi` and `1024Mi` are equal; `1G` and `1Gi` are not. A quantity that cannot be represented without overflow MUST produce an `error` finding.

---

## 4. Requests, limits, and QoS classes

### 4.1 QoS derivation

Kubernetes assigns every Pod a QoS class based on the resource requests and limits of its containers:

| QoS class | Criteria |
|---|---|
| **Guaranteed** | Every container has a memory limit and memory request, both greater than zero, and they are equal; every container has a CPU limit and CPU request, both greater than zero, and they are equal. |
| **Burstable** | The Pod does not meet the Guaranteed criteria, but at least one container has a CPU or memory request or limit. |
| **BestEffort** | No container has any CPU or memory requests or limits. |

The QoS class determines eviction order under node pressure: BestEffort Pods are evicted first, followed by Burstable, and finally Guaranteed.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-QOS-001** | A container has a `limits` but no `requests` for the same resource | `warning` | The request defaults to the limit, which may over-reserve capacity. |
| **SCHED-QOS-002** | `requests` exceeds `limits` for the same resource | `error` | The API server rejects the Pod. |
| **SCHED-QOS-003** | A container has neither `requests` nor `limits` | `info` | BestEffort QoS; most likely to be evicted under pressure. |
| **SCHED-QOS-004** | A Pod is BestEffort but has multiple replicas and no PDB | `info` | BestEffort Pods are evicted first; a PDB does not protect against eviction but reduces voluntary disruption. |
| **SCHED-QOS-005** | A Guaranteed Pod has a single replica and no PDB | `info` | The workload is non-disruptible but has no availability guarantee. |

### 4.2 Pod-level resources

Kubernetes v1.34 graduated **Pod-level resources** to beta. When both pod-level and container-level resources are specified, pod-level requests and limits take precedence. The pod-level QoS criteria mirror the container-level criteria: the Pod must have a Pod-level memory limit and memory request with equal values, and a Pod-level CPU limit and CPU request with equal values.

**Heuristic check (SCHED-QOS-006):** A Pod that specifies both pod-level and container-level resources SHOULD produce an `info` finding stating that pod-level resources take precedence, and that the container-level values may be ignored for scheduling and QoS purposes.

**K8S-SCHED-REQ-004:** When pod-level resources are present, KubeCheck MUST use the pod-level values for QoS derivation and MUST NOT combine pod-level and container-level values in a way that produces a QoS class the API server would not assign.

---

## 5. Node selectors, node affinity, pod affinity, anti-affinity, and topology spread

### 5.1 Node selectors

`spec.nodeSelector` is a map of node label keys to values. A Pod is scheduled only onto nodes whose labels match every entry.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-NODE-001** | `nodeSelector` contains a key that is not a valid label key | `error` | Invalid selector syntax. |
| **SCHED-NODE-002** | `nodeSelector` conflicts with `nodeAffinity` required terms (heuristic) | `warning` | The two constraints may be jointly unsatisfiable; the finding names the conflicting keys. |

**K8S-SCHED-REQ-005:** A `nodeSelector` that matches no node is **restrictive**, not impossible. KubeCheck MUST NOT report it as an `error`; it may report that the selector is syntactically valid and that node-label matching requires cluster state.

### 5.2 Node affinity

`nodeAffinity` has two scheduling terms:

| Term | Semantics |
|---|---|
| `requiredDuringSchedulingIgnoredDuringExecution` | The Pod is scheduled only onto nodes that match the term. If no node matches, the Pod remains `Pending`. |
| `preferredDuringSchedulingIgnoredDuringExecution` | The scheduler prefers matching nodes but may place the Pod elsewhere. Each preferred term has a `weight` (1–100). |

The `IgnoredDuringExecution` suffix means that a node label change after scheduling does not evict the Pod.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-AFF-001** | A required node affinity term has no `matchExpressions` or `matchFields` | `error` | An empty term matches all nodes or is malformed; the API server may reject it. |
| **SCHED-AFF-002** | A preferred node affinity term has `weight` outside 1–100 | `error` | Invalid weight. |
| **SCHED-AFF-003** | A required node affinity term uses an operator not in `In`, `NotIn`, `Exists`, `DoesNotExist`, `Gt`, `Lt` | `error` | Invalid operator. |

### 5.3 Pod affinity and anti-affinity

Pod affinity and anti-affinity constrain a Pod relative to other Pods. Both have required and preferred forms and use a `topologyKey` to define the domain (e.g., `kubernetes.io/hostname`, `topology.kubernetes.io/zone`). `labelSelector` selects the related Pods; `namespaces` and `namespaceSelector` control which namespaces are searched.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-AFF-004** | A pod affinity or anti-affinity term has an empty `topologyKey` | `error` | `topologyKey` is required. |
| **SCHED-AFF-005** | A required pod affinity term references a namespace that does not exist in the input set (when namespaces are supplied) | `info` | Namespace existence is cluster-dependent. |
| **SCHED-AFF-006** | A pod anti-affinity term with `requiredDuringSchedulingIgnoredDuringExecution` is combined with `replicas: 1` | `info` | The anti-affinity has no effect with a single replica. |

**K8S-SCHED-REQ-006:** A required pod affinity or anti-affinity that cannot be satisfied is **restrictive**, not impossible: the outcome depends on the Pods present in the cluster. KubeCheck MUST NOT report it as an `error`.

### 5.4 Topology spread constraints

`topologySpreadConstraints` controls how Pods are distributed across topology domains. Each constraint has:

| Field | Required | Semantics |
|---|---|---|
| `maxSkew` | Yes | The maximum permitted difference in matching Pod counts across domains. Must be greater than zero. |
| `topologyKey` | Yes | The node label that defines the domain. |
| `whenUnsatisfiable` | Yes | `DoNotSchedule` (hard constraint) or `ScheduleAnyway` (soft constraint). |
| `labelSelector` | Yes | Selects the Pods to spread. |
| `minDomains` | No | The minimum number of eligible domains; valid only with `DoNotSchedule`. |
| `matchLabelKeys` | No | Keys used to select the Pods to spread (beta since v1.27). |
| `nodeAffinityPolicy` | No | `Honor` or `Ignore` (beta since v1.26). |
| `nodeTaintsPolicy` | No | `Honor` or `Ignore` (beta since v1.26). |

For `whenUnsatisfiable: DoNotSchedule`, `maxSkew` defines the maximum permitted difference between the number of matching Pods in the target topology and the global minimum. For `whenUnsatisfiable: ScheduleAnyway`, the scheduler still prefers balanced placement but does not enforce the skew.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-AFF-007** | `maxSkew` is `0` or negative | `error` | Must be greater than zero. |
| **SCHED-AFF-008** | `whenUnsatisfiable` is not `DoNotSchedule` or `ScheduleAnyway` | `error` | Invalid enum. |
| **SCHED-AFF-009** | `minDomains` is set with `whenUnsatisfiable: ScheduleAnyway` | `error` | `minDomains` is valid only with `DoNotSchedule`. |
| **SCHED-AFF-010** | Two constraints share the same `topologyKey` and `whenUnsatisfiable` | `error` | For a given `topologyKey` and `whenUnsatisfiable` value, only one constraint is permitted. |

**Heuristic check (SCHED-AFF-011):** A `topologySpreadConstraints` with `whenUnsatisfiable: DoNotSchedule` and a `topologyKey` that defines fewer domains than the replica count MAY produce an `info` finding stating that the constraint may be unsatisfiable; the finding does not claim impossibility, because the cluster's domain count is unknown offline.

---

## 6. Taints, tolerations, node conditions, and priority classes

### 6.1 Taints and tolerations

A **taint** is a key-value pair with an effect that repels Pods from a node. A **toleration** on a Pod allows it to be scheduled onto a node with a matching taint. The three effects are:

| Effect | Behavior |
|---|---|
| `NoSchedule` | Pods that do not tolerate the taint are not scheduled onto the node; existing Pods are not evicted. |
| `PreferNoSchedule` | The scheduler avoids scheduling non-tolerating Pods but may do so if necessary. |
| `NoExecute` | Non-tolerating Pods are evicted from the node, and new Pods are not scheduled. |

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-TAINT-001** | A toleration's `effect` is not `NoSchedule`, `PreferNoSchedule`, or `NoExecute` | `error` | Invalid enum. |
| **SCHED-TAINT-002** | A toleration has `operator: Exists` and a non-empty `value` | `error` | `Exists` does not use a value. |
| **SCHED-TAINT-003** | A toleration has `operator: Equal` and an empty `value` | `warning` | The toleration will not match a taint with a value. |
| **SCHED-TAINT-004** | A DaemonSet has no toleration for a known control-plane taint (`node-role.kubernetes.io/control-plane:NoSchedule`) | `info` | The DaemonSet will not run on control-plane nodes unless tolerated. |

**K8S-SCHED-REQ-007:** A Pod that does not tolerate a taint present in the cluster is **restrictive**, not impossible. The taints are node state; KubeCheck MUST NOT report the absence of a toleration as an `error` (`SCHED-TAINT-004` is `info`).

### 6.2 Node conditions as runtime-only evidence

Node conditions (`Ready`, `MemoryPressure`, `DiskPressure`, `PIDPressure`, `NetworkUnavailable`) are **runtime-only** evidence. They cannot be observed from a manifest.

**K8S-SCHED-REQ-008:** KubeCheck MUST NOT infer node conditions from manifest fields. A Pod that is `Pending` due to a node condition is a live-status failure, not a manifest defect.

### 6.3 Priority classes

A `PriorityClass` is a cluster-scoped object with a `value` (integer, 0 to 1,000,000,000 for non-system classes), an optional `globalDefault`, and a `preemptionPolicy` (`PreemptLowerPriority` default or `Never`). A Pod references a PriorityClass via `spec.priorityClassName`. Higher-priority Pods are placed first when capacity is scarce; if a higher-priority Pod cannot schedule, the scheduler may evict lower-priority Pods unless `preemptionPolicy: Never` is set.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-PRIO-001** | A `PriorityClass` `value` exceeds 1,000,000,000 (excluding `system-` prefixed classes) | `error` | Out-of-range value. |
| **SCHED-PRIO-002** | A `PriorityClass` with `globalDefault: true` already exists in the input set | `error` | Only one global default is permitted. |
| **SCHED-PRIO-003** | A Pod's `priorityClassName` references a PriorityClass not in the input set | `info` | Existence is cluster-dependent. |
| **SCHED-PRIO-004** | A Pod's `priorityClassName` is `system-cluster-critical` or `system-node-critical` outside the `kube-system` namespace | `warning` | System priority classes are reserved for cluster components. |

**K8S-SCHED-REQ-009:** A missing PriorityClass reference is `info`, not `error`, because the class may exist in the cluster. The finding MUST state that the Pod's priority and preemption behavior cannot be determined offline.

---

## 7. PodDisruptionBudgets

### 7.1 Structure and mutual exclusivity

A PDB has a `selector` (which Pods it applies to) and exactly one of `minAvailable` or `maxUnavailable`, both `IntOrString` (a count or a percentage). `maxUnavailable` means an eviction is allowed if at most `maxUnavailable` Pods selected by the selector are unavailable after the eviction. `minAvailable` means an eviction is allowed if at least `minAvailable` Pods remain available after the eviction. The two fields are mutually exclusive.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-PDB-001** | Both `minAvailable` and `maxUnavailable` are set | `error` | The API server rejects the PDB. |
| **SCHED-PDB-002** | Neither `minAvailable` nor `maxUnavailable` is set | `error` | A PDB must specify one of the two. |
| **SCHED-PDB-003** | `maxUnavailable` is `0` | `warning` | All voluntary evictions are blocked; node drains will stall. |
| **SCHED-PDB-004** | `minAvailable` is `100%` | `warning` | All voluntary evictions are blocked. |
| **SCHED-PDB-005** | `minAvailable` or `maxUnavailable` uses a percentage that does not resolve to an integer for the replica count | `info` | The effective count depends on the workload's replicas, which may not be in the input set. |
| **SCHED-PDB-006** | A PDB's selector matches no workload in the input set | `info` | The PDB may match a workload in the cluster. |
| **SCHED-PDB-007** | A Deployment with `replicas > 1` has no matching PDB (heuristic) | `info` | The workload is exposed to voluntary disruption. |

**K8S-SCHED-REQ-010:** The `SCHED-PDB-006` check MUST be `info`, not `error`, because a PDB may legitimately target a workload not included in the analyzed input set. KubeCheck cannot prove that the PDB is orphaned.

### 7.2 PDB and workload relationship

A PDB protects Pods selected by its `selector`. If the selector matches multiple workloads, the PDB's budget is shared across them. If the selector matches no workload, the PDB has no effect.

**Heuristic check (SCHED-PDB-008):** A PDB whose selector matches workloads in multiple namespaces in the input set SHOULD produce an `info` finding stating that the budget is shared across namespaces; the finding notes that `kubectl` behavior may differ.

---

## 8. HorizontalPodAutoscalers

### 8.1 Structure

An HPA (`autoscaling/v2`) has a `scaleTargetRef` (the workload to scale), `minReplicas`, `maxReplicas`, `metrics` (a list of metric specifications), and `behavior` (scale-up and scale-down policies). Each metric has a `type` (`Resource`, `Pods`, `Object`, or `External`) and a `target` with a `type` (`Utilization`, `Value`, or `AverageValue`).

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-HPA-001** | `minReplicas` is greater than `maxReplicas` | `error` | The API server rejects the HPA. |
| **SCHED-HPA-002** | `minReplicas` is less than `1` | `error` | Minimum is 1. |
| **SCHED-HPA-003** | A metric `type` is not `Resource`, `Pods`, `Object`, or `External` | `error` | Invalid enum. |
| **SCHED-HPA-004** | A `Resource` metric targets `memory` with `type: Utilization` and the target Pods have no memory requests | `warning` | Utilization is a percentage of requests; without requests, the HPA cannot compute the metric. |
| **SCHED-HPA-005** | A `Pods` metric with `type: AverageValue` has no `averageValue` set | `error` | `averageValue` is required for this target type. |
| **SCHED-HPA-006** | `scaleTargetRef` references a workload not in the input set | `info` | The workload may exist in the cluster. |

**K8S-SCHED-REQ-011:** An HPA that targets a Deployment with `replicas` set in the Deployment spec and no `behavior` policy SHOULD produce an `info` finding stating that the HPA and the Deployment's `replicas` may conflict; the finding recommends removing `replicas` from the Deployment manifest when an HPA manages it.

### 8.2 HPA and metrics availability

The metrics API and the metrics pipeline provide only CPU and memory metrics by default. Custom metrics require an adapter (e.g., Prometheus Adapter) and an external metrics API.

**K8S-SCHED-REQ-012:** A `Pods`, `Object`, or `External` metric MUST produce an `info` finding stating that the metric requires a custom-metrics adapter or external-metrics adapter, and that the adapter's availability is cluster-dependent.

---

## 9. ResourceQuota and LimitRange

### 9.1 ResourceQuota

A ResourceQuota limits aggregate resource consumption per namespace. Common quota keys include `requests.cpu`, `requests.memory`, `limits.cpu`, `limits.memory`, `requests.ephemeral-storage`, `limits.ephemeral-storage`, `pods`, `services`, `secrets`, `configmaps`, `persistentvolumeclaims`, and `count/<resource>.<group>`. When a quota is enforced for `cpu` or `memory`, every new Pod must specify `requests` or `limits` for that resource; otherwise, the quota system rejects the Pod.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-QUOTA-001** | A ResourceQuota's `requests.cpu` exceeds `limits.cpu` | `error` | The quota is internally inconsistent; requests cannot exceed limits in aggregate. |
| **SCHED-QUOTA-002** | A ResourceQuota's `requests.memory` exceeds `limits.memory` | `error` | Same as above. |
| **SCHED-QUOTA-003** | A ResourceQuota is present in the namespace but a Pod in the input set does not specify `requests` for a quota-enforced resource | `warning` | The Pod may be rejected by the quota system. |
| **SCHED-QUOTA-004** | A ResourceQuota's `count/<resource>` key is not a valid resource name | `error` | Invalid quota key. |

**K8S-SCHED-REQ-013:** The `SCHED-QUOTA-003` check MUST only fire when a ResourceQuota for the namespace is in the input set. If the quota is not in the input set, KubeCheck MUST NOT assume that a quota is enforced.

### 9.2 LimitRange

A LimitRange constrains per-object resource allocations in a namespace. It can enforce minimum and maximum CPU and memory per Pod or container, a request-to-limit ratio, and default requests and limits. If a LimitRange for compute resources is present, Pods that omit requests or limits receive the defaults at admission; if the default limit is less than the explicit request, the resulting Pod is not schedulable.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-LIMIT-001** | A LimitRange `max` is less than its `min` | `error` | Internally inconsistent. |
| **SCHED-LIMIT-002** | A LimitRange `default` limit is less than its `defaultRequest` | `error` | The default limit would be less than the default request. |
| **SCHED-LIMIT-003** | A Pod's explicit `requests` exceeds the LimitRange `max` | `error` | The Pod will be rejected at admission. |
| **SCHED-LIMIT-004** | A Pod's explicit `requests` is below the LimitRange `min` | `error` | The Pod will be rejected at admission. |
| **SCHED-LIMIT-005** | A Pod in a namespace with a LimitRange omits `requests` or `limits` | `info` | The LimitRange defaults will be applied; the effective request may differ from the user's intent. |
| **SCHED-LIMIT-006** | Two or more LimitRanges exist in the same namespace with conflicting defaults | `warning` | It is not deterministic which default is applied. |

**K8S-SCHED-REQ-014:** The `SCHED-LIMIT-003` and `SCHED-LIMIT-004` checks MUST compare the Pod's explicit values against the LimitRange's `min` and `max` only when both the LimitRange and the Pod are in the input set. The comparison MUST use canonical quantity values.

---

## 10. Persistent storage and scheduling interactions

### 10.1 volumeBindingMode

A StorageClass's `volumeBindingMode` controls when a PVC binds: `Immediate` binds as soon as the PVC is created; `WaitForFirstConsumer` delays binding until a Pod using the PVC is scheduled.

**Heuristic check (SCHED-STOR-001):** A StatefulSet using a StorageClass with `volumeBindingMode: Immediate` on a topology-constrained provisioner SHOULD produce an `info` finding stating that the volume may be provisioned in a zone that cannot schedule the Pod.

### 10.2 Access modes and scheduling

A PVC with `ReadWriteOnce` can be mounted by a single **node**; a `ReadWriteOncePod` PVC can be mounted by a single **Pod**. A Deployment with multiple replicas using `ReadWriteOnce` PVCs may fail to schedule on nodes that do not have the volume attached.

**Deterministic check (SCHED-STOR-002):** A workload with `replicas > 1` that mounts a `ReadWriteOncePod` PVC MUST produce an `error` finding, because only one Pod can mount the volume and the other replicas will remain `Pending`.

**Heuristic check (SCHED-STOR-003):** A workload with multiple replicas using `ReadWriteOnce` PVCs SHOULD produce an `info` finding stating that the replicas may be scheduled to different nodes and that the volume attachment is node-dependent.

---

## 11. Architecture and operating-system scheduling constraints

Kubernetes nodes advertise architecture (`kubernetes.io/arch`: `amd64`, `arm64`, etc.) and OS (`kubernetes.io/os`: `linux`, `windows`) as node labels. A Pod can constrain itself via `nodeSelector` or node affinity.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SCHED-ARCH-001** | A `nodeSelector` uses `kubernetes.io/arch` with a value not in `amd64`, `arm64`, `ppc64le`, `s390x` | `warning` | The value may be valid in a custom cluster but is not a standard architecture. |
| **SCHED-ARCH-002** | A `nodeSelector` uses `kubernetes.io/os` with a value not in `linux`, `windows` | `warning` | Same as above. |
| **SCHED-ARCH-003** | A Pod with a Windows `nodeSelector` uses a Linux-only feature (e.g., `hostPID`, `privileged: true`) | `error` | The combination is unsatisfiable on Windows nodes. |

**K8S-SCHED-REQ-015:** The `SCHED-ARCH-003` check MUST only fire when the Pod's `kubernetes.io/os` selector is `windows` and the feature is known to be unsupported on Windows. KubeCheck MUST NOT infer the OS from the image name.

---

## 12. Common pending-Pod, OOMKilled, and resource-pressure patterns

| Pattern | Manifest finding | Runtime symptom | Live-status check required |
|---|---|---|---|
| Request exceeds limit | `SCHED-QOS-002` (`error`) | Pod rejected by API server | — |
| BestEffort Pod evicted under pressure | `SCHED-QOS-003` (`info`) | Evicted; rescheduled | **Cluster-dependent** |
| Node selector matches no node | `SCHED-NODE-001`/`SCHED-AFF-001` (syntax only) | Pod `Pending` | **Cluster-dependent** |
| Required pod anti-affinity unsatisfiable | `SCHED-AFF-004`/`005` (syntax only) | Pod `Pending` | **Cluster-dependent** |
| Missing toleration for tainted node | `SCHED-TAINT-004` (`info`) | Pod `Pending` on that node pool | **Cluster-dependent** |
| PDB `maxUnavailable: 0` blocks drain | `SCHED-PDB-003` (`warning`) | Node drain stalls | **Cluster-dependent** |
| HPA `minReplicas > maxReplicas` | `SCHED-HPA-001` (`error`) | HPA rejected | — |
| ResourceQuota requires requests but Pod omits them | `SCHED-QUOTA-003` (`warning`) | Pod rejected by quota | **Cluster-dependent** |
| LimitRange default limit < explicit request | `SCHED-LIMIT-002` (`error`) | Pod unschedulable | — |
| `ReadWriteOncePod` PVC with multiple replicas | `SCHED-STOR-002` (`error`) | Replicas `Pending` | — |
| OOMKilled | — (manifest may set a low memory limit; `SCHED-QOS-002` only if request > limit) | Container exit code 137 | **Cluster-dependent** |

**K8S-SCHED-REQ-016:** Every row in §12 MUST distinguish the manifest finding from the runtime symptom. A manifest finding is what KubeCheck can report; a runtime symptom requires live status and MUST NOT be claimed as a KubeCheck finding (`K8S-WORKLOAD-REQ-017`).

---

## 13. Version-specific scheduling fields

| Field | Change | KubeCheck handling |
|---|---|---|
| Pod-level `resources` | Beta in v1.34; pod-level takes precedence over container-level. | Validate; `info` if both levels are set (`SCHED-QOS-006`). |
| `topologySpreadConstraints.minDomains` | Beta since v1.27; valid only with `DoNotSchedule`. | Validate (`SCHED-AFF-009`). |
| `topologySpreadConstraints.matchLabelKeys` | Beta since v1.27. | Validate field presence; `info` if target version < 1.27. |
| `topologySpreadConstraints.nodeAffinityPolicy` | Beta since v1.26. | Validate field presence; `info` if target version < 1.26. |
| `topologySpreadConstraints.nodeTaintsPolicy` | Beta since v1.26. | Validate field presence; `info` if target version < 1.26. |
| `PriorityClass.preemptionPolicy` | Stable since v1.24. | Validate enum. |
| `PDB.unhealthyPodEvictionPolicy` | Beta since v1.26. | Validate enum; `info` if target version < 1.26. |
| `HPA.autoscaling/v2` | Stable since v1.23. | Select the v2 schema for `apiVersion: autoscaling/v2`. |
| `ephemeral-storage` | Stable since v1.25 (local storage capacity isolation GA). | Validate quantities. |

**K8S-SCHED-REQ-017:** Version-specific field findings MUST cite the target Kubernetes version and the version in which the field became stable or beta. The lifecycle engine (`SARCH-REQ-040` through `SARCH-REQ-042`) provides the metadata.

---

## 14. Consolidated finding register

### 14.1 Deterministic findings

| Finding ID | Family | Severity |
|---|---|---|
| SCHED-QTY-001 | CPU fraction below 1m | `error` |
| SCHED-QTY-002 | Invalid CPU suffix | `error` |
| SCHED-QTY-003 | Negative CPU | `error` |
| SCHED-QTY-004 | Invalid memory suffix | `error` |
| SCHED-QTY-005 | Negative memory | `error` |
| SCHED-QTY-006 | Invalid ephemeral-storage suffix | `error` |
| SCHED-QTY-008 | Fractional extended resource | `error` |
| SCHED-QOS-002 | Request exceeds limit | `error` |
| SCHED-NODE-001 | Invalid node selector key | `error` |
| SCHED-AFF-001 | Empty required node affinity term | `error` |
| SCHED-AFF-002 | Preferred weight out of range | `error` |
| SCHED-AFF-003 | Invalid node affinity operator | `error` |
| SCHED-AFF-004 | Empty `topologyKey` | `error` |
| SCHED-AFF-007 | `maxSkew` ≤ 0 | `error` |
| SCHED-AFF-008 | Invalid `whenUnsatisfiable` | `error` |
| SCHED-AFF-009 | `minDomains` with `ScheduleAnyway` | `error` |
| SCHED-AFF-010 | Duplicate topology constraint | `error` |
| SCHED-TAINT-001 | Invalid toleration effect | `error` |
| SCHED-TAINT-002 | `Exists` with value | `error` |
| SCHED-PRIO-001 | PriorityClass value out of range | `error` |
| SCHED-PRIO-002 | Multiple global defaults | `error` |
| SCHED-PDB-001 | Both PDB fields set | `error` |
| SCHED-PDB-002 | Neither PDB field set | `error` |
| SCHED-HPA-001 | `minReplicas` > `maxReplicas` | `error` |
| SCHED-HPA-002 | `minReplicas` < 1 | `error` |
| SCHED-HPA-003 | Invalid metric type | `error` |
| SCHED-HPA-005 | Missing `averageValue` | `error` |
| SCHED-QUOTA-001 | Quota requests > limits (CPU) | `error` |
| SCHED-QUOTA-002 | Quota requests > limits (memory) | `error` |
| SCHED-QUOTA-004 | Invalid `count/` key | `error` |
| SCHED-LIMIT-001 | LimitRange max < min | `error` |
| SCHED-LIMIT-002 | LimitRange default limit < defaultRequest | `error` |
| SCHED-LIMIT-003 | Pod request > LimitRange max | `error` |
| SCHED-LIMIT-004 | Pod request < LimitRange min | `error` |
| SCHED-STOR-002 | `ReadWriteOncePod` with multiple replicas | `error` |
| SCHED-ARCH-003 | Windows selector with Linux-only feature | `error` |

### 14.2 Heuristic and informational findings

| Finding ID | Family | Severity |
|---|---|---|
| SCHED-QOS-001 | Limit without request | `warning` |
| SCHED-QOS-003 | No requests or limits | `info` |
| SCHED-QOS-004 | BestEffort with replicas and no PDB | `info` |
| SCHED-QOS-005 | Guaranteed single replica no PDB | `info` |
| SCHED-QOS-006 | Both pod-level and container-level resources | `info` |
| SCHED-QTY-007 | `emptyDir.sizeLimit` > ephemeral-storage limit | `warning` |
| SCHED-QTY-009 | Hugepage size not supported (heuristic) | `info` |
| SCHED-NODE-002 | nodeSelector conflicts with nodeAffinity | `warning` |
| SCHED-AFF-005 | Pod affinity namespace not in set | `info` |
| SCHED-AFF-006 | Anti-affinity with 1 replica | `info` |
| SCHED-AFF-011 | Topology constraint may be unsatisfiable | `info` |
| SCHED-TAINT-003 | `Equal` with empty value | `warning` |
| SCHED-TAINT-004 | DaemonSet without control-plane toleration | `info` |
| SCHED-PRIO-003 | PriorityClass not in input set | `info` |
| SCHED-PRIO-004 | System priority outside `kube-system` | `warning` |
| SCHED-PDB-003 | `maxUnavailable: 0` | `warning` |
| SCHED-PDB-004 | `minAvailable: 100%` | `warning` |
| SCHED-PDB-005 | Percentage does not resolve to integer | `info` |
| SCHED-PDB-006 | PDB selector matches no workload | `info` |
| SCHED-PDB-007 | Multi-replica workload without PDB | `info` |
| SCHED-PDB-008 | PDB spans multiple namespaces | `info` |
| SCHED-HPA-004 | Memory utilization without requests | `warning` |
| SCHED-HPA-006 | `scaleTargetRef` not in input set | `info` |
| SCHED-QUOTA-003 | Pod omits quota-enforced requests | `warning` |
| SCHED-LIMIT-005 | Pod omits requests in LimitRange namespace | `info` |
| SCHED-LIMIT-006 | Conflicting LimitRange defaults | `warning` |
| SCHED-STOR-001 | `Immediate` binding with topology constraints | `info` |
| SCHED-STOR-003 | `ReadWriteOnce` with multiple replicas | `info` |
| SCHED-ARCH-001 | Non-standard architecture value | `warning` |
| SCHED-ARCH-002 | Non-standard OS value | `warning` |

---

## 15. Valid and invalid configuration examples

### 15.1 Valid: Guaranteed QoS with topology spread

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 4
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app: web
      containers:
      - name: app
        image: nginx
        resources:
          requests:
            cpu: 500m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 256Mi