# KubeCheck — Configuration and Storage

**Document ID:** K8S-STORAGE-001  
**File:** `docs/kubernetes/CONFIGURATION_AND_STORAGE.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Kubernetes Storage and Configuration Diagnostics  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`
- `docs/kubernetes/WORKLOADS_AND_PODS.md`
- `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md`
- `docs/kubernetes/KUBERNETES_SCHEMA_VALIDATION.md`
- `docs/kubernetes/KUBERNETES_API_VERSION_LIFECYCLE.md`
- `docs/kubernetes/CRD_AND_CUSTOM_RESOURCE_HANDLING.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/security/THREAT_MODEL.md`
- `docs/testing/DETERMINISM_TESTS.md`

---

## 1. Purpose and scope

This document defines KubeCheck's **offline checks for configuration resources and persistent storage** in Kubernetes manifests. It covers ConfigMaps, Secrets, environment references, projected volumes, volume mounts, ephemeral volumes, PersistentVolumes (PVs), PersistentVolumeClaims (PVCs), StorageClasses, and relevant Container Storage Interface (CSI) configuration fields.

**Governing distinction:**

> **K8S-STORAGE-REQ-001 — KubeCheck MUST clearly separate deterministic reference errors from runtime binding, permission, attachment, and capacity problems.** A manifest can reference a Secret that does not exist in the input set yet exists in the cluster; a PVC can be structurally valid yet fail to bind because no PV satisfies its request. KubeCheck validates the manifest, not the live storage state.

**In scope:** ConfigMap and Secret reference validation; volume and mount name consistency; read-only versus writable mounts; PVC binding assumptions and access modes; StorageClass references; StatefulSet `volumeClaimTemplates`; optional references; Secret exposure vectors; configuration update behavior; size and schema constraints where authoritative; CSI configuration fields.

**Out of scope:** live PV binding status (see `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md` §13), CSI driver implementation details, cloud provider storage provisioning, and Secret encryption-at-rest configuration (a cluster-level concern).

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **ConfigMap** | A namespaced object that stores non-confidential configuration data as key-value pairs. |
| **Secret** | A namespaced object that stores sensitive data (passwords, tokens, keys) encoded as base64 strings. |
| **Projected volume** | A volume that maps multiple sources (Secret, ConfigMap, downward API, ServiceAccount token) into a single directory. |
| **emptyDir** | An ephemeral volume that shares the Pod's lifetime and is initially empty. |
| **PersistentVolume (PV)** | A cluster-scoped piece of storage with a lifecycle independent of any Pod. |
| **PersistentVolumeClaim (PVC)** | A namespaced request for storage that binds to a PV or triggers dynamic provisioning. |
| **StorageClass** | A cluster-scoped resource that describes a class of storage with a provisioner, parameters, and binding behavior. |
| **CSI** | Container Storage Interface, a standard for exposing storage systems to container workloads. |
| **CSI inline ephemeral volume** | A CSI volume defined directly in the Pod spec with a lifecycle tied to the Pod. |
| **ReadWriteOnce (RWO)** | The volume can be mounted as read-write by a single node. |
| **ReadOnlyMany (ROX)** | The volume can be mounted read-only by many nodes. |
| **ReadWriteMany (RWX)** | The volume can be mounted read-write by many nodes. |
| **ReadWriteOncePod (RWOP)** | The volume can be mounted read-write by a single Pod (Kubernetes 1.22+). |
| **volumeBindingMode** | A StorageClass field controlling when binding occurs: `Immediate` (default) or `WaitForFirstConsumer`. |

**Requirement IDs** use `K8S-STORAGE-REQ-NNN`. Finding IDs use `STG-<FAMILY>-NNN`.

---

## 3. ConfigMaps and Secrets

### 3.1 Structure and mutual exclusivity

