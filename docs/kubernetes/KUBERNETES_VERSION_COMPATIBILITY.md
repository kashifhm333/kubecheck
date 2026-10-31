# KubeCheck — Kubernetes Version Compatibility

**Document ID:** K8S-VER-001  
**File:** `docs/kubernetes/KUBERNETES_VERSION_COMPATIBILITY.md`  
**Status:** Normative — Approved  
**Version:** 1.1.0  
**Product:** KubeCheck  
**Owner:** Kubernetes API Lifecycle and Compatibility Architecture  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/architecture/PRODUCT_VISION.md`
- `docs/architecture/SYSTEM_ARCHITECTURE.md`
- `docs/architecture/REQUIREMENTS_AND_NFRS.md`
- `docs/architecture/ROADMAP_AND_DECISIONS.md`
- `docs/architecture/OFFLINE_KNOWLEDGE_BUNDLES.md`
- `docs/architecture/ADR/ADR-002-yaml-parser-selection.md`
- `docs/architecture/ADR/ADR-003-knowledge-bundle-format.md`
- `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`
- `docs/kubernetes/KUBERNETES_API_VERSION_LIFECYCLE.md`
- `docs/kubernetes/KUBERNETES_SCHEMA_VALIDATION.md`
- `docs/kubernetes/CRD_AND_CUSTOM_RESOURCE_HANDLING.md`
- `docs/kubernetes/WORKLOADS_AND_PODS.md`
- `docs/kubernetes/SECURITY_AND_RBAC.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/cli/COMMAND_REFERENCE.md`
- `docs/cli/EXIT_CODE_CONTRACT.md`
- `docs/output/JSON_SCHEMA.md`
- `docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`
- `docs/testing/DETERMINISM_TESTS.md`
- `docs/testing/REQUIREMENT_TRACEABILITY.md`

---

## 1. Purpose and scope

This document defines how KubeCheck supports **multiple Kubernetes minor versions** without confusing schemas, conflating deprecation policies, or producing misleading diagnostics. It establishes the normative contract for:

- The **supported-version policy** and end-of-support criteria.
- **Schema provenance** and version pinning.
- **API lifecycle tracking**: additions, deprecations, removals, and replacement APIs.
- **Version-specific required fields, types, defaults, and constraints**.
- **Feature gates** and fields whose validation depends on component configuration.
- **CRD and operator version compatibility**.
- **Unknown or ambiguous target-version behavior**.
- **Multi-version analysis** and per-version diagnostic differences.
- **Schema bundle generation, verification, and regression tests**.
- **Release compatibility and migration guidance**.
- A **compatibility matrix template**.
- A **deterministic schema-selection algorithm** for both known and unknown target versions.

### 1.1 Governing principle

> **K8S-VER-REQ-001 — KubeCheck MUST NOT infer a precise target cluster version from a manifest unless authoritative evidence establishes it.** The `apiVersion` field identifies the API version a manifest was authored against, not the cluster's running version. A manifest using `apps/v1` is servable by any Kubernetes minor release that includes `apps/v1`, which spans many minor versions. The target version is an **explicit input**, not a derived fact.

### 1.2 In scope

Version-aware schema selection; lifecycle status determination; feature-gate dependency tracking; multi-version differential analysis; bundle generation and verification; migration guidance.

### 1.3 Out of scope

- Live cluster version discovery (see `KUBERNETES_API_FUNDAMENTALS.md` §13).
- Admission controller behavior and webhook configuration.
- Rule-specific diagnostic semantics (see `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`).
- Cluster upgrade orchestration.
- Kubernetes release engineering.

### 1.4 Assumptions

- **ASSUMPTION-VER-01:** The user can supply a target Kubernetes minor version explicitly (e.g., via `--kubernetes-version`). If not, KubeCheck uses the newest bundled version and emits a warning (§9.1).
- **ASSUMPTION-VER-02:** The schema bundle is generated from an authoritative upstream Kubernetes release tag, and the generation pipeline is reproducible (§11).
- **ASSUMPTION-VER-03:** Feature-gate metadata is available in the bundle for every gate that affects a field KubeCheck validates. If not, gate-dependent validation is reported as `info` rather than `error` (§7.3).
- **ASSUMPTION-VER-04:** CRD schemas, when supplied, are sourced from the CRD's `spec.versions[*].schema.openAPIV3Schema` and are associated with the CRD's declared `apiVersion` (§8).

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **Target version** | The Kubernetes minor version (`x.y`) against which KubeCheck validates a manifest. Supplied explicitly by the user via `--kubernetes-version`. |
| **Pinned patch** | The exact patch release (`x.y.z`) whose schemas are bundled for a given minor version. |
| **Schema bundle** | A versioned, signed artifact containing schemas, lifecycle metadata, feature-gate metadata, and provenance. |
| **GVK** | Group, Version, Kind — the tuple identifying an object's schema. |
| **Lifecycle status** | `GA`, `Beta`, `Alpha`, `Deprecated`, or `Removed` for a GVK against a target version. |
| **Feature gate** | A `key=value` configuration that enables or disables a Kubernetes feature on a component. |
| **Multi-version analysis** | Running the same manifest against two or more target versions and reporting per-version findings and differences. |
| **Supported-version window** | The set of Kubernetes minor versions for which KubeCheck maintains a schema bundle. |
| **Authoritative evidence** | An explicit user flag, a bundle manifest, or a cluster-info file that states the version. Not a manifest annotation, comment, or field value. |
| **Provisionally normative** | A requirement dependent on an OPEN ADR (e.g., ADR-002, ADR-003). |

**Requirement IDs:** `K8S-VER-REQ-NNN`.  
**Finding IDs:** `VER-NNN`.  
**Test IDs:** `TEST-VER-NNN`.

---

## 3. Supported-version policy

### 3.1 Upstream support window

The Kubernetes project maintains release branches for the **most recent three minor releases** and provides approximately **one year of patch support** for versions 1.19 and newer. The exact policy is documented in the Kubernetes Version Skew Policy.

| Upstream property | Value |
|---|---|
| Release branches maintained | 3 most recent minor versions |
| Patch support duration | ~12 months (1.19+); ~9 months (≤ 1.18) |
| Version skew for kubelet | Up to 3 minor versions older than kube-apiserver |
| Version skew for kube-proxy, kube-controller-manager, kube-scheduler | Must not be newer than kube-apiserver |
| Version skew for kubectl | Within one minor version of kube-apiserver |

### 3.2 KubeCheck supported-version window

The KubeCheck supported-version window is a **product-level parameter**, not a hard-coded engine constant (`CON-004` in `REQUIREMENTS_AND_NFRS.md`). The window is published in each schema bundle's manifest and is subject to the following policy:

| Policy element | KubeCheck position |
|---|---|
| **Minimum supported minor version** | Declared per bundle; not fixed in this document. |
| **Maximum supported minor version** | The most recent Kubernetes minor release at bundle generation time. |
| **Default analysis target** | The most recent minor version in the active bundle, used only when no target version is supplied (§9.1). |
| **Patch pinning** | Each bundle pins an exact patch release per minor version (e.g., `1.34.2`). |
| **Window size** | Default: the three most recent minor versions at bundle generation time, matching upstream's maintained branches. |
| **End-of-support trigger** | A minor version is dropped when it falls outside the upstream maintained window and a new bundle is generated without it. |
| **Grace period** | One KubeCheck minor release of `Deprecated` status before removal from the supported window (§3.3). |

**K8S-VER-REQ-002:** The supported-version window MUST be declared in the schema bundle manifest and MUST be validated by the engine at load time. An engine that does not support the bundle's declared schema version MUST reject the bundle (`COMP-REQ-001`).

**K8S-VER-REQ-003:** Adding a new Kubernetes minor version to the supported window MUST NOT remove any previously supported version in the same KubeCheck minor release. Removal requires a KubeCheck minor release and a documented deprecation window.

**K8S-VER-REQ-004:** The engine MUST expose the supported-version window through the `kubecheck version --output json` command so that CI pipelines can detect when their target version is approaching end-of-support.

### 3.3 End-of-support lifecycle

A Kubernetes minor version transitions through three states within KubeCheck:

```mermaid
stateDiagram-v2
    [*] --> Supported: Bundle includes minor version
    Supported --> Deprecated: Upstream branch exits maintenance
    Deprecated --> Unsupported: Next KubeCheck minor release drops it
    Unsupported --> [*]: Target version rejected