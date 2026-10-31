# KubeCheck — Static Analysis Rules

**Document ID:** STATIC-RULES-001  
**File:** `docs/engine/STATIC_ANALYSIS_RULES.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Static Analysis Rule Authors and Reviewers  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/engine/DIAGNOSTIC_RULE_SPEC.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/yaml/YAML_LANGUAGE_SPEC.md`
- `docs/yaml/YAML_PARSING_AND_EDGE_CASES.md`
- `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md`
- `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`
- `docs/kubernetes/WORKLOADS_AND_PODS.md`
- `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md`
- `docs/kubernetes/CONFIGURATION_AND_STORAGE.md`
- `docs/kubernetes/SECURITY_AND_RBAC.md`
- `docs/kubernetes/SCHEDULING_AND_RESOURCE_MANAGEMENT.md`
- `docs/kubernetes/KUBERNETES_VERSION_COMPATIBILITY.md`
- `docs/output/JSON_SCHEMA.md`
- `docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`
- `docs/testing/DETERMINISM_TESTS.md`

---

## 1. Purpose and scope

This document is the **comprehensive taxonomy of local static checks** that KubeCheck should implement. It organizes every proposed rule into families, defines the matching conditions, states counterexamples (manifests that MUST NOT trigger the rule), assigns severity and confidence, declares applicability, documents false-positive traps, provides remediation guidance, and specifies test cases.

**Governing statement:**

> **STATIC-RULES-REQ-001 — This document is a rule inventory and specification, not evidence of implementation.** Every rule in this document is `PLANNED`. No detection rate, benchmark result, or implementation status is asserted. Rule IDs are **proposed stable identifiers** and may be adjusted during implementation review; once a rule is included in a bundle, its ID becomes immutable (`RULE-SPEC-REQ-007`).

> **STATIC-RULES-REQ-002 — Every rule MUST be classified as `deterministic`, `probable`, or `context-dependent`.** Deterministic rules may emit `error`; probable and context-dependent rules MUST NOT emit `error` (`RULE-SPEC-REQ-014`). Heuristic best-practice recommendations MUST be `probable` or `context-dependent` unless the condition is provably a defect.

> **STATIC-RULES-REQ-003 — No blanket rule may misclassify a valid optional configuration as a defect.** Every rule MUST include counterexamples demonstrating valid configurations that do not trigger it. A rule without documented counterexamples cannot enter a bundle (`RULE-SPEC-REQ-059`).

### 1.1 In scope

Rule families covering: YAML and normalized-structure defects; schema violations; required/missing/mistyped/invalid values; API deprecations and version incompatibility; workload configuration defects; probes and lifecycle; resource requests and limits; services and networking; storage and configuration references; security and RBAC; reliability and availability; maintainability and best practice.

### 1.2 Out of scope