A ConfigMap has `data` (UTF-8 strings) and `binaryData` (base64-encoded bytes); a key must appear in at most one of the two. A Secret has `data` (base64-encoded) and `stringData` (write-only convenience for plain strings); `stringData` is merged into `data` on write, with `data` taking precedence for duplicate keys.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-CM-001** | A ConfigMap key appears in both `data` and `binaryData` | `error` | The API server rejects duplicate keys. |
| **STG-CM-002** | A ConfigMap or Secret key is not a valid `[-._a-zA-Z0-9]+` string | `error` | Invalid key format. |
| **STG-SEC-001** | A Secret key appears in both `data` and `stringData` with different values | `warning` | `data` takes precedence; the `stringData` value is silently ignored, which may be a mistake. |
| **STG-SEC-002** | `stringData` is used in a manifest applied with `kubectl apply` | `info` | `stringData` is write-only and is not preserved on read; this is a `kubectl` behavior, not an API rejection. |

### 3.2 Secret type

The `type` field is a string; the built-in types include `Opaque`, `kubernetes.io/service-account-token`, `kubernetes.io/dockercfg`, `kubernetes.io/dockerconfigjson`, `kubernetes.io/basic-auth`, `kubernetes.io/ssh-auth`, `kubernetes.io/tls`, and `bootstrap.kubernetes.io/token`.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-SEC-003** | `type: kubernetes.io/tls` but `tls.crt` or `tls.key` key is absent | `error` | The API server validates required keys for known types. |
| **STG-SEC-004** | `type: kubernetes.io/dockerconfigjson` but `.dockerconfigjson` key is absent | `error` | Same as above. |
| **STG-SEC-005** | `type: kubernetes.io/basic-auth` but `username` or `password` key is absent | `warning` | The keys are required for the type's semantics; some consumers may not enforce them. |

### 3.3 Immutable ConfigMaps and Secrets

The `immutable` field, when `true`, prevents updates to the object's data after creation; only metadata can be modified. This is useful for preventing accidental configuration drift.

**Heuristic check (STG-CM-003):** A ConfigMap or Secret with `immutable: true` that is referenced by a workload whose configuration is expected to change SHOULD produce an `info` finding stating that updates require recreating the object and restarting the Pods.

**K8S-STORAGE-REQ-002:** KubeCheck MUST validate the `immutable` field's type and MUST report the update implications as `info`, not `error`, because immutability is a valid design choice.

---

## 4. Environment references

### 4.1 `env` and `envFrom`

A container references a ConfigMap or Secret via:

- `env[*].valueFrom.configMapKeyRef` — a single key.
- `env[*].valueFrom.secretKeyRef` — a single key.
- `envFrom[*].configMapRef` — all keys from a ConfigMap.
- `envFrom[*].secretRef` — all keys from a Secret.

**Critical namespace rule:** A ConfigMap or Secret must be in the **same namespace** as the Pod; a reference cannot specify a namespace. `ConfigMapKeySelector` and `SecretKeySelector` have `name`, `key`, and `optional` fields but **no `namespace` field**. Attempting to set `namespace` inside `secretKeyRef` or `configMapKeyRef` produces a validation error: `unknown field "namespace" in io.k8s.api.core.v1.SecretKeySelector`[reference:0].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-ENV-001** | `configMapKeyRef` or `secretKeyRef` includes a `namespace` field | `error` | The API server rejects the unknown field. |
| **STG-ENV-002** | The referenced ConfigMap or Secret is in the input set but in a different namespace from the Pod | `error` | Cross-namespace references are not permitted. |
| **STG-ENV-003** | The referenced ConfigMap or Secret is not in the input set | `info` | Existence is cluster-dependent (`K8S-API-REQ-023`). |
| **STG-ENV-004** | `envFrom` references a ConfigMap or Secret whose keys include invalid environment variable names | `warning` | Keys with invalid `C_IDENTIFIER` names are skipped by the kubelet; the Pod starts but the variables are absent. |

**K8S-STORAGE-REQ-003:** The namespace-mismatch check (`STG-ENV-002`) MUST only fire when both the referencing Pod and the referenced object are in the input set. If the referenced object is not in the input set, the finding is `info`, not `error`.

### 4.2 Optional references

