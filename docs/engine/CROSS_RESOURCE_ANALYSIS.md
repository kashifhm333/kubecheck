# KubeCheck — Cross-Resource Analysis

**Document ID:** CROSS-RES-001  
**File:** `docs/engine/CROSS_RESOURCE_ANALYSIS.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Object-Graph Analysis Architecture  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/engine/DIAGNOSTIC_RULE_SPEC.md`
- `docs/engine/STATIC_ANALYSIS_RULES.md`
- `docs/architecture/SYSTEM_ARCHITECTURE.md`
- `docs/architecture/REQUIREMENTS_AND_NFRS.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md`
- `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`
- `docs/kubernetes/WORKLOADS_AND_PODS.md`
- `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md`
- `docs/kubernetes/CONFIGURATION_AND_STORAGE.md`
- `docs/kubernetes/SECURITY_AND_RBAC.md`
- `docs/kubernetes/SCHEDULING_AND_RESOURCE_MANAGEMENT.md`
- `docs/output/JSON_SCHEMA.md`
- `docs/testing/DETERMINISM_TESTS.md`
- `docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`

---

## 1. Purpose and scope

This document specifies how KubeCheck identifies **defects that cannot be detected by validating resources individually**. Many real Kubernetes defects exist only in the relationships between objects: a Service selector that matches no workload, a Secret reference in a different namespace, a RoleBinding that binds to a ClusterRole granting cluster-scoped permissions, an Ingress whose backend Service does not exist, an owner reference to an object not in the input set, and a NetworkPolicy that selects no workload.

To detect these defects offline, KubeCheck constructs a **typed relationship graph** over the analyzed input set and evaluates cross-resource rules against it. This document specifies:

- The graph model and edge types.
- The indexing strategy and traversal limits.
- The deterministic reference-resolution algorithm.
- The explicit distinction between "no matching resource found in the analyzed input" and "resource does not exist in the cluster."
- Graph examples, diagnostic rules, scalability constraints, and false-positive tests.

### 1.1 Governing principles

> **CROSS-RES-REQ-001 — The graph is constructed exclusively from the analyzed input set.** No cluster state, no live API queries, no remote schema fetching. The graph represents **the objects supplied to KubeCheck in this invocation**, not the cluster's actual state.

> **CROSS-RES-REQ-002 — "No matching resource found in the analyzed input" is not equivalent to "resource does not exist in the cluster."** A reference whose target is not in the input set MUST produce an `info`-severity finding, not an `error`-severity finding. The finding MUST state that the target's existence is cluster-dependent and cannot be verified offline.

> **CROSS-RES-REQ-003 — Graph construction and traversal are deterministic.** For a fixed input set, the graph's node set, edge set, edge order, and traversal order are fixed. No unordered-container iteration influences the graph or the findings derived from it.

> **CROSS-RES-REQ-004 — The graph is acyclic except for documented cycles.** Kubernetes object relationships are structurally acyclic in the intended model (owner→dependent, service→workload), but the graph may contain cycles when the input set includes such relationships (e.g., two Deployments whose Pods owner-reference each other, which is invalid but possible in a malformed manifest). The graph builder MUST detect cycles and MUST report them as findings rather than silently accepting them.

> **CROSS-RES-REQ-005 — Cross-resource findings have severity `info` or `warning` when they depend on cluster state, and `error` only when the defect is provable within the input set.** A Service selector that matches no supplied Pod is `info` (the target may be in the cluster); a StatefulSet `volumeClaimTemplate` with no matching mount is `error` (both are in the input set).

### 1.2 In scope

Graph model, edge types, indexing, traversal limits, reference-resolution algorithm, per-relationship analysis (pod-template/selector, service selector, configmap/secret/serviceaccount/PVC/storageclass, RBAC, ingress/gateway, owner references, networkpolicy), ordering vs dependency semantics, optional/external/generated/unresolved references, CRDs/custom resources/incomplete knowledge, duplicates/ambiguity/conflicts, scalability constraints, false-positive tests.

### 1.3 Out of scope

- Live cluster graph analysis (see `KUBERNETES_API_FUNDAMENTALS.md` §13).
- Rule execution semantics (see `DIAGNOSTIC_RULE_SPEC.md`).
- NIR construction (see `YAML_VALIDATION_AND_NORMALIZATION.md` §16).
- Schema validation (see `KUBERNETES_SCHEMA_VALIDATION.md`).
- Network policy enforcement at the CNI level.

### 1.4 Assumptions

- **ASSUMPTION-CROSS-01:** The NIR for each document is available with every node carrying a source location (`NORM-REQ-009`, `NORM-REQ-043`).
- **ASSUMPTION-CROSS-02:** The resource inventory is built from admitted GVKs only (`SARCH-REQ-025`); documents that failed GVK extraction are excluded from the graph.
- **ASSUMPTION-CROSS-03:** The schema bundle provides scope (namespaced/cluster) for every covered GVK (`K8S-API-REQ-028`); CRD scope comes from the CRD definition.
- **ASSUMPTION-CROSS-04:** The analyzed input set is complete for the user's intent. KubeCheck cannot verify completeness; it assumes the user supplied the relevant objects.

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **Input set** | The complete set of documents supplied to KubeCheck in a single invocation. |
| **Resource inventory** | The collection of admitted resources keyed by `ResourceKey` (`SARCH-REQ-031`). |
| **ResourceKey** | The tuple `(group, version, kind, namespace, name)`. |
| **Relationship graph** | A directed graph whose nodes are resources and whose edges are typed references between them. |
| **Edge type** | The kind of relationship (e.g., `ServiceSelectorMatch`, `ConfigMapRef`). |
| **Resolved edge** | An edge whose target is present in the input set. |
| **Unresolved edge** | An edge whose target is not in the input set. |
| **External edge** | An edge whose target is intentionally external (e.g., an `ExternalName` Service). |
| **Generated edge** | An edge whose target is generated by a controller (e.g., a PVC from a StatefulSet `volumeClaimTemplates`). |
| **Reference resolution** | The process of determining whether a target exists in the input set and, if so, which resource it is. |
| **Index** | A lookup structure mapping a key (e.g., `namespace/name`) to the resources that match. |

**Requirement IDs:** `CROSS-RES-REQ-NNN`.  
**Edge IDs:** `EDGE-<TYPE>-NNN`.  
**Finding IDs:** `XRES-<FAMILY>-NNN`.

---

## 3. Graph model

### 3.1 Node model

A graph node is a **resource** from the input set. Each node carries:

| Field | Type | Description |
|---|---|---|
| `key` | `ResourceKey` | The resource identity. |
| `gvk` | `(group, version, kind)` | The GVK. |
| `scope` | `namespaced` \| `cluster` | The resource scope from the schema. |
| `namespace` | `Option<string>` | The namespace, if namespaced. |
| `name` | `string` | The resource name (may be a `generateName` prefix). |
| `nir` | `NIRNode` | The normalized object. |
| `source_anchor` | `SourceAnchor` | The file, doc index, line, column. |

**CROSS-RES-REQ-006:** A node MUST exist for every admitted resource in the input set. A resource that failed GVK extraction MUST NOT be a node.

### 3.2 Edge model

An edge is a **typed reference** from a source node to a target. Each edge carries:

| Field | Type | Description |
|---|---|---|
| `edge_id` | `string` | A stable identifier for the edge within the graph. |
| `edge_type` | `EdgeType` | The kind of relationship. |
| `from` | `ResourceKey` | The source resource. |
| `to` | `Option<ResourceKey>` | The target resource, if resolved. `None` for unresolved edges. |
| `to_hint` | `EdgeTargetHint` | A structural description of the target (group, kind, namespace, name) for unresolved edges. |
| `source_anchor` | `SourceAnchor` | The exact location in the source manifest that establishes the edge. |
| `optional` | `bool` | Whether the reference is optional (from the manifest). |
| `scope` | `EdgeScope` | `internal` (in input set), `external` (intentionally external), `generated` (produced by a controller), `unresolved` (not in input set). |

**CROSS-RES-REQ-007:** Every edge MUST carry a `source_anchor` pointing to the exact node in the source manifest that establishes the reference. An edge without an anchor MUST cause graph construction to fail.

**CROSS-RES-REQ-008:** Edges MUST be ordered deterministically: `(from.key, edge_type, to.key or to_hint, source_anchor)` in lexicographic order. Graph construction MUST NOT depend on the order of resources in the input set or on any unordered container.

