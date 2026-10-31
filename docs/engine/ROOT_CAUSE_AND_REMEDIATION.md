# KubeCheck — Root Cause Analysis and Remediation

**Document ID:** RCA-REMED-001  
**File:** `docs/engine/ROOT_CAUSE_AND_REMEDIATION.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Incident Analysis and Remediation Engineering  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/engine/DIAGNOSTIC_RULE_SPEC.md`
- `docs/engine/STATIC_ANALYSIS_RULES.md`
- `docs/engine/CROSS_RESOURCE_ANALYSIS.md`
- `docs/architecture/SYSTEM_ARCHITECTURE.md`
- `docs/architecture/REQUIREMENTS_AND_NFRS.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/security/THREAT_MODEL.md`
- `docs/security/SECURITY_MODEL.md`
- `docs/output/JSON_SCHEMA.md`
- `docs/cli/EXIT_CODE_CONTRACT.md`
- `docs/testing/DETERMINISM_TESTS.md`
- `docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`

---

## 1. Purpose and scope

This document defines how KubeCheck groups individual findings into **understandable explanations** and **safe remediation suggestions**. A user who receives a single finding often cannot tell whether it is a symptom of a deeper defect, a contributing factor to a larger failure, or a standalone issue. KubeCheck's root-cause analysis (RCA) layer groups related findings into **causal clusters**, ranks them by evidence, and provides **remediation guidance** that is safe, minimal, and context-preserving.

### 1.1 Governing principles

> **RCA-REMED-REQ-001 — KubeCheck never modifies the user's manifest automatically.** No finding, report, or RCA cluster triggers a file write. Remediation is delivered as **advisory text** or, in a future explicitly approved editing feature, as an **explicitly invoked, separately approved patch**. The default behavior is advisory-only.

> **RCA-REMED-REQ-002 — Remediation never echoes secret values and never recommends unsafe credential handling.** Every remediation suggestion is scanned against the secret-bearing field set defined in `docs/security/THREAT_MODEL.md`. A suggestion that would echo a secret value, recommend storing a credential in a ConfigMap, or recommend a literal `value` for a sensitive field is rejected at rule-authoring time (`RULE-SPEC-REQ-018`, `K8S-SEC-REQ-013`).

> **RCA-REMED-REQ-003 — KubeCheck never claims that a remediation will resolve a runtime failure.** A finding is a static observation. A remediation is a suggestion for a static correction. Whether the correction resolves a runtime failure (Pod not scheduling, container crash-looping, traffic not reaching a Service) depends on cluster state, controller behavior, and application behavior, all of which are outside KubeCheck's offline analysis scope. The remediation MUST state this boundary explicitly.

> **RCA-REMED-REQ-004 — Root-cause ranking is evidence-based, not heuristic scoring.** A finding's position in the causal cluster is determined by (a) its evidence class, (b) its relationship to other findings in the cluster, and (c) its dependency position. KubeCheck MUST NOT assign a numeric "root cause probability" or a "confidence percentage" to a finding (`ACC-REQ-011`).

> **RCA-REMED-REQ-005 — Duplicate and cascading findings are grouped, not suppressed.** A finding that duplicates another or that cascades from another is not deleted from the report; it is **grouped** under a root-cause cluster so that the user sees the full picture. Suppression is reserved for exclusions declared by the rule author (`RULE-SPEC-REQ-031`).

### 1.2 In scope

Symptom/cause/contributing-factor relationships; rule dependencies; evidence-based root-cause ranking; duplicate and cascading diagnostics; confidence and severity aggregation; minimal correction suggestions; context preservation; patch generation vs advisory-only; multiple valid corrections; uncertain/conflicting/incomplete evidence; source references; secret-safe explanations; prevention of runtime claims; explanation of why a recommendation applies; four complete examples; remediation type contract (preconditions, expected changes, validation steps, risks).

### 1.3 Out of scope

- Live cluster remediation (e.g., `kubectl apply`, cluster mutation).
- Automatic patch application.
- Runtime diagnosis (see `KUBERNETES_API_FUNDAMENTALS.md` §13).
- Secret scanning beyond the rule-authoring remediation check.
- Application-level fixes (e.g., "change your application code to read the ConfigMap").

### 1.4 Assumptions

- **ASSUMPTION-RCA-01:** Findings are produced by the diagnostic engine with the metadata required by `DIAGNOSTIC_RULE_SPEC.md` §4.3.
- **ASSUMPTION-RCA-02:** Every finding carries a source anchor, evidence class, severity, and rule ID (`SARCH-REQ-022`, `RULE-SPEC-REQ-038`).
- **ASSUMPTION-RCA-03:** The relationship graph is available for cross-resource finding grouping (`CROSS-RES-001` §3).
- **ASSUMPTION-RCA-04:** No editing feature is approved in the MVP. The RCA layer is advisory-only by default.

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **Finding** | A single diagnostic record produced by a rule (`RULE-SPEC-001` §4.2). |
| **Symptom** | A finding that is observable in the manifest but is not the underlying defect. |
| **Cause** | A finding that is the underlying defect for a symptom. |
| **Contributing factor** | A finding that makes a defect more likely to produce a failure but is not itself the defect. |
| **Root-cause cluster** | A group of findings that are causally linked. |
| **Root finding** | The finding in a cluster that is the deepest cause (most upstream in the dependency graph). |
| **Downstream finding** | A finding in a cluster that cascades from the root finding. |
| **Remediation** | A suggestion that describes a static correction. |
| **Advisory-only** | A remediation that provides text guidance without producing a patch. |
| **Patch** | A structured change to a manifest that could be applied by an editing feature. KubeCheck does not generate patches in the MVP. |
| **RCA cluster ID** | A stable identifier for a root-cause cluster (`RCA-<domain>-<NNN>`). |
| **Evidence class** | `deterministic`, `probable`, or `context-dependent` (`DIAGNOSTIC_TAXONOMY.md`). |
| **Confidence basis** | A short, human-readable statement of the evidence (`RULE-SPEC-REQ-012`). |

**Requirement IDs:** `RCA-REMED-REQ-NNN`.  
**Cluster IDs:** `RCA-<domain>-<NNN>`.  
**Remediation type IDs:** `REMED-TYPE-<NNN>`.

---

## 3. Symptoms, causes, and contributing factors

### 3.1 Definitions and examples