`configMapKeyRef.optional` and `secretKeyRef.optional` control whether the Pod fails to start when the referenced object or key is missing. `envFrom[*].optional` controls bulk import behavior. The default is `false`: a missing reference causes `CreateContainerConfigError` and the Pod never starts[reference:1].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-ENV-005** | A required (non-optional) `secretKeyRef` points to a key not present in a Secret that **is** in the input set | `error` | The kubelet will fail to assemble the container config. |
| **STG-ENV-006** | An optional reference points to a key not present | `info` | The Pod starts; the variable is absent. The application must handle absence. |

**K8S-STORAGE-REQ-004:** A required reference to a missing key in a Secret that is in the input set is a deterministic `error`, because the key's absence is visible offline. A required reference to a missing object is `info` when the object is not in the input set.

---

## 5. Volume and mount consistency

### 5.1 Volume sources

A Pod declares `spec.volumes` at the Pod level. Supported sources include `configMap`, `secret`, `projected`, `emptyDir`, `hostPath`, `persistentVolumeClaim`, `csi` (inline ephemeral), `downwardAPI`, `nfs`, `iscsi`, and others. Each volume has a `name` that must be unique within the Pod and must be a valid DNS label.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-VOL-001** | Two volumes share the same `name` | `error` | The API server rejects duplicate volume names. |
| **STG-VOL-002** | A volume name is not a valid DNS label | `error` | Invalid volume name. |
| **STG-VOL-003** | A volume has more than one source type (e.g., both `configMap` and `secret`) | `error` | A volume must have exactly one source. |
| **STG-VOL-004** | A `configMap` or `secret` volume references a name not in the input set | `info` | Existence is cluster-dependent. |
| **STG-VOL-005** | A `configMap` or `secret` volume references a name in the input set but in a different namespace | `error` | Volume sources are namespace-local. |

### 5.2 Volume mounts

A container's `volumeMounts` must reference a declared volume. `mountPath` must be absolute and must not collide with another mount in the same container.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-MNT-001** | `volumeMounts[*].name` references a volume not declared in `spec.volumes` | `error` | The kubelet rejects the Pod. |
| **STG-MNT-002** | `mountPath` is not absolute | `error` | The API server requires an absolute path. |
| **STG-MNT-003** | Two mounts in the same container share a `mountPath` | `error` | The kubelet rejects duplicate mount paths. |
| **STG-MNT-004** | A declared volume is not mounted by any container | `warning` | The volume is unused; likely a configuration error. |
| **STG-MNT-005** | `subPath` is set but the volume source does not support it | `error` | Not all volume types support `subPath`. |

### 5.3 Read-only versus writable mounts

`volumeMounts[*].readOnly` controls whether the mount is read-only. For `configMap` and `secret` volumes, the `readOnly` field also exists at the **volume source** level (`configMap.readOnly`, `secret.readOnly`).

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-MNT-006** | A `configMap` or `secret` volume is mounted writable (`readOnly: false`) | `warning` | ConfigMaps and Secrets are not writable by the kubelet; the mount is effectively read-only regardless of the field. |
| **STG-MNT-007** | A `persistentVolumeClaim` volume is mounted read-write but the PVC's access mode is `ReadOnlyMany` | `warning` | The mount may fail at runtime if the underlying volume does not support write access. |

**K8S-STORAGE-REQ-005:** The mount-readOnly check (`STG-MNT-006`) MUST be a `warning`, not an `error`, because the API server accepts the configuration and the kubelet ignores the writable setting for ConfigMap and Secret volumes.

---

## 6. Projected volumes and ephemeral volumes

### 6.1 Projected volumes

A `projected` volume maps multiple sources into a single directory. Sources include `secret`, `configMap`, `downwardAPI`, `serviceAccountToken`, and `clusterTrustBundle`. Each source has a `name` (for Secret/ConfigMap) or a path configuration.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-PROJ-001** | A projected source has `optional: false` and references a name not in the input set | `info` | Existence is cluster-dependent; the Pod will fail to start if the object is missing in the cluster. |
| **STG-PROJ-002** | Two projected sources map to the same relative path | `error` | The kubelet rejects conflicting paths. |
| **STG-PROJ-003** | A `serviceAccountToken` source has `expirationSeconds` below 600 or above 3600 | `warning` | The kubelet enforces a minimum of 600 seconds; values outside the range may be rejected or clamped. |