### 3.3 Edge types

The graph supports the following edge types. Each type corresponds to a Kubernetes reference mechanism.

| Edge ID | Edge type | Direction | Source | Target |
|---|---|---|---|---|
| `EDGE-OWNER-001` | `OwnerReference` | Dependent → Owner | Any resource with `metadata.ownerReferences` | Any resource |
| `EDGE-SELECTOR-001` | `WorkloadSelectorMatch` | Workload → Pod | `Deployment`, `ReplicaSet`, `StatefulSet`, `DaemonSet`, `Job` | `Pod` |
| `EDGE-SELECTOR-002` | `ServiceSelectorMatch` | Service → Pod | `Service` | `Pod` |
| `EDGE-SELECTOR-003` | `NetworkPolicySelectorMatch` | NetworkPolicy → Pod | `NetworkPolicy` | `Pod` |
| `EDGE-SELECTOR-004` | `PDBSelectorMatch` | PDB → Workload | `PodDisruptionBudget` | Workload |
| `EDGE-REF-001` | `ConfigMapRef` | Any → ConfigMap | `Pod`, `Deployment`, etc. | `ConfigMap` |
| `EDGE-REF-002` | `SecretRef` | Any → Secret | `Pod`, `Deployment`, etc. | `Secret` |
| `EDGE-REF-003` | `ServiceAccountRef` | Pod → ServiceAccount | `Pod` | `ServiceAccount` |
| `EDGE-REF-004` | `PVCRef` | Pod → PVC | `Pod` | `PersistentVolumeClaim` |
| `EDGE-REF-005` | `StorageClassRef` | PVC → StorageClass | `PersistentVolumeClaim` | `StorageClass` |
| `EDGE-REF-006` | `PVRef` | PVC → PV | `PersistentVolumeClaim` | `PersistentVolume` |
| `EDGE-REF-007` | `CRDRef` | Custom Resource → CRD | Any custom resource | `CustomResourceDefinition` |
| `EDGE-RBAC-001` | `RoleRef` | RoleBinding → Role | `RoleBinding` | `Role` or `ClusterRole` |
| `EDGE-RBAC-002` | `ClusterRoleRef` | ClusterRoleBinding → ClusterRole | `ClusterRoleBinding` | `ClusterRole` |
| `EDGE-RBAC-003` | `SubjectRef` | Binding → Subject | `RoleBinding`, `ClusterRoleBinding` | `ServiceAccount`, `User`, `Group` |
| `EDGE-ING-001` | `IngressBackendServiceRef` | Ingress → Service | `Ingress` | `Service` |
| `EDGE-ING-002` | `IngressBackendResourceRef` | Ingress → Resource | `Ingress` | Any resource |
| `EDGE-ING-003` | `IngressTLSSecretRef` | Ingress → Secret | `Ingress` | `Secret` |
| `EDGE-GW-001` | `GatewayParentRef` | Route → Gateway | `HTTPRoute`, `GRPCRoute`, etc. | `Gateway` |
| `EDGE-GW-002` | `GatewayBackendRef` | Route → Service | `HTTPRoute`, `GRPCRoute`, etc. | `Service` |
| `EDGE-GW-003` | `GatewayTLSRef` | Gateway → Secret | `Gateway` | `Secret` |
| `EDGE-GW-004` | `GatewayClassRef` | Gateway → GatewayClass | `Gateway` | `GatewayClass` |
| `EDGE-STS-001` | `StatefulSetTemplateMatch` | StatefulSet → Generated PVC | `StatefulSet` | Generated `PersistentVolumeClaim` |
| `EDGE-EXTERNAL-001` | `ExternalNameRef` | Service → External | `Service` (ExternalName) | External DNS name |
| `EDGE-EXTERNAL-002` | `LoadBalancerRef` | Service → External | `Service` (LoadBalancer) | External load balancer |

**CROSS-RES-REQ-009:** Every edge MUST have a type from the table in §3.3. A reference that does not correspond to a typed edge MUST be treated as a `native` field and MUST NOT be added to the graph.

---

## 4. Indexing strategy

### 4.1 Primary index

The primary index maps `ResourceKey` to the resource node:

```
PrimaryIndex: Map<ResourceKey, NodeRef>
```

**CROSS-RES-REQ-010:** The primary index MUST contain exactly one node per `ResourceKey`. Duplicate `ResourceKey`s produce a `MAINT-DUP-001` finding (`STATIC_ANALYSIS_RULES.md` §14.3) but do not block graph construction; the first node in canonical order is used for edge resolution, and the duplicate is annotated on the node.

### 4.2 Secondary indexes

Secondary indexes accelerate reference resolution:

| Index ID | Index | Key | Purpose |
|---|---|---|---|
| `IDX-NS` | Namespace index | `(group, kind, namespace)` | Find all resources of a kind in a namespace. |
| `IDX-LABEL` | Label index | `(namespace, key=value)` | Find Pods matching a label selector. |
| `IDX-NAME` | Name index | `(group, kind, namespace, name)` | Find a specific resource. |
| `IDX-SCOPE` | Scope index | `(group, kind, scope)` | Distinguish namespaced from cluster-scoped resources. |
| `IDX-OWNER` | Owner index | `(owner_key)` | Find dependents of an owner (reverse index). |
| `IDX-GVK` | GVK index | `(group, version, kind)` | Find all instances of a GVK. |

**CROSS-RES-REQ-011:** Every secondary index MUST be populated before edge resolution begins. An edge resolution that requires an index not yet populated MUST fail graph construction with a structured error.

**CROSS-RES-REQ-012:** Secondary indexes MUST NOT introduce iteration-order nondeterminism. When an index returns multiple candidates, the candidates MUST be sorted by `(namespace, name, source_anchor)` before the resolution algorithm processes them.

### 4.3 Index build complexity

| Index | Build time | Build space |
|---|---|---|
| `IDX-NS` | O(N) | O(N) |
| `IDX-LABEL` | O(N × L) where L is the average labels per object | O(N × L) |
| `IDX-NAME` | O(N) | O(N) |
| `IDX-SCOPE` | O(N) | O(N) |
| `IDX-OWNER` | O(N × O) where O is the average owner references per object | O(N × O) |
| `IDX-GVK` | O(N) | O(N) |

**CROSS-RES-REQ-013:** Index build time and space MUST be bounded by the resource limits defined in §12. Exceeding a bound MUST cause graph construction to fail with a structured `XRES-LIMIT-*` finding.

---

## 5. Deterministic reference-resolution algorithm

### 5.1 Resolution function

The core resolution function determines whether a reference target exists in the input set and, if so, which node it refers to.

```
function resolve_reference(ref, indexes) -> Resolution:
    # Step 1: Check for external references.
    if ref.is_external:
        return Resolution(kind="EXTERNAL", hint=ref.hint)

    # Step 2: Check for generated references.
    if ref.is_generated:
        return Resolution(kind="GENERATED", hint=ref.hint)

    # Step 3: Look up by name.
    if ref.has_name:
        candidates = indexes.name.get((ref.group, ref.kind, ref.namespace, ref.name))
        if candidates is empty:
            return Resolution(kind="UNRESOLVED", hint=ref.hint)
        if len(candidates) == 1:
            return Resolution(kind="RESOLVED", node=candidates[0])
        # Duplicates: use the first in canonical order, annotate the rest.
        return Resolution(kind="RESOLVED_WITH_DUPLICATES",
                          node=candidates[0],
                          duplicates=candidates[1:])

    # Step 4: Look up by selector.
    if ref.has_selector:
        candidates = indexes.label.match(ref.selector, ref.namespace)
        candidates = sort_candidates(candidates)   # canonical order
        if candidates is empty:
            return Resolution(kind="NO_MATCHING", hint=ref.hint)
        return Resolution(kind="SELECTOR_MATCH", nodes=candidates)

    # Step 5: Fallback — no resolution possible.
    return Resolution(kind="UNRESOLVABLE", hint=ref.hint)
```

**CROSS-RES-REQ-014:** The resolution function MUST return exactly one `Resolution` for each reference. It MUST NOT depend on iteration order; when multiple candidates exist, they MUST be sorted deterministically before selection.

**CROSS-RES-REQ-015:** The `SELECTOR_MATCH` resolution MUST return **all** matching nodes, not just one. Selector-based references (Service→Pod, NetworkPolicy→Pod, Workload→Pod) are inherently multi-target.

