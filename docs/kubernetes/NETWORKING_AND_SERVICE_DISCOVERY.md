# KubeCheck — Networking and Service Discovery

**Document ID:** K8S-NET-001  
**File:** `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Kubernetes Networking Diagnostics  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`
- `docs/kubernetes/WORKLOADS_AND_PODS.md`
- `docs/kubernetes/KUBERNETES_SCHEMA_VALIDATION.md`
- `docs/kubernetes/KUBERNETES_API_VERSION_LIFECYCLE.md`
- `docs/kubernetes/CRD_AND_CUSTOM_RESOURCE_HANDLING.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/security/THREAT_MODEL.md`
- `docs/testing/DETERMINISM_TESTS.md`

---

## 1. Purpose and scope

This document defines KubeCheck's **offline analysis of service discovery, network routing, and workload connectivity** in Kubernetes manifests. It specifies which networking checks are possible from manifests alone and which require cluster state, controller behavior, DNS resolution, or runtime networking.

**Governing distinction:**

> **K8S-NET-REQ-001 — KubeCheck MUST clearly separate manifest-level network defects from connectivity failures that depend on cluster state, CNI behavior, controller configuration, or runtime DNS.** A manifest can be structurally correct and still produce a connectivity failure (empty EndpointSlice due to unready Pods, CNI ignoring NetworkPolicy, ingress controller not installed). KubeCheck validates the manifest, not the live network.

**In scope:** Services (ClusterIP, NodePort, LoadBalancer, ExternalName), selectors, EndpointSlices, port mapping, Ingress, Gateway API, NetworkPolicy, DNS names, headless and selectorless Services, TLS references, cross-resource consistency, and the boundary between static analysis and runtime networking.

**Out of scope:** CNI plugin implementation details, live DNS resolution, controller reconciliation, and actual packet forwarding.

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **Service** | A named abstraction that exposes a set of Pods as a network endpoint, with a selector, ports, and a type. |
| **Selector** | A label selector that determines which Pods back a Service. |
| **EndpointSlice** | A scalable API object that groups network endpoints for a Service by protocol, port, and Service name. |
| **targetPort** | The port or named port on the Pod that the Service forwards traffic to. |
| **Ingress** | A resource that exposes HTTP/HTTPS routes from outside the cluster to Services. |
| **IngressClass** | A cluster-scoped resource that identifies the controller that should implement an Ingress. |
| **Gateway API** | A set of CRDs (Gateway, HTTPRoute, GRPCRoute, etc.) for expressive, role-oriented traffic routing. |
| **NetworkPolicy** | A namespaced resource that declares allowed ingress and egress traffic for selected Pods. |
| **Headless Service** | A Service with `clusterIP: None`; DNS returns the Pod IPs directly instead of a virtual IP. |
| **ExternalName Service** | A Service that maps to an external DNS name via a CNAME record. |
| **Selectorless Service** | A Service without a selector; endpoints are managed manually or by an external controller. |

**Requirement IDs** use `K8S-NET-REQ-NNN`. Finding IDs use `NET-<FAMILY>-NNN`.

---

## 3. Services and service types

### 3.1 Service structure

A Service has `spec.type` (default `ClusterIP`), `spec.selector`, `spec.ports`, and type-specific fields. The four types are:

| Type | Behavior | Exposes |
|---|---|---|
| **ClusterIP** (default) | Exposes the Service on a cluster-internal IP. | Cluster-internal only. |
| **NodePort** | Exposes the Service on each Node's IP at a static port (default range 30000–32767). | Cluster-external via `NodeIP:NodePort`. |
| **LoadBalancer** | Builds on NodePort and creates an external load balancer if the cloud provider supports it. | Cluster-external via a cloud load balancer. |
| **ExternalName** | Maps the Service to the contents of `externalName` via a CNAME record. | DNS alias; no proxying. |

Sources: Service type behavior is documented in the Kubernetes Service concept and API reference[reference:0][reference:1].

**K8S-NET-REQ-002:** KubeCheck MUST validate `spec.type` against the enum `ClusterIP`, `NodePort`, `LoadBalancer`, `ExternalName`. An unknown type is an `error`-severity schema finding.