### 6.2 emptyDir

`emptyDir` is an ephemeral volume that shares the Pod's lifetime. `sizeLimit` bounds the volume's storage usage; when the limit is exceeded, the Pod is evicted. The `medium` field can be `Memory` to use a tmpfs.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-ED-001** | `sizeLimit` is not a valid resource quantity | `error` | Invalid quantity syntax. |
| **STG-ED-002** | `medium` is not `Memory` or empty | `error` | Invalid enum. |
| **STG-ED-003** | `emptyDir` is used for data that must survive Pod restarts (heuristic) | `info` | The data is lost when the Pod is deleted; the finding is advisory. |

**K8S-STORAGE-REQ-006:** The `sizeLimit` of an `emptyDir` with `medium: Memory` is bounded by the sum of the memory limits of all containers in the Pod in addition to `sizeLimit` itself[reference:2]. KubeCheck MUST report this interaction as an `info` finding when both are present.

### 6.3 CSI inline ephemeral volumes

A CSI inline ephemeral volume is defined directly in the Pod spec via `spec.volumes[*].csi`. It has a lifecycle tied to the Pod: the volume is created when the Pod is scheduled and destroyed when the Pod is deleted. The `driver` field must match a `CSIDriver` object with `volumeLifecycleModes` containing `Ephemeral`.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-CSI-001** | `csi.driver` is absent | `error` | The driver is required. |
| **STG-CSI-002** | `csi.readOnly` is set to `false` on a volume that the driver only supports as read-only | `info` | The driver's capabilities are not visible offline; the finding is advisory. |
| **STG-CSI-003** | `csi.volumeAttributes` contains a key that the driver does not recognize | `info` | Driver-specific attributes are not validated offline. |

**K8S-STORAGE-REQ-007:** A CSI inline ephemeral volume is **not** a PVC; KubeCheck MUST NOT require a `persistentVolumeClaim` source for it and MUST NOT report it as an orphaned PVC.

---

## 7. PersistentVolumes, PersistentVolumeClaims, and StorageClasses

### 7.1 PVC structure and access modes

A PVC has `spec.accessModes`, `spec.resources.requests.storage`, `spec.storageClassName`, `spec.volumeMode`, and optionally `spec.selector` and `spec.volumeName`. The access modes are:

| Mode | Abbreviation | Meaning |
|---|---|---|
| `ReadWriteOnce` | RWO | Mounted read-write by a single **node**. |
| `ReadOnlyMany` | ROX | Mounted read-only by many nodes. |
| `ReadWriteMany` | RWX | Mounted read-write by many nodes. |
| `ReadWriteOncePod` | RWOP | Mounted read-write by a single **Pod** (Kubernetes 1.22+). |

`ReadWriteOncePod` cannot be combined with other access modes[reference:3].

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-PVC-001** | `accessModes` is empty | `error` | At least one access mode is required. |
| **STG-PVC-002** | `accessModes` contains a value not in the enum | `error` | Invalid access mode. |
| **STG-PVC-003** | `ReadWriteOncePod` is combined with another access mode | `error` | The API server rejects the combination. |
| **STG-PVC-004** | `resources.requests.storage` is absent or not a valid quantity | `error` | Storage request is required. |
| **STG-PVC-005** | `volumeMode` is not `Filesystem` or `Block` | `error` | Invalid enum. |

### 7.2 StorageClass references

`spec.storageClassName` selects a StorageClass. If `storageClassName` is `""` (empty string), the PVC explicitly requests no StorageClass and will only bind to a PV with no class. If the field is omitted, the default StorageClass is used (if one is marked default).

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-PVC-006** | `storageClassName` references a StorageClass not in the input set | `info` | Existence is cluster-dependent; the PVC may still bind if the class exists in the cluster. |
| **STG-PVC-007** | `storageClassName` is `""` and the PVC requests dynamic provisioning | `info` | An empty class means no dynamic provisioning; the PVC will remain `Pending` unless a matching static PV exists. |