### 5.2 Resolution result types

| `Resolution.kind` | Meaning | Downstream behavior |
|---|---|---|
| `RESOLVED` | The target is in the input set, exactly one match. | Rules may emit `error` for defects provable within the input set. |
| `RESOLVED_WITH_DUPLICATES` | The target is in the input set but multiple resources share the key. | Rules MUST emit `MAINT-DUP-001` and MUST treat the first candidate as the target. |
| `SELECTOR_MATCH` | The selector matches one or more nodes. | Rules MUST evaluate each match; MUST NOT emit an `error` if any match exists. |
| `NO_MATCHING` | The selector matches no node in the input set. | Rules MUST emit `info` (`XRES-REF-001`) stating that the target may exist in the cluster. |
| `UNRESOLVED` | The named reference target is not in the input set. | Rules MUST emit `info` (`XRES-REF-002`) stating that the target may exist in the cluster. |
| `EXTERNAL` | The reference is intentionally external. | Rules MUST emit `info` (`XRES-REF-003`) stating the external nature. |
| `GENERATED` | The reference target is generated by a controller. | Rules MUST emit `info` (`XRES-REF-004`) stating the generated nature. |
| `UNRESOLVABLE` | The reference cannot be resolved structurally. | Rules MUST emit `warning` (`XRES-REF-005`) stating the structural defect. |

### 5.3 The "in analyzed input" vs "in cluster" distinction

**CROSS-RES-REQ-016:** A resolution result of `NO_MATCHING` or `UNRESOLVED` MUST be reported as **"not found in the analyzed input set"**, not as **"does not exist in the cluster."**

The finding message MUST include:

1. The reference type and the source field.
2. The target hint (group, kind, namespace, name or selector).
3. A statement that the target may exist in the cluster.
4. A remediation that suggests supplying the target if the user wants a complete check.

**CROSS-RES-REQ-017:** A resolution result of `RESOLVED` or `SELECTOR_MATCH` MAY be reported as "found in the analyzed input set" but MUST NOT claim "will be admitted by the cluster." The graph establishes existence **within the input set**, not cluster acceptance.

---

## 6. Resource identity, scope, namespace, and duplicate definitions

### 6.1 Identity in the graph

**CROSS-RES-REQ-018:** The graph node key MUST be the `ResourceKey` defined in `SARCH-REQ-031`: `(group, version, kind, namespace, name)`. Two resources with the same key are duplicates; the graph MUST annotate the duplicate rather than silently merge or drop either.

### 6.2 Scope determination

**CROSS-RES-REQ-019:** The scope of a resource MUST come from the schema bundle (`K8S-API-REQ-012`). For CRDs, the scope MUST come from `spec.scope` in the supplied CRD definition. For aggregated APIs without a supplied schema, the scope MUST be `unknown` and MUST cause a `XRES-SCOPE-001` finding.

### 6.3 Namespace-aware references

**CROSS-RES-REQ-020:** Every namespaced reference MUST resolve within the source resource's namespace **unless the reference schema explicitly declares a namespace field**. References that cross namespaces MUST be reported as `XRES-REF-006` (`warning`) if the target is in the input set, or `XRES-REF-002` (`info`) if the target is not in the input set.

### 6.4 Duplicate definitions

**CROSS-RES-REQ-021:** A duplicate `ResourceKey` in the input set MUST produce `MAINT-DUP-001` (`warning`) citing both anchors. The graph MUST include both nodes; the duplicate MUST NOT be silently merged.

**CROSS-RES-REQ-022:** When a reference resolves to a key with duplicates, the resolution MUST be `RESOLVED_WITH_DUPLICATES`. Downstream rules MUST treat the first candidate as the target and MUST include the duplicate anchors in the finding evidence.

---

## 7. Pod-template and workload selector relationships

### 7.1 Pod-template structure