### 3.2 Selector and targetPort

`spec.selector` is a map of label keys to values. The Service forwards traffic to Pods whose labels match the selector. If `selector` is empty or absent, Kubernetes assumes an external process manages the endpoints; for `ExternalName`, the selector is ignored[reference:2].

`spec.ports[*].targetPort` is an `IntOrString`: a numeric port (1–65535) or a named port that is looked up in the target Pod's container ports. If `targetPort` is omitted, it defaults to the value of `port`[reference:3].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-SVC-001** | `spec.type` is not a valid enum value | `error` | API server rejects unknown types. |
| **NET-SVC-002** | `spec.ports[*].port` is outside 1–65535 | `error` | Invalid port number. |
| **NET-SVC-003** | `spec.ports[*].targetPort` is a number outside 1–65535 | `error` | Invalid target port. |
| **NET-SVC-004** | `spec.ports[*].targetPort` is a named port that does not appear in any matching Pod's `containerPort` names | `error` if matching Pods are in the input set; `warning` if no matching Pod is in the input set | A named `targetPort` must resolve to a container port name. |
| **NET-SVC-005** | `spec.ports[*].protocol` is not `TCP`, `UDP`, or `SCTP` | `error` | Invalid protocol enum. |
| **NET-SVC-006** | Two `spec.ports` entries have the same `name` | `error` | Port names must be unique per Service. |
| **NET-SVC-007** | Two `spec.ports` entries have the same `(port, protocol)` pair | `error` | Duplicate port/protocol is rejected. |

**K8S-NET-REQ-003:** The named-port resolution check (`NET-SVC-004`) MUST search the container ports of Pods whose labels match `spec.selector`. If no matching Pod is in the input set, the check is `warning` and states that resolution cannot be verified offline.

### 3.3 NodePort constraints

A `NodePort` Service requires `spec.ports[*].nodePort` to be either unset (auto-assigned) or within the configured node-port range. The default range is 30000–32767. If `nodePort` is set on a Service that does not require it (e.g., `ClusterIP`), the API server rejects the Service.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-NP-001** | `spec.ports[*].nodePort` is set on a `ClusterIP` Service | `error` | The API server rejects it. |
| **NET-NP-002** | `spec.ports[*].nodePort` is outside the documented node-port range | `error` | The API server rejects an out-of-range port. |
| **NET-NP-003** | Two `NodePort` Services in the input set use the same `nodePort` on the same protocol | `warning` | The API server may reject the second; the conflict is detectable within the input set. |

**K8S-NET-REQ-004:** The node-port range is a cluster configuration parameter, not a manifest property. KubeCheck MUST use the default range (30000–32767) and MUST emit an `info` finding when a `nodePort` is set, stating that the range depends on cluster configuration.

### 3.4 LoadBalancer-specific fields

`spec.externalTrafficPolicy`, `spec.loadBalancerIP`, `spec.loadBalancerSourceRanges`, and `spec.allocateLoadBalancerNodePorts` affect LoadBalancer behavior. These are largely **cluster-dependent**: the cloud controller manager and the cloud provider determine whether the requested load balancer can be provisioned.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-LB-001** | `spec.externalTrafficPolicy` is not `Cluster` or `Local` | `error` | Invalid enum. |
| **NET-LB-002** | `spec.loadBalancerSourceRanges` contains an invalid CIDR | `error` | Invalid CIDR syntax. |
| **NET-LB-003** | `spec.loadBalancerIP` is set on a non-LoadBalancer Service | `warning` | The field is ignored; likely a configuration mistake. |

**Heuristic checks:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **NET-LB-004** | `externalTrafficPolicy: Local` with a Deployment that has replicas only on some nodes | `info` | Traffic is routed only to nodes with local endpoints; the finding is advisory. |

---

## 4. EndpointSlices and target ports

### 4.1 EndpointSlice role

EndpointSlice is the scalable replacement for the legacy Endpoints API. The control plane automatically creates EndpointSlices for Services that have a selector. Each EndpointSlice contains references to a set of network endpoints and groups them by unique protocol, port number, and Service name. Each slice holds up to 100 endpoints by default (configurable up to 1000). EndpointSlices support IPv4, IPv6, and FQDN address types[reference:4].