**K8S-STORAGE-REQ-008:** A missing StorageClass reference is `info`, not `error`, because KubeCheck cannot verify the cluster's StorageClass inventory. The finding MUST state that the PVC may fail to bind if the class is absent in the target cluster.

### 7.3 volumeBindingMode

A StorageClass's `volumeBindingMode` controls when binding occurs: `Immediate` (bind as soon as the PVC is created) or `WaitForFirstConsumer` (delay binding until a Pod using the PVC is scheduled). `WaitForFirstConsumer` is recommended for topology-constrained volumes because it ensures the volume is provisioned in the same zone as the Pod[reference:4].

**Heuristic check (STG-PVC-008):** A StatefulSet using a StorageClass with `volumeBindingMode: Immediate` and a topology-constrained provisioner SHOULD produce an `info` finding stating that the volume may be provisioned in a zone that cannot schedule the Pod.

### 7.4 StatefulSet `volumeClaimTemplates`

A StatefulSet's `volumeClaimTemplates` is a list of PVC templates. The StatefulSet controller creates one PVC per replica per template, named `<template-name>-<statefulset-name>-<ordinal>`. The template's `metadata.name` becomes the **claim name** that must be referenced by a `volumeMount` with `claimName` equal to the template name.

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-STS-001** | A `volumeClaimTemplates` entry has no matching `volumeMount` in any container | `error` | The PVC is created but never used; this is almost always a configuration error. |
| **STG-STS-002** | A `volumeMount` references a `claimName` that matches no template | `error` | The mount will fail because no PVC is generated. |
| **STG-STS-003** | A `volumeClaimTemplates` entry omits `accessModes` or `resources.requests.storage` | `error` | Required fields for PVC creation. |

**K8S-STORAGE-REQ-009:** The StatefulSet template-to-mount matching check MUST compare the `metadata.name` of the template with the `claimName` of the volume mount. The generated PVC name includes the StatefulSet name and ordinal, but the **mount reference** uses the template name, not the generated name.

### 7.5 PV and PVC binding assumptions

A PVC binds to a PV when the PV's capacity, access modes, StorageClass, and selector match the PVC's request. This binding is **cluster-dependent**: KubeCheck cannot determine whether a matching PV exists or whether dynamic provisioning will succeed.

**Deterministic checks (input-set only):**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-PV-001** | A PV in the input set has `spec.claimRef` pointing to a PVC that is not in the input set | `info` | The PVC may exist in the cluster. |
| **STG-PV-002** | A PVC in the input set has `spec.volumeName` pointing to a PV that is not in the input set | `info` | The PV may exist in the cluster. |
| **STG-PV-003** | A PV's `accessModes` do not include the PVC's requested mode | `error` if both are in the input set | The PVC cannot bind to this PV. |
| **STG-PV-004** | A PV's `storageClassName` differs from the PVC's `storageClassName` | `error` if both are in the input set | The PVC cannot bind to this PV. |

**K8S-STORAGE-REQ-010:** Binding checks are deterministic **only when both the PV and PVC are in the input set**. If either is absent, the finding is `info` and states that binding depends on cluster state.

---

## 8. CSI configuration fields

### 8.1 CSI persistent volume fields

A CSI PV has `spec.csi` with the following fields:

| Field | Purpose | Required |
|---|---|---|
| `driver` | The CSI driver name. | Yes |
| `volumeHandle` | Unique volume identifier passed to the driver. | Yes |
| `readOnly` | Whether the volume is published read-only. | No (default `false`) |
| `fsType` | Filesystem type for `Filesystem` volume mode. | No |
| `volumeAttributes` | Static properties passed to the driver. | No |
| `controllerPublishSecretRef`, `nodeStageSecretRef`, `nodePublishSecretRef`, `controllerExpandSecretRef`, `nodeExpandSecretRef` | Secret references for CSI operations. | No |

**Deterministic checks:**

| Finding ID | Check | Severity | Rationale |
|---|---|---|---|
| **STG-CSI-004** | `csi.driver` is absent or empty | `error` | The driver is required. |
| **STG-CSI-005** | `csi.volumeHandle` is absent or empty | `error` | The handle is required. |
| **STG-CSI-006** | A CSI secret reference points to a Secret not in the input set | `info` | Existence is cluster-dependent. |
| **STG-CSI-007** | A CSI secret reference points to a Secret in a different namespace | `error` if both are in the input set | CSI secret references are namespace-local. |