| Role | Definition | Example |
|---|---|---|
| **Symptom** | A finding that is observable in the manifest but is a consequence of a deeper defect. | A `Service` selector matches no supplied Pod (`XRES-SVC-001`, `info`). |
| **Cause** | The finding that the symptom follows from. | The `Deployment`'s `spec.template.metadata.labels` uses `app: api`, while the `Service` selector is `app: api-service` (`NET-SVC-008`/`XRES-SVC-001`). |
| **Contributing factor** | A finding that makes the defect more likely to produce a failure but is not itself the defect. | The `Service` has no `readinessProbe` on its Pods, so even a matching selector would route traffic to unready Pods (`WL-PROBE-004`, `context-dependent`). |

### 3.2 The causal ordering

The RCA layer establishes a **causal ordering** among findings in a cluster:

```
Contributing factor  →  Cause  →  Symptom
```

A root-cause cluster contains one or more causes, zero or more contributing factors, and one or more symptoms. The **root finding** is the finding with no incoming causal edge within the cluster.

### 3.3 The role assignment rules

**RCA-REMED-REQ-006:** The RCA layer MUST assign a role (`root`, `contributing`, `symptom`) to each finding in a cluster. The assignment is deterministic and is based on the following rules:

| Condition | Role |
|---|---|
| The finding is the source of the cluster (no incoming causal edge). | `root` |
| The finding has an outgoing causal edge to another finding in the cluster and no incoming causal edge. | `root` |
| The finding has an incoming causal edge from a root finding and no outgoing causal edge. | `symptom` |
| The finding has an incoming causal edge and contributes to the root finding's matching condition (e.g., a missing readiness probe that is not itself part of the selector mismatch). | `contributing` |
| The finding has both incoming and outgoing causal edges. | `symptom` (intermediate) |

**RCA-REMED-REQ-007:** A finding that does not participate in any causal relationship MUST be its own singleton cluster with role `root`. This ensures that every finding appears in exactly one cluster.

**RCA-REMED-REQ-008:** A finding MUST appear in exactly one cluster. The RCA layer MUST NOT duplicate findings across clusters.

### 3.4 What is NOT a contributing factor

**RCA-REMED-REQ-009:** A finding MUST NOT be classified as a contributing factor merely because it is in the same file, the same namespace, or the same kind. Contributing-factor classification requires an **explicit causal edge** derived from:

1. The rule's `dependencies` field (`RULE-SPEC-REQ-034`).
2. The rule's `conflict_with` field (`RULE-SPEC-REQ-037`).
3. The relationship graph (`CROSS-RES-REQ-007`).
4. The same evidence anchor (two findings with the same anchor are causally linked if one is deterministic and the other is probable).

**RCA-REMED-REQ-010:** Co-location in a file, namespace, or kind MUST NOT be used as a causal relationship. The RCA layer MUST NOT group findings by "they look related."

---

## 4. Rule dependencies and evidence-based root-cause ranking

### 4.1 Rule dependency edges

**RCA-REMED-REQ-011:** The RCA layer MUST use the rule dependency graph (from `RULE-SPEC-REQ-034`) to establish causal edges. If rule `A` declares `dependencies: [B]`, then a finding from rule `B` is a candidate root for a finding from rule `A` in the same object.

**RCA-REMED-REQ-012:** A dependency edge is **directional**: `B → A`. The edge indicates that `A` is downstream of `B`. If `A` emits a finding for an object and `B` also emits a finding for the same object, `B` is the root and `A` is the symptom.

### 4.2 Evidence-based ranking

**RCA-REMED-REQ-013:** Within a cluster, findings MUST be ranked by:

1. **Evidence class rank:** `deterministic` > `probable` > `context-dependent`.
2. **Dependency position:** findings with no incoming causal edge (roots) outrank findings with incoming edges (symptoms).
3. **Rule ID:** lexicographic order (deterministic tie-breaker).
4. **Source anchor:** `(file, doc_index, line, column, json_path)` lexicographic order (final tie-breaker).

**RCA-REMED-REQ-014:** The ranking MUST NOT use a numeric "confidence score" or "probability." A finding's rank is an ordering, not a probability. The RCA layer MUST NOT expose a numeric rank in the output; it MUST expose the **role** (`root`/`contributing`/`symptom`) and the **evidence class**.

### 4.3 The root finding selection

**RCA-REMED-REQ-015:** The **root finding** of a cluster is the highest-ranked finding whose role is `root`. If the cluster has multiple `root` findings (multiple independent causes), the RCA layer MUST report **multiple roots** and MUST NOT arbitrarily pick one.

**RCA-REMED-REQ-016:** When a cluster has multiple roots, the report MUST present the cluster as "multiple independent causes" rather than "one root cause." The user is informed that fixing one root may not resolve the symptoms.

### 4.4 Root-cause ranking example

Consider a Deployment with a schema error and a selector mismatch:

| Finding | Rule | Evidence class | Dependency |
|---|---|---|---|
| `K8S-SCHEMA-001` (unknown field) | `K8S-SCHEMA` | `deterministic` | None |
| `WL-POD-001` (selector mismatch) | `WL-POD` | `deterministic` | Depends on `K8S-SCHEMA-001` |

The RCA layer:
- Creates one cluster with `K8S-SCHEMA-001` as `root` and `WL-POD-001` as `symptom`.
- Ranks `K8S-SCHEMA-001` first (root, deterministic).
- Recommends: "Fix the schema error first; the selector mismatch may resolve when the unknown field is corrected."

---

## 5. Duplicate and cascading diagnostics

### 5.1 Duplicate findings

**RCA-REMED-REQ-017:** Two findings are **duplicates** if they have the same `(rule_id, source_anchor, resource_identity)`. The aggregator already deduplicates exact duplicates (`SARCH-REQ-043`); the RCA layer handles **near-duplicates** — findings that describe the same defect but differ in evidence anchors or rule IDs.

**RCA-REMED-REQ-018:** Near-duplicates MUST be grouped into the same cluster and MUST be presented as one defect with multiple evidence locations. The report MUST NOT present them as separate defects.

**RCA-REMED-REQ-019:** A near-duplicate is identified by:

1. Same `rule_id` and different `source_anchor` that resolves to the same field in the same resource (e.g., a rule fires twice on the same field through two code paths).
2. Different `rule_id` but the same evidence anchor and the same remediation text (e.g., `WL-POD-001` and `K8S-SCHEMA-003` both fire on the same missing `spec.selector`).

The RCA layer MUST group case 1 and case 2 into the same cluster; the report MUST present the cluster as one defect with multiple rule references.

### 5.2 Cascading findings

**RCA-REMED-REQ-020:** A **cascade** occurs when one finding causes another finding to fire on a different resource. Examples:

- A `ConfigMap` with a missing key (`STG-CM-001`) causes a `Pod`'s `envFrom` to fail (`WL-ENV-004`).
- A `Service` with a selector mismatch (`NET-SVC-008`) causes an `Ingress` backend to be unreachable (`XRES-ING-001`).