**K8S-NET-REQ-005:** KubeCheck MUST NOT attempt to read EndpointSlices from a cluster. It MAY validate an EndpointSlice manifest supplied as input for structural conformance, but it MUST NOT claim that the EndpointSlice reflects the live state of a Service.

### 4.2 Named ports and EndpointSlices

When a Service uses named `targetPort` values, Pods may have different numeric ports for the same name; this requires different EndpointSlices. All ports in an EndpointSlice must have a unique name[reference:5][reference:6].

**K8S-NET-REQ-006:** A Service with a named `targetPort` MUST be checked for consistency: the same port name MUST resolve to a container port name in every matching Pod. If different matching Pods expose the name at different numeric ports, that is valid and produces multiple EndpointSlices; KubeCheck MUST NOT report it as an error.

### 4.3 Empty EndpointSlice diagnosis

An empty EndpointSlice has three possible causes: no Pod matches the selector, matching Pods are not Ready, or the container port does not exist. KubeCheck can detect the **manifest-level** condition (no Pod in the input set matches the selector) but cannot detect readiness or live endpoint state.

**K8S-NET-REQ-007:** When a Service's selector matches no Pod in the input set, KubeCheck MUST emit an `info`-severity finding stating that the selector matches no supplied Pod and that cluster state is required to determine whether any Pod in the cluster matches. It MUST NOT emit an `error` claiming the Service has no endpoints.

---

## 5. Ingress resources, ingress classes, and TLS

### 5.1 Ingress structure

An Ingress has `spec.ingressClassName`, `spec.rules` (host and path routing), and `spec.tls` (TLS termination). `ingressClassName` references an IngressClass cluster resource that identifies the controller. If the field is omitted or misnamed and multiple controllers are installed, no controller claims the Ingress and nothing serves it[reference:8][reference:9].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-ING-001** | `spec.rules[*].http.paths[*].backend` has neither `service` nor `resource` | `error` | A path must have exactly one backend. |
| **NET-ING-002** | `backend.service.port` has neither `name` nor `number` | `error` | Port must be specified. |
| **NET-ING-003** | `spec.rules[*].http.paths[*].pathType` is not `Exact`, `Prefix`, or `ImplementationSpecific` | `error` | Invalid enum. |
| **NET-ING-004** | `spec.tls[*].secretName` references a Secret not in the input set | `info` | Existence is cluster-dependent; the reference is structurally valid. |
| **NET-ING-005** | `spec.tls` is present but `spec.rules` has no matching host | `warning` | TLS hosts should correspond to routing hosts. |

**K8S-NET-REQ-008:** An Ingress that omits `ingressClassName` MUST produce an `info` finding stating that the controller that claims the Ingress is determined by cluster configuration and cannot be verified offline. It MUST NOT be reported as an error, because some clusters default the class.

### 5.2 TLS secret namespace

A TLS secret referenced by an Ingress **must be in the same namespace as the Ingress**. If the secret is in a different namespace, the controller silently falls back to a default certificate, producing a "not secure" warning with a certificate for the wrong host[reference:10][reference:11].

**Deterministic check (NET-ING-006):** If a TLS secret is present in the input set and its namespace differs from the Ingress namespace, emit an `error` finding stating that the secret must be in the same namespace. If the secret is not in the input set, emit an `info` finding stating that namespace co-location cannot be verified offline.

### 5.3 TLS host alignment

Three things must align: `tls[*].hosts`, `rules[*].host`, and a Subject Alternative Name on the certificate. When they diverge, the certificate is served for the wrong host or a default certificate is used[reference:12].

**Heuristic check (NET-ING-007):** If `tls[*].hosts` and `rules[*].host` differ, emit a `warning` finding naming both values. KubeCheck cannot inspect the certificate's SANs offline, so the finding is limited to the manifest-level divergence.

---

## 6. Gateway API resources and versioned schemas

### 6.1 Gateway API versioning

Gateway API is supplied as a set of CRDs. Each release has a **bundle version** (e.g., `v1.0.0`) that represents a specific combination of CRDs, API versions, and validating webhook. Gateway API has two versioning schemes: **API versions** (e.g., `v1alpha2`, `v1beta1`, `v1`) and **bundle versions** (semantic versions like `v0.4.0`). Significant changes such as removing or renaming fields are represented in API versions; a bundle may include multiple API versions[reference:13].