### 8.2 CSI driver capabilities

A `CSIDriver` object declares `spec.attachRequired`, `spec.podInfoOnMount`, `spec.volumeLifecycleModes`, `spec.storageCapacity`, `spec.fsGroupPolicy`, and `spec.seLinuxMount`. These capabilities determine whether a CSI volume can be used in a particular mode.

**Heuristic checks:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **STG-CSI-008** | A CSI inline ephemeral volume is used but the `CSIDriver` does not list `Ephemeral` in `volumeLifecycleModes` | `warning` | The driver may support ephemeral volumes without declaring it, or the `CSIDriver` may not be in the input set. |
| **STG-CSI-009** | `fsGroupPolicy` is `None` and the Pod sets `securityContext.fsGroup` | `info` | The `fsGroup` has no effect on the CSI volume. |

---

## 9. Secret exposure vectors

### 9.1 Environment variables versus volume mounts

Secrets consumed as environment variables are visible to every process in the container and are more likely to leak through crash dumps, logs, or child processes. Secrets consumed as mounted files are isolated by file permissions and are automatically updated when the Secret changes (unless `immutable: true`). Kubernetes guidance recommends volume mounts for sensitive data and reserves environment variables for simple, non-critical configuration[reference:5][reference:6].

**Heuristic checks:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **STG-SEC-006** | A Secret is consumed via `env` or `envFrom` rather than a volume mount | `info` | The application may require environment variables; the finding is advisory. |
| **STG-SEC-007** | A Secret is consumed via `env` and the container has multiple containers, only one of which needs the Secret | `warning` | All containers in the Pod share the environment if `envFrom` is used; the finding suggests restricting access[reference:7]. |

### 9.2 Literals and logs

A Secret value embedded as a literal in a manifest (e.g., `value: "my-password"` in an `env` entry) is not protected by Secret semantics and is visible in the manifest, in `kubectl get -o yaml`, and in version control. A Secret value echoed in a log or error message is a leak regardless of the storage mechanism.

**Heuristic checks:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **STG-SEC-008** | A field that appears to contain a credential (e.g., `password`, `token`, `apiKey`) is set as a literal `value` rather than a `valueFrom` reference | `warning` | The field may be a non-sensitive configuration value with a misleading name. |
| **STG-SEC-009** | A Secret is mounted read-write when it only needs to be read | `info` | The mount is effectively read-only for Secret volumes; the finding is advisory. |

**K8S-STORAGE-REQ-011:** Remediation for Secret-exposure findings MUST NOT echo the secret value, MUST NOT recommend storing credentials in ConfigMaps, and MUST recommend using a Secret with `valueFrom.secretKeyRef` or a volume mount, or an external secret store[reference:8].

### 9.3 `stringData` and base64

`stringData` is a write-only convenience field; on read, the API server returns the merged result in `data` as base64. A manifest that uses `stringData` in a GitOps workflow may cause perpetual diffs because the field is not preserved.

**Heuristic check (STG-SEC-010):** A Secret manifest that uses `stringData` in a workflow that compares live state to the manifest SHOULD produce an `info` finding stating that `stringData` is write-only and will not appear in `kubectl get -o yaml`.

---

## 10. Configuration update behavior and restart assumptions

### 10.1 ConfigMap and Secret updates