**RCA-REMED-REQ-021:** Cascading findings MUST be grouped into the same cluster. The upstream finding is `root`; the downstream findings are `symptoms`.

**RCA-REMED-REQ-022:** The RCA layer MUST establish cascade edges using the relationship graph (`CROSS-RES-REQ-007`). Two findings are cascade-linked if:

1. Their respective resources are connected by an edge in the graph.
2. The upstream finding's rule family is a known cause of the downstream finding's rule family.

The known-cause relationship is declared in the rule metadata's `causes` field. A rule that does not declare a `causes` field MUST NOT participate in cascade grouping.

### 5.3 Duplicate suppression in output

**RCA-REMED-REQ-023:** The RCA layer MUST NOT suppress duplicates or cascades from the output. Every finding MUST appear in the output. The RCA layer adds a `cluster_id`, `role`, and `caused_by` field to each finding so that the user can see the grouping without losing the individual finding.

---

## 6. Confidence and severity aggregation

### 6.1 Cluster-level confidence

**RCA-REMED-REQ-024:** A root-cause cluster's **confidence** is the evidence class of its root finding. It is **not** a numeric score, and it is **not** the maximum or minimum of the members' evidence classes.

| Root evidence class | Cluster confidence |
|---|---|
| `deterministic` | `deterministic` |
| `probable` | `probable` |
| `context-dependent` | `context-dependent` |

**RCA-REMED-REQ-025:** The cluster confidence MUST NOT be presented as "high," "medium," or "low" without specifying the evidence class. "High confidence" is a marketing term, not an evidence statement.

### 6.2 Cluster-level severity

**RCA-REMED-REQ-026:** A root-cause cluster's **severity** is the maximum severity among its findings, capped by the evidence class of the root finding:

| Root evidence class | Permitted cluster severities |
|---|---|
| `deterministic` | `error`, `warning`, `info` |
| `probable` | `warning`, `info` |
| `context-dependent` | `warning`, `info` |

**RCA-REMED-REQ-027:** A cluster with a `probable` root and an `error`-severity symptom MUST NOT present the cluster as `error`-severity. The cluster severity is capped at `warning`, because the root cause is not deterministic (`RULE-SPEC-REQ-014`).

### 6.3 Why aggregation matters

A user seeing a single `error`-severity finding may act immediately. A user seeing a `warning`-severity cluster with a `probable` root knows that the finding requires investigation before action. The aggregation reflects the **strength of evidence**, not the **impact of the defect**.

---

## 7. Minimal correction suggestions and context preservation

### 7.1 Minimality

**RCA-REMED-REQ-028:** A remediation suggestion MUST be **minimal**: it MUST describe the smallest change that addresses the root finding. It MUST NOT bundle unrelated fixes, reformat the manifest, or rename resources.

**RCA-REMED-REQ-029:** A remediation suggestion MUST preserve the **context** of the original manifest:

- The resource's identity (`metadata.name`, `metadata.namespace`) MUST NOT change.
- The resource's kind and `apiVersion` MUST NOT change unless the finding is an API-version lifecycle finding.
- The labels, annotations, and configuration values that the finding does not concern MUST NOT change.

### 7.2 Context preservation

**RCA-REMED-REQ-030:** The remediation text MUST include the **field path** (JSON path) of the finding's source anchor so that the user can locate the change precisely (`OUT-REQ-005`).

**RCA-REMED-REQ-031:** When the remediation involves a value change, the text MUST include the current value (if it is not a secret-bearing field) and the recommended value. When the current value is secret-bearing, the text MUST identify the field by path and MUST NOT include the value (`K8S-SEC-REQ-013`).

### 7.3 Remediation text structure

**RCA-REMED-REQ-032:** Every remediation MUST include the following sections, in order:

1. **What changed** — a one-sentence description of the defect.
2. **Why it matters** — a one-sentence description of the impact (referencing the rule's `confidence_basis`).
3. **Where to change it** — the field path.
4. **How to change it** — the recommended value or steps.
5. **What to verify** — the validation steps.
6. **What is out of scope** — a statement that runtime behavior and cluster acceptance are not verified.

### 7.4 Example structure

```
What changed:    Service "web" has no matching Pods in the analyzed input set.
Why it matters:  The Service will have no endpoints, so traffic will not reach any Pod.
Where:           metadata.namespace: production, spec.selector: {app: web}
How:             Either correct the Service's selector to match the Pods, or add the
                 missing labels to the Pods' metadata.labels.
Verify:          Re-run kubecheck analyze with both the Service and the workload in
                 the input set; the selector-match finding should not appear.
Out of scope:    KubeCheck does not verify whether the Pods are Ready or whether the
                 cluster's EndpointSlice controller has processed the Service.
```

---

## 8. Patch generation versus advisory-only recommendations

### 8.1 The default: advisory-only

**RCA-REMED-REQ-033:** In the MVP, every remediation MUST be **advisory-only**. KubeCheck MUST NOT produce a patch, a modified manifest, or a diff. The output is text guidance.

### 8.2 Future patch generation

**RCA-REMED-REQ-034:** If a future release introduces patch generation, the following constraints MUST hold:

1. **Explicit invocation:** Patch generation MUST require an explicit flag (e.g., `--generate-patch`). It MUST NOT be the default.
2. **Separate approval:** The editing feature MUST be approved by an ADR and MUST be documented as a distinct capability, not a variation of analysis.
3. **Read-only input:** The input manifest MUST NOT be modified. The patch MUST be written to a separate output path or `stdout`.
4. **Deterministic patch:** For a fixed input and bundle, the patch MUST be byte-identical across runs (`ACC-REQ-001`).
5. **No secret values:** The patch MUST NOT include secret values, even if the original manifest does. When a finding concerns a secret-bearing field, the patch MUST use a placeholder that the user replaces manually.
6. **Preconditions:** The patch MUST declare its preconditions (e.g., "the manifest must be schema-valid"). If a precondition is not met, the patch MUST be refused.
7. **Validation:** The patch MUST include validation steps that the user can run to verify the change.
8. **Risk assessment:** The patch MUST declare its risks (e.g., "this change may break existing workloads if the selector is intentionally different").
9. **Advisory alongside patch:** The patch MUST be accompanied by advisory text that explains the change in the same structure as §7.3.

### 8.3 Patch scope

**RCA-REMED-REQ-035:** A patch, if generated, MUST be limited to the fields identified in the finding's source anchor and MUST NOT touch unrelated fields. The patch MUST be a structured diff that the user can review line by line.

### 8.4 No automatic application

**RCA-REMED-REQ-036:** KubeCheck MUST NOT apply a patch to a cluster, a file, or a repository. Patch application is the user's responsibility, using their own tooling.

---

## 9. Handling multiple valid corrections

### 9.1 The multiple-corrections problem

Many findings have more than one valid correction. A Service selector mismatch can be fixed by correcting the Service's selector or by correcting the Pods' labels. A missing Secret reference can be fixed by creating the Secret or by removing the reference.

**RCA-REMED-REQ-037:** When a finding has multiple valid corrections, the remediation MUST list **all** of them, each with its own preconditions, expected changes, validation steps, and risks. The remediation MUST NOT arbitrarily choose one.

### 9.2 Ordering of multiple corrections

**RCA-REMED-REQ-038:** Multiple corrections MUST be ordered by:

1. **Reversibility:** reversible corrections first.
2. **Scope:** corrections that touch fewer fields first.
3. **Likelihood of correctness:** corrections that align with the finding's evidence first.

The ordering MUST be explicit; the remediation MUST state why the corrections are in the order shown.

### 9.3 Example: Service selector mismatch

The finding `XRES-SVC-001` has two valid corrections:

**Correction A (Service-side):**
- **Preconditions:** The user owns the Service and the intended Pods exist.
- **Expected change:** Update `spec.selector` to match the actual Pod labels.
- **Validation:** Re-run KubeCheck with the Service and the workload; the finding does not appear.
- **Risk:** If the Service is used by other workloads, changing the selector may disconnect them.

**Correction B (Workload-side):**
- **Preconditions:** The user owns the workload and the Service's selector is the intended one.
- **Expected change:** Add the missing labels to `spec.template.metadata.labels`.
- **Validation:** Re-run KubeCheck; the finding does not appear.
- **Risk:** If the workload's Pods are selected by other Services, adding labels may inadvertently match them.

The remediation MUST present both corrections and explain the trade-off.

---

## 10. Uncertain, conflicting, and incomplete evidence

### 10.1 Uncertain evidence

**RCA-REMED-REQ-039:** When the root finding's evidence class is `probable` or `context-dependent`, the remediation MUST state the uncertainty explicitly:

- What is known (the manifest-level fact).
- What is unknown (the cluster-level or application-level fact).
- What the user can do to resolve the uncertainty (e.g., supply additional manifests, check the cluster).

**RCA-REMED-REQ-040:** The remediation MUST NOT present an uncertain finding as a confirmed defect. Wording such as "This may cause..." is preferred over "This causes..."

### 10.2 Conflicting evidence

**RCA-REMED-REQ-041:** When two findings conflict (e.g., `RULE-SPEC-REQ-037` emits both findings with `conflict_with`), the remediation MUST present both findings and MUST state that they conflict. The remediation MUST NOT recommend fixing one without noting the other.

**RCA-REMED-REQ-042:** A conflicting pair MUST be presented as a **decision point** for the user, not as a single recommended fix.

### 10.3 Incomplete evidence

**RCA-REMED-REQ-043:** When a finding depends on a target that is not in the input set, the remediation MUST state what additional input would complete the analysis:

- "Supply the referenced Secret to verify the key exists."
- "Supply the workload to verify the selector match."
- "Supply the CRD schema to verify the custom resource."

**RCA-REMED-REQ-044:** The remediation MUST NOT present incomplete evidence as a defect. It MUST present the finding as "the reference is structurally valid; existence cannot be verified offline."

### 10.4 Unknown runtime state

**RCA-REMED-REQ-045:** When a finding depends on runtime state (readiness, scheduling, controller behavior), the remediation MUST state the runtime dependency explicitly:

- "This finding is about the manifest. Whether the Pod is Ready depends on the readiness probe and the application."
- "This finding is about the selector. Whether the EndpointSlice controller has populated endpoints depends on the Pods' readiness."

---

## 11. Source references and documentation links

### 11.1 Mandatory references

**RCA-REMED-REQ-046:** Every remediation MUST cite at least one authoritative reference from the rule's `references` field (`RULE-SPEC-REQ-020`). The reference MUST be a stable URL (Kubernetes documentation, YAML specification, CVE, or internal document path) with a retrieval date.

### 11.2 Reference format

**RCA-REMED-REQ-047:** References MUST be rendered as:

```
Reference: <title>
  URL: <url>
  Retrieved: <YYYY-MM-DD>
```

A reference without a retrieval date MUST be rejected at rule-authoring time.

### 11.3 What references are NOT

**RCA-REMED-REQ-048:** A reference is not a claim. Citing a Kubernetes documentation page does not mean KubeCheck has verified that the page's behavior applies to the user's cluster. The reference is provided so that the user can read the authoritative source.

**RCA-REMED-REQ-049:** KubeCheck MUST NOT cite a reference that it has not verified to exist. The bundle verifier MUST check that every reference URL returns a valid response **at bundle generation time** (not at analysis time, which must be offline). A bundle with an invalid reference MUST be blocked at release (`GATE-REQ-008`).

---

## 12. Secret-safe explanations and redaction

### 12.1 The redaction boundary

**RCA-REMED-REQ-050:** Remediation text MUST NOT echo the value of any field identified as secret-bearing. The secret-bearing field set is defined in `docs/security/THREAT_MODEL.md` and includes at minimum:

- `Secret.data.*`
- `Secret.stringData.*`
- Fields matching the documented sensitive-name patterns (`password`, `token`, `apiKey`, `secret`, `credential`).
- TLS key material (`tls.key`).

**RCA-REMED-REQ-051:** When a remediation concerns a secret-bearing field, the text MUST identify the field by path and MUST NOT include the value. For example:

```
Change:       spec.template.spec.containers[0].env[2].valueFrom.secretKeyRef.key
Current:      [REDACTED]
Recommended:  Add the key "DB_PASSWORD" to the Secret "db-creds" in namespace "production".
Verify:       Re-run kubecheck analyze with the Secret in the input set.
```

### 12.2 The redaction check

**RCA-REMED-REQ-052:** The engine MUST scan every remediation text for any value present in a secret-bearing field of the analyzed manifest. If a match is found, the engine MUST reject the rule (`RULE-SPEC-REQ-018`).

### 12.3 Unsafe remediation rejection

**RCA-REMED-REQ-053:** The engine MUST reject a remediation that:

1. Recommends storing a credential in a ConfigMap.
2. Recommends a literal `value` for a credential-like field.
3. Recommends disabling `readOnly: true` on a mounted Secret.
4. Recommends `stringData` for a workflow that reads the object back.
5. Recommends `privileged: true` to fix a capability error.
6. Recommends `allowPrivilegeEscalation: true` to fix a permission error.
7. Recommends removing a `NetworkPolicy` to fix connectivity.
8. Recommends `--insecure` flags or equivalent security-disabling options.

**RCA-REMED-REQ-054:** The rejection MUST cause the rule to be disabled (`RULE-SPEC-REQ-056`) and MUST emit an internal `ENGINE-REMED-UNSAFE` finding naming the rule and the offending suggestion.

### 12.4 Safe remediation patterns

**RCA-REMED-REQ-055:** When the remediation involves a secret, the text MUST recommend one of:

- A `valueFrom.secretKeyRef` reference.
- A Secret volume mount.
- An external secret store integration.

These are the only safe patterns for credential handling (`K8S-SEC-REQ-013`).

---

## 13. Prevention of unsupported runtime claims

### 13.1 The runtime-claim boundary

**RCA-REMED-REQ-056:** KubeCheck MUST NOT claim that:

- A remediation will resolve a runtime failure.
- A remediation will make a Pod schedule.
- A remediation will make a Service route traffic.
- A remediation will make a container start successfully.
- A remediation will make a workload pass admission.

These are runtime outcomes that depend on cluster state, controller behavior, application behavior, and admission controllers. KubeCheck's static analysis establishes static conformance, not runtime behavior.

### 13.2 What KubeCheck CAN claim

**RCA-REMED-REQ-057:** KubeCheck MAY claim that:

- A remediation will resolve a **static defect** (e.g., "the request no longer exceeds the limit").
- A remediation will make a manifest **schema-valid** for the target version.
- A remediation will make a reference **resolve within the input set** (if the user supplies the target).
- A remediation will **remove a finding** that KubeCheck produces.

### 13.3 The boundary statement

**RCA-REMED-REQ-058:** Every remediation MUST include a **boundary statement**:

> "KubeCheck validates the manifest. Whether this change resolves the runtime issue depends on cluster state, controller behavior, and application behavior, which are outside KubeCheck's offline analysis scope."

The boundary statement MUST appear in every cluster's remediation and MUST NOT be abbreviated.

### 13.4 Examples of correct and incorrect claims

| Claim | Correct? | Why |
|---|---|---|
| "This change removes the selector-mismatch finding." | ✅ | Static claim. |
| "This change will make the Service route traffic." | ❌ | Runtime claim. |
| "The Pod's request now equals its limit." | ✅ | Static claim. |
| "The Pod will now be Guaranteed QoS." | ✅ | Static claim (QoS is derived from manifest). |
| "The Pod will now be scheduled." | ❌ | Runtime claim. |
| "The container will not be OOMKilled." | ❌ | Runtime claim (depends on actual memory usage). |
| "The Deployment's selector now matches the template labels." | ✅ | Static claim. |
| "The Deployment will roll out successfully." | ❌ | Runtime claim. |
| "The reference now resolves within the analyzed input set." | ✅ | Static claim. |
| "The Service now has endpoints." | ❌ | Runtime claim (depends on EndpointSlice controller and Pod readiness). |

---

## 14. Why a recommendation applies

### 14.1 The explanation requirement

**RCA-REMED-REQ-059:** Every remediation MUST include an explanation of **why the recommendation applies** to the specific finding. The explanation MUST connect the finding's evidence to the remediation's expected change.

### 14.2 The explanation structure

**RCA-REMED-REQ-060:** The explanation MUST be structured as:

1. **The evidence:** What the manifest says (the finding's evidence fields).
2. **The mismatch:** What is wrong (the finding's rule logic, in plain language).
3. **The correction:** What change would make the evidence match the expected state.
4. **The result:** What the static state would be after the correction.

### 14.3 Example: selector mismatch

```
Evidence:    The Service "web" has spec.selector {app: web}. The Deployment "web"
             has spec.template.metadata.labels {app: api}.
Mismatch:    The selector requires the label app=web, but the Pods have app=api.
             No Pod in the analyzed input set has the required label.
Correction:  Change the Service selector to {app: api}, or change the Deployment
             template labels to include app=web.
Result:      After the change, the Service selector would match the Pod labels
             within the analyzed input set.
```

### 14.4 Example: missing ConfigMap reference

```
Evidence:    The Pod "app" has envFrom[0].configMapRef.name = "app-config". The
             ConfigMap "app-config" is not in the analyzed input set.
Mismatch:    The reference points to a ConfigMap that is not in the input set.
             KubeCheck cannot verify whether the ConfigMap exists in the cluster.
Correction:  Supply the ConfigMap "app-config" in the input set, or correct the
             reference to the intended ConfigMap name.
Result:      After supplying the ConfigMap, KubeCheck can verify that the
             referenced keys exist.
```

---

## 15. Complete examples

### 15.1 Example A: Service selector mismatch

#### Findings

```json
[
  {
    "rule_id": "XRES-SVC-001",
    "category": "semantic",
    "severity": "info",
    "evidence_class": "context-dependent",
    "source_anchor": {
      "file": "manifest.yaml",
      "doc_index": 1,
      "line": 4,
      "column": 3,
      "json_path": "/spec/selector"
    },
    "resource_identity": {
      "group": "", "version": "v1", "kind": "Service",
      "namespace": "production", "name": "web"
    },
    "evidence": {
      "selector": {"app": "web"},
      "matching_pods_in_input_set": 0
    }
  }
]
```

#### RCA cluster

```json
{
  "cluster_id": "RCA-SVC-001",
  "root_finding": "XRES-SVC-001",
  "role": "root",
  "confidence": "context-dependent",
  "severity": "info",
  "members": ["XRES-SVC-001"]
}
```

#### Remediation (advisory-only)

```
Cluster: RCA-SVC-001 — Service selector matches no supplied Pod

Root finding: XRES-SVC-001 (Service "web" in namespace "production")

What changed:    The Service's selector is {app: web}, but no Pod in the analyzed
                 input set has the label app=web.
Why it matters:  The Service will have no matching endpoints in the analyzed
                 input set. Whether the Service has endpoints in the cluster
                 depends on the cluster state.
Where:           manifest.yaml, document 1 (Service "web"), spec.selector
How:             Two valid corrections:
                 A. Change the Service's selector to match the Pods' labels.
                    Precondition: you own the Service.
                    Risk: may affect other consumers of this Service.
                 B. Add the label app=web to the workload's Pod template.
                    Precondition: you own the workload.
                    Risk: may cause the workload to be selected by other Services.
Verify:          Re-run kubecheck analyze with both the Service and the workload
                 in the input set. The XRES-SVC-001 finding should not appear.
Out of scope:    KubeCheck does not verify whether the Pods are Ready or whether
                 the EndpointSlice controller has populated endpoints.

Reference: Kubernetes Documentation — Services
  URL: https://kubernetes.io/docs/concepts/services-networking/service/
  Retrieved: 2026-10-31
```

### 15.2 Example B: Missing configuration reference

#### Findings

```json
[
  {
    "rule_id": "XRES-REF-002",
    "category": "semantic",
    "severity": "info",
    "evidence_class": "context-dependent",
    "source_anchor": {
      "file": "manifest.yaml",
      "doc_index": 0,
      "line": 12,
      "column": 9,
      "json_path": "/spec/template/spec/containers/0/envFrom/0/configMapRef/name"
    },
    "resource_identity": {
      "group": "apps", "version": "v1", "kind": "Deployment",
      "namespace": "production", "name": "app"
    },
    "evidence": {
      "reference_field": "envFrom[0].configMapRef",
      "target": {"kind": "ConfigMap", "namespace": "production", "name": "app-config"},
      "in_input_set": false
    }
  }
]
```

#### RCA cluster

```json
{
  "cluster_id": "RCA-REF-001",
  "root_finding": "XRES-REF-002",
  "role": "root",
  "confidence": "context-dependent",
  "severity": "info",
  "members": ["XRES-REF-002"]
}
```

#### Remediation (advisory-only)

```
Cluster: RCA-REF-001 — ConfigMap reference not found in analyzed input

Root finding: XRES-REF-002 (Deployment "app" in namespace "production")

What changed:    The Deployment references ConfigMap "app-config" via
                 envFrom[0].configMapRef, but the ConfigMap is not in the
                 analyzed input set.
Why it matters:  The reference is structurally valid. Whether the ConfigMap
                 exists in the cluster determines whether the Pod's envFrom
                 will succeed.
Where:           manifest.yaml, document 0 (Deployment "app"),
                 spec.template.spec.containers[0].envFrom[0].configMapRef.name
How:             Two valid corrections:
                 A. Supply the ConfigMap "app-config" in the analyzed input set
                    to verify the reference resolves.
                    Precondition: the ConfigMap exists in the cluster.
                    Risk: none (adding input to the analysis).
                 B. Correct the reference to the intended ConfigMap name.
                    Precondition: you know the correct ConfigMap name.
                    Risk: the Pod's environment may change if the name changes.
Verify:          Re-run kubecheck analyze with the ConfigMap in the input set.
                 The XRES-REF-002 finding should not appear.
Out of scope:    KubeCheck does not verify whether the ConfigMap has the keys
                 the container expects, nor whether the Pod will start.

Reference: Kubernetes Documentation — ConfigMaps
  URL: https://kubernetes.io/docs/concepts/configuration/configmap/
  Retrieved: 2026-10-31
```

### 15.3 Example C: Invalid API version

#### Findings

```json
[
  {
    "rule_id": "DEP-API-001",
    "category": "lifecycle",
    "severity": "error",
    "evidence_class": "deterministic",
    "source_anchor": {
      "file": "manifest.yaml",
      "doc_index": 0,
      "line": 1,
      "column": 1,
      "json_path": "/apiVersion"
    },
    "resource_identity": {
      "group": "networking.k8s.io", "version": "v1beta1",
      "kind": "Ingress", "namespace": "production", "name": "web"
    },
    "evidence": {
      "gvk": "networking.k8s.io/v1beta1/Ingress",
      "target_version": "1.24",
      "removed_in": "1.22",
      "replacement": "networking.k8s.io/v1/Ingress"
    }
  }
]
```

#### RCA cluster

```json
{
  "cluster_id": "RCA-DEP-001",
  "root_finding": "DEP-API-001",
  "role": "root",
  "confidence": "deterministic",
  "severity": "error",
  "members": ["DEP-API-001"]
}
```

#### Remediation (advisory-only)

```
Cluster: RCA-DEP-001 — Removed API version

Root finding: DEP-API-001 (Ingress "web" in namespace "production")

What changed:    The manifest declares apiVersion: networking.k8s.io/v1beta1,
                 which was removed in Kubernetes 1.22. The target version is 1.24.
Why it matters:  The target cluster will not serve the removed API version, so
                 the manifest will be rejected by the API server.
Where:           manifest.yaml, document 0, apiVersion
How:             Change apiVersion to networking.k8s.io/v1. Verify that the
                 Ingress's backend and path fields use the v1 structure
                 (backend.service.name and backend.service.port), not the
                 v1beta1 structure (backend.serviceName and backend.servicePort).
Verify:          Re-run kubecheck analyze against the target version. The
                 DEP-API-001 finding should not appear.
Out of scope:    KubeCheck does not verify whether the Ingress controller is
                 installed or whether the v1 Ingress will be admitted.

Reference: Kubernetes Deprecated API Migration Guide
  URL: https://kubernetes.io/docs/reference/using-api/deprecation-guide/
  Retrieved: 2026-10-31

Reference: Kubernetes Documentation — Ingress
  URL: https://kubernetes.io/docs/concepts/services-networking/ingress/
  Retrieved: 2026-10-31
```

### 15.4 Example D: Resource scheduling constraint

#### Findings

```json
[
  {
    "rule_id": "WL-RES-001",
    "category": "schema",
    "severity": "error",
    "evidence_class": "deterministic",
    "source_anchor": {
      "file": "manifest.yaml",
      "doc_index": 0,
      "line": 20,
      "column": 9,
      "json_path": "/spec/template/spec/containers/0/resources/requests/cpu"
    },
    "resource_identity": {
      "group": "apps", "version": "v1", "kind": "Deployment",
      "namespace": "production", "name": "api"
    },
    "evidence": {
      "request": "2",
      "limit": "1",
      "resource": "cpu"
    }
  },
  {
    "rule_id": "WL-RES-002",
    "category": "operational",
    "severity": "warning",
    "evidence_class": "probable",
    "source_anchor": {
      "file": "manifest.yaml",
      "doc_index": 0,
      "line": 21,
      "column": 9,
      "json_path": "/spec/template/spec/containers/0/resources/limits/cpu"
    },
    "resource_identity": {
      "group": "apps", "version": "v1", "kind": "Deployment",
      "namespace": "production", "name": "api"
    },
    "evidence": {
      "limit_without_matching_request": true,
      "resource": "cpu"
    }
  }
]
```

#### RCA cluster

```json
{
  "cluster_id": "RCA-RES-001",
  "root_finding": "WL-RES-001",
  "role": "root",
  "confidence": "deterministic",
  "severity": "error",
  "members": ["WL-RES-001", "WL-RES-002"]
}
```

#### Remediation (advisory-only)

```
Cluster: RCA-RES-001 — Resource request exceeds limit

Root finding: WL-RES-001 (Deployment "api" in namespace "production")

What changed:    The container's CPU request is 2, but its CPU limit is 1.
                 The request exceeds the limit.
Why it matters:  The API server rejects a Pod whose request exceeds its limit.
Where:           manifest.yaml, document 0 (Deployment "api"),
                 spec.template.spec.containers[0].resources
How:             Lower the request to 1 or raise the limit to 2. If you lower
                 the request, the Pod's scheduling footprint decreases; if you
                 raise the limit, the container can use more CPU.
Verify:          Re-run kubecheck analyze. The WL-RES-001 finding should not
                 appear. The WL-RES-002 finding (limit without matching request)
                 may still appear; consider whether the request should also be
                 lowered to match the limit.

Related finding: WL-RES-002 (warning) — the container has a limit but no
                 matching request for CPU. This finding is a symptom of the same
                 configuration and may resolve when WL-RES-001 is fixed.

Out of scope:    KubeCheck does not verify whether the Pod will be scheduled on
                 any node. The scheduling outcome depends on node capacity,
                 taints, and other Pods on the node.

Reference: Kubernetes Documentation — Resource Management for Pods and Containers
  URL: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
  Retrieved: 2026-10-31
```

---

## 16. Remediation type contract

Every remediation MUST be associated with a **remediation type** from the following table. The type determines the preconditions, expected changes, validation steps, and risks that the remediation must declare.

### 16.1 Remediation types

| Remediation type ID | Type | Preconditions | Expected change | Validation | Risks |
|---|---|---|---|---|---|
| `REMED-TYPE-001` | **Schema correction** | The manifest is schema-invalid for the target version. | Change the field value or remove the field to conform to the schema. | Re-run KubeCheck; the schema finding disappears. | The corrected value may not reflect the user's intent; the user must verify the value. |
| `REMED-TYPE-002` | **Reference correction** | The reference target is not in the input set, or the reference field name is misspelled. | Correct the field name, or supply the target resource. | Re-run KubeCheck with the target in the input set; the reference finding disappears. | Correcting the field name may not match the cluster's actual resource; the user must verify. |
| `REMED-TYPE-003` | **Selector alignment** | A selector does not match the labels of its target. | Change the selector to match the labels, or change the labels to match the selector. | Re-run KubeCheck with both resources in the input set; the selector finding disappears. | Changing the selector may affect other consumers; changing labels may inadvertently match other selectors. |
| `REMED-TYPE-004` | **API version migration** | The target GVK is removed or deprecated for the target version. | Change `apiVersion` to the replacement GVK and migrate field names as required. | Re-run KubeCheck; the lifecycle finding disappears. | The replacement API may have different field names or semantics; the user must verify field compatibility. |
| `REMED-TYPE-005` | **Resource quantity correction** | A resource quantity is invalid, negative, or violates a constraint. | Correct the quantity to a valid value. | Re-run KubeCheck; the quantity finding disappears. | The corrected value may change scheduling behavior; the user must verify node capacity. |
| `REMED-TYPE-006` | **Missing field addition** | A required field is absent. | Add the field with a valid value. | Re-run KubeCheck; the required-field finding disappears. | The added value may not reflect the user's intent; the user must verify. |
| `REMED-TYPE-007` | **Security context hardening** | A security context field is set in a way that violates a security recommendation. | Change the field to the recommended value. | Re-run KubeCheck; the security finding disappears or is downgraded. | The change may break the workload if the application depends on the original setting (e.g., running as root). |
| `REMED-TYPE-008` | **Secret reference correction** | A secret reference points to a different namespace or a missing key. | Create the secret in the correct namespace, or correct the key name. | Re-run KubeCheck with the secret in the input set; the finding disappears. | Creating a secret requires the user to supply the secret value, which KubeCheck never echoes. |
| `REMED-TYPE-009` | **API version migration (deprecated)** | The target GVK is deprecated but still served. | Plan migration to the replacement GVK before removal. | Re-run KubeCheck against a newer target version; the deprecation finding becomes removal. | Migration may require field changes; the user must read the migration guide. |
| `REMED-TYPE-010` | **External reference** | A reference is intentionally external. | No change; confirm the external target exists. | Re-run KubeCheck; the external reference is annotated as `info`. | None; the reference is by design. |
| `REMED-TYPE-011` | **Generated reference** | A reference target is generated by a controller. | No change; confirm the controller is configured as expected. | Re-run KubeCheck; the generated reference is annotated as `info`. | None; the reference is by design. |
| `REMED-TYPE-012` | **Limit range alignment** | A pod's requests or limits violate a LimitRange. | Adjust the values to fit the LimitRange, or adjust the LimitRange. | Re-run KubeCheck; the LimitRange finding disappears. | The LimitRange may be a cluster-wide policy; the user must coordinate with the cluster administrator. |
| `REMED-TYPE-013` | **Quota alignment** | A pod's requests exceed the ResourceQuota. | Reduce requests, or request a quota increase. | Re-run KubeCheck; the quota finding disappears. | The quota may be a cluster-wide policy; the user must coordinate with the cluster administrator. |
| `REMED-TYPE-014` | **PDB correction** | A PDB has invalid fields or blocks all disruptions. | Correct the PDB's `minAvailable`/`maxUnavailable` or the selector. | Re-run KubeCheck; the PDB finding disappears. | Loosening a PDB may allow disruptions; the user must evaluate availability requirements. |
| `REMED-TYPE-015` | **Probe correction** | A probe is misconfigured (identical probes, timeout > period, etc.). | Change the probe's fields to the recommended values. | Re-run KubeCheck; the probe finding disappears. | The change may affect the workload's readiness or liveness behavior; the user must verify. |

### 16.2 Remediation type selection

**RCA-REMED-REQ-061:** Every rule MUST declare its remediation type in the rule's metadata. A rule that does not declare a remediation type MUST be rejected at load time.

**RCA-REMED-REQ-062:** When a rule emits a finding, the remediation text MUST use the remediation type's preconditions, expected changes, validation steps, and risks as the template. The rule author MAY add rule-specific details, but MUST NOT omit the type's mandatory fields.

### 16.3 Type-specific risk statements

**RCA-REMED-REQ-063:** Every remediation MUST include a risk statement specific to its type. The risk statement MUST state:

1. What could go wrong if the change is applied.
2. What the user should check before applying.
3. What the user should check after applying.

---

## 17. Consolidated remediation output format

### 17.1 Per-cluster output

Every cluster's remediation MUST be rendered in the following format:

```
Cluster: <cluster_id> — <cluster_title>

Root finding: <root_rule_id> (<resource description>)

What changed:    <one-sentence defect description>
Why it matters:  <one-sentence impact statement>
Where:           <file>, document <n> (<resource>), <json_path>
How:             <remediation steps or recommended value>
Verify:          <validation steps>
Out of scope:    <runtime boundary statement>

Related findings: <list of related finding IDs in the cluster>

References:
  - <title>
    URL: <url>
    Retrieved: <date>
```

### 17.2 Per-finding output (for findings not in a multi-member cluster)

```
Finding: <rule_id> — <title>

Severity:        <severity>
Evidence class:  <evidence_class>
Confidence:      <confidence_basis>
Where:           <file>, document <n> (<resource>), <json_path>
Evidence:        <evidence fields>
Remediation:     <remediation text>
References:
  - <title>
    URL: <url>
    Retrieved: <date>
```

### 17.3 Multiple valid corrections

When a finding has multiple valid corrections, the output MUST list them with explicit labels:

```
How (correction A — Service-side):
  <steps>
  Preconditions: <preconditions>
  Risk: <risk>

How (correction B — Workload-side):
  <steps>
  Preconditions: <preconditions>
  Risk: <risk>
```

---

## 18. Authoritative references

| Reference | Scope | Type | Retrieved |
|---|---|---|---|
| `docs/engine/DIAGNOSTIC_RULE_SPEC.md` (internal) | Rule format, evidence bindings, remediation rules, conflict resolution | Internal | 2026-10-31 |
| `docs/engine/STATIC_ANALYSIS_RULES.md` (internal) | Rule families and findings used in examples | Internal | 2026-10-31 |
| `docs/engine/CROSS_RESOURCE_ANALYSIS.md` (internal) | Cross-resource findings and the graph used for cascade grouping | Internal | 2026-10-31 |
| `docs/security/THREAT_MODEL.md` (internal) | Secret-bearing field set and redaction requirements | Internal | 2026-10-31 |
| `docs/security/SECURITY_MODEL.md` (internal) | Secure remediation patterns | Internal | 2026-10-31 |
| `docs/output/JSON_SCHEMA.md` (internal) | RCA cluster and finding schema | Internal | 2026-10-31 |
| Kubernetes Documentation — Services | Selector-to-Pod matching | Official documentation | 2026-10-31 |
| Kubernetes Documentation — ConfigMaps | ConfigMap references | Official documentation | 2026-10-31 |
| Kubernetes Deprecated API Migration Guide | API version removal and replacement | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Resource Management | Request/limit constraints | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Good Practices for Kubernetes Secrets | Safe credential patterns | Official documentation | 2026-10-31 |

**Licensing note:** Kubernetes documentation is licensed under CC BY 4.0. No line numbers, benchmark results, or implementation status are asserted beyond what the cited sources state.

---

## 19. Open questions and unresolved decisions

| ID | Question | Status | Owner |
|---|---|---|---|
| `OPEN-RCA-01` | Whether the RCA layer will be part of the MVP or deferred to Phase 2. The current specification assumes it is part of the analysis output from Phase 1, but the exact cluster grouping may be deferred. | **OPEN** | Product Architecture |
| `OPEN-RCA-02` | Whether the RCA cluster schema will be exposed in the JSON output as a top-level field or nested under findings. The current design nests `cluster_id`, `role`, and `caused_by` in each finding. | **OPEN** | Output Architecture |
| `OPEN-RCA-03` | Whether the `causes` field in rule metadata will be mandatory for cascade grouping, or whether cascade grouping can be inferred from the relationship graph alone. | **OPEN** | Diagnostic Engine Architecture |
| `OPEN-RCA-04` | Whether multiple-root clusters will be presented as one cluster with multiple roots or as separate clusters. The current design uses one cluster with multiple roots. | **OPEN** | Diagnostics Architecture |
| `OPEN-RCA-05` | Whether the remediation type contract will be enforced at rule-authoring time (rejecting rules without a remediation type) or at runtime (disabling rules without a remediation type). The current design enforces at authoring time. | **OPEN** | Diagnostic Engine Architecture |
| `OPEN-RCA-06` | Whether a future patch-generation feature will be specified in this document or in a separate ADR. | **OPEN** | Product Architecture |

---

## 20. Definition of done

This document is complete when the following conditions hold:

1. The relationship between symptoms, causes, and contributing factors is defined with role-assignment rules.
2. Rule dependencies and evidence-based root-cause ranking are specified with a deterministic ordering and no numeric probability.
3. Duplicate and cascading diagnostics are grouped without suppression.
4. Confidence and severity aggregation are specified with evidence-class caps.
5. Minimal correction suggestions and context preservation are specified.
6. Patch generation versus advisory-only is specified, with the MVP being advisory-only and a future patch feature constrained by nine requirements.
7. Multiple valid corrections are specified with ordering rules.
8. Uncertain, conflicting, and incomplete evidence are handled with explicit statements.
9. Source references are mandatory and verified at bundle generation.
10. Secret-safe explanations and redaction are specified with a mandatory engine scan.
11. Unsupported runtime claims are prohibited with a boundary statement.
12. The explanation of why a recommendation applies is specified with a four-part structure.
13. Four complete examples are provided: Service selector mismatch, missing configuration reference, invalid API version, and resource scheduling constraint.
14. Every remediation type has preconditions, expected changes, validation steps, and risks.
15. No automatic change application is permitted (`RCA-REMED-REQ-001`, `RCA-REMED-REQ-036`).
16. Every requirement has a stable ID (`RCA-REMED-REQ-NNN`), and every cluster and remediation type has a stable ID.

---

## 21. Consistency and completeness review

A review was performed against the requested scope before finalization.

- **Symptoms, causes, contributing factors:** §3.
- **Rule dependencies and evidence-based ranking:** §4.
- **Duplicate and cascading diagnostics:** §5.
- **Confidence and severity aggregation:** §6.
- **Minimal corrections and context preservation:** §7.
- **Patch generation vs advisory-only:** §8.
- **Multiple valid corrections:** §9.
- **Uncertain, conflicting, incomplete evidence:** §10.
- **Source references:** §11.
- **Secret-safe explanations and redaction:** §12.
- **Prevention of unsupported runtime claims:** §13.
- **Why a recommendation applies:** §14.
- **Examples (Service selector, missing ref, API version, scheduling):** §15.
- **Remediation type contract (preconditions, expected changes, validation, risks):** §16.
- **No automatic change application:** `RCA-REMED-REQ-001`, §8.4.

No empty boilerplate section was added solely to satisfy a checklist. The document is complete for its assigned responsibility.