CRD and webhook validation is "nice UX" but not final enforcement; it may not be present or up to date in all clusters, and may be loosened in future releases[reference:14].

**K8S-NET-REQ-009:** KubeCheck MUST validate Gateway API resources against the CRD schema supplied in the local bundle for the declared `apiVersion`. It MUST NOT assume that a Gateway API CRD is installed in the target cluster or that the validating webhook is present. It MUST emit an `info` finding stating that Gateway API controller behavior and webhook enforcement are cluster-dependent.

### 6.2 Gateway and route resources

Gateway API defines `GatewayClass`, `Gateway`, `HTTPRoute`, `GRPCRoute`, `TCPRoute`, `TLSRoute`, and `UDPRoute`. The `Gateway` defines listeners (port, protocol, hostname, TLS); routes attach to gateways via `parentRefs` and define matching and backend rules.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-GW-001** | `Gateway.spec.listeners[*].protocol` is not `HTTP`, `HTTPS`, `TLS`, `TCP`, or `UDP` | `error` | Invalid enum. |
| **NET-GW-002** | `HTTPRoute.spec.parentRefs` is empty | `warning` | A route with no parent is not attached to a gateway. |
| **NET-GW-003** | `HTTPRoute.spec.rules[*].backendRefs` has a `port` outside 1–65535 | `error` | Invalid port. |
| **NET-GW-004** | A `Gateway` listener with `protocol: HTTPS` has no `tls` configuration | `error` | HTTPS requires TLS configuration. |

**K8S-NET-REQ-010:** Gateway API CRDs are versioned independently of Kubernetes. KubeCheck MUST select the CRD schema by `apiVersion` from the local bundle and MUST NOT substitute a different API version's schema (`K8S-API-REQ-020`).

---

## 7. NetworkPolicy selectors and rules

### 7.1 NetworkPolicy structure

A NetworkPolicy has `spec.podSelector` (the Pods the policy applies to), `spec.policyTypes` (`Ingress`, `Egress`, or both), `spec.ingress` (allowed inbound rules), and `spec.egress` (allowed outbound rules). An empty `podSelector` matches all Pods in the policy's namespace. Multiple policies that select the same Pods are combined **additively**. If `policyTypes` is omitted, it defaults based on the presence of `ingress` or `egress` sections; an egress-only policy must explicitly set `policyTypes: ["Egress"]`.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-NP-001** | `spec.policyTypes` contains a value other than `Ingress` or `Egress` | `error` | Invalid enum. |
| **NET-NP-002** | `spec.egress` is present but `policyTypes` does not include `Egress` | `warning` | The egress rules may be ignored or the policy may be misinterpreted. |
| **NET-NP-003** | `spec.ingress[*].ports[*].port` is outside 1–65535 | `error` | Invalid port. |
| **NET-NP-004** | `spec.ingress[*].from` has more than one of `podSelector`, `namespaceSelector`, `ipBlock` | `error` | Only certain combinations are allowed. |
| **NET-NP-005** | `spec.egress[*].to` has both `podSelector` and `ipBlock` | `error` | Mutually exclusive peers. |

### 7.2 NetworkPolicy enforcement

A NetworkPolicy is only a firewall if the cluster's CNI plugin enforces it. Not all CNI plugins enforce NetworkPolicy; Amazon VPC CNI did not support native enforcement until version 1.14 in October 2022. Native NetworkPolicy also cannot do TLS termination, mutual authentication, or Layer 7 filtering by HTTP method or path[reference:16][reference:17].

**K8S-NET-REQ-011:** KubeCheck MUST validate NetworkPolicy selectors and rules structurally but MUST emit an `info` finding on every NetworkPolicy stating that enforcement depends on the CNI plugin and that offline analysis cannot verify enforcement. It MUST NOT claim that a NetworkPolicy will block or allow any specific traffic at runtime.

### 7.3 CNI-dependent behavior

