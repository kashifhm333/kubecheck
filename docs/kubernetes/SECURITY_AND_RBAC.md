# KubeCheck — Security and RBAC

**Document ID:** K8S-SEC-001  
**File:** `docs/kubernetes/SECURITY_AND_RBAC.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Kubernetes Security Architecture  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`
- `docs/kubernetes/WORKLOADS_AND_PODS.md`
- `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md`
- `docs/kubernetes/CONFIGURATION_AND_STORAGE.md`
- `docs/kubernetes/KUBERNETES_SCHEMA_VALIDATION.md`
- `docs/kubernetes/KUBERNETES_API_VERSION_LIFECYCLE.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/security/THREAT_MODEL.md`
- `docs/security/SECURITY_MODEL.md`
- `docs/testing/DETERMINISM_TESTS.md`

---

## 1. Purpose and scope

This document defines KubeCheck's **Kubernetes security knowledge and analysis boundaries**. It specifies the RBAC, security-context, Pod Security Standards, secret-handling, image-security, host-access, and network-isolation checks that KubeCheck can perform from manifests alone, and the security facts it **cannot** establish offline.

**Governing principle:**

> **K8S-SEC-REQ-001 — KubeCheck MUST NOT claim that a manifest alone proves the actual effective permissions of a live identity or the security of an image.** A RoleBinding declares intent; whether the bound identity actually has the permission depends on the live Role/ClusterRole, aggregated ClusterRoles, admission webhooks, and the authorizer chain. An image reference declares provenance; whether the image is safe depends on its contents, signatures, and supply-chain policy. KubeCheck validates the manifest, not the cluster's authorization outcome or image contents.

**In scope:** ServiceAccounts, Roles, ClusterRoles, RoleBindings, ClusterRoleBindings; API groups, resources, verbs, resource names, subresources; namespace and cluster-scope binding semantics; Pod and container security contexts; privileged mode, capabilities, host namespaces, hostPath, privilege escalation; non-root execution, read-only root filesystems, seccomp; Pod Security Standards (PSS) and version-dependent behavior; Secret references and exposure; image security policy references; host access and dangerous mounts; NetworkPolicy isolation; misconfigurations that create excessive privilege.