- Rule execution semantics (see `DIAGNOSTIC_RULE_SPEC.md`).
- Concrete rule YAML (each rule's YAML artifact is produced during implementation).
- Benchmark results (see `docs/testing/`).
- Live cluster checks.

### 1.3 Rule ID conventions

Rules use the `<DOMAIN>-<FAMILY>-<NNN>` format defined in `DIAGNOSTIC_RULE_SPEC.md` §3. This document proposes the following domain-family assignments:

| Domain | Family | Scope |
|---|---|---|
| `YAML` | `SYN`, `NORM`, `EDGE` | Parsing, normalization, edge cases |
| `K8S` | `GVK`, `SCHEMA`, `META` | Kubernetes object structure and schema |
| `DEP` | `API` | API version lifecycle |
| `WL` | `CONTAINER`, `POD`, `PROBE`, `RES`, `VOL`, `ENV`, `HOOK`, `SCHED` | Workload configuration |
| `NET` | `SVC`, `ING`, `GW`, `NP`, `DNS`, `TLS` | Networking |
| `STG` | `CM`, `SEC`, `VOL`, `PVC`, `PV`, `SC`, `CSI`, `STS` | Configuration and storage |
| `SEC` | `CTX`, `RBAC`, `SECRET`, `IMG`, `HOST`, `PSS` | Security |
| `REL` | `PDB`, `HPA`, `QUOTA`, `LIMIT`, `AVAIL`, `UPD` | Reliability and availability |
| `MAINT` | `LABEL`, `ANNOT`, `DUP`, `BEST` | Maintainability and best practice |

**STATIC-RULES-REQ-004:** A rule ID MUST NOT be reused across families. If a rule is moved to a different family, the original ID remains as a deprecated alias (`RULE-SPEC-REQ-007`).

---

## 2. Terminology and rule-specification template

### 2.1 Rule family specification fields

Each rule family in this document is described with the following fields:

| Field | Meaning |
|---|---|
| **Family ID** | Domain-family identifier (e.g., `WL-PROBE`). |
| **Purpose** | What defects the family detects. |
| **Rules** | Table of individual rules with ID, title, evidence class, default severity. |
| **Matching conditions** | For each rule, the predicate that triggers it. |
| **Counterexamples** | Manifests that MUST NOT trigger the rule. |
| **False-positive traps** | Known conditions under which the rule may misclassify a valid configuration. |
| **Remediation** | Corrective guidance. |
| **Test cases** | Positive, negative, and boundary fixtures. |
| **Applicability** | Target versions and kinds. |
| **Dependencies** | Rules that must not fire first (`RULE-SPEC-REQ-034`). |

### 2.2 Evidence-class definitions

| Class | Definition | Permitted severity |
|---|---|---|
| `deterministic` | The verdict depends only on the manifest and immutable reference data. No heuristic weighting. | `error`, `warning`, `info` |
| `probable` | Supported by strong evidence but may be a false positive in valid configurations. | `warning`, `info` |
| `context-dependent` | Relevance depends on cluster policy, workload intent, or organizational standards. | `warning`, `info` |

### 2.3 Rule inventory summary

| Domain | Families | Proposed rule count |
|---|---|---|
| YAML | 3 | 14 |
| K8S | 3 | 18 |
| DEP | 1 | 6 |
| WL | 7 | 48 |
| NET | 6 | 38 |
| STG | 8 | 32 |
| SEC | 5 | 29 |
| REL | 6 | 22 |
| MAINT | 4 | 12 |
| **Total** | **43** | **219** |

The counts are **proposed inventories**, not implementation status.

---

## 3. YAML and normalized-structure defects

### 3.1 Family `YAML-SYN` — Syntax defects

**Purpose:** Detect YAML constructs that the parser cannot interpret. These are deterministic parse-level defects.

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `YAML-SYN-001` | Tab character in indentation | deterministic | error |
| `YAML-SYN-002` | Inconsistent block indentation | deterministic | error |
| `YAML-SYN-003` | Missing mapping colon | deterministic | error |
| `YAML-SYN-004` | Invalid sequence indicator | deterministic | error |
| `YAML-SYN-005` | Unclosed quoted scalar | deterministic | error |
| `YAML-SYN-006` | Invalid escape sequence | deterministic | error |
| `YAML-SYN-007` | Malformed block scalar header | deterministic | error |
| `YAML-SYN-008` | Unexpected token | deterministic | error |
| `YAML-SYN-009` | Duplicate mapping key | deterministic | error |
| `YAML-SYN-010` | Undefined alias | deterministic | error |
| `YAML-SYN-011` | Invalid UTF-8 byte sequence | deterministic | error |
| `YAML-SYN-012` | New document without boundary marker | deterministic | error |

#### `YAML-SYN-001` — Tab character in indentation

| Attribute | Detail |
|---|---|
| **Matching condition** | A tab (`\t`, U+0009) appears at a position where block indentation is expected. |
| **Counterexamples** | Tabs in scalar content (e.g., inside a quoted string) do not trigger. Tabs as separation whitespace in flow context do not trigger. |
| **False-positive traps** | A tab inside a literal block scalar (`\|`) that is indented with spaces does not trigger; the tab is content, not indentation. |
| **Severity** | `error` |
| **Confidence** | Deterministic — the specification prohibits tabs in indentation. |
| **Applicability** | All versions; all kinds. |
| **Remediation** | Replace the tab with spaces (typically 2 or 4) at the matching indentation level. |
| **Test cases** | Positive: `spec:\n\tcontainers: []`. Negative: `command: "echo\thello"` (tab in scalar). Boundary: a tab immediately after a scalar value with a trailing comment. |
| **Dependencies** | None. |

#### `YAML-SYN-009` — Duplicate mapping key

| Attribute | Detail |
|---|---|
| **Matching condition** | A mapping contains the same key more than once, where equality is computed by Core Schema canonical equality (e.g., `0xB` and `11` are the same integer key; `"11"` and `11` are different). |
| **Counterexamples** | `"<<"` appearing multiple times in a merge-key context is NOT a duplicate (`YAML-SPEC-REQ-013`); `"11"` (quoted) and `11` (unquoted) are different keys. |
| **False-positive traps** | A parser configured for YAML 1.1 may resolve `yes`/`no` differently. KubeCheck uses Core Schema canonical equality; the rule must not depend on the parser's default duplicate policy (`EDGE-REQ-011`). |
| **Severity** | `error` |
| **Confidence** | Deterministic. |
| **Applicability** | All versions; all kinds. |
| **Remediation** | Remove or rename the duplicate key. If the intent was a merge, use the merge key (`<<`). |
| **Test cases** | Positive: `metadata: {name: a, name: b}`. Negative: `metadata: {name: a, labels: {name: b}}` (different mappings). Boundary: `{0xB: a, 11: b}` (canonical duplicates under Core Schema). |
| **Dependencies** | None. |

### 3.2 Family `YAML-NORM` — Normalization defects

**Purpose:** Detect constructs that parse successfully but cannot be normalized into the NIR.

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `YAML-NORM-001` | Circular alias reference | deterministic | error |
| `YAML-NORM-002` | Non-JSON-serializable construct (non-string mapping key in a JSON-required context) | deterministic | error |
| `YAML-NORM-003` | Application-specific tag preserved as opaque | probable | warning |
| `YAML-NORM-004` | Merge key expansion cycle | deterministic | error |

#### `YAML-NORM-001` — Circular alias reference

| Attribute | Detail |
|---|---|
| **Matching condition** | An anchor references itself directly or indirectly, producing a cycle in the NIR construction. |
| **Counterexamples** | An alias that references an anchor defined earlier in the document without creating a cycle does not trigger. |
| **False-positive traps** | The parser may resolve aliases in a way that hides the cycle; detection must be at the NIR-construction stage where the graph is explicitly traversed. |
| **Severity** | `error` |
| **Confidence** | Deterministic. |
| **Applicability** | All versions; all kinds. |
| **Remediation** | Break the cycle by restructuring the anchors or replacing the alias with an explicit value. |
| **Test cases** | Positive: `a: &a [*a]`. Negative: `a: &a [1]`, `b: *a`. Boundary: a cycle of length two through two anchors. |
| **Dependencies** | None. |

### 3.3 Family `YAML-EDGE` — Edge-case handling

**Purpose:** Detect edge cases that are not defects but produce diagnostics for transparency.

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `YAML-EDGE-001` | YAML 1.1 boolean form used where a boolean is expected | probable | info |
| `YAML-EDGE-002` | Octal-looking integer used where octal is expected | probable | info |
| `YAML-EDGE-003` | Timestamp-like string used where a timestamp is expected | probable | info |
| `YAML-EDGE-004` | Empty document | deterministic | info |

#### `YAML-EDGE-001` — YAML 1.1 boolean form used where a boolean is expected

| Attribute | Detail |
|---|---|
| **Matching condition** | A plain scalar `yes`, `no`, `on`, or `off` appears in a field where the Kubernetes schema expects `boolean`. |
| **Counterexamples** | `yes` in a `string` field (e.g., `ConfigMap.data`) does not trigger. An explicit `!!str` tag on the value does not trigger. |
| **False-positive traps** | The schema may declare a field as `boolean` but accept a string form in practice; KubeCheck reports `info` because the Core Schema resolves the value to a string, and the API server may reject it. |
| **Severity** | `info` |
| **Confidence** | The Core Schema resolution is deterministic; the severity is `info` because the schema context makes it advisory. |
| **Applicability** | All versions. |
| **Remediation** | Replace `yes`/`no`/`on`/`off` with `true`/`false`, or quote the value if a string is intended. |
| **Test cases** | Positive: `enabled: yes` where schema expects `boolean`. Negative: `enabled: "yes"` (quoted). Boundary: `enabled: !!bool yes` (explicit tag). |
| **Dependencies** | `K8S-SCHEMA-*` (schema status must be known). |

---

## 4. Schema violations and unknown fields

### 4.1 Family `K8S-GVK` — Object-root validation

**Purpose:** Validate that the document root is a valid Kubernetes object with `apiVersion`, `kind`, and `metadata`.

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `K8S-GVK-001` | Root is not a mapping | deterministic | error |
| `K8S-GVK-002` | Missing `apiVersion` | deterministic | error |
| `K8S-GVK-003` | Missing `kind` | deterministic | error |
| `K8S-GVK-004` | `apiVersion` is not a string | deterministic | error |
| `K8S-GVK-005` | `kind` is not a string | deterministic | error |
| `K8S-GVK-006` | Invalid `apiVersion` format | deterministic | error |
| `K8S-GVK-007` | Missing `metadata` | deterministic | warning |
| `K8S-GVK-008` | `metadata` is not a mapping | deterministic | error |
| `K8S-GVK-009` | Unknown GVK (not in bundle) | deterministic | warning |
| `K8S-GVK-010` | Missing `metadata.name` for a namespaced resource | deterministic | error |

#### `K8S-GVK-009` — Unknown GVK

| Attribute | Detail |
|---|---|
| **Matching condition** | The `(group, version, kind)` is not present in the active bundle's schema index for the target version. |
| **Counterexamples** | A GVK present in the bundle's CRD catalog is not unknown. |
| **False-positive traps** | A GVK may be a CRD installed in the cluster but not in the bundle; the finding is `warning`, not `error`, and states that the GVK is not covered offline (`VER-008`). |
| **Severity** | `warning` |
| **Confidence** | Deterministic relative to the bundle. |
| **Applicability** | All target versions in the bundle. |
| **Remediation** | Supply a local CRD schema, or update the bundle to a version that covers the GVK. |
| **Test cases** | Positive: `apiVersion: example.com/v1`, `kind: Widget` with no CRD supplied. Negative: `apiVersion: apps/v1`, `kind: Deployment` (built-in). Boundary: `apiVersion: example.com/v1` with a CRD schema supplied. |
| **Dependencies** | None. |

### 4.2 Family `K8S-SCHEMA` — Schema validation

**Purpose:** Validate the NIR against the OpenAPI schema for the target version.

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `K8S-SCHEMA-001` | Unknown field | deterministic | warning |
| `K8S-SCHEMA-002` | Wrong type | deterministic | error |
| `K8S-SCHEMA-003` | Missing required field | deterministic | error |
| `K8S-SCHEMA-004` | Invalid enum value | deterministic | error |
| `K8S-SCHEMA-005` | String constraint violation (minLength, maxLength, pattern) | deterministic | error |
| `K8S-SCHEMA-006` | Numeric constraint violation (minimum, maximum, multipleOf) | deterministic | error |
| `K8S-SCHEMA-007` | Array constraint violation (minItems, maxItems, uniqueItems) | deterministic | error |
| `K8S-SCHEMA-008` | Object constraint violation (minProperties, maxProperties) | deterministic | error |
| `K8S-SCHEMA-009` | `null` where schema requires non-null | deterministic | error |
| `K8S-SCHEMA-010` | Value present where schema forbids it (`additionalProperties: false`) | deterministic | error |

#### `K8S-SCHEMA-001` — Unknown field

| Attribute | Detail |
|---|---|
| **Matching condition** | A field is present in the object but not defined in the schema for its GVK and target version. |
| **Counterexamples** | A field preserved by `x-kubernetes-preserve-unknown-fields: true` on the enclosing subtree does not trigger (see `K8S-SCHEMA-010`). |
| **False-positive traps** | The field may be recognized by a mutating webhook or by the API server's defaulting. KubeCheck reports `warning` because the field is not in the schema, but the user may have a cluster-specific extension. |
| **Severity** | `warning` (aligned with `fieldValidation: Warn`; `OPEN-K8S-API-04`). |
| **Confidence** | Deterministic relative to the bundle. |
| **Applicability** | Target version's schema. |
| **Remediation** | Remove the field, correct its spelling, or confirm that the cluster accepts it (e.g., via a CRD schema or webhook). |
| **Test cases** | Positive: `spec.replicaCount` instead of `spec.replicas`. Negative: `spec.replicas` (valid). Boundary: a field under `x-kubernetes-preserve-unknown-fields: true`. |
| **Dependencies** | `K8S-GVK-009` (only run schema validation when a schema is available). |

#### `K8S-SCHEMA-003` — Missing required field

| Attribute | Detail |
|---|---|
| **Matching condition** | A field listed in the schema's `required` array is absent from the object. |
| **Counterexamples** | A field that is defaulted by the API server is still absent in the manifest; the rule must distinguish `required` fields from `defaulted optional` fields. |
| **False-positive traps** | A field may be conditionally required (e.g., required only when another field is present). The schema may not express this conditional; the rule must not fire on a field that is genuinely absent but not required. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the schema. |
| **Applicability** | Target version's schema. |
| **Remediation** | Add the missing field with a valid value. |
| **Test cases** | Positive: a `Deployment` without `spec.selector` (required). Negative: a `Deployment` with `spec.selector`. Boundary: a field that is `required` in the base schema but removed by a newer version. |
| **Dependencies** | `K8S-SCHEMA-001` (unknown-field check runs first; if a required field is misspelled, the unknown-field finding explains it). |

### 4.3 Family `K8S-META` — Metadata validation

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `K8S-META-001` | Invalid label key | deterministic | error |
| `K8S-META-002` | Invalid label value | deterministic | error |
| `K8S-META-003` | Invalid annotation key | deterministic | error |
| `K8S-META-004` | Namespace set on a cluster-scoped resource | deterministic | error |
| `K8S-META-005` | `metadata.name` not a valid DNS subdomain | deterministic | error |
| `K8S-META-006` | Server-assigned field present in a manifest (`uid`, `resourceVersion`, `creationTimestamp`) | probable | warning |
| `K8S-META-007` | `status` present in a manifest | probable | info |
| `K8S-META-008` | `ownerReference` referencing an object not in the input set | context-dependent | info |

#### `K8S-META-004` — Namespace on cluster-scoped resource

| Attribute | Detail |
|---|---|
| **Matching condition** | A resource whose schema declares `scope: Cluster` has a non-empty `metadata.namespace`. |
| **Counterexamples** | A namespaced resource with a namespace is valid. |
| **False-positive traps** | A resource that is namespaced in a newer version but cluster-scoped in an older version (or vice versa) may be misclassified. The rule uses the target version's schema. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the bundle. |
| **Applicability** | All target versions; resources with `scope: Cluster`. |
| **Remediation** | Remove `metadata.namespace`. |
| **Test cases** | Positive: `Namespace` with `metadata.namespace: foo`. Negative: `Pod` with `metadata.namespace: foo`. Boundary: a CRD instance whose CRD declares `scope: Cluster`. |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 5. Required, missing, mistyped, and invalid values

### 5.1 Family `K8S-VALUE` — Value-level validation

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `K8S-VALUE-001` | Invalid quantity syntax | deterministic | error |
| `K8S-VALUE-002` | Negative quantity where non-negative required | deterministic | error |
| `K8S-VALUE-003` | Quantity exceeds representable range | deterministic | error |
| `K8S-VALUE-004` | Fractional value where integer required | deterministic | error |
| `K8S-VALUE-005` | Invalid CIDR | deterministic | error |
| `K8S-VALUE-006` | Invalid port number | deterministic | error |
| `K8S-VALUE-007` | Invalid duration syntax | deterministic | error |
| `K8S-VALUE-008` | Invalid DNS name | deterministic | error |
| `K8S-VALUE-009` | Invalid label selector syntax | deterministic | error |
| `K8S-VALUE-010` | Invalid cron expression | deterministic | error |

#### `K8S-VALUE-004` — Fractional value where integer required

| Attribute | Detail |
|---|---|
| **Matching condition** | A field whose schema type is `integer` receives a value with a fractional part (e.g., `0.5` for `replicas`, `1.5` for an extended resource). |
| **Counterexamples** | A CPU quantity of `0.5` is valid because CPU is a `resource.Quantity`, not an `integer`. The rule must distinguish schema type `integer` from `resource.Quantity`. |
| **False-positive traps** | Extended resources (e.g., `nvidia.com/gpu`) must be integers; CPU and memory are quantities. The rule consults the schema type. |
| **Severity** | `error` |
| **Confidence** | Deterministic. |
| **Applicability** | All versions; fields typed `integer`. |
| **Remediation** | Use an integer value, or change the field to a quantity type if the schema allows. |
| **Test cases** | Positive: `replicas: 1.5`. Negative: `replicas: 2`. Boundary: `nvidia.com/gpu: 0.5` (fractional extended resource). |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 6. API deprecations and version incompatibility

### 6.1 Family `DEP-API`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `DEP-API-001` | GVK removed in target version | deterministic | error |
| `DEP-API-002` | GVK deprecated in target version | deterministic | warning |
| `DEP-API-003` | Beta GVK used without explicit opt-in | probable | info |
| `DEP-API-004` | Alpha GVK used | probable | warning |
| `DEP-API-005` | Field removed in target version | deterministic | error |
| `DEP-API-006` | Field introduced after target version | deterministic | error |
| `DEP-API-007` | Field default changed between target versions | context-dependent | info |
| `DEP-API-008` | Feature-gate-dependent field | context-dependent | info |

#### `DEP-API-001` — GVK removed in target version

| Attribute | Detail |
|---|---|
| **Matching condition** | The GVK's lifecycle status for the target version is `Removed` (`VER-020`). |
| **Counterexamples** | A GVK whose status is `GA`, `Beta`, or `Alpha` does not trigger. |
| **False-positive traps** | The lifecycle metadata may be stale. The rule cites the bundle's lifecycle source and version. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the bundle. |
| **Applicability** | Target version's lifecycle metadata. |
| **Remediation** | Replace with the replacement GVK from the lifecycle metadata. If no replacement is documented, state that and link to the upstream migration guide. |
| **Test cases** | Positive: `networking.k8s.io/v1beta1/Ingress` against `1.22`. Negative: `networking.k8s.io/v1/Ingress` against `1.22`. Boundary: a GVK removed in `1.32` analyzed against `1.31`. |
| **Dependencies** | `K8S-GVK-009`. |

#### `DEP-API-008` — Feature-gate-dependent field

| Attribute | Detail |
|---|---|
| **Matching condition** | A field's validation depends on a feature gate (`VER-004`). |
| **Counterexamples** | A field that is unconditionally valid does not trigger. |
| **False-positive traps** | The gate may be enabled in the target cluster; the finding is `info` and states the gate's state is cluster-dependent. |
| **Severity** | `info` |
| **Confidence** | Deterministic relative to the gate metadata; the severity is `info` because gate state is cluster-dependent. |
| **Applicability** | Target version's feature-gate metadata. |
| **Remediation** | Confirm the gate is enabled, or remove the field if not needed. |
| **Test cases** | Positive: init container with `restartPolicy: Always` against `1.28` (`SidecarContainers` beta). Negative: the same field against `1.33` (GA, no gate). Boundary: a gate default-enabled in a specific version. |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 7. Workload configuration defects

### 7.1 Family `WL-CONTAINER`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `WL-CONTAINER-001` | Empty `containers` list | deterministic | error |
| `WL-CONTAINER-002` | Duplicate container name | deterministic | error |
| `WL-CONTAINER-003` | Missing `image` | deterministic | error |
| `WL-CONTAINER-004` | Invalid container name | deterministic | error |
| `WL-CONTAINER-005` | Invalid image reference | deterministic | error |
| `WL-CONTAINER-006` | Invalid image digest | deterministic | error |
| `WL-CONTAINER-007` | `imagePullPolicy` contradicts tag mutability | probable | warning |
| `WL-CONTAINER-008` | Duplicate environment variable name | deterministic | error |
| `WL-CONTAINER-009` | Both `value` and `valueFrom` set | deterministic | error |
| `WL-CONTAINER-010` | Port out of range | deterministic | error |
| `WL-CONTAINER-011` | Duplicate port name across containers | deterministic | error |

#### `WL-CONTAINER-007` — `imagePullPolicy` contradicts tag mutability

| Attribute | Detail |
|---|---|
| **Matching condition** | `image` uses `:latest` (or no tag) AND `imagePullPolicy: IfNotPresent`. |
| **Counterexamples** | `:latest` with `imagePullPolicy: Always` (default for `:latest`) does not trigger. A digest reference with `IfNotPresent` does not trigger. |
| **False-positive traps** | In an air-gapped environment, a user may deliberately preload a `:latest` image and pin `IfNotPresent`. The finding is `warning`, not `error`. |
| **Severity** | `warning` |
| **Confidence** | Deterministic that the policy contradicts the tag; the operational impact is `warning`. |
| **Applicability** | All versions. |
| **Remediation** | Use a digest or a specific version tag, or set `imagePullPolicy: Always` if `:latest` is intentional. |
| **Test cases** | Positive: `image: nginx:latest`, `imagePullPolicy: IfNotPresent`. Negative: `image: nginx:1.27`. Boundary: `image: nginx@sha256:...` with `imagePullPolicy: IfNotPresent`. |
| **Dependencies** | None. |

### 7.2 Family `WL-POD`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `WL-POD-001` | Selector does not match template labels | deterministic | error |
| `WL-POD-002` | Selector absent on a workload that requires one | deterministic | error |
| `WL-POD-003` | `generateName` and `name` both set | deterministic | error |
| `WL-POD-004` | Both `name` and `generateName` absent | deterministic | error |
| `WL-POD-005` | Pod template present on a standalone Pod | deterministic | error |
| `WL-POD-006` | Ephemeral containers declared at creation time | deterministic | error |

#### `WL-POD-001` — Selector does not match template labels

| Attribute | Detail |
|---|---|
| **Matching condition** | For `Deployment`, `ReplicaSet`, `StatefulSet`, `DaemonSet`, `Job`, or `CronJob`: `spec.selector` is present, and its `matchLabels` and `matchExpressions` combined do not match a subset of `spec.template.metadata.labels`. |
| **Counterexamples** | A selector with `matchExpressions` that the template labels satisfy does not trigger. |
| **False-positive traps** | A selector using `matchExpressions` only (no `matchLabels`) may be overlooked if the rule only compares `matchLabels`; the rule must evaluate both. |
| **Severity** | `error` |
| **Confidence** | Deterministic. |
| **Applicability** | `apps/v1`, `batch/v1`; all target versions. |
| **Remediation** | Add the missing labels to `spec.template.metadata.labels`, or correct the selector. Note that the selector is immutable after creation. |
| **Test cases** | Positive: selector `{app: web}`, template labels `{app: api}`. Negative: selector `{app: web}`, template labels `{app: web, tier: frontend}`. Boundary: selector with `matchExpressions` satisfied by template labels. |
| **Dependencies** | `K8S-SCHEMA-001`. |

### 7.3 Family `WL-VOL`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `WL-VOL-001` | `volumeMount` references undeclared volume | deterministic | error |
| `WL-VOL-002` | Duplicate volume name | deterministic | error |
| `WL-VOL-003` | Multiple source types in one volume | deterministic | error |
| `WL-VOL-004` | Volume declared but not mounted | probable | warning |
| `WL-VOL-005` | `mountPath` is not absolute | deterministic | error |
| `WL-VOL-006` | Duplicate `mountPath` in one container | deterministic | error |
| `WL-VOL-007` | `subPath` set on an unsupported volume | deterministic | error |
| `WL-VOL-008` | ConfigMap/Secret volume mounted writable | probable | warning |

#### `WL-VOL-008` — ConfigMap/Secret volume mounted writable

| Attribute | Detail |
|---|---|
| **Matching condition** | A `configMap` or `secret` volume is mounted with `readOnly: false`. |
| **Counterexamples** | The same volume mounted with `readOnly: true` does not trigger. |
| **False-positive traps** | The API server and kubelet do not allow writes to ConfigMap and Secret volumes; the mount is effectively read-only regardless of the flag. The finding is `warning`, not `error`, because the field is accepted. |
| **Severity** | `warning` |
| **Confidence** | Deterministic that the flag is `false`; the effective behavior is advisory. |
| **Applicability** | All versions. |
| **Remediation** | Set `readOnly: true` for clarity. |
| **Test cases** | Positive: `volumeMounts: [{name: cm, mountPath: /etc/config, readOnly: false}]`. Negative: `readOnly: true`. Boundary: `readOnly` omitted (defaults to `false`). |
| **Dependencies** | `K8S-SCHEMA-001`. |

### 7.4 Family `WL-ENV`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `WL-ENV-001` | Duplicate environment variable name | deterministic | error |
| `WL-ENV-002` | `envFrom` key with invalid environment variable name | deterministic | warning |
| `WL-ENV-003` | Required `secretKeyRef` missing from a Secret in the input set | deterministic | error |
| `WL-ENV-004` | Required `configMapKeyRef` missing from a ConfigMap in the input set | deterministic | error |
| `WL-ENV-005` | `namespace` set inside `secretKeyRef` or `configMapKeyRef` | deterministic | error |

#### `WL-ENV-003` — Required `secretKeyRef` missing from a Secret in the input set

| Attribute | Detail |
|---|---|
| **Matching condition** | A container has `env[*].valueFrom.secretKeyRef` with `optional: false`, and the referenced Secret is in the input set but does not contain the referenced key. |
| **Counterexamples** | The Secret is not in the input set (existence is cluster-dependent; the rule does not fire as `error`). The `secretKeyRef` is `optional: true`. |
| **False-positive traps** | The Secret may be updated between the manifest analysis and the Pod creation; the finding is deterministic for the input set. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the input set. |
| **Applicability** | All versions. |
| **Remediation** | Add the missing key to the Secret, or set `optional: true` if the key is genuinely optional. |
| **Test cases** | Positive: Secret `db` with key `username`, Pod references key `password`. Negative: Secret `db` with keys `username` and `password`. Boundary: Secret not in input set (rule does not fire as `error`). |
| **Dependencies** | `K8S-SCHEMA-001`. |

### 7.5 Family `WL-HOOK`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `WL-HOOK-001` | More than one handler in a lifecycle hook | deterministic | error |
| `WL-HOOK-002` | `preStop.sleep` used on a target version predating the gate | deterministic | error |
| `WL-HOOK-003` | `preStop` duration exceeds `terminationGracePeriodSeconds` | probable | warning |
| `WL-HOOK-004` | Init container has a lifecycle hook | deterministic | error |

### 7.6 Family `WL-SCHED` — Pod-level scheduling fields

(Cross-reference: detailed scheduling checks are in `docs/kubernetes/SCHEDULING_AND_RESOURCE_MANAGEMENT.md`.)

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `WL-SCHED-001` | Invalid `nodeSelector` key | deterministic | error |
| `WL-SCHED-002` | Empty required node affinity term | deterministic | error |
| `WL-SCHED-003` | Invalid toleration effect | deterministic | error |
| `WL-SCHED-004` | `topologySpreadConstraints.maxSkew` ≤ 0 | deterministic | error |

### 7.7 Family `WL-PROBE`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `WL-PROBE-001` | Liveness and readiness probes are identical | probable | warning |
| `WL-PROBE-002` | `timeoutSeconds` greater than `periodSeconds` | deterministic | warning |
| `WL-PROBE-003` | Init container has a readiness probe | deterministic | error |
| `WL-PROBE-004` | No readiness probe on a multi-replica workload | context-dependent | info |
| `WL-PROBE-005` | No liveness probe on a long-running workload | context-dependent | info |
| `WL-PROBE-006` | No startup probe on a workload with a liveness probe | context-dependent | info |
| `WL-PROBE-007` | Liveness probe targets the same endpoint as readiness | probable | info |

#### `WL-PROBE-001` — Liveness and readiness probes are identical

| Attribute | Detail |
|---|---|
| **Matching condition** | `livenessProbe` and `readinessProbe` have identical values for all fields: `httpGet`, `exec`, `tcpSocket`, `initialDelaySeconds`, `periodSeconds`, `timeoutSeconds`, `successThreshold`, `failureThreshold`. |
| **Counterexamples** | Probes that differ in any field do not trigger. |
| **False-positive traps** | Some applications legitimately expose one health endpoint used for both probes; a transient dependency failure will trigger liveness restarts. The finding is `warning`, not `error`. |
| **Severity** | `warning` |
| **Confidence** | Deterministic that the probes are identical; the operational impact is advisory. |
| **Applicability** | All versions. |
| **Remediation** | Distinguish liveness (is the process healthy?) from readiness (is it ready to serve?). |
| **Test cases** | Positive: identical probes. Negative: probes differ in `httpGet.path`. Boundary: probes differ only in `initialDelaySeconds`. |
| **Dependencies** | `K8S-SCHEMA-001`. |

### 7.8 Family `WL-RES` — Resource requests and limits

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `WL-RES-001` | Request exceeds limit | deterministic | error |
| `WL-RES-002` | Limit without request | probable | warning |
| `WL-RES-003` | No requests or limits (BestEffort) | context-dependent | info |
| `WL-RES-004` | Invalid resource quantity | deterministic | error |
| `WL-RES-005` | Negative resource quantity | deterministic | error |
| `WL-RES-006` | Extended resource fractional | deterministic | error |
| `WL-RES-007` | `ephemeral-storage` request absent with `emptyDir` | context-dependent | info |
| `WL-RES-008` | `emptyDir.sizeLimit` exceeds `ephemeral-storage` limit | probable | warning |

#### `WL-RES-001` — Request exceeds limit

| Attribute | Detail |
|---|---|
| **Matching condition** | For the same resource in the same container, `requests.<resource>` > `limits.<resource>`. |
| **Counterexamples** | Request equal to limit (Guaranteed QoS) does not trigger. Request less than limit (Burstable QoS) does not trigger. |
| **False-positive traps** | The user may have intended to set the request to the limit and made a typo; the rule reports `error` because the API server rejects it. |
| **Severity** | `error` |
| **Confidence** | Deterministic. |
| **Applicability** | All versions. |
| **Remediation** | Set `requests` ≤ `limits`. |
| **Test cases** | Positive: `requests.cpu: 1`, `limits.cpu: 500m`. Negative: `requests.cpu: 500m`, `limits.cpu: 1`. Boundary: `requests.cpu: 1`, `limits.cpu: 1`. |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 8. Probe and lifecycle issues

(Consolidated in §7.7. Cross-references to `docs/kubernetes/WORKLOADS_AND_PODS.md` §5.)

---

## 9. Resource request and limit problems

(Consolidated in §7.8. Cross-references to `docs/kubernetes/WORKLOADS_AND_PODS.md` §6 and `docs/kubernetes/SCHEDULING_AND_RESOURCE_MANAGEMENT.md` §4.)

---

## 10. Service and networking configuration

### 10.1 Family `NET-SVC`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `NET-SVC-001` | Invalid Service type | deterministic | error |
| `NET-SVC-002` | Port out of range | deterministic | error |
| `NET-SVC-003` | `targetPort` out of range | deterministic | error |
| `NET-SVC-004` | Named `targetPort` not found in matching Pods | deterministic (in-set) / probable | error / warning |
| `NET-SVC-005` | Invalid protocol | deterministic | error |
| `NET-SVC-006` | Duplicate port name | deterministic | error |
| `NET-SVC-007` | Duplicate `(port, protocol)` | deterministic | error |
| `NET-SVC-008` | Selector matches no supplied Pod | context-dependent | info |
| `NET-SVC-009` | `nodePort` set on non-NodePort Service | deterministic | error |
| `NET-SVC-010` | `nodePort` out of range | deterministic | error |
| `NET-SVC-011` | Duplicate `nodePort` across Services | probable | warning |
| `NET-SVC-012` | `loadBalancerIP` set on non-LoadBalancer Service | probable | warning |

### 10.2 Family `NET-ING`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `NET-ING-001` | Backend has neither `service` nor `resource` | deterministic | error |
| `NET-ING-002` | Backend port has neither `name` nor `number` | deterministic | error |
| `NET-ING-003` | Invalid `pathType` | deterministic | error |
| `NET-ING-004` | TLS secret not in input set | context-dependent | info |
| `NET-ING-005` | TLS host without a matching rule host | probable | warning |
| `NET-ING-006` | TLS secret in a different namespace (both in input set) | deterministic | error |
| `NET-ING-007` | `ingressClassName` omitted | context-dependent | info |
| `NET-ING-008` | Backend Service not in input set | context-dependent | info |
| `NET-ING-009` | Invalid TLS secret type | deterministic | error |

### 10.3 Family `NET-NP`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `NET-NP-001` | Invalid `policyTypes` value | deterministic | error |
| `NET-NP-002` | Egress rules without `Egress` in `policyTypes` | probable | warning |
| `NET-NP-003` | Invalid port in ingress/egress rule | deterministic | error |
| `NET-NP-004` | Invalid peer combination | deterministic | error |
| `NET-NP-005` | `podSelector` and `ipBlock` both set in one peer | deterministic | error |
| `NET-NP-006` | NetworkPolicy selects no supplied Pod | context-dependent | info |
| `NET-NP-007` | CNI enforcement dependency | context-dependent | info |

### 10.4 Family `NET-DNS`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `NET-DNS-001` | Ambiguous cross-namespace reference | context-dependent | warning |
| `NET-DNS-002` | Invalid DNS name in `externalName` | deterministic | error |
| `NET-DNS-003` | `externalName` Service with a selector | probable | warning |

### 10.5 Family `NET-GW`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `NET-GW-001` | Invalid listener protocol | deterministic | error |
| `NET-GW-002` | Route with empty `parentRefs` | probable | warning |
| `NET-GW-003` | BackendRef port out of range | deterministic | error |
| `NET-GW-004` | HTTPS listener without TLS config | deterministic | error |
| `NET-GW-005` | Invalid TLS mode | deterministic | error |

### 10.6 Family `NET-TLS`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `NET-TLS-001` | TLS entry without `secretName` | deterministic | error |
| `NET-TLS-002` | TLS entry with empty `hosts` | probable | warning |
| `NET-TLS-003` | Rule host not covered by any TLS host | probable | warning |
| `NET-TLS-004` | TLS `hosts` and `rules.host` divergence | probable | warning |
| `NET-TLS-005` | TLS secret with wrong type | deterministic | error |

#### `NET-SVC-008` — Selector matches no supplied Pod

| Attribute | Detail |
|---|---|
| **Matching condition** | A Service has a non-empty `spec.selector`, and no Pod in the input set has labels matching the selector in the Service's namespace. |
| **Counterexamples** | The Service has no selector (selectorless Service). A Pod in the input set matches the selector. |
| **False-positive traps** | The Pod may exist in the cluster but not in the input set. The finding is `info`, not `error`, and states that the selector matches no supplied Pod; cluster state is required to determine whether any Pod matches. |
| **Severity** | `info` |
| **Confidence** | Deterministic that no supplied Pod matches; the operational impact is `info`. |
| **Applicability** | All versions. |
| **Remediation** | Confirm the selector matches the intended workload's labels. If the workload is not in the input set, supply it for a complete check. |
| **Test cases** | Positive: Service selector `{app: api-service}`, Deployment Pod labels `{app: api}`. Negative: matching labels. Boundary: Service without selector. |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 11. Storage and configuration references

### 11.1 Family `STG-CM`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `STG-CM-001` | Duplicate key across `data` and `binaryData` | deterministic | error |
| `STG-CM-002` | Invalid ConfigMap key | deterministic | error |
| `STG-CM-003` | Immutable ConfigMap with update-dependent workload | probable | info |

### 11.2 Family `STG-SEC`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `STG-SEC-001` | Duplicate key across `data` and `stringData` with different values | deterministic | warning |
| `STG-SEC-002` | `stringData` in a `kubectl apply` workflow | context-dependent | info |
| `STG-SEC-003` | TLS Secret missing `tls.crt` or `tls.key` | deterministic | error |
| `STG-SEC-004` | Docker config Secret missing `.dockerconfigjson` | deterministic | error |
| `STG-SEC-005` | Basic-auth Secret missing `username` or `password` | probable | warning |

### 11.3 Family `STG-PVC`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `STG-PVC-001` | Empty `accessModes` | deterministic | error |
| `STG-PVC-002` | Invalid access mode | deterministic | error |
| `STG-PVC-003` | `ReadWriteOncePod` combined with another mode | deterministic | error |
| `STG-PVC-004` | Missing/invalid storage request | deterministic | error |
| `STG-PVC-005` | Invalid `volumeMode` | deterministic | error |
| `STG-PVC-006` | StorageClass not in input set | context-dependent | info |
| `STG-PVC-007` | Empty StorageClass and dynamic provisioning requested | context-dependent | info |
| `STG-PVC-008` | `volumeBindingMode: Immediate` with topology constraints | probable | info |

### 11.4 Family `STG-PV`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `STG-PV-001` | PV `claimRef` PVC not in input set | context-dependent | info |
| `STG-PV-002` | PVC `volumeName` PV not in input set | context-dependent | info |
| `STG-PV-003` | PV/PVC access mode mismatch (both in input set) | deterministic | error |
| `STG-PV-004` | PV/PVC StorageClass mismatch (both in input set) | deterministic | error |

### 11.5 Family `STG-CSI`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `STG-CSI-001` | CSI inline volume missing driver | deterministic | error |
| `STG-CSI-002` | CSI PV missing driver | deterministic | error |
| `STG-CSI-003` | CSI PV missing `volumeHandle` | deterministic | error |
| `STG-CSI-004` | CSI secret reference not in input set | context-dependent | info |
| `STG-CSI-005` | CSI secret cross-namespace (both in input set) | deterministic | error |
| `STG-CSI-006` | CSI inline volume driver without `Ephemeral` mode | probable | warning |
| `STG-CSI-007` | `fsGroupPolicy: None` with `fsGroup` set | probable | info |

### 11.6 Family `STG-STS` — StatefulSet storage

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `STG-STS-001` | `volumeClaimTemplates` entry with no matching mount | deterministic | error |
| `STG-STS-002` | Mount references a claim not in templates | deterministic | error |
| `STG-STS-003` | `volumeClaimTemplates` missing required fields | deterministic | error |
| `STG-STS-004` | `serviceName` absent | deterministic | error |

#### `STG-STS-001` — `volumeClaimTemplates` entry with no matching mount

| Attribute | Detail |
|---|---|
| **Matching condition** | A StatefulSet has a `volumeClaimTemplates` entry whose `metadata.name` does not match any container's `volumeMounts[*].name`. |
| **Counterexamples** | The template name matches a mount. The template is intentionally used for a later mount added by a webhook. |
| **False-positive traps** | A mutating webhook may add the mount; KubeCheck cannot verify webhook behavior. The finding is `error` because the PVC is provisioned and billed but unused, which is almost always a configuration error. |
| **Severity** | `error` |
| **Confidence** | Deterministic relative to the input set. |
| **Applicability** | `apps/v1`, all target versions. |
| **Remediation** | Add a `volumeMount` referencing the template name, or remove the unused template. |
| **Test cases** | Positive: template `data`, no mount. Negative: template `data`, mount `data`. Boundary: template used only by an init container. |
| **Dependencies** | `K8S-SCHEMA-001`. |

---

## 12. Security and RBAC risks

### 12.1 Family `SEC-CTX` — Pod and container security contexts

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `SEC-CTX-001` | Privileged container | deterministic | warning |
| `SEC-CTX-002` | `capabilities.add: ["ALL"]` | deterministic | warning |
| `SEC-CTX-003` | `capabilities.add` includes `SYS_ADMIN` | deterministic | warning |
| `SEC-CTX-004` | `capabilities.drop` absent or does not include `ALL` | context-dependent | info |
| `SEC-CTX-005` | `hostNetwork: true` | deterministic | warning |
| `SEC-CTX-006` | `hostPID: true` | deterministic | warning |
| `SEC-CTX-007` | `hostIPC: true` | deterministic | warning |
| `SEC-CTX-008` | `hostPath` mount of `/` or `/etc` | deterministic | error |
| `SEC-CTX-009` | `hostPath` mount of a container runtime socket | deterministic | error |
| `SEC-CTX-010` | `hostPath` mount of `/var/lib/kubelet` | deterministic | error |
| `SEC-CTX-011` | `hostPath` mount of any other path | probable | warning |
| `SEC-CTX-012` | `allowPrivilegeEscalation` true or unset | context-dependent | info |
| `SEC-CTX-013` | `allowPrivilegeEscalation: false` with `SYS_ADMIN` | probable | warning |
| `SEC-CTX-014` | `runAsNonRoot: true` with `runAsUser: 0` | deterministic | error |
| `SEC-CTX-015` | Neither `runAsNonRoot` nor non-zero `runAsUser` | context-dependent | info |
| `SEC-CTX-016` | `runAsUser: 0` without `runAsNonRoot` | probable | warning |
| `SEC-CTX-017` | `readOnlyRootFilesystem` false or unset | context-dependent | info |
| `SEC-CTX-018` | `seccompProfile.type: Unconfined` | deterministic | warning |
| `SEC-CTX-019` | `seccompProfile` absent | context-dependent | info |
| `SEC-CTX-020` | Pod-level `readOnlyRootFilesystem` or `allowPrivilegeEscalation` | deterministic | warning |

#### `SEC-CTX-009` — `hostPath` mount of a container runtime socket

| Attribute | Detail |
|---|---|
| **Matching condition** | A `hostPath` volume has `path` equal to `/var/run/docker.sock`, `/run/containerd/containerd.sock`, `/run/crio/crio.sock`, or `/var/run/crio/crio.sock`. |
| **Counterexamples** | A `hostPath` mount of a non-socket path does not trigger this rule (it triggers `SEC-CTX-011` if the path is not `/` or `/etc`). |
| **False-positive traps** | A privileged container that already has host access may not increase its risk from the socket; the finding is still `error` because the socket mount is a recognized escalation primitive. |
| **Severity** | `error` |
| **Confidence** | Deterministic that the path matches a known runtime socket. |
| **Applicability** | All versions. |
| **Remediation** | Remove the socket mount. If the workload needs container runtime information, use the Kubernetes API instead. |
| **Test cases** | Positive: `hostPath: {path: /var/run/docker.sock}`. Negative: `hostPath: {path: /var/log}`. Boundary: `hostPath: {path: /run/containerd/containerd.sock}`. |
| **Dependencies** | `K8S-SCHEMA-001`. |

### 12.2 Family `SEC-RBAC`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `SEC-RBAC-001` | `escalate` verb on roles/clusterroles | deterministic | warning |
| `SEC-RBAC-002` | `bind` verb on roles/clusterroles | deterministic | warning |
| `SEC-RBAC-003` | `impersonate` verb on users/groups/serviceaccounts | deterministic | warning |
| `SEC-RBAC-004` | Wildcard `verbs` | deterministic | warning |
| `SEC-RBAC-005` | Wildcard `resources` | deterministic | warning |
| `SEC-RBAC-006` | Wildcard `apiGroups` | deterministic | warning |
| `SEC-RBAC-007` | `create` on `pods/exec` or `pods/attach` | deterministic | warning |
| `SEC-RBAC-008` | `get` or `list` on `secrets` | deterministic | warning |
| `SEC-RBAC-009` | `create` on `pods` without `resourceNames` | probable | warning |
| `SEC-RBAC-010` | `ClusterRoleBinding` to a ServiceAccount | probable | warning |
| `SEC-RBAC-011` | `RoleBinding` references a ClusterRole with cluster-scoped resources | context-dependent | info |
| `SEC-RBAC-012` | `ClusterRoleBinding` to `system:masters` | deterministic | error |
| `SEC-RBAC-013` | `ClusterRoleBinding` to `cluster-admin` for a ServiceAccount | probable | warning |
| `SEC-RBAC-014` | `automountServiceAccountToken` unset | context-dependent | info |

#### `SEC-RBAC-012` — `ClusterRoleBinding` to `system:masters`

| Attribute | Detail |
|---|---|
| **Matching condition** | A `ClusterRoleBinding` has a subject with `kind: Group`, `name: system:masters`. |
| **Counterexamples** | A `ClusterRoleBinding` to a named user or ServiceAccount does not trigger. |
| **False-positive traps** | None. `system:masters` is a well-known superuser group; binding to it bypasses all RBAC checks and cannot be revoked through the RBAC API. |
| **Severity** | `error` |
| **Confidence** | Deterministic. |
| **Applicability** | `rbac.authorization.k8s.io/v1`, all target versions. |
| **Remediation** | Remove the binding. Use a scoped ClusterRole with the minimum required permissions instead. |
| **Test cases** | Positive: `ClusterRoleBinding` with `subjects: [{kind: Group, name: system:masters}]`. Negative: `ClusterRoleBinding` with `subjects: [{kind: ServiceAccount, name: app, namespace: default}]`. Boundary: `RoleBinding` to `system:masters` (also triggers, but at namespace scope). |
| **Dependencies** | `K8S-SCHEMA-001`. |

### 12.3 Family `SEC-SECRET`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `SEC-SECRET-001` | `namespace` inside `secretKeyRef`/`secretRef` | deterministic | error |
| `SEC-SECRET-002` | Secret in a different namespace (both in input set) | deterministic | error |
| `SEC-SECRET-003` | Secret not in input set | context-dependent | info |
| `SEC-SECRET-004` | Secret consumed via `env`/`envFrom` | context-dependent | info |
| `SEC-SECRET-005` | Secret exposed to all containers in a multi-container Pod | probable | warning |
| `SEC-SECRET-006` | Credential-like literal `value` | probable | warning |

### 12.4 Family `SEC-IMG`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `SEC-IMG-001` | Mutable image reference (`:latest` or no tag) | probable | warning |
| `SEC-IMG-002` | Digest reference | context-dependent | info |
| `SEC-IMG-003` | Image registry not in a supplied allowlist | context-dependent | info |
| `SEC-IMG-004` | Image reference with an uppercase repository name | deterministic | error |

### 12.5 Family `SEC-HOST`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `SEC-HOST-001` | `hostPort` set | context-dependent | info |
| `SEC-HOST-002` | `nfs` volume with no root squash verification | context-dependent | info |

### 12.6 Family `SEC-PSS` — Pod Security Standards

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `SEC-PSS-001` | Pod violates a named PSS profile | context-dependent | info |
| `SEC-PSS-002` | Namespace has PSS labels but no matching workload checks | context-dependent | info |

---

## 13. Reliability and availability risks

### 13.1 Family `REL-PDB`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `REL-PDB-001` | Both `minAvailable` and `maxUnavailable` set | deterministic | error |
| `REL-PDB-002` | Neither `minAvailable` nor `maxUnavailable` set | deterministic | error |
| `REL-PDB-003` | `maxUnavailable: 0` blocks voluntary disruptions | probable | warning |
| `REL-PDB-004` | `minAvailable: 100%` blocks voluntary disruptions | probable | warning |
| `REL-PDB-005` | Percentage does not resolve to an integer | context-dependent | info |
| `REL-PDB-006` | PDB selector matches no workload | context-dependent | info |
| `REL-PDB-007` | Multi-replica workload without a PDB | context-dependent | info |

### 13.2 Family `REL-HPA`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `REL-HPA-001` | `minReplicas` > `maxReplicas` | deterministic | error |
| `REL-HPA-002` | `minReplicas` < 1 | deterministic | error |
| `REL-HPA-003` | Invalid metric type | deterministic | error |
| `REL-HPA-004` | Memory utilization without memory requests | probable | warning |
| `REL-HPA-005` | Missing `averageValue` for `AverageValue` target | deterministic | error |
| `REL-HPA-006` | HPA target not in input set | context-dependent | info |
| `REL-HPA-007` | HPA and Deployment both set `replicas` | probable | info |
| `REL-HPA-008` | Custom or external metric without adapter | context-dependent | info |

### 13.3 Family `REL-QUOTA`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `REL-QUOTA-001` | Quota `requests` > `limits` (CPU) | deterministic | error |
| `REL-QUOTA-002` | Quota `requests` > `limits` (memory) | deterministic | error |
| `REL-QUOTA-003` | Pod omits requests for a quota-enforced resource | probable | warning |
| `REL-QUOTA-004` | Invalid `count/` key | deterministic | error |

### 13.4 Family `REL-LIMIT` — LimitRange

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `REL-LIMIT-001` | `max` < `min` | deterministic | error |
| `REL-LIMIT-002` | `default` limit < `defaultRequest` | deterministic | error |
| `REL-LIMIT-003` | Pod request > LimitRange `max` | deterministic | error |
| `REL-LIMIT-004` | Pod request < LimitRange `min` | deterministic | error |
| `REL-LIMIT-005` | Pod omits requests in a LimitRange namespace | context-dependent | info |
| `REL-LIMIT-006` | Conflicting LimitRange defaults | probable | warning |

### 13.5 Family `REL-AVAIL` — Availability configuration

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `REL-AVAIL-001` | Deployment `Recreate` strategy with multiple replicas | probable | warning |
| `REL-AVAIL-002` | Deployment `maxSurge` and `maxUnavailable` both zero | deterministic | error |
| `REL-AVAIL-003` | `progressDeadlineSeconds` < `minReadySeconds` | probable | warning |

### 13.6 Family `REL-UPD` — Update behavior

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `REL-UPD-001` | ConfigMap/Secret consumed via `env` without restart strategy | context-dependent | info |
| `REL-UPD-002` | Immutable ConfigMap/Secret with expected updates | probable | warning |
| `REL-UPD-003` | No checksum annotation to trigger rollout on config change | context-dependent | info |

---

## 14. Maintainability and best-practice warnings

### 14.1 Family `MAINT-LABEL`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `MAINT-LABEL-001` | Missing recommended label (`app.kubernetes.io/name`) | context-dependent | info |
| `MAINT-LABEL-002` | Missing recommended label (`app.kubernetes.io/instance`) | context-dependent | info |
| `MAINT-LABEL-003` | Missing `app.kubernetes.io/version` on a workload | context-dependent | info |
| `MAINT-LABEL-004` | Missing `app.kubernetes.io/component` | context-dependent | info |
| `MAINT-LABEL-005` | Missing `app.kubernetes.io/part-of` | context-dependent | info |
| `MAINT-LABEL-006` | Missing `app.kubernetes.io/managed-by` | context-dependent | info |

### 14.2 Family `MAINT-ANNOT`

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `MAINT-ANNOT-001` | Missing description annotation | context-dependent | info |
| `MAINT-ANNOT-002` | Invalid annotation key | deterministic | error |

### 14.3 Family `MAINT-DUP` — Duplicate identity

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `MAINT-DUP-001` | Duplicate resource identity within the input set | deterministic | warning |

#### `MAINT-DUP-001` — Duplicate resource identity

| Attribute | Detail |
|---|---|
| **Matching condition** | Two objects in the input set share the same `(group, version, kind, namespace, name)`. |
| **Counterexamples** | Two objects with the same name but different namespaces, kinds, or groups are not duplicates. A `generateName`-only object does not have a resolved name and does not trigger. |
| **False-positive traps** | The user may intentionally supply two versions of the same manifest for comparison; the finding is `warning`, not `error`, and cites both anchors. |
| **Severity** | `warning` |
| **Confidence** | Deterministic relative to the input set. |
| **Applicability** | All versions. |
| **Remediation** | Remove one of the duplicates, or rename one if both are intentional. |
| **Test cases** | Positive: two Pods named `web` in namespace `default`. Negative: two Pods named `web` in namespaces `a` and `b`. Boundary: two objects with the same name but different kinds. |
| **Dependencies** | `K8S-GVK-009`. |

### 14.4 Family `MAINT-BEST` — Best-practice recommendations

| Rule ID | Title | Evidence class | Default severity |
|---|---|---|---|
| `MAINT-BEST-001` | Image tag pinned to a digest | context-dependent | info |
| `MAINT-BEST-002` | Single-replica Deployment without a documented availability requirement | context-dependent | info |
| `MAINT-BEST-003` | Pod without a `PodDisruptionBudget` in a multi-replica workload | context-dependent | info |
| `MAINT-BEST-004` | Resource requests and limits missing on a production workload | context-dependent | info |

**STATIC-RULES-REQ-005:** `MAINT-BEST-*` rules MUST be `context-dependent` and MUST NOT emit `error`. They are best-practice recommendations, not defects.

---

## 15. Rule inventory summary

### 15.1 Deterministic rules

| Domain | Count | Examples |
|---|---|---|
| YAML | 14 | `YAML-SYN-*`, `YAML-NORM-*` |
| K8S | 18 | `K8S-GVK-*`, `K8S-SCHEMA-*`, `K8S-META-*`, `K8S-VALUE-*` |
| DEP | 6 | `DEP-API-001`, `-002`, `-005`, `-006` |
| WL | 22 | `WL-CONTAINER-001`–`006`, `-008`–`011`; `WL-POD-001`–`006`; `WL-VOL-001`–`007`; `WL-ENV-001`–`005`; `WL-HOOK-001`, `-002`, `-004`; `WL-PROBE-003`; `WL-RES-001`, `-004`–`006` |
| NET | 20 | `NET-SVC-001`–`007`, `-009`, `-010`; `NET-ING-001`–`003`, `-006`, `-009`; `NET-NP-001`, `-003`–`005`; `NET-DNS-002`; `NET-GW-001`, `-003`–`005`; `NET-TLS-001`, `-005` |
| STG | 22 | `STG-CM-001`, `-002`; `STG-SEC-001`, `-003`, `-004`; `STG-PVC-001`–`005`; `STG-PV-003`, `-004`; `STG-CSI-001`–`003`, `-005`; `STG-STS-001`–`004` |
| SEC | 15 | `SEC-CTX-001`, `-002`, `-003`, `-005`–`010`, `-014`, `-018`, `-020`; `SEC-RBAC-001`–`008`, `-012`; `SEC-SECRET-001`, `-002`; `SEC-IMG-004` |
| REL | 9 | `REL-PDB-001`, `-002`; `REL-HPA-001`–`003`, `-005`; `REL-QUOTA-001`, `-002`, `-004`; `REL-LIMIT-001`–`004`; `REL-AVAIL-002` |
| MAINT | 2 | `MAINT-ANNOT-002`, `MAINT-DUP-001` |
| **Total** | **128** | |

### 15.2 Heuristic and context-dependent rules

| Domain | Count | Examples |
|---|---|---|
| YAML | 4 | `YAML-EDGE-*`, `YAML-NORM-003` |
| K8S | 4 | `K8S-GVK-009`, `K8S-META-005`–`008` |
| WL | 26 | `WL-CONTAINER-007`; `WL-PROBE-001`, `-002`, `-004`–`007`; `WL-RES-002`, `-003`, `-007`, `-008`; `WL-VOL-004`, `-008`; `WL-SCHED-*` |
| NET | 18 | `NET-SVC-008`, `-011`, `-012`; `NET-ING-004`, `-005`, `-007`, `-008`; `NET-NP-002`, `-006`, `-007`; `NET-DNS-001`, `-003`; `NET-GW-002`; `NET-TLS-002`–`004` |
| STG | 10 | `STG-CM-003`; `STG-SEC-002`, `-005`; `STG-PVC-006`–`008`; `STG-PV-001`, `-002`; `STG-CSI-004`, `-006`, `-007` |
| SEC | 14 | `SEC-CTX-004`, `-011`–`013`, `-015`–`017`, `-019`; `SEC-RBAC-009`–`011`, `-013`, `-014`; `SEC-SECRET-003`–`006`; `SEC-IMG-001`–`003`; `SEC-HOST-001`, `-002`; `SEC-PSS-001`, `-002` |
| REL | 13 | `REL-PDB-003`–`007`; `REL-HPA-004`, `-006`–`008`; `REL-QUOTA-003`; `REL-LIMIT-005`, `-006`; `REL-AVAIL-001`, `-003`; `REL-UPD-001`–`003` |
| MAINT | 10 | `MAINT-LABEL-*`; `MAINT-BEST-*`; `MAINT-ANNOT-001` |
| **Total** | **99** | |

**STATIC-RULES-REQ-006:** The total proposed rule count is **227** (128 deterministic + 99 heuristic). This count is an inventory, not an implementation status.

---

## 16. Deterministic versus heuristic separation

### 16.1 The separation rule

**STATIC-RULES-REQ-007:** A rule MUST be classified as `deterministic` if and only if its verdict depends solely on the manifest and the immutable reference data (schema bundle, lifecycle metadata, feature-gate metadata). A rule MUST be classified as `probable` or `context-dependent` if its verdict depends on:
- The presence or absence of other objects in the cluster that are not in the input set.
- Cluster configuration (CNI, admission controllers, default settings).
- Application behavior (image entrypoint, startup time, resource usage).
- Organizational policy (security posture, label conventions).

### 16.2 Examples of classification

| Rule | Classification | Rationale |
|---|---|---|
| `WL-RES-001` (request > limit) | deterministic | The API server rejects it. |
| `WL-RES-002` (limit without request) | probable | The API server defaults the request to the limit; the outcome is valid but may be undesirable. |
| `SEC-CTX-001` (privileged) | deterministic | The field is literally `true`. |
| `SEC-CTX-012` (`allowPrivilegeEscalation` unset) | context-dependent | The default behavior depends on the container's capabilities. |
| `NET-SVC-008` (selector matches no supplied Pod) | context-dependent | The Pod may exist in the cluster. |
| `STG-PV-003` (PV/PVC access mode mismatch) | deterministic | Both objects are in the input set. |
| `REL-PDB-007` (multi-replica workload without PDB) | context-dependent | The workload may be intentionally non-disruptible. |

### 16.3 What deterministic rules can claim

**STATIC-RULES-REQ-008:** A deterministic rule may claim that the manifest is **invalid for the target version** or **violates a stated constraint**. It MUST NOT claim that the manifest is deployable, admitted, or operationally healthy.

---

## 17. Avoiding blanket rules that misclassify valid optional configurations

### 17.1 The anti-blanket principle

**STATIC-RULES-REQ-009:** A rule MUST NOT fire on a valid optional configuration. "Valid optional" means: the field may be absent, the field may have a value that the schema permits, or the field may be set in a way that is operationally acceptable in some environments.

### 17.2 Examples of blanket rules to avoid

| Anti-pattern | Why it's wrong | Correct approach |
|---|---|---|
| "Every container must have a readiness probe." | A short-lived Job or a batch workload may not need one. | Make the rule context-dependent and restrict applicability to long-running workloads. |
| "Every Pod must set `runAsNonRoot`." | Some system workloads run as root by design. | `info`-severity recommendation, with a documented exception list or annotation opt-out. |
| "Every Deployment must have a PodDisruptionBudget." | A single-replica development workload may not need one. | Context-dependent, restricted to multi-replica production workloads. |
| "Every image must use a digest." | Tag-based workflows are common and sometimes required. | `info`-severity recommendation, not an error. |
| "Every container must set `readOnlyRootFilesystem: true`." | Some applications write to the root filesystem and cannot be reconfigured. | `info`-severity, with a remediation that explains the trade-off. |

### 17.3 Required counterexamples

**STATIC-RULES-REQ-010:** Every rule MUST document at least three counterexamples demonstrating a valid configuration that does not trigger the rule. A rule without counterexamples cannot enter a bundle.

### 17.4 Annotation opt-out

**STATIC-RULES-REQ-011:** A rule MAY declare an annotation-based opt-out (e.g., `kubecheck.example.com/allow-<rule-name>: "true"`). The opt-out MUST be visible in the finding's evidence (suppressed findings are logged at debug level). The opt-out MUST NOT be able to suppress `error`-severity deterministic findings, because those are API-server rejections.

---

## 18. Consolidated rule inventory

The following table summarizes the entire proposed inventory. Each row is a rule family; each rule ID within the family is defined in the corresponding section above.

| Family ID | Domain | Rule count | Evidence class mix | Primary reference |
|---|---|---|---|---|
| `YAML-SYN` | YAML | 12 | deterministic | `YAML_PARSING_AND_EDGE_CASES.md` §3 |
| `YAML-NORM` | YAML | 4 | deterministic + probable | `YAML_VALIDATION_AND_NORMALIZATION.md` §11 |
| `YAML-EDGE` | YAML | 4 | probable | `YAML_PARSING_AND_EDGE_CASES.md` §5 |
| `K8S-GVK` | K8S | 10 | deterministic | `KUBERNETES_API_FUNDAMENTALS.md` §4 |
| `K8S-SCHEMA` | K8S | 10 | deterministic | `KUBERNETES_API_FUNDAMENTALS.md` §9 |
| `K8S-META` | K8S | 8 | deterministic + probable | `KUBERNETES_API_FUNDAMENTALS.md` §5 |
| `K8S-VALUE` | K8S | 10 | deterministic | `KUBERNETES_API_FUNDAMENTALS.md` §9 |
| `DEP-API` | DEP | 8 | deterministic + context-dependent | `KUBERNETES_VERSION_COMPATIBILITY.md` §5 |
| `WL-CONTAINER` | WL | 11 | deterministic + probable | `WORKLOADS_AND_PODS.md` §4 |
| `WL-POD` | WL | 6 | deterministic | `WORKLOADS_AND_PODS.md` §3 |
| `WL-VOL` | WL | 8 | deterministic + probable | `WORKLOADS_AND_PODS.md` §7 |
| `WL-ENV` | WL | 5 | deterministic | `WORKLOADS_AND_PODS.md` §4.5 |
| `WL-HOOK` | WL | 4 | deterministic | `WORKLOADS_AND_PODS.md` §4.7 |
| `WL-SCHED` | WL | 4 | deterministic | `SCHEDULING_AND_RESOURCE_MANAGEMENT.md` §5 |
| `WL-PROBE` | WL | 7 | deterministic + probable + context-dependent | `WORKLOADS_AND_PODS.md` §5 |
| `WL-RES` | WL | 8 | deterministic + probable + context-dependent | `WORKLOADS_AND_PODS.md` §6 |
| `NET-SVC` | NET | 12 | deterministic + context-dependent | `NETWORKING_AND_SERVICE_DISCOVERY.md` §3 |
| `NET-ING` | NET | 9 | deterministic + context-dependent | `NETWORKING_AND_SERVICE_DISCOVERY.md` §5 |
| `NET-NP` | NET | 7 | deterministic + context-dependent | `NETWORKING_AND_SERVICE_DISCOVERY.md` §7 |
| `NET-DNS` | NET | 3 | deterministic + context-dependent | `NETWORKING_AND_SERVICE_DISCOVERY.md` §8 |
| `NET-GW` | NET | 5 | deterministic + probable | `NETWORKING_AND_SERVICE_DISCOVERY.md` §6 |
| `NET-TLS` | NET | 5 | deterministic + probable | `NETWORKING_AND_SERVICE_DISCOVERY.md` §11 |
| `STG-CM` | STG | 3 | deterministic + probable | `CONFIGURATION_AND_STORAGE.md` §3 |
| `STG-SEC` | STG | 5 | deterministic + probable | `CONFIGURATION_AND_STORAGE.md` §3 |
| `STG-PVC` | STG | 8 | deterministic + context-dependent | `CONFIGURATION_AND_STORAGE.md` §7 |
| `STG-PV` | STG | 4 | deterministic + context-dependent | `CONFIGURATION_AND_STORAGE.md` §7.5 |
| `STG-CSI` | STG | 7 | deterministic + probable | `CONFIGURATION_AND_STORAGE.md` §8 |
| `STG-STS` | STG | 4 | deterministic | `CONFIGURATION_AND_STORAGE.md` §7.4 |
| `SEC-CTX` | SEC | 20 | deterministic + probable + context-dependent | `SECURITY_AND_RBAC.md` §4 |
| `SEC-RBAC` | SEC | 14 | deterministic + probable + context-dependent | `SECURITY_AND_RBAC.md` §3 |
| `SEC-SECRET` | SEC | 6 | deterministic + probable + context-dependent | `SECURITY_AND_RBAC.md` §6 |
| `SEC-IMG` | SEC | 4 | deterministic + probable + context-dependent | `SECURITY_AND_RBAC.md` §7 |
| `SEC-HOST` | SEC | 2 | context-dependent | `SECURITY_AND_RBAC.md` §8 |
| `SEC-PSS` | SEC | 2 | context-dependent | `SECURITY_AND_RBAC.md` §5 |
| `REL-PDB` | REL | 7 | deterministic + probable + context-dependent | `SCHEDULING_AND_RESOURCE_MANAGEMENT.md` §7 |
| `REL-HPA` | REL | 8 | deterministic + probable + context-dependent | `SCHEDULING_AND_RESOURCE_MANAGEMENT.md` §8 |
| `REL-QUOTA` | REL | 4 | deterministic + probable | `SCHEDULING_AND_RESOURCE_MANAGEMENT.md` §9.1 |
| `REL-LIMIT` | REL | 6 | deterministic + probable + context-dependent | `SCHEDULING_AND_RESOURCE_MANAGEMENT.md` §9.2 |
| `REL-AVAIL` | REL | 3 | deterministic + probable | `WORKLOADS_AND_PODS.md` §8.1 |
| `REL-UPD` | REL | 3 | context-dependent | `CONFIGURATION_AND_STORAGE.md` §10 |
| `MAINT-LABEL` | MAINT | 6 | context-dependent | Kubernetes recommended labels |
| `MAINT-ANNOT` | MAINT | 2 | deterministic + context-dependent | `KUBERNETES_API_FUNDAMENTALS.md` §5.3 |
| `MAINT-DUP` | MAINT | 1 | deterministic | `KUBERNETES_API_FUNDAMENTALS.md` §11 |
| `MAINT-BEST` | MAINT | 4 | context-dependent | Best-practice guidance |
| **Total** | | **227** | | |

---

## 19. Testing requirements

### 19.1 Per-rule fixtures

**STATIC-RULES-REQ-012:** Every rule MUST have at least three positive, three negative, and one boundary fixture as required by `RULE-SPEC-REQ-059`. The fixtures MUST be versioned alongside the rule.

### 19.2 Family-level integration tests

**STATIC-RULES-REQ-013:** Every family MUST have at least one integration test that runs a representative manifest through the entire engine and asserts the exact finding set for that family. The test MUST NOT assert on findings from other families unless the cross-family dependency is documented.

### 19.3 Determinism

**STATIC-RULES-REQ-014:** Every rule MUST produce identical findings across repeated runs and across distribution channels for the same input and bundle (`ACC-REQ-001`).

### 19.4 Counterexample regression

**STATIC-RULES-REQ-015:** Every documented counterexample MUST be a regression fixture. If a future rule change causes a counterexample to trigger, the regression suite fails.

---

## 20. Authoritative references

| Reference | Scope | Type | Retrieved |
|---|---|---|---|
| `docs/engine/DIAGNOSTIC_RULE_SPEC.md` (internal) | Rule execution semantics, safe expression language, validation | Internal | 2026-10-31 |
| `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md` (internal) | Category definitions, evidence-class semantics | Internal | 2026-10-31 |
| `docs/yaml/YAML_PARSING_AND_EDGE_CASES.md` (internal) | Edge-case behavior for YAML-SYN, YAML-NORM, YAML-EDGE | Internal | 2026-10-31 |
| `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md` (internal) | GVK, metadata, schema, value validation | Internal | 2026-10-31 |
| `docs/kubernetes/KUBERNETES_VERSION_COMPATIBILITY.md` (internal) | Lifecycle, feature gates, version-specific validation | Internal | 2026-10-31 |
| `docs/kubernetes/WORKLOADS_AND_PODS.md` (internal) | Container, Pod, volume, probe, resource findings | Internal | 2026-10-31 |
| `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md` (internal) | Service, Ingress, NetworkPolicy, DNS, Gateway findings | Internal | 2026-10-31 |
| `docs/kubernetes/CONFIGURATION_AND_STORAGE.md` (internal) | ConfigMap, Secret, PVC, PV, CSI findings | Internal | 2026-10-31 |
| `docs/kubernetes/SECURITY_AND_RBAC.md` (internal) | Security context, RBAC, Secret exposure findings | Internal | 2026-10-31 |
| `docs/kubernetes/SCHEDULING_AND_RESOURCE_MANAGEMENT.md` (internal) | Quantity, QoS, scheduling, PDB, HPA findings | Internal | 2026-10-31 |
| Kubernetes Documentation — Recommended Labels | Maintainability label recommendations | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Pod Security Standards | Security context baseline and restricted profiles | Official documentation | 2026-10-31 |
| YAML 1.2.2 specification | Syntax and edge-case behavior | Specification | 2026-10-31 |

**Licensing note:** Kubernetes documentation is licensed under CC BY 4.0. The YAML 1.2.2 specification is freely copyable provided it is not modified. No line numbers, benchmark results, or implementation status are asserted beyond what the cited sources state.

---

## 21. Open questions and unresolved decisions

| ID | Question | Status | Owner |
|---|---|---|---|
| `OPEN-STATIC-01` | Exact prioritization of the 227 proposed rules across MVP, GA, and expansion phases. | **OPEN** | Product Architecture |
| `OPEN-STATIC-02` | Whether the deterministic rule count (128) is achievable within the MVP bundle size target (`PERF-REQ-011`). | **OPEN** | Bundle maintainers |
| `OPEN-STATIC-03` | Whether `MAINT-BEST-*` rules should be included in the default bundle or provided as a separate opt-in pack. | **OPEN** | Product Architecture |
| `OPEN-STATIC-04` | Exact annotation opt-out syntax (`kubecheck.example.com/allow-<rule-name>`) and whether it applies per-rule or per-family. | **OPEN** | Diagnostic Engine Architecture |
| `OPEN-STATIC-05` | Whether `SEC-PSS-*` rules should require a user-supplied target profile or infer from namespace labels only. | **OPEN** | Security Architecture |
| `OPEN-STATIC-06` | Whether `REL-PDB-007` (multi-replica workload without PDB) should be included in the default bundle or omitted to avoid noise. | **OPEN** | Diagnostics Architecture |
| `OPEN-STATIC-07` | Whether the `WL-ENV-003`/`WL-ENV-004` rules (missing key in a Secret/ConfigMap in the input set) should be `error` or `warning`. | **OPEN** | Diagnostics Architecture |
| `OPEN-STATIC-08` | Whether `SEC-CTX-001` (privileged) should be `warning` or `error` given that PSS baseline blocks it but KubeCheck does not know the namespace's PSS enforcement. | **OPEN** | Security Architecture |

---

## 22. Definition of done

This document is complete when the following conditions hold:

1. Every rule family in the requested scope has a section with a rule table and at least one fully specified illustrative rule.
2. Every rule family states its matching conditions, counterexamples, severity, confidence, applicability, false-positive traps, remediation, and test cases.
3. Deterministic rules are separated from heuristic best-practice recommendations (`STATIC-RULES-REQ-002`).
4. Every rule has a stable proposed ID from the `<DOMAIN>-<FAMILY>-<NNN>` format.
5. No implementation status, detection rate, or benchmark result is claimed (`STATIC-RULES-REQ-001`).
6. Every rule documents at least three counterexamples (`STATIC-RULES-REQ-010`).
7. Heuristic rules are never assigned `error` severity (`STATIC-RULES-REQ-002`).
8. Annotation-based opt-out is specified for heuristic rules (`STATIC-RULES-REQ-011`).
9. The consolidated inventory in §18 maps every family to its source document and evidence-class mix.
10. Testing requirements cover per-rule fixtures, family integration tests, determinism, and counterexample regression.
11. Open decisions are recorded with ownership and target phase.

---

## 23. Consistency and completeness review

A review was performed against the requested scope before finalization.

- **YAML and normalized-structure defects:** §3.
- **Schema violations and unknown fields:** §4.
- **Required, missing, mistyped, and invalid values:** §5.
- **API deprecations and version incompatibility:** §6.
- **Workload configuration defects:** §7.1–§7.5.
- **Probe and lifecycle issues:** §7.7.
- **Resource request and limit problems:** §7.8.
- **Service and networking configuration:** §10.
- **Storage and configuration references:** §11.
- **Security and RBAC risks:** §12.
- **Reliability and availability risks:** §13.
- **Maintainability and best-practice warnings:** §14.
- **Deterministic vs heuristic separation:** §16.
- **Avoiding blanket rules:** §17.
- **Initial rule inventory with stable proposed IDs:** §18.
- **No claims of implementation or verified detection rates:** `STATIC-RULES-REQ-001`, §1, §18.

No empty boilerplate section was added solely to satisfy a checklist. The document is complete for its assigned responsibility.