| Behavior | CNI-dependent? | KubeCheck handling |
|---|---|---|
| Whether NetworkPolicy is enforced at all | Yes | `info`: enforcement depends on CNI. |
| Whether a specific podSelector matches | No (manifest-level) | Validate selector syntax and matching within the input set. |
| Whether traffic is actually blocked | Yes | Cannot verify offline. |
| Layer 7 filtering | Yes (native API does not support it) | `info` if a policy appears to rely on L7 semantics. |

**K8S-NET-REQ-012:** KubeCheck MUST NOT report a NetworkPolicy as "secure" or "enforced." It may report that the policy is structurally valid and that its runtime effect is CNI-dependent.

---

## 8. DNS names and namespace-aware service references

### 8.1 DNS record forms

Kubernetes DNS creates records for Services and Pods. A normal Service gets A/AAAA records of the form `my-svc.my-namespace.svc.cluster-domain.example`, resolving to the cluster IP. A headless Service resolves to the set of Pod IPs. SRV records are created for named ports[reference:18].

A client Pod's DNS search list includes the Pod's own namespace and the cluster's default domain. A query without a namespace is limited to the Pod's namespace. To reach a Service in another namespace, the query must include the namespace (e.g., `data.prod`) or the fully qualified name (`data.prod.svc.cluster.local`)[reference:19].

**K8S-NET-REQ-013:** KubeCheck MUST validate that a Service reference embedded in another manifest (e.g., an Ingress backend, a Gateway backendRef, or an `ExternalName`) uses a syntactically valid DNS name. It MUST NOT resolve the name or verify that the target Service exists, because resolution and existence are cluster-dependent.

### 8.2 Namespace-aware references

A reference such as `data.prod` is namespace-qualified. A reference such as `data` is namespace-local to the querying Pod. KubeCheck can validate the **syntax** of the reference and, if the target Service is in the input set, whether the namespace matches. It cannot verify that the querying Pod's namespace is the intended one without cluster context.

**Heuristic check (NET-DNS-001):** A cross-namespace Service reference in an Ingress or Gateway backend that omits the namespace MAY be a `warning` if the input set contains a Service with the same name in multiple namespaces. The finding states that the reference is ambiguous without cluster context.

---

## 9. Headless, ExternalName, and selectorless Services

### 9.1 Headless Services

A **headless Service** has `spec.clusterIP: None`. It has no virtual IP; DNS returns the Pod IPs directly. A headless Service can have a selector (DNS returns the selected Pod IPs) or no selector (DNS returns no records unless endpoints are manually created)[reference:20].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-HL-001** | `spec.clusterIP` is `None` and `spec.type` is `LoadBalancer` | `error` | Headless and LoadBalancer are incompatible. |
| **NET-HL-002** | `spec.clusterIP` is `None` and `spec.ports[*].targetPort` is set | `info` | `targetPort` is ignored for headless Services. |

### 9.2 ExternalName Services

An **ExternalName Service** maps to `spec.externalName` via a CNAME record. It has no selector and no ports in the proxying sense. It requires kube-dns v1.7+ or CoreDNS 0.8+[reference:21].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-EXT-001** | `spec.type: ExternalName` and `spec.selector` is set | `warning` | Selector is ignored for ExternalName. |
| **NET-EXT-002** | `spec.type: ExternalName` and `spec.externalName` is absent | `error` | `externalName` is required. |
| **NET-EXT-003** | `spec.externalName` is not a valid DNS name | `error` | CNAME targets must be valid DNS names. |

### 9.3 Selectorless Services

A Service with no selector does not get an EndpointSlice from the control plane; endpoints must be managed manually or by an external controller[reference:22]. KubeCheck MUST emit an `info` finding stating that the Service has no selector and that its endpoints are not managed by Kubernetes.

**K8S-NET-REQ-014:** A selectorless Service MUST NOT be reported as having "no endpoints" or "no matching Pods" as an error, because the absence of a selector is a valid configuration for manually managed endpoints.

---

## 10. CNI-dependent behavior and limitations of static analysis

### 10.1 What static analysis can establish