When a ConfigMap or Secret is updated, the kubelet syncs the change to mounted volumes after a delay (typically up to the kubelet's sync period). Environment variables are **not** updated: a Pod that consumed a ConfigMap or Secret via `env` or `envFrom` retains the old values until the Pod is restarted. An `immutable: true` object cannot be updated at all.

**Heuristic checks:**

| Finding ID | Check | Severity | False-positive conditions |
|---|---|---|---|
| **STG-UPD-001** | A workload consumes a ConfigMap or Secret via `env` and the application does not handle restart | `info` | The application may be designed to restart on configuration change. |
| **STG-UPD-002** | A workload consumes a ConfigMap or Secret via a volume mount and the application does not watch for file changes | `info` | The application may reread the file on each request. |
| **STG-UPD-003** | A ConfigMap or Secret with `immutable: true` is referenced by a workload that expects updates | `warning` | The user may have intentionally chosen immutability for stability. |

**K8S-STORAGE-REQ-012:** KubeCheck MUST NOT assume that a configuration change triggers a restart. It MAY report the update-propagation behavior as `info` and state that restart requirements depend on the application.

### 10.2 Checksum annotations and rollout triggers

A common pattern is to add a checksum annotation to the Pod template so that a ConfigMap or Secret change triggers a rolling update. KubeCheck cannot verify that the annotation is present or correct without inspecting the template and the referenced object.

**Heuristic check (STG-UPD-004):** A Deployment whose Pod template does not reference the ConfigMap or Secret via an annotation or hash MAY produce an `info` finding stating that configuration changes may not trigger a rollout.

---

## 11. Common cross-resource errors and symptoms

| Manifest condition | KubeCheck finding | Likely runtime symptom | Live-status check required |
|---|---|---|---|
| Secret referenced via `secretKeyRef` is in a different namespace | `STG-ENV-002` (`error`) | `CreateContainerConfigError` | **Cluster-dependent** (namespace check) |
| Secret referenced via `secretKeyRef` is not in the input set | `STG-ENV-003` (`info`) | `CreateContainerConfigError` if the Secret is missing in the cluster | **Cluster-dependent** |
| Required key absent from a Secret in the input set | `STG-ENV-005` (`error`) | `CreateContainerConfigError` | — |
| `volumeMounts` references an undeclared volume | `STG-MNT-001` (`error`) | Pod stuck in `ContainerCreating`; kubelet error | — |
| StatefulSet `volumeClaimTemplates` has no matching mount | `STG-STS-001` (`error`) | PVC created but unused; storage wasted | **Cluster-dependent** (PVC creation) |
| PVC requests `ReadWriteOncePod` with another mode | `STG-PVC-003` (`error`) | PVC rejected by API server | — |
| StorageClass not in input set | `STG-PVC-006` (`info`) | PVC stays `Pending` if class is absent | **Cluster-dependent** |
| PV access modes do not match PVC | `STG-PV-003` (`error` if both in set) | PVC stays `Pending` | **Cluster-dependent** (binding) |
| CSI inline volume uses a driver without `Ephemeral` mode | `STG-CSI-008` (`warning`) | Pod stuck in `ContainerCreating` | **Cluster-dependent** (driver capabilities) |
| Secret consumed via `env` rather than volume | `STG-SEC-006` (`info`) | Secret visible to all processes; potential leak | — |

**K8S-STORAGE-REQ-013:** Every row in §11 MUST distinguish the manifest finding from the runtime symptom. A manifest finding is what KubeCheck can report; a runtime symptom requires live status and MUST NOT be claimed as a KubeCheck finding (`K8S-WORKLOAD-REQ-017`).

---

## 12. Consolidated finding register

### 12.1 Deterministic findings

| Finding ID | Family | Severity |
|---|---|---|
| STG-CM-001 | ConfigMap data/binaryData overlap | `error` |
| STG-CM-002 | Invalid ConfigMap/Secret key | `error` |
| STG-SEC-001 | Secret data/stringData conflict | `warning` |
| STG-SEC-003 | TLS Secret missing required key | `error` |
| STG-SEC-004 | Docker config Secret missing required key | `error` |
| STG-ENV-001 | `namespace` in `secretKeyRef`/`configMapKeyRef` | `error` |
| STG-ENV-002 | Cross-namespace reference (both in set) | `error` |
| STG-ENV-005 | Required key missing from Secret in set | `error` |
| STG-VOL-001 | Duplicate volume name | `error` |
| STG-VOL-003 | Multiple source types in one volume | `error` |
| STG-VOL-005 | Cross-namespace volume source (both in set) | `error` |
| STG-MNT-001 | Mount references undeclared volume | `error` |
| STG-MNT-002 | Non-absolute `mountPath` | `error` |
| STG-MNT-003 | Duplicate `mountPath` | `error` |
| STG-MNT-005 | `subPath` on unsupported volume | `error` |
| STG-PROJ-002 | Duplicate projected path | `error` |
| STG-ED-001 | Invalid `emptyDir.sizeLimit` | `error` |
| STG-ED-002 | Invalid `emptyDir.medium` | `error` |
| STG-CSI-001 | CSI inline volume missing driver | `error` |
| STG-CSI-004 | CSI PV missing driver | `error` |
| STG-CSI-005 | CSI PV missing `volumeHandle` | `error` |
| STG-CSI-007 | CSI secret cross-namespace (both in set) | `error` |
| STG-PVC-001 | Empty `accessModes` | `error` |
| STG-PVC-002 | Invalid access mode | `error` |
| STG-PVC-003 | `ReadWriteOncePod` combined with another mode | `error` |
| STG-PVC-004 | Missing/invalid storage request | `error` |
| STG-PVC-005 | Invalid `volumeMode` | `error` |
| STG-PV-003 | PV/PVC access mode mismatch (both in set) | `error` |
| STG-PV-004 | PV/PVC StorageClass mismatch (both in set) | `error` |
| STG-STS-001 | Template with no matching mount | `error` |
| STG-STS-002 | Mount with no matching template | `error` |
| STG-STS-003 | Template missing required fields | `error` |

### 12.2 Heuristic and informational findings

| Finding ID | Family | Severity |
|---|---|---|
| STG-CM-003 | Immutable ConfigMap/Secret update implication | `info` |
| STG-SEC-002 | `stringData` in `kubectl apply` workflow | `info` |
| STG-SEC-005 | Basic-auth Secret missing key | `warning` |
| STG-ENV-003 | Referenced object not in input set | `info` |
| STG-ENV-004 | `envFrom` invalid environment variable names | `warning` |
| STG-ENV-006 | Optional key absent | `info` |
| STG-VOL-004 | Volume source not in input set | `info` |
| STG-MNT-004 | Volume not mounted | `warning` |
| STG-MNT-006 | ConfigMap/Secret mounted writable | `warning` |
| STG-MNT-007 | PVC mounted read-write but access mode is ROX | `warning` |
| STG-PROJ-001 | Projected source not in input set | `info` |
| STG-PROJ-003 | `serviceAccountToken` expiration out of range | `warning` |
| STG-ED-003 | `emptyDir` used for persistent data | `info` |
| STG-CSI-002 | CSI inline volume read-only advisory | `info` |
| STG-CSI-003 | Unknown CSI `volumeAttributes` key | `info` |
| STG-CSI-006 | CSI secret not in input set | `info` |
| STG-CSI-008 | CSI driver without `Ephemeral` mode | `warning` |
| STG-CSI-009 | `fsGroupPolicy: None` with `fsGroup` set | `info` |
| STG-PVC-006 | StorageClass not in input set | `info` |
| STG-PVC-007 | Empty StorageClass and dynamic provisioning | `info` |
| STG-PVC-008 | `volumeBindingMode: Immediate` with topology constraints | `info` |
| STG-PV-001 | PV claimRef PVC not in input set | `info` |
| STG-PV-002 | PVC volumeName not in input set | `info` |
| STG-SEC-006 | Secret consumed via environment variable | `info` |
| STG-SEC-007 | Secret accessible to all containers | `warning` |
| STG-SEC-008 | Credential-like literal `value` | `warning` |
| STG-SEC-009 | Secret mounted read-write advisory | `info` |
| STG-SEC-010 | `stringData` not preserved in GitOps | `info` |
| STG-UPD-001–004 | Configuration update assumptions | `info`/`warning` |

---

## 13. Valid and invalid configuration examples

### 13.1 Valid: Secret consumed via volume mount

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: app
spec:
  containers:
  - name: app
    image: nginx
    volumeMounts:
    - name: tls
      mountPath: /etc/tls
      readOnly: true
  volumes:
  - name: tls
    secret:
      secretName: app-tls