**Out of scope:** live authorization outcomes, admission webhook behavior, image vulnerability scanning, runtime security, and Secret encryption-at-rest (a cluster-level concern). These are referenced but not claimed as KubeCheck capabilities.

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **Role** | A namespaced grouping of PolicyRules that can be referenced by a RoleBinding. |
| **ClusterRole** | A cluster-scoped grouping of PolicyRules that can be referenced by a RoleBinding or ClusterRoleBinding, or used for non-resource URLs. |
| **RoleBinding** | A namespaced binding of a Role or ClusterRole to subjects (Users, Groups, ServiceAccounts) within a namespace. |
| **ClusterRoleBinding** | A cluster-scoped binding of a ClusterRole to subjects across all namespaces. |
| **PolicyRule** | A rule specifying `apiGroups`, `resources`, `verbs`, optional `resourceNames`, and optional `nonResourceURLs`. |
| **Verbs** | The actions a rule permits: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`, `deletecollection`, and the escalation-sensitive `escalate`, `bind`, `impersonate`. |
| **Subresource** | A sub-path of a resource, written as `resource/subresource` (e.g., `pods/log`, `deployments/status`). |
| **Effective permissions** | The actual permissions an identity has after all bindings, aggregated ClusterRoles, admission webhooks, and the authorizer chain are evaluated. |
| **Manifest-level permission intent** | The permissions a Role or ClusterRole declares, and the bindings that reference them. |
| **PSS** | Pod Security Standards: `privileged`, `baseline`, `restricted`. |
| **PSA** | Pod Security Admission, the built-in admission controller that enforces PSS at namespace level. |

**Requirement IDs** use `K8S-SEC-REQ-NNN`. Finding IDs use `SEC-RBAC-*`, `SEC-CTX-*`, `SEC-PSS-*`, `SEC-SECRET-*`, `SEC-IMG-*`, `SEC-HOST-*`, `SEC-NET-*`, and `SEC-MISCONFIG-*`.

---

## 3. RBAC: Roles, ClusterRoles, and bindings

### 3.1 PolicyRule structure

A Role or ClusterRole contains a `rules` list of `PolicyRule` objects. Each `PolicyRule` has the following fields:

| Field | Type | Required | Semantics |
|---|---|---|---|
| `apiGroups` | `[]string` | Yes | API groups containing the resources. `""` means the core group; `"*"` means all groups. |
| `resources` | `[]string` | Yes | Resources the rule applies to. `"*"` means all resources. Subresources are written as `resource/subresource`. |
| `verbs` | `[]string` | Yes | Verbs that apply to all resources in the rule. `"*"` means all verbs. |
| `resourceNames` | `[]string` | No | A whitelist of resource names. An empty set means all names. |
| `nonResourceURLs` | `[]string` | No | URL paths for non-resource endpoints; only valid in a ClusterRole referenced by a ClusterRoleBinding. |

The Role API documentation confirms that `apiGroups`, `resources`, and `verbs` are required, that `"*"` is permitted in each, and that `resourceNames` is an optional whitelist[reference:0].

**K8S-SEC-REQ-002:** KubeCheck MUST validate that every `PolicyRule` has non-empty `apiGroups`, `resources`, and `verbs`, and MUST reject a rule that sets both `resources` and `nonResourceURLs`.

### 3.2 Verbs and escalation-sensitive verbs

The standard verbs are `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`, and `deletecollection`. Three verbs are **escalation-sensitive** because they can grant permissions the subject does not already hold:

| Verb | Effect | Risk |
|---|---|---|
| `escalate` on `roles`/`clusterroles` | Allows creating or updating a Role/ClusterRole with permissions the subject does not hold. | Direct privilege escalation. |
| `bind` on `roles`/`clusterroles` | Allows creating a RoleBinding/ClusterRoleBinding to a role with more permissions than the subject holds. | Binding to `cluster-admin` or another powerful role. |
| `impersonate` on `users`, `groups`, `serviceaccounts` | Allows acting as another identity. | Horizontal and vertical privilege escalation. |

Kubernetes' RBAC good-practices guidance explicitly warns that `escalate`, `bind`, and `impersonate` "may all potentially lead to privilege escalation and should be tightly controlled"[reference:1][reference:2].

**Deterministic checks:**

| Finding ID | Check | Severity | Evidence | Confidence |
|---|---|---|---|---|
| **SEC-RBAC-001** | A `PolicyRule` includes `escalate` on `roles` or `clusterroles` | `warning` | Rule index, verb, resource, and the role's name. | Deterministic: the verb is present. |
| **SEC-RBAC-002** | A `PolicyRule` includes `bind` on `roles` or `clusterroles` | `warning` | Same as above. | Deterministic. |
| **SEC-RBAC-003** | A `PolicyRule` includes `impersonate` on `users`, `groups`, or `serviceaccounts` | `warning` | Same as above. | Deterministic. |
| **SEC-RBAC-004** | A `PolicyRule` uses `"*"` in `verbs` | `warning` | Rule index and the wildcard field. | Deterministic. |
| **SEC-RBAC-005** | A `PolicyRule` uses `"*"` in `resources` | `warning` | Rule index. | Deterministic. |
| **SEC-RBAC-006** | A `PolicyRule` uses `"*"` in `apiGroups` | `warning` | Rule index. | Deterministic. |

**K8S-SEC-REQ-003:** The wildcard and escalation-verb checks MUST be `warning`, not `error`, because they are syntactically valid and may be intentional for infrastructure roles. The finding MUST state that the rule grants capabilities that should be justified.

**False-positive risks:** A `cluster-admin`-equivalent role may legitimately use wildcards. A CI/CD role may legitimately need `escalate` to apply manifests. The finding is advisory; it does not claim the role is misconfigured.

**Remediation:** Replace `"*"` with the specific verbs and resources the workload requires. If `escalate`, `bind`, or `impersonate` is necessary, document the justification and restrict the role's subjects.

### 3.3 Subresources

Subresources are addressed in `resources` using the `resource/subresource` form. Common subresources include `pods/log`, `pods/exec`, `pods/portforward`, `deployments/status`, `deployments/scale`, and `nodes/proxy`. Granting `pods/exec` or `pods/attach` is equivalent to granting interactive access to a running container and is a high-risk permission.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-RBAC-007** | A rule grants `create` on `pods/exec` or `pods/attach` | `warning` | Interactive access to containers is a high-risk capability. |
| **SEC-RBAC-008** | A rule grants `get` or `list` on `secrets` | `warning` | Reading Secrets exposes credentials. |
| **SEC-RBAC-009** | A rule grants `create` on `pods` without `resourceNames` or namespace scoping | `warning` | Any Pod creation can mount any ServiceAccount in the namespace and access its token. |

**K8S-SEC-REQ-004:** A rule that grants `create` on `pods` without `resourceNames` MUST be flagged as a privilege-escalation surface, because a subject who can create a Pod can mount any ServiceAccount in that namespace and obtain its token.

### 3.4 Namespace versus cluster scope

A **RoleBinding** grants permissions within a single namespace. A **ClusterRoleBinding** grants permissions across all namespaces and for cluster-scoped resources. Kubernetes' RBAC good practices recommend assigning permissions at the namespace level where possible and using RoleBindings rather than ClusterRoleBindings[reference:3].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-RBAC-010** | A `ClusterRoleBinding` binds a ClusterRole to a `ServiceAccount` subject | `warning` | The ServiceAccount's token is usable from any namespace where the Pod runs; a cluster-wide binding expands the blast radius. |
| **SEC-RBAC-011** | A `RoleBinding` references a `ClusterRole` with cluster-scoped resources (e.g., `nodes`) | `info` | A RoleBinding can reference a ClusterRole but only grants namespaced access; the finding states the scope limitation. |
| **SEC-RBAC-012** | A `ClusterRoleBinding` binds to a subject in `system:masters` | `error` | `system:masters` bypasses all RBAC checks and cannot be revoked by deleting bindings. |

The good-practices guidance states that adding users to `system:masters` gives unrestricted superuser access that cannot be revoked by removing RoleBindings or ClusterRoleBindings, and that membership also bypasses authorization webhooks[reference:4].

**K8S-SEC-REQ-005:** A `ClusterRoleBinding` to `system:masters` MUST be an `error`-severity finding, not a warning, because the permission cannot be revoked through the RBAC API.

---

## 4. Pod and container security contexts

### 4.1 Two levels and override precedence

`securityContext` exists at the **Pod level** (`spec.securityContext`) and the **container level** (`spec.containers[i].securityContext`). Container-level settings override Pod-level settings for fields present at both levels. Some fields are **container-only**: `readOnlyRootFilesystem`, `allowPrivilegeEscalation`, and `capabilities` are meaningful only at the container level[reference:5].

**K8S-SEC-REQ-006:** KubeCheck MUST validate each security-context field against its permitted level. A Pod-level `readOnlyRootFilesystem` or `allowPrivilegeEscalation` MUST produce a `warning` stating that the field is ignored at the Pod level and must be set per container.

### 4.2 Privileged mode

`securityContext.privileged: true` gives the container "nearly all host capabilities and effectively removes the isolation boundary"[reference:6]. It is appropriate only for a small set of infrastructure workloads (CNI plugins, kubelet-adjacent tooling).

**Deterministic check:**

| Finding ID | Check | Severity | Evidence | Confidence |
|---|---|---|---|---|
| **SEC-CTX-001** | A container sets `privileged: true` | `warning` | Container name and JSON path. | Deterministic. |

**False-positive risks:** A privileged DaemonSet for a CNI plugin or storage driver is a legitimate infrastructure workload. The finding MUST include the container name and MUST NOT claim the workload is compromised; it states that privileged mode removes the isolation boundary and requires justification.

**Remediation:** Replace `privileged: true` with the minimum set of Linux capabilities the workload needs (`capabilities.add`), and drop `ALL` by default.

### 4.3 Linux capabilities

`capabilities` has `add`, `drop`, and `default`? In the current API, `add` and `drop` are the fields. `drop: ["ALL"]` removes all capabilities; `add` re-adds only the required ones.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-CTX-002** | `capabilities.add` includes `ALL` | `warning` | Equivalent to granting all kernel capabilities. |
| **SEC-CTX-003** | `capabilities.add` includes `SYS_ADMIN` | `warning` | `CAP_SYS_ADMIN` is the broadest capability and is a common container-escape primitive. |
| **SEC-CTX-004** | `capabilities.drop` is absent or does not include `ALL` | `info` | The container retains the runtime's default capability set. |

**K8S-SEC-REQ-007:** The `capabilities.add: ["ALL"]` check MUST be `warning`, not `error`, because the field is syntactically valid and the API server accepts it. The finding states that the container has all capabilities and recommends `drop: ["ALL"]` with a minimal `add` list.

### 4.4 Host namespaces: hostNetwork, hostPID, hostIPC

`spec.hostNetwork`, `spec.hostPID`, and `spec.hostIPC` share the node's network, process, and IPC namespaces with the container. They are blocked by the **baseline** Pod Security Standard because they "can be used to access shared data or escalate privileges"[reference:7][reference:8].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-CTX-005** | `hostNetwork: true` | `warning` | The container sees all host network interfaces and can bind host ports. |
| **SEC-CTX-006** | `hostPID: true` | `warning` | The container can see and signal all host processes. |
| **SEC-CTX-007** | `hostIPC: true` | `warning` | The container can access host IPC namespaces. |

**False-positive risks:** Node agents, log collectors, and monitoring DaemonSets legitimately use `hostPID` or `hostNetwork`. The finding MUST name the field and state that it is a baseline-profile violation, not that the workload is malicious.

### 4.5 HostPath and dangerous mounts

`hostPath` mounts a directory or file from the node into the container. Mounting sensitive host paths (e.g., `/`, `/etc`, `/var/run/docker.sock`, `/var/lib/kubelet`) allows the container to read credentials, modify host configuration, or escape to the host.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-CTX-008** | A `hostPath` volume mounts `/` or `/etc` | `error` | Direct host filesystem access; effectively root on the node. |
| **SEC-CTX-009** | A `hostPath` volume mounts `/var/run/docker.sock` or `/run/containerd/containerd.sock` | `error` | Container runtime socket access enables container escape. |
| **SEC-CTX-010** | A `hostPath` volume mounts `/var/lib/kubelet` | `error` | Kubelet directory access exposes credentials and pod data. |
| **SEC-CTX-011** | A `hostPath` volume mounts any other path | `warning` | HostPath mounts expand the attack surface; the finding names the path. |

**K8S-SEC-REQ-008:** A `hostPath` mount of `/`, `/etc`, a container runtime socket, or `/var/lib/kubelet` MUST be `error`-severity, because the mount provides a direct path to node compromise. Other `hostPath` mounts are `warning`-severity.

### 4.6 Privilege escalation

`allowPrivilegeEscalation` controls whether a process can gain more privileges than its parent. When `true` (or unset, which defaults to `true` unless the container is privileged or has `CAP_SYS_ADMIN`), a process can use `setuid` binaries or file capabilities to elevate[reference:9].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-CTX-012** | `allowPrivilegeEscalation` is `true` or unset | `info` | The container may escalate privileges; the finding recommends `false`. |
| **SEC-CTX-013** | `allowPrivilegeEscalation: false` but `capabilities.add` includes `SYS_ADMIN` | `warning` | `CAP_SYS_ADMIN` can be used to escalate despite the flag. |

### 4.7 Non-root execution

`runAsNonRoot: true` causes the kubelet to refuse to start the container if the image's user is root (UID 0) and no non-root `runAsUser` is set[reference:10]. `runAsUser` sets a specific UID.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-CTX-014** | `runAsNonRoot: true` but `runAsUser: 0` | `error` | The kubelet rejects the container. |
| **SEC-CTX-015** | Neither `runAsNonRoot: true` nor a non-zero `runAsUser` is set | `info` | The container runs as the image's default user, which may be root. |
| **SEC-CTX-016** | `runAsUser` is set to `0` without `runAsNonRoot` | `warning` | The container runs as root. |

### 4.8 Read-only root filesystem

`readOnlyRootFilesystem: true` mounts the container's root filesystem read-only, preventing an attacker from writing tools or modifying binaries[reference:11]. It is a container-level field.

**Deterministic check:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-CTX-017** | `readOnlyRootFilesystem` is `false` or unset | `info` | The container can write to its root filesystem; a hardening opportunity. |

**Remediation:** Set `readOnlyRootFilesystem: true` and mount writable `emptyDir` volumes at the specific paths the application needs.

### 4.9 Seccomp

`seccompProfile` restricts the system calls a container can make. `RuntimeDefault` uses the container runtime's default profile; `Localhost` uses a profile from the node; `Unconfined` disables seccomp.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-CTX-018** | `seccompProfile.type` is `Unconfined` | `warning` | Seccomp is disabled; the container can make any syscall. |
| **SEC-CTX-019** | `seccompProfile` is absent | `info` | The container uses the runtime default (if configured) or unconfined. |

**K8S-SEC-REQ-009:** The `seccompProfile: Unconfined` check MUST be `warning`, not `error`, because the field is syntactically valid and the API server accepts it. The finding states that seccomp is disabled and recommends `RuntimeDefault`.

---

## 5. Pod Security Standards and version-dependent behavior

### 5.1 The three profiles

Kubernetes defines three Pod Security Standards:

| Profile | Description | Representative controls |
|---|---|---|
| **Privileged** | No restrictions; reserved for system and infrastructure namespaces. | No field restrictions. |
| **Baseline** | Prevents known privilege escalations while remaining broadly compatible. | Blocks `privileged`, `hostPID`, `hostIPC`, `hostNetwork`, and most `hostPath` mounts[reference:12]. |
| **Restricted** | Heavily restricted; enforces baseline plus non-root, no privilege escalation, seccomp, and volume-type limits[reference:13]. | Requires `runAsNonRoot`, `allowPrivilegeEscalation: false`, `seccompProfile`, and dropped capabilities. |

### 5.2 Enforcement modes

PSA enforces PSS through namespace labels: `pod-security.kubernetes.io/enforce`, `pod-security.kubernetes.io/warn`, and `pod-security.kubernetes.io/audit`. The `enforce` mode rejects violating Pods; `warn` allows but warns; `audit` allows and records[reference:14].

**K8S-SEC-REQ-010:** KubeCheck MUST validate a namespace's PSS labels if the Namespace object is in the input set, and MUST report the profile and enforcement mode. It MUST NOT assume that a namespace without PSS labels is unenforced, because the cluster may apply a default.

### 5.3 Version-dependent PSS behavior

The PSS profiles evolve across Kubernetes versions. For example, the `restricted` profile's seccomp requirement and the `baseline` profile's hostPath restrictions may differ across releases. PSA allows pinning the profile version via `pod-security.kubernetes.io/enforce-version` (e.g., `v1.36`).

**Heuristic check (SEC-PSS-001):** A Pod that violates the `restricted` profile SHOULD produce an `info` finding naming the specific violated control, but only when the namespace's PSS labels are in the input set or the user supplies a target profile. The finding MUST state that the actual admission outcome depends on the cluster's PSA configuration.

**K8S-SEC-REQ-011:** KubeCheck MUST NOT claim that a Pod will be rejected by PSA. It may report that the Pod's manifest fields are inconsistent with a named profile, but the admission outcome is cluster-dependent.

---

## 6. Secrets: references and exposure risks

### 6.1 Reference forms

A Pod references a Secret via `env[*].valueFrom.secretKeyRef`, `envFrom[*].secretRef`, a `secret` volume, or a `projected` volume source. A Secret must be in the **same namespace** as the Pod; `SecretKeySelector` has no `namespace` field.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-SECRET-001** | `secretKeyRef` or `secretRef` includes a `namespace` field | `error` | The API server rejects the unknown field. |
| **SEC-SECRET-002** | A referenced Secret is in the input set but in a different namespace | `error` | Cross-namespace references are not permitted. |
| **SEC-SECRET-003** | A referenced Secret is not in the input set | `info` | Existence is cluster-dependent. |

### 6.2 Environment variables versus volume mounts

Secrets consumed as environment variables are visible to every process in the container and to child processes; they are more likely to leak through crash dumps or logs. Secrets mounted as files are isolated by file permissions and are updated when the Secret changes. Kubernetes guidance recommends volume mounts for sensitive data and reserves environment variables for simple, non-critical configuration[reference:15].

**Heuristic checks:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **SEC-SECRET-004** | A Secret is consumed via `env` or `envFrom` rather than a volume mount | `info` | The application may require environment variables. |
| **SEC-SECRET-005** | A Secret is consumed via `envFrom` on a Pod with multiple containers, exposing it to containers that do not need it | `warning` | All containers share the environment; the finding suggests scoping. |

**K8S-SEC-REQ-012:** A Secret consumed via `envFrom` on a multi-container Pod MUST be a `warning` if only one container's name or image suggests it needs the Secret; the finding recommends defining the `envFrom` only on the container that needs it.

### 6.3 Literal credentials

A credential embedded as a literal `value` in an `env` entry is not protected by Secret semantics and is visible in the manifest, in version control, and in `kubectl get -o yaml`.

**Heuristic check:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **SEC-SECRET-006** | A field named `password`, `token`, `apiKey`, `secret`, or similar is set as a literal `value` | `warning` | The field may be non-sensitive configuration with a misleading name. |

**Remediation rules:**

**K8S-SEC-REQ-013:** Remediation for any Secret-exposure finding MUST NOT echo the Secret value, MUST NOT recommend storing credentials in a ConfigMap, MUST NOT recommend a literal `value`, and MUST recommend one of: a `secretKeyRef`, a Secret volume mount, or an external secret store.

---

## 7. Image security and supply-chain policy references

### 7.1 Image reference and pull policy

KubeCheck can validate image reference syntax deterministically (see `docs/kubernetes/WORKLOADS_AND_PODS.md` §4.2). It cannot determine whether an image is safe, signed, or free of vulnerabilities.

**Heuristic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-IMG-001** | An image uses `:latest` or omits both tag and digest | `warning` | Mutable reference; prevents reproducible deployments. |
| **SEC-IMG-002** | An image uses a digest reference (`@sha256:...`) | `info` | Immutable; the finding notes that digest pinning is the reproducible form. |
| **SEC-IMG-003** | An image registry is not in a documented allowlist (if a policy is supplied) | `info` | The allowlist is policy-dependent; the finding is advisory. |

### 7.2 Supply-chain policy boundary

Kubernetes admission controllers such as Sigstore Policy Controller verify image signatures and attestations before admitting an image[reference:16]. KubeCheck does not perform signature verification; it can only note when a workload relies on an admission controller for supply-chain enforcement.

**K8S-SEC-REQ-014:** KubeCheck MUST NOT claim that an image is signed, verified, or vulnerability-free. Any image-security finding MUST be limited to manifest-level properties (reference syntax, tag mutability, digest presence) and MUST state that image contents and signatures require a registry, scanner, or admission controller.

---

## 8. Host access and dangerous mounts

### 8.1 Host ports

`hostPort` binds a container port to the node's network. It is a scheduling constraint and a potential port conflict. `hostNetwork: true` removes the need for `hostPort` because the container shares the host network.

**Deterministic check:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **SEC-HOST-001** | `hostPort` is set | `info` | The Pod is constrained to nodes where the port is free; the finding notes the scheduling implication. |

### 8.2 Dangerous volume types

Beyond `hostPath`, the following volume types expose host resources: `nfs` (network filesystem; may allow privilege escalation if `no_root_squash` is set on the server), `iscsi`, `fc`, `rbd`, and `csi` with a driver that provides host-level access. KubeCheck can identify the volume type but cannot assess the server-side configuration.

**Heuristic check:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **SEC-HOST-002** | A Pod uses an `nfs` volume | `info` | The NFS server may be properly configured; the finding notes that server-side `no_root_squash` is a known escalation vector. |

---

## 9. NetworkPolicy and isolation considerations

### 9.1 Default-deny posture

By default, a Pod is **non-isolated**: all ingress and egress traffic is allowed. A NetworkPolicy isolates a Pod only when the policy selects it and includes the relevant `policyTypes`[reference:17]. A recommended posture is **default deny**, then allow only what each workload needs[reference:18].

**Heuristic checks:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **SEC-NET-001** | A namespace contains workloads but no NetworkPolicy selects them | `info` | The cluster may enforce isolation through other means (service mesh, CNI policy). |
| **SEC-NET-002** | A NetworkPolicy has an empty `podSelector` and no `ingress`/`egress` rules (default deny) | `info` | The policy is a deliberate default-deny; the finding is informational. |
| **SEC-NET-003** | A NetworkPolicy's `policyTypes` omits `Egress` while the namespace is expected to be egress-isolated | `warning` | The namespace may not require egress isolation. |

### 9.2 CNI enforcement boundary

A NetworkPolicy is enforced only if the CNI plugin supports NetworkPolicy enforcement. KubeCheck cannot verify CNI capabilities offline.

**K8S-SEC-REQ-015:** Every NetworkPolicy finding MUST state that enforcement depends on the CNI plugin, and KubeCheck MUST NOT claim that a policy blocks or allows any specific traffic at runtime.

---

## 10. Misconfigurations that create excessive privilege

| Misconfiguration | Finding ID | Severity | Escalation path |
|---|---|---|---|
| `ClusterRoleBinding` to `cluster-admin` for a ServiceAccount | `SEC-RBAC-013` | `warning` | The ServiceAccount token grants full cluster control. |
| `create` on `pods` without `resourceNames` | `SEC-RBAC-009` | `warning` | Create a Pod that mounts a privileged ServiceAccount. |
| `escalate` on `roles` | `SEC-RBAC-001` | `warning` | Grant yourself any permission. |
| `bind` on `clusterroles` | `SEC-RBAC-002` | `warning` | Bind to `cluster-admin`. |
| `impersonate` on `users` | `SEC-RBAC-003` | `warning` | Act as a cluster administrator. |
| `privileged: true` | `SEC-CTX-001` | `warning` | Full host capabilities; container escape. |
| `hostPath` mount of `/` or a runtime socket | `SEC-CTX-008`/`009` | `error` | Direct host compromise. |
| `hostPID: true` combined with `privileged: true` | `SEC-CTX-001` + `SEC-CTX-006` | `warning` | Signal and ptrace host processes. |
| `automountServiceAccountToken: true` (default) on a Pod that does not need the API | `SEC-RBAC-014` | `info` | The token is available to any process in the container. |
| `Secret` read permission (`get`/`list` on `secrets`) | `SEC-RBAC-008` | `warning` | Read all Secrets in the namespace. |

**SEC-RBAC-013:** A `ClusterRoleBinding` that binds the `cluster-admin` ClusterRole to a `ServiceAccount` MUST be a `warning`-severity finding, because the ServiceAccount's token grants full cluster control and is accessible from any Pod that uses the ServiceAccount.

**SEC-RBAC-014:** A Pod with `automountServiceAccountToken` not explicitly set to `false` SHOULD produce an `info` finding stating that the default ServiceAccount token is mounted and available to any process in the container. Kubernetes guidance recommends setting `automountServiceAccountToken: false` on the default ServiceAccount and enabling it only for workloads that need API access[reference:19][reference:20].

---

## 11. Secure and insecure examples

### 11.1 Secure Pod security context

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: secure-api
spec:
  securityContext:
    runAsNonRoot: true
    seccompProfile:
      type: RuntimeDefault
  containers:
  - name: api
    image: registry.example.com/api@sha256:abc123...
    securityContext:
      runAsUser: 1000
      readOnlyRootFilesystem: true
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]