| Fact | Derivability |
|---|---|
| Service selector syntax and matching within the input set | **DERIVABLE (local)** |
| Port and protocol validity | **DERIVABLE (local)** |
| Ingress path and backend structure | **DERIVABLE (local)** |
| TLS secret namespace co-location (if secret is in the input set) | **DERIVABLE (local)** |
| NetworkPolicy selector syntax | **DERIVABLE (local)** |
| DNS name syntax | **DERIVABLE (local)** |
| Gateway API CRD schema conformance | **DERIVABLE (local)** if CRD schema is supplied |

### 10.2 What requires cluster state or runtime

| Fact | Requires |
|---|---|
| Whether a Service has live endpoints | Cluster state (EndpointSlice controller) |
| Whether a Pod is Ready | Runtime (kubelet, readiness probe) |
| Whether a CNI enforces NetworkPolicy | CNI plugin configuration |
| Whether an ingress controller is installed | Cluster state |
| Whether a LoadBalancer is provisioned | Cloud controller manager |
| Whether DNS resolves a name | CoreDNS and cluster network |
| Whether traffic is actually forwarded | kube-proxy / eBPF / CNI dataplane |
| Whether a TLS certificate matches its host | Certificate content and SANs |

**K8S-NET-REQ-015:** For every networking finding, KubeCheck MUST label its derivability as `local` or `cluster-dependent`. A `cluster-dependent` finding MUST be `info`-severity and MUST state that the fact cannot be verified offline.

---

## 11. TLS references and routing configuration

### 11.1 TLS in Ingress

`spec.tls` is a list of `IngressTLS` objects with `hosts` and `secretName`. The secret must be of type `kubernetes.io/tls` and contain `tls.crt` and `tls.key`[reference:23][reference:24].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-TLS-001** | `spec.tls[*].secretName` is absent | `error` | A TLS entry requires a secret name. |
| **NET-TLS-002** | `spec.tls[*].hosts` is empty | `warning` | A TLS entry with no hosts cannot match any routing host. |
| **NET-TLS-003** | A host appears in `spec.rules` but not in any `spec.tls[*].hosts` while TLS is present | `warning` | The host is served over HTTP while TLS is configured for other hosts. |

**Heuristic checks:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **NET-TLS-004** | `tls.hosts` and `rules.host` differ | `warning` | The user may intend to serve a default certificate. |
| **NET-TLS-005** | A TLS secret in the input set has a `type` other than `kubernetes.io/tls` | `error` if `type` is explicitly set; `info` if absent | The controller may accept other types in some implementations. |

### 11.2 TLS in Gateway API

A `Gateway` listener with `protocol: HTTPS` or `TLS` requires `tls.certificateRefs` and optionally `tls.mode` (`Terminate` or `Passthrough`).

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **NET-GWTLS-001** | HTTPS listener has no `tls.certificateRefs` | `error` | TLS termination requires a certificate reference. |
| **NET-GWTLS-002** | `tls.mode` is not `Terminate` or `Passthrough` | `error` | Invalid enum. |

---

## 12. Common cross-resource errors and likely connectivity symptoms

| Manifest condition | KubeCheck finding | Likely connectivity symptom | Live-status check required |
|---|---|---|---|
| Service selector matches no supplied Pod | `NET-SVC-008` (`info`) | Empty EndpointSlice; connection refused or 502 | **Cluster-dependent**: `kubectl get endpoints` |
| Service `targetPort` named port not in matching Pod | `NET-SVC-004` (`error` if Pods in set) | Traffic forwarded to wrong port; connection refused | **Cluster-dependent** if Pods not in set |
| Service selector typo (e.g., `app: web` vs `app: web-app`) | `NET-SVC-008` (`info`) | Empty endpoints; 503 through Ingress | **Cluster-dependent** |
| Ingress backend references nonexistent Service | `NET-ING-008` (`info` if Service not in set) | 503 from Ingress controller | **Cluster-dependent** |
| Ingress `ingressClassName` misnamed | `NET-ING-009` (`info`) | Ingress not served; 404 | **Cluster-dependent** |
| TLS secret in wrong namespace | `NET-ING-006` (`error` if secret in set) | Default certificate; browser warning | **Cluster-dependent** |
| NetworkPolicy selects no Pods | `NET-NP-006` (`info`) | Policy has no effect | **Cluster-dependent** |
| NetworkPolicy egress-only without `policyTypes` | `NET-NP-002` (`warning`) | Policy may not block egress as intended | **Cluster-dependent** |
| Headless Service with `targetPort` | `NET-HL-002` (`info`) | Port ignored; DNS returns Pod IPs | **Cluster-dependent** |
| ExternalName missing `externalName` | `NET-EXT-002` (`error`) | Service cannot resolve | — |