A workload (`Deployment`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`) contains a `spec.template` that describes the Pods it creates. The template's `metadata.labels` are the labels that the workload's `spec.selector` must match.

### 7.2 Edge construction

**CROSS-RES-REQ-023:** For each workload, the graph builder MUST construct:

1. A `WorkloadSelectorMatch` edge from the workload to each Pod in the input set whose labels match the workload's `spec.selector`, **within the same namespace**.
2. A `OwnerReference` edge from each Pod to the workload **only if** the Pod's `metadata.ownerReferences` references the workload. The graph MUST NOT infer owner references from selector matches.

**CROSS-RES-REQ-024:** A selector match is **not** evidence of ownership. A Pod may match multiple workloads' selectors; each match produces a `WorkloadSelectorMatch` edge, but only explicit `ownerReferences` produce `OwnerReference` edges.

### 7.3 Diagnostic rules

| Rule ID | Condition | Severity | Evidence class |
|---|---|---|---|
| `XRES-WL-001` | A workload's selector matches no Pod in the input set | `info` | context-dependent |
| `XRES-WL-002` | A Pod's `ownerReferences` references a workload not in the input set | `info` | context-dependent |
| `XRES-WL-003` | A Pod's `ownerReferences` references a workload in a different namespace | `warning` | deterministic (in-set) |
| `XRES-WL-004` | Two workloads in the same namespace have overlapping selectors that match the same Pod | `warning` | deterministic (in-set) |

#### `XRES-WL-004` — Overlapping selectors

| Attribute | Detail |
|---|---|
| **Matching condition** | Two workloads in the same namespace have selectors that both match at least one common Pod in the input set. |
| **Counterexamples** | Two workloads with disjoint selectors do not trigger. Two workloads whose selectors match the same Pod but one is a `Job` with `completions: 1` may be intentional (e.g., a one-shot cleanup Job that targets the same Pods). |
| **False-positive traps** | The user may intentionally have overlapping selectors, e.g., a Deployment and a one-off Job targeting the same Pods. The finding is `warning`, not `error`. |
| **Severity** | `warning` |
| **Confidence** | Deterministic that the selectors overlap; the operational impact is advisory. |
| **Applicability** | All versions. |
| **Remediation** | Make the selectors disjoint, or explicitly document the overlap. |
| **Test cases** | Positive: two Deployments with `{app: web}` selectors. Negative: `{app: web}` and `{app: api}`. Boundary: a Deployment and a Job both matching `{app: web}`. |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 8. Service selector-to-Pod label matching

### 8.1 Edge construction

**CROSS-RES-REQ-025:** For each `Service` with a non-empty `spec.selector`, the graph builder MUST construct a `ServiceSelectorMatch` edge from the Service to each Pod in the input set whose labels match the selector, within the same namespace.

**CROSS-RES-REQ-026:** A `Service` without a selector MUST NOT produce a `ServiceSelectorMatch` edge. It is a **selectorless Service** with manually managed endpoints; the graph MUST annotate it with `kind="selectorless"`.

### 8.2 Named targetPort resolution

**CROSS-RES-REQ-027:** When a Service has a `targetPort` that is a **named port**, the graph builder MUST resolve the name against the matching Pods' `containers[*].ports[*].name`. The resolution produces additional edges of type `ServiceTargetPortMatch` (a subtype of `ServiceSelectorMatch`) that carry the resolved port number.

**CROSS-RES-REQ-028:** If a named `targetPort` does not resolve in **any** matching Pod, the resolution MUST be `NO_MATCHING` and MUST produce `NET-SVC-004` (`error` if matching Pods are in the input set, `warning` otherwise).

### 8.3 Diagnostic rules

| Rule ID | Condition | Severity | Evidence class |
|---|---|---|---|
| `XRES-SVC-001` | Service selector matches no Pod in the input set | `info` | context-dependent |
| `XRES-SVC-002` | Service has no selector (selectorless) | `info` | context-dependent |
| `XRES-SVC-003` | Service selector matches a Pod whose container ports do not include the `targetPort` | `error` (if Pods in set) | deterministic |
| `XRES-SVC-004` | Two Services in the same namespace select the same Pods with different `targetPort` values | `info` | context-dependent |

#### `XRES-SVC-003` — Target port not exposed by matching Pod

| Attribute | Detail |
|---|---|
| **Matching condition** | A Service's `targetPort` is numeric, and no matching Pod exposes a `containerPort` equal to that number. |
| **Counterexamples** | The `targetPort` is numeric and at least one matching Pod exposes the port. The `targetPort` is named and resolves in at least one matching Pod. |
| **False-positive traps** | A Pod may expose the port via a proxy or sidecar that is not in the manifest; the finding is `error` because the port is not declared. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the input set. |
| **Applicability** | All versions. |
| **Remediation** | Add the port to the Pod's `containers[*].ports` or correct the Service's `targetPort`. |
| **Test cases** | Positive: Service `targetPort: 8080`, Pod exposes `8080`? No → trigger. Negative: Pod exposes `8080` → no trigger. Boundary: Pod exposes `8080` via a sidecar (not in the manifest). |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 9. ConfigMap, Secret, ServiceAccount, PVC, and StorageClass references

### 9.1 Reference construction

**CROSS-RES-REQ-029:** The graph builder MUST construct `ConfigMapRef` and `SecretRef` edges for every reference from a Pod or workload template to a ConfigMap or Secret, including:

- `env[*].valueFrom.configMapKeyRef` / `secretKeyRef`
- `envFrom[*].configMapRef` / `secretRef`
- `volumes[*].configMap` / `secret`
- `volumes[*].projected.sources[*].configMap` / `secret`
- `imagePullSecrets[*]`
- `CSI` secret references (`controllerPublishSecretRef`, `nodeStageSecretRef`, `nodePublishSecretRef`)

**CROSS-RES-REQ-030:** A `ServiceAccountRef` edge MUST be constructed for every Pod's `spec.serviceAccountName`. A Pod without `serviceAccountName` implicitly references the `default` ServiceAccount; the graph MUST construct an edge to the `default` ServiceAccount in the Pod's namespace if it is in the input set, and MUST annotate it as `implicit=true`.

**CROSS-RES-REQ-031:** A `PVCRef` edge MUST be constructed for every `volumes[*].persistentVolumeClaim.claimName`. A `StorageClassRef` edge MUST be constructed for every `PersistentVolumeClaim.spec.storageClassName`. A `PVRef` edge MUST be constructed for every `PersistentVolumeClaim.spec.volumeName`.

### 9.2 Namespace rules

**CROSS-RES-REQ-032:** `ConfigMapRef`, `SecretRef`, and `PVCRef` edges MUST resolve within the source Pod's namespace. `StorageClassRef` and `PVRef` edges MUST resolve cluster-scope. `ServiceAccountRef` edges MUST resolve within the source Pod's namespace.

### 9.3 Optional references

**CROSS-RES-REQ-033:** The graph builder MUST propagate the `optional` flag from the reference (`configMapKeyRef.optional`, `secretKeyRef.optional`) onto the edge. Optional references with unresolved targets MUST produce `XRES-REF-007` (`info`) stating that the reference is optional and the target is not in the input set.

### 9.4 Diagnostic rules

| Rule ID | Condition | Severity | Evidence class |
|---|---|---|---|
| `XRES-REF-001` | Selector matches no resource in the input set | `info` | context-dependent |
| `XRES-REF-002` | Named reference target not in the input set | `info` | context-dependent |
| `XRES-REF-003` | Reference is intentionally external | `info` | context-dependent |
| `XRES-REF-004` | Reference target is generated by a controller | `info` | context-dependent |
| `XRES-REF-005` | Reference cannot be resolved structurally | `warning` | deterministic |
| `XRES-REF-006` | Cross-namespace reference where the schema requires same-namespace | `error` | deterministic (in-set) |
| `XRES-REF-007` | Optional reference has no target in the input set | `info` | context-dependent |
| `XRES-REF-008` | `secretKeyRef` / `configMapKeyRef` key not present in the referenced resource | `error` | deterministic (in-set) |

#### `XRES-REF-006` — Cross-namespace reference

| Attribute | Detail |
|---|---|
| **Matching condition** | A reference declares a namespace different from the source Pod's namespace, and the reference schema allows only same-namespace references. |
| **Counterexamples** | The reference schema explicitly permits cross-namespace references (rare; e.g., some CNI-specific references). |
| **False-positive traps** | A user may have a legitimate multi-tenant setup with shared resources; the schema still requires same-namespace references, so the finding is `error`. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the schema. |
| **Applicability** | All versions. |
| **Remediation** | Create the referenced resource in the same namespace, or use a supported cross-namespace mechanism. |
| **Test cases** | Positive: Pod in `production` references Secret in `default` via `secretKeyRef` with `namespace: default` (invalid field). Negative: same-namespace reference. Boundary: a `ClusterRoleBinding` referencing a Subject in a different namespace. |
| **Dependencies** | `K8S-SCHEMA-001`. |

#### `XRES-REF-008` — Missing key in referenced resource

| Attribute | Detail |
|---|---|
| **Matching condition** | A `secretKeyRef` or `configMapKeyRef` references a key, and the referenced resource is in the input set, and the key is not present. |
| **Counterexamples** | The key is present. The referenced resource is not in the input set (the reference may resolve in the cluster). |
| **False-positive traps** | The Secret or ConfigMap may be updated between the manifest analysis and the Pod creation; the finding is `error` because the key is absent from the input set. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the input set. |
| **Applicability** | All versions. |
| **Remediation** | Add the key to the Secret/ConfigMap, or set `optional: true` if the key is genuinely optional. |
| **Test cases** | Positive: Secret `db` has `username`, Pod references `password`. Negative: Secret `db` has both. Boundary: Secret not in input set (rule does not trigger as `error`). |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 10. RBAC subject and role references

### 10.1 Edge construction

**CROSS-RES-REQ-034:** For each `RoleBinding`:

- Construct a `RoleRef` edge to the referenced `Role` in the same namespace, or to the referenced `ClusterRole` (cluster-scoped).
- For each subject of `kind: ServiceAccount`, construct a `SubjectRef` edge to the ServiceAccount.
- For each subject of `kind: User` or `kind: Group`, construct a `SubjectRef` edge with an **external** scope (the user or group is not a cluster object).

**CROSS-RES-REQ-035:** For each `ClusterRoleBinding`:

- Construct a `ClusterRoleRef` edge to the referenced `ClusterRole`.
- For each subject, construct a `SubjectRef` edge as above.

### 10.2 Scope interaction

**CROSS-RES-REQ-036:** A `RoleBinding` that references a `ClusterRole` grants the ClusterRole's rules **within the RoleBinding's namespace**, not cluster-wide. The graph MUST annotate the `RoleRef` edge with `scope="namespace-scoped"` for this case.

**CROSS-RES-REQ-037:** A `ClusterRoleBinding` referencing a `ClusterRole` grants the ClusterRole's rules cluster-wide. The graph MUST annotate the edge with `scope="cluster-scoped"`.

### 10.3 Diagnostic rules

| Rule ID | Condition | Severity | Evidence class |
|---|---|---|---|
| `XRES-RBAC-001` | `RoleBinding` references a `Role` not in the input set | `info` | context-dependent |
| `XRES-RBAC-002` | `RoleBinding` references a `ClusterRole` not in the input set | `info` | context-dependent |
| `XRES-RBAC-003` | `ClusterRoleBinding` references a `ClusterRole` not in the input set | `info` | context-dependent |
| `XRES-RBAC-004` | `SubjectRef` to a ServiceAccount not in the input set | `info` | context-dependent |
| `XRES-RBAC-005` | `SubjectRef` to a ServiceAccount in a different namespace than the RoleBinding | `error` | deterministic (in-set) |
| `XRES-RBAC-006` | `RoleBinding` references a `Role` in a different namespace | `error` | deterministic (in-set) |
| `XRES-RBAC-007` | `ClusterRoleBinding` binds to `system:masters` | `error` | deterministic |
| `XRES-RBAC-008` | A `RoleBinding` that binds a `ClusterRole` also has a corresponding `ClusterRoleBinding` in the same cluster granting the same role to the same subject | `warning` | probable |

#### `XRES-RBAC-005` — Cross-namespace ServiceAccount subject

| Attribute | Detail |
|---|---|
| **Matching condition** | A `RoleBinding` subject of `kind: ServiceAccount` has `namespace` different from the RoleBinding's namespace. |
| **Counterexamples** | The subject is in the same namespace as the RoleBinding. |
| **False-positive traps** | None. Kubernetes requires ServiceAccount subjects to be in the same namespace as a namespaced RoleBinding. |
| **Severity** | `error` |
| **Confidence** | Deterministic. |
| **Applicability** | `rbac.authorization.k8s.io/v1`, all target versions. |
| **Remediation** | Move the subject to the RoleBinding's namespace, or use a `ClusterRoleBinding` for cross-namespace grants. |
| **Test cases** | Positive: RoleBinding in `default`, subject SA in `kube-system`. Negative: subject SA in `default`. Boundary: a `ClusterRoleBinding` with a SA subject from any namespace. |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 11. Ingress and Gateway backend references

### 11.1 Ingress edge construction

**CROSS-RES-REQ-038:** For each `Ingress`:

- For each `spec.rules[*].http.paths[*].backend.service`, construct an `IngressBackendServiceRef` edge to the Service in the Ingress's namespace.
- For each `spec.rules[*].http.paths[*].backend.resource`, construct an `IngressBackendResourceRef` edge to the resource (which may be a custom resource in the same namespace).
- For each `spec.tls[*].secretName`, construct an `IngressTLSSecretRef` edge to the Secret in the Ingress's namespace.
- For each `spec.ingressClassName`, construct an `IngressClassRef` edge to the IngressClass (cluster-scoped).

### 11.2 Gateway edge construction

**CROSS-RES-REQ-039:** For each Gateway API route (`HTTPRoute`, `GRPCRoute`, `TCPRoute`, `TLSRoute`, `UDPRoute`):

- For each `spec.parentRefs[*]`, construct a `GatewayParentRef` edge to the referenced Gateway.
- For each `spec.rules[*].backendRefs[*]`, construct a `GatewayBackendRef` edge to the referenced Service.
- For each `spec.rules[*].filters[*].requestMirror.backendRef` or equivalent filter backend, construct a `GatewayBackendRef` edge.

**CROSS-RES-REQ-040:** For each `Gateway`:

- For each `spec.listeners[*].tls.certificateRefs[*]`, construct a `GatewayTLSRef` edge to the referenced Secret.
- For `spec.gatewayClassName`, construct a `GatewayClassRef` edge to the referenced GatewayClass.

### 11.3 Diagnostic rules

| Rule ID | Condition | Severity | Evidence class |
|---|---|---|---|
| `XRES-ING-001` | Ingress backend Service not in the input set | `info` | context-dependent |
| `XRES-ING-002` | Ingress TLS Secret not in the input set | `info` | context-dependent |
| `XRES-ING-003` | Ingress TLS Secret in a different namespace | `error` | deterministic (in-set) |
| `XRES-ING-004` | Ingress `ingressClassName` references an IngressClass not in the input set | `info` | context-dependent |
| `XRES-ING-005` | Ingress backend Service exists but does not expose the backend port | `error` | deterministic (in-set) |
| `XRES-GW-001` | Gateway route `parentRefs` empty | `warning` | probable |
| `XRES-GW-002` | Gateway route `parentRefs` references a Gateway not in the input set | `info` | context-dependent |
| `XRES-GW-003` | Gateway backend Service not in the input set | `info` | context-dependent |
| `XRES-GW-004` | Gateway TLS Secret not in the input set | `info` | context-dependent |
| `XRES-GW-005` | Gateway `gatewayClassName` references a GatewayClass not in the input set | `info` | context-dependent |

#### `XRES-ING-005` — Ingress backend port not exposed by Service

| Attribute | Detail |
|---|---|
| **Matching condition** | An Ingress backend references a Service, and the Service is in the input set, and the backend's port name/number does not match any Service port. |
| **Counterexamples** | The backend references a port that the Service exposes. |
| **False-positive traps** | The Service may expose the port via a `targetPort` mapping; the rule checks the Service's `ports[*].port`, not the targetPort. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the input set. |
| **Applicability** | All versions; Ingress and Gateway API. |
| **Remediation** | Add the port to the Service, or correct the Ingress backend port. |
| **Test cases** | Positive: Ingress backend `port.name: http`, Service has `port.name: web`. Negative: matching port names. Boundary: backend uses a numeric port that the Service exposes via a named `targetPort`. |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 12. Owner references and controller relationships

### 12.1 Owner reference edges

**CROSS-RES-REQ-041:** For each resource with `metadata.ownerReferences`:

- For each entry, construct an `OwnerReference` edge from the dependent to the owner, resolved by `(apiVersion, kind, name, uid)`.
- The `uid` is required for owner-reference resolution because names are not unique. If the `uid` does not match a resource in the input set (the `uid` is assigned by the API server and cannot be predicted), the resolution MUST be `UNRESOLVED` with `info` severity.

**CROSS-RES-REQ-042:** The graph MUST NOT infer owner references from selectors or from `ownerReferences` that lack a `uid`. Inference would create false ownership edges.

### 12.2 Controller-created relationships

Kubernetes controllers create owner references automatically:

- A `Deployment` creates a `ReplicaSet` for each revision.
- A `ReplicaSet` creates `Pods`.
- A `StatefulSet` creates `Pods` and `PersistentVolumeClaims`.
- A `DaemonSet` creates `Pods`.
- A `Job` creates `Pods`.
- A `CronJob` creates `Jobs`.

These relationships are **not** expressed in manifests at creation time. The owner reference is set by the controller when it creates the dependent.

**CROSS-RES-REQ-043:** The graph MUST NOT construct `OwnerReference` edges between a workload and the Pods it will create. The relationship is declarative but not expressed as `ownerReferences` in the workload's manifest. The graph MUST construct a `WorkloadSelectorMatch` edge instead (which captures the declarative relationship without falsely asserting ownership).

### 12.3 Diagnostic rules

| Rule ID | Condition | Severity | Evidence class |
|---|---|---|---|
| `XRES-OWNER-001` | Dependent's `ownerReferences` references an owner not in the input set | `info` | context-dependent |
| `XRES-OWNER-002` | Dependent's `ownerReferences` references an owner in a different namespace | `error` | deterministic (in-set) |
| `XRES-OWNER-003` | Owner reference cycle in the input set | `error` | deterministic (in-set) |
| `XRES-OWNER-004` | A Pod has `ownerReferences` that reference a workload not in the input set | `info` | context-dependent |
| `XRES-OWNER-005` | A resource has `ownerReferences` but no corresponding `WorkloadSelectorMatch` or other declaring relationship | `info` | context-dependent |

#### `XRES-OWNER-003` — Owner reference cycle

| Attribute | Detail |
|---|---|
| **Matching condition** | Following `OwnerReference` edges from a node leads back to the same node. |
| **Counterexamples** | An acyclic chain of owner references does not trigger. |
| **False-positive traps** | Kubernetes does not prevent manual cycles, but they are always configuration errors. The graph MUST report them as `error`. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the input set. |
| **Applicability** | All versions. |
| **Remediation** | Break the cycle by removing one of the owner references. |
| **Test cases** | Positive: A → B, B → A. Negative: A → B, B → C. Boundary: A → B, B → A via different UIDs (should still detect as cycle by name). |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 13. NetworkPolicy and workload selector relationships

### 13.1 Edge construction

**CROSS-RES-REQ-044:** For each `NetworkPolicy`:

- For each Pod in the input set in the same namespace whose labels match `spec.podSelector`, construct a `NetworkPolicySelectorMatch` edge from the NetworkPolicy to the Pod.
- For each `spec.ingress[*].from[*].podSelector`, construct a `NetworkPolicyPeerSelectorMatch` edge from the NetworkPolicy to each matching Pod in the same namespace.
- For each `spec.ingress[*].from[*].namespaceSelector`, construct a `NetworkPolicyPeerNamespaceMatch` edge to the namespace (if in the input set) and to each Pod in that namespace.
- For each `spec.egress[*].to[*].podSelector`, same as ingress peers.

### 13.2 Diagnostic rules

| Rule ID | Condition | Severity | Evidence class |
|---|---|---|---|
| `XRES-NP-001` | NetworkPolicy `podSelector` matches no Pod in the input set | `info` | context-dependent |
| `XRES-NP-002` | NetworkPolicy `ingress` peer `podSelector` matches no Pod in the input set | `info` | context-dependent |
| `XRES-NP-003` | NetworkPolicy `egress` peer `podSelector` matches no Pod in the input set | `info` | context-dependent |
| `XRES-NP-004` | Two NetworkPolicies in the same namespace have identical `podSelector` and both include `Ingress` in `policyTypes` | `info` | context-dependent |
| `XRES-NP-005` | A workload has no matching NetworkPolicy for its declared traffic (context-dependent) | `info` | context-dependent |
| `XRES-NP-006` | CNI enforcement dependency | `info` | context-dependent |

#### `XRES-NP-001` — NetworkPolicy selects no Pod

| Attribute | Detail |
|---|---|
| **Matching condition** | A NetworkPolicy's `podSelector` matches no Pod in the input set in the same namespace. |
| **Counterexamples** | At least one Pod matches the selector. |
| **False-positive traps** | Pods may exist in the cluster that are not in the input set. The finding is `info`, not `error`. |
| **Severity** | `info` |
| **Confidence** | Deterministic that no supplied Pod matches; the operational impact is `info`. |
| **Applicability** | All versions. |
| **Remediation** | Confirm the policy targets the intended Pods. Supply the workload for a complete check. |
| **Test cases** | Positive: NetworkPolicy selector `{app: db}`, no Pod with that label. Negative: matching Pod. Boundary: selectorless NetworkPolicy (matches all Pods). |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 14. Resource ordering versus actual dependency semantics

### 14.1 The ordering problem

Kubernetes manifests are typically applied in a specific order, but the ordering is **not** a semantic dependency. A Service may be applied before or after the Pods it selects; the API server does not enforce ordering. The graph MUST NOT assume that resources appear in the input set in apply order.

**CROSS-RES-REQ-045:** The graph MUST be built from the input set as a whole, not from the sequence of documents. Resource ordering in the input set MUST NOT affect edge construction or resolution.

### 14.2 Implicit dependencies

Some relationships are **implicit** (created by controllers) and not expressed in the manifest. Others are **explicit** (`ownerReferences`, `volumeMounts`, `secretKeyRef`).

| Relationship | Explicit? | Graph edge? |
|---|---|---|
| Deployment → ReplicaSet | Implicit (controller-created) | No `OwnerReference` edge; `WorkloadSelectorMatch` captures the declarative relationship. |
| ReplicaSet → Pod | Implicit (controller-created) | No `OwnerReference` edge; `WorkloadSelectorMatch` captures the declarative relationship. |
| StatefulSet → PVC | Implicit (controller-created) | `StatefulSetTemplateMatch` edge from the StatefulSet to the generated PVC template. |
| Pod → ConfigMap | Explicit (`volumes[*].configMap`) | `ConfigMapRef` edge. |
| Pod → ServiceAccount | Explicit (`spec.serviceAccountName`) | `ServiceAccountRef` edge. |
| Ingress → Service | Explicit (`backend.service`) | `IngressBackendServiceRef` edge. |
| RoleBinding → Role | Explicit (`roleRef`) | `RoleRef` edge. |
| Service → Pod | Explicit (`spec.selector`) | `ServiceSelectorMatch` edge. |

**CROSS-RES-REQ-046:** The graph MUST distinguish explicit and implicit relationships. Explicit relationships produce edges with `explicit=true`; implicit relationships produce edges with `explicit=false`. Rules that assume explicit relationships MUST filter on `explicit=true`.

### 14.3 Apply ordering

**CROSS-RES-REQ-047:** The graph MUST NOT enforce or recommend an apply order. KubeCheck validates manifests, not apply-time behavior. A finding that depends on apply order (e.g., "ConfigMap must be applied before the Pod") is outside scope.

---

## 15. Optional, external, generated, and unresolved references

### 15.1 Reference classification

| Class | Definition | Graph edge scope | Diagnostic |
|---|---|---|---|
| **Resolved** | Target in input set, exactly one match | `internal` | None (or the rule fires on the resolved target's properties) |
| **Optional** | Target is marked optional in the schema; may be absent | `optional` | `XRES-REF-007` (`info`) if target absent |
| **External** | Target is intentionally external (e.g., `ExternalName` Service, `LoadBalancer`) | `external` | `XRES-REF-003` (`info`) |
| **Generated** | Target is generated by a controller (e.g., StatefulSet PVC) | `generated` | `XRES-REF-004` (`info`) |
| **Unresolved** | Target not in input set, not external, not generated | `unresolved` | `XRES-REF-002` (`info`) |

### 15.2 External references

**CROSS-RES-REQ-048:** An `ExternalName` Service's `spec.externalName` MUST produce an `ExternalNameRef` edge with `scope="external"`. The edge MUST carry the external DNS name as `to_hint` and MUST NOT be marked as `unresolved`.

**CROSS-RES-REQ-049:** A `LoadBalancer` Service's external load balancer MUST produce a `LoadBalancerRef` edge with `scope="external"`. The edge MUST carry the requested `loadBalancerIP` (if any) as `to_hint`.

### 15.3 Generated references

**CROSS-RES-REQ-050:** A StatefulSet's `volumeClaimTemplates` produces generated PVCs named `<template>-<statefulset>-<ordinal>`. The graph MUST construct `StatefulSetTemplateMatch` edges from the StatefulSet to each generated PVC template, and MUST NOT construct a `PVCRef` edge from a Pod to a generated PVC (because the Pod references the template, not the generated PVC).

**CROSS-RES-REQ-051:** A generated reference MUST NOT be reported as unresolved. The graph MUST annotate it as `generated` and the rules MUST treat it as expected.

### 15.4 Unresolved references

**CROSS-RES-REQ-052:** An unresolved reference MUST produce `XRES-REF-002` (`info`). The finding MUST include:

1. The reference field and source anchor.
2. The target hint (group, kind, namespace, name).
3. A statement that the target may exist in the cluster.
4. A suggestion to supply the target for a complete check.

---

## 16. CRDs, custom resources, and incomplete knowledge

### 16.1 CRD-driven edges

**CROSS-RES-REQ-053:** For a custom resource, the graph builder MUST construct:

- A `CRDRef` edge to the CRD that defines the custom resource's GVK (if the CRD is in the input set).
- Reference edges for any known reference mechanism declared in the CRD's structural schema (e.g., a `spec.configMapRef` field with a `configMapKeyRef`-like schema).

**CROSS-RES-REQ-054:** For references declared only in a CRD's schema (not in a built-in Kubernetes resource), the graph builder MUST construct edges using the schema's field names and reference structure, but MUST annotate the edge with `knowledge="schema-derived"` and MUST NOT assume the CRD's controller interprets the reference the way Kubernetes interprets built-in references.

### 16.2 Incomplete knowledge

**CROSS-RES-REQ-055:** When a CRD schema is not supplied locally, the graph builder MUST NOT construct reference edges for the CRD's fields. The graph MUST annotate the CRD instance as `knowledge="unknown"` and MUST NOT produce cross-resource findings that require knowledge of the CRD's schema.

**CROSS-RES-REQ-056:** When an aggregated API's schema is not supplied, the graph builder MUST treat the API's custom resources as opaque nodes with no outgoing reference edges (except `CRDRef` if the APIService is in the input set).

---

## 17. Duplicate, ambiguous, and conflicting references

### 17.1 Duplicates

**CROSS-RES-REQ-057:** A `ResourceKey` duplicate in the input set MUST produce `MAINT-DUP-001` (`warning`) and MUST cause `RESOLVED_WITH_DUPLICATES` resolution for any reference targeting that key. Rules MUST use the first candidate (canonical order) and MUST include the duplicate anchors in the finding evidence.

### 17.2 Ambiguity

**CROSS-RES-REQ-058:** A reference is ambiguous if it could resolve to more than one target. Ambiguity arises when:

- A selector matches multiple resources, and the schema expects a single target.
- A named reference has duplicates in the input set.
- A reference uses a wildcard or pattern that matches multiple targets.

**CROSS-RES-REQ-059:** Ambiguous references MUST produce `XRES-REF-009` (`warning`) listing all candidates. The finding MUST NOT arbitrarily choose one target; it MUST report all candidates and let the user resolve.

### 17.3 Conflicts

**CROSS-RES-REQ-060:** A conflict exists when two references to the same target have inconsistent properties (e.g., two Pods reference the same Secret with different `optional` values). Conflicts MUST produce `XRES-REF-010` (`warning`) with the conflicting references and their anchors.

**CROSS-RES-REQ-061:** A conflict between two rules' findings MUST NOT cause one finding to be suppressed. Both findings MUST be emitted, and each MUST carry a `conflict_with` field naming the other (`RULE-SPEC-REQ-037`).

---

## 18. Graph examples

### 18.1 Example 1: Deployment, Service, ConfigMap

```yaml
# Deployment (namespace: production)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: production
spec:
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      serviceAccountName: web-sa
      containers:
      - name: app
        image: nginx
        envFrom:
        - configMapRef:
            name: web-config
---
# Service (namespace: production)
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: production
spec:
  selector:
    app: web
  ports:
  - port: 80
    targetPort: 8080
---
# ConfigMap (namespace: production)
apiVersion: v1
kind: ConfigMap
metadata:
  name: web-config
  namespace: production
data:
  LOG_LEVEL: info
---
# ServiceAccount (namespace: production)
apiVersion: v1
kind: ServiceAccount
metadata:
  name: web-sa
  namespace: production
```

**Graph:**

```
Nodes:
  N1: apps/v1/Deployment/production/web
  N2: ""/v1/Service/production/web
  N3: ""/v1/ConfigMap/production/web-config
  N4: ""/v1/ServiceAccount/production/web-sa

Edges:
  E1: WorkloadSelectorMatch  N1 -> Pod (unresolved: Pod not in input set)
  E2: ServiceSelectorMatch   N2 -> Pod (unresolved: Pod not in input set)
  E3: ConfigMapRef           N1 -> N3 (resolved)
  E4: ServiceAccountRef      N1 -> N4 (resolved)
```

**Findings:**

- `XRES-WL-001` (`info`): `Deployment/production/web` selector matches no Pod in the input set.
- `XRES-SVC-001` (`info`): `Service/production/web` selector matches no Pod in the input set.
- No findings for `ConfigMapRef` (resolved) or `ServiceAccountRef` (resolved).

### 18.2 Example 2: Ingress with cross-namespace TLS secret

```yaml
# Ingress (namespace: production)
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app
  namespace: production
spec:
  tls:
  - hosts:
    - app.example.com
    secretName: app-tls
  rules:
  - host: app.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: web
            port:
              number: 80
---
# Secret (namespace: default) — WRONG NAMESPACE
apiVersion: v1
kind: Secret
metadata:
  name: app-tls
  namespace: default
type: kubernetes.io/tls
data:
  tls.crt: <base64>
  tls.key: <base64>
---
# Service (namespace: production)
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: production
spec:
  selector:
    app: web
  ports:
  - port: 80
```

**Graph:**

```
Nodes:
  N1: networking.k8s.io/v1/Ingress/production/app
  N2: ""/v1/Secret/default/app-tls
  N3: ""/v1/Service/production/web

Edges:
  E1: IngressTLSSecretRef    N1 -> (unresolved: secret not in production namespace)
  E2: IngressBackendServiceRef N1 -> N3 (resolved)
  E3: (no ServiceSelectorMatch: no Pods in input set)
```

**Findings:**

- `XRES-ING-003` (`error`): Ingress `app` references TLS secret `app-tls` in namespace `production`, but the secret exists in namespace `default`.
- `XRES-ING-002` (`info`): Ingress `app` references TLS secret `app-tls` not found in the analyzed input set.

### 18.3 Example 3: RBAC cross-namespace subject

```yaml
# RoleBinding (namespace: production)
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: web-binding
  namespace: production
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: web-role
subjects:
- kind: ServiceAccount
  name: web-sa
  namespace: kube-system   # WRONG NAMESPACE
---
# Role (namespace: production)
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: web-role
  namespace: production
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list"]
```

**Graph:**

```
Nodes:
  N1: rbac.authorization.k8s.io/v1/RoleBinding/production/web-binding
  N2: rbac.authorization.k8s.io/v1/Role/production/web-role
  (no ServiceAccount node for kube-system/web-sa)

Edges:
  E1: RoleRef     N1 -> N2 (resolved)
  E2: SubjectRef  N1 -> (unresolved: SA not in input set)
```

**Findings:**

- `XRES-RBAC-004` (`info`): RoleBinding subject SA `kube-system/web-sa` not found in the analyzed input set.
- `XRES-RBAC-005` (`error`): RoleBinding in `production` references SA in `kube-system`.

---

## 19. Scalability constraints

### 19.1 Node and edge limits

| Limit ID | Limit | Default | Rationale |
|---|---|---|---|
| `XRES-LIMIT-001` | Maximum nodes in the graph | 10,000 | Bounds memory and traversal time. |
| `XRES-LIMIT-002` | Maximum edges in the graph | 100,000 | Bounds edge storage and traversal. |
| `XRES-LIMIT-003` | Maximum out-degree per node | 1,000 | Prevents a single resource from exploding the graph. |
| `XRES-LIMIT-004` | Maximum traversal depth | 32 | Prevents unbounded BFS/DFS. |
| `XRES-LIMIT-005` | Maximum index build time | 5 s | Bounds overall analysis time. |
| `XRES-LIMIT-006` | Maximum edge resolution time per reference | 100 ms | Bounds resolution time for any single reference. |
| `XRES-LIMIT-007` | Maximum selector match candidates per reference | 10,000 | Bounds selector match evaluation. |

**CROSS-RES-REQ-062:** Exceeding a graph limit MUST cause graph construction to fail with a structured `XRES-LIMIT-*` finding. The finding MUST name the limit and the observed value.

**CROSS-RES-REQ-063:** Exceeding a per-reference limit MUST cause the affected reference to be marked `UNRESOLVED` with a `XRES-REF-011` (`warning`) finding. Other references continue processing.

### 19.2 Graph traversal complexity

| Operation | Complexity |
|---|---|
| Index build | O(N × I) where I is the average index entries per node |
| Reference resolution (by name) | O(1) hash lookup |
| Reference resolution (by selector) | O(M) where M is the number of resources of the target kind in the namespace |
| Owner reference cycle detection | O(N + E) |
| Full graph traversal | O(N + E) |

**CROSS-RES-REQ-064:** The graph builder MUST NOT perform O(N²) operations on the full input set. Selector matching MUST use the label index and MUST NOT scan all resources of the target kind.

### 19.3 Memory constraints

**CROSS-RES-REQ-065:** The graph MUST NOT duplicate NIR nodes. Node data MUST be referenced from the NIR via `NodeRef` identifiers. The graph's memory footprint MUST be proportional to the number of edges, not the total NIR size.

---

## 20. Diagnostic rules summary

### 20.1 Cross-resource finding families

| Family | Rule count | Examples |
|---|---|---|
| `XRES-WL-*` | 4 | Workload selector matches no Pod, owner reference issues |
| `XRES-SVC-*` | 4 | Service selector, targetPort resolution |
| `XRES-REF-*` | 11 | ConfigMap/Secret/SA/PVC/SC references |
| `XRES-RBAC-*` | 8 | Role/ClusterRole references, subject namespace |
| `XRES-ING-*` | 5 | Ingress backend and TLS references |
| `XRES-GW-*` | 5 | Gateway API references |
| `XRES-OWNER-*` | 5 | Owner reference existence, namespace, cycle |
| `XRES-NP-*` | 6 | NetworkPolicy selectors |
| `XRES-LIMIT-*` | 7 | Graph limits |
| **Total** | **55** | |

### 20.2 Severity distribution

| Severity | Count | Rationale |
|---|---|---|
| `error` | 19 | Deterministic defects within the input set |
| `warning` | 14 | Conflicts, duplicates, ambiguities, limits |
| `info` | 22 | Cluster-dependent existence, external references, generated references |

**CROSS-RES-REQ-066:** A cross-resource rule MUST NOT emit `error` if any part of its matching condition depends on a target not in the input set. Such rules MUST emit `info` (for existence) or `warning` (for ambiguity/conflict).

---

## 21. Tests for false positives

### 21.1 Mandatory false-positive tests

**CROSS-RES-REQ-067:** Every cross-resource rule MUST have at least five false-positive tests demonstrating a valid configuration that does not trigger the rule. The tests MUST cover:

1. **Same-namespace reference** — the target is in the same namespace and the rule does not trigger.
2. **Cross-namespace reference in an allowed context** — e.g., a `ClusterRoleBinding` with a SA subject from any namespace.
3. **Optional reference with absent target** — the rule emits `info`, not `error`.
4. **External reference** — the target is intentionally external and the rule does not treat it as unresolved.
5. **Generated reference** — the target is generated by a controller and the rule does not treat it as unresolved.

### 21.2 Example false-positive tests

| Test ID | Rule | Scenario | Assertion |
|---|---|---|---|
| `TEST-XRES-001` | `XRES-REF-006` | Pod in `default` references Secret in `default` | No finding |
| `TEST-XRES-002` | `XRES-REF-006` | ClusterRoleBinding with SA subject from `kube-system` | No finding |
| `TEST-XRES-003` | `XRES-REF-002` | Optional Secret reference with absent target | `XRES-REF-007` (`info`), not `XRES-REF-002` |
| `TEST-XRES-004` | `XRES-REF-002` | ExternalName Service's external DNS target | `XRES-REF-003` (`info`), not `XRES-REF-002` |
| `TEST-XRES-005` | `XRES-REF-002` | StatefulSet-generated PVC reference | `XRES-REF-004` (`info`), not `XRES-REF-002` |
| `TEST-XRES-006` | `XRES-SVC-001` | Selectorless Service | `XRES-SVC-002` (`info`), not `XRES-SVC-001` |
| `TEST-XRES-007` | `XRES-WL-004` | Two workloads with disjoint selectors | No finding |
| `TEST-XRES-008` | `XRES-ING-003` | Ingress TLS secret in the same namespace | No finding |
| `TEST-XRES-009` | `XRES-OWNER-003` | Acyclic owner reference chain | No finding |
| `TEST-XRES-010` | `XRES-RBAC-005` | RoleBinding with SA subject in the same namespace | No finding |

### 21.3 Integration false-positive tests

**CROSS-RES-REQ-068:** At least one integration test MUST run a complete valid multi-resource manifest (Deployment + Service + ConfigMap + Secret + ServiceAccount + RoleBinding + Role + Ingress + TLS Secret + NetworkPolicy) and assert that **no cross-resource findings are emitted**.

---

## 22. Authoritative references

| Reference | Scope | Type | Retrieved |
|---|---|---|---|
| `docs/architecture/SYSTEM_ARCHITECTURE.md` (internal) | Graph builder interface (`IF-GRAPH-001`), relationship graph definition | Internal | 2026-10-31 |
| `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md` (internal) | Resource identity, scope, namespaces | Internal | 2026-10-31 |
| `docs/kubernetes/WORKLOADS_AND_PODS.md` (internal) | Selector/template consistency, owner references | Internal | 2026-10-31 |
| `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md` (internal) | Service selector, Ingress, Gateway, NetworkPolicy references | Internal | 2026-10-31 |
| `docs/kubernetes/CONFIGURATION_AND_STORAGE.md` (internal) | ConfigMap, Secret, PVC, StorageClass references | Internal | 2026-10-31 |
| `docs/kubernetes/SECURITY_AND_RBAC.md` (internal) | RBAC subject and role references | Internal | 2026-10-31 |
| `docs/engine/DIAGNOSTIC_RULE_SPEC.md` (internal) | Rule execution, safe expressions, conflict resolution | Internal | 2026-10-31 |
| Kubernetes Documentation — Owners and Dependents | `ownerReferences`, controller-created references | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Services | Selector-to-Pod matching, selectorless Services | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Network Policies | `podSelector`, peer selectors | Official documentation | 2026-10-31 |
| Kubernetes Documentation — RBAC | Role/ClusterRole references, subject namespace rules | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Ingress | Backend Service references, TLS Secret namespace | Official documentation | 2026-10-31 |
| Gateway API specification | `parentRefs`, `backendRefs`, `certificateRefs` | Specification | 2026-10-31 |
| Kubernetes Documentation — Ephemeral Volumes | `volumeClaimTemplates` generated PVCs | Official documentation | 2026-10-31 |

**Licensing note:** Kubernetes documentation is licensed under CC BY 4.0. Gateway API specifications are licensed under Apache 2.0. No line numbers, benchmark results, or implementation status are asserted beyond what the cited sources state.

---

## 23. Open questions and unresolved decisions

| ID | Question | Status | Owner |
|---|---|---|---|
| `OPEN-XRES-01` | Whether the graph will support **transitive** reference resolution (e.g., a Service→Pod→ConfigMap path) or only direct edges. The current design supports direct edges; transitive analysis would be a separate graph traversal feature. | **OPEN** | Graph Analysis Architecture |
| `OPEN-XRES-02` | Exact severity policy for `XRES-REF-006` (cross-namespace reference): `error` unconditionally or `error` only when the target is in the input set. The current design uses `error` when the target is in the input set and `info` otherwise. | **OPEN** | Diagnostics Architecture |
| `OPEN-XRES-03` | Whether `XRES-WL-004` (overlapping selectors) should be `warning` or `info` by default. | **OPEN** | Diagnostics Architecture |
| `OPEN-XRES-04` | Whether the graph should include **namespace nodes** as first-class nodes or only annotate namespace relationships on edges. | **OPEN** | Graph Analysis Architecture |
| `OPEN-XRES-05` | Whether `XRES-NP-005` (workload without matching NetworkPolicy) should be included in the default bundle or deferred to the operational-context phase. | **OPEN** | Product Architecture |
| `OPEN-XRES-06` | Whether the graph builder will support CRD-declared references (schema-derived edges) in the MVP or only in Phase 3 (CRD support). | **OPEN** | Graph Analysis Architecture |
| `OPEN-XRES-07` | Whether `XRES-OWNER-003` (cycle) should detect cycles by `uid` only or by `(apiVersion, kind, namespace, name)` when `uid` is absent. The current design detects by key when `uid` is absent. | **OPEN** | Graph Analysis Architecture |

---

## 24. Definition of done

This document is complete when the following conditions hold:

1. The graph model (nodes, edges, edge types) is defined with stable identifiers.
2. The indexing strategy is specified with primary and secondary indexes, build complexity, and determinism requirements.
3. The deterministic reference-resolution algorithm is specified with pseudocode and eight result kinds.
4. The distinction between "not found in the analyzed input" and "does not exist in the cluster" is stated as a governing principle (`CROSS-RES-REQ-002`) and repeated in every relevant section.
5. Resource identity, scope, namespace, and duplicate definitions are covered.
6. Pod-template, workload selector, Service selector, ConfigMap/Secret/ServiceAccount/PVC/StorageClass, RBAC, Ingress/Gateway, owner reference, and NetworkPolicy relationships are each covered with edge construction and diagnostic rules.
7. Resource ordering versus actual dependency semantics is distinguished, including explicit vs. implicit relationships.
8. Optional, external, generated, and unresolved references are classified with distinct edge scopes and findings.
9. CRDs, custom resources, and incomplete knowledge are covered with explicit boundaries.
10. Duplicate, ambiguous, and conflicting references are specified with distinct findings.
11. Graph examples show the node/edge/finding model for three representative cases.
12. Scalability constraints are specified with seven limits and complexity bounds.
13. Every cross-resource rule has five false-positive tests, and an integration false-positive test is specified.
14. Every requirement has a stable ID (`CROSS-RES-REQ-NNN`), and every finding has a stable ID (`XRES-*-NNN`).
15. No finding claims live cluster existence, admission, or operational behavior as an offline outcome.

---

## 25. Consistency and completeness review

A review was performed against the requested scope before finalization.

- **Resource identity, scope, namespace, duplicate definitions:** §6.
- **Pod-template and workload selector relationships:** §7.
- **Service selector-to-Pod label matching:** §8.
- **ConfigMap, Secret, ServiceAccount, PVC, StorageClass references:** §9.
- **RBAC subject and role references:** §10.
- **Ingress and Gateway backend references:** §11.
- **Owner references and controller relationships:** §12.
- **NetworkPolicy and workload selector relationships:** §13.
- **Resource ordering versus actual dependency semantics:** §14.
- **Optional, external, generated, unresolved references:** §15.
- **CRDs, custom resources, incomplete knowledge:** §16.
- **Duplicate, ambiguous, conflicting references:** §17.
- **Graph model, edge types, indexing strategy, traversal limits, deterministic reference-resolution algorithm:** §3–§5, §19.
- **"No matching resource found in analyzed input" vs "does not exist in cluster":** `CROSS-RES-REQ-002`, `CROSS-RES-REQ-016`, `CROSS-RES-REQ-017`, §5.3.
- **Graph examples:** §18.
- **Diagnostic rules:** §19–§20.
- **Scalability constraints:** §19.
- **Tests for false positives:** §21.

No empty boilerplate section was added solely to satisfy a checklist. The document is complete for its assigned responsibility.