**K8S-NET-REQ-016:** Every row in §12 MUST distinguish the manifest finding from the runtime symptom. A manifest finding is what KubeCheck can report; a runtime symptom requires live status and MUST NOT be claimed as a KubeCheck finding (`K8S-WORKLOAD-REQ-017`).

---

## 13. Consolidated finding register

### 13.1 Deterministic findings

| Finding ID | Family | Severity |
|---|---|---|
| NET-SVC-001 | Service type enum | `error` |
| NET-SVC-002 | Service port range | `error` |
| NET-SVC-003 | targetPort numeric range | `error` |
| NET-SVC-004 | Named targetPort resolution | `error`/`warning` |
| NET-SVC-005 | Protocol enum | `error` |
| NET-SVC-006 | Port name uniqueness | `error` |
| NET-SVC-007 | Port/protocol uniqueness | `error` |
| NET-NP-001 | nodePort on ClusterIP | `error` |
| NET-NP-002 | nodePort out of range | `error` |
| NET-LB-001 | externalTrafficPolicy enum | `error` |
| NET-LB-002 | loadBalancerSourceRanges CIDR | `error` |
| NET-ING-001 | Ingress backend missing | `error` |
| NET-ING-002 | Ingress backend port missing | `error` |
| NET-ING-003 | pathType enum | `error` |
| NET-ING-006 | TLS secret namespace mismatch | `error` |
| NET-GW-001 | Gateway listener protocol | `error` |
| NET-GW-003 | BackendRef port range | `error` |
| NET-GW-004 | HTTPS listener without TLS | `error` |
| NET-NP-003 | NetworkPolicy port range | `error` |
| NET-NP-004 | Ingress peer combination | `error` |
| NET-NP-005 | Egress peer combination | `error` |
| NET-HL-001 | Headless + LoadBalancer | `error` |
| NET-EXT-002 | ExternalName missing | `error` |
| NET-EXT-003 | externalName invalid DNS | `error` |
| NET-TLS-001 | TLS secretName absent | `error` |
| NET-GWTLS-001 | HTTPS listener no certificate | `error` |
| NET-GWTLS-002 | TLS mode enum | `error` |

### 13.2 Heuristic and informational findings

| Finding ID | Family | Severity |
|---|---|---|
| NET-SVC-008 | Selector matches no supplied Pod | `info` |
| NET-NP-003 | nodePort conflict in input set | `warning` |
| NET-LB-003 | loadBalancerIP on non-LB | `warning` |
| NET-LB-004 | externalTrafficPolicy Local advisory | `info` |
| NET-ING-004 | TLS secret not in input set | `info` |
| NET-ING-005 | TLS host without rule host | `warning` |
| NET-ING-007 | TLS host / rule host divergence | `warning` |
| NET-ING-008 | Backend Service not in input set | `info` |
| NET-ING-009 | ingressClassName omitted | `info` |
| NET-NP-002 | Egress without policyTypes | `warning` |
| NET-NP-006 | NetworkPolicy selects no supplied Pod | `info` |
| NET-HL-002 | Headless with targetPort | `info` |
| NET-EXT-001 | ExternalName with selector | `warning` |
| NET-TLS-002 | TLS hosts empty | `warning` |
| NET-TLS-003 | Rule host not in TLS hosts | `warning` |
| NET-TLS-004 | TLS/rule host divergence | `warning` |
| NET-TLS-005 | TLS secret type not kubernetes.io/tls | `error`/`info` |
| NET-DNS-001 | Ambiguous cross-namespace reference | `warning` |

---

## 14. Valid and invalid configuration examples

### 14.1 Valid Service with selector and named targetPort

```yaml
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: production
spec:
  type: ClusterIP
  selector:
    app: web
  ports:
  - name: http
    port: 80
    targetPort: http
    protocol: TCP