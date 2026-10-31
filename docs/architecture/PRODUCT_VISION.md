# KubeCheck — Product Vision and Scope

**Document ID:** ARCH-VISION-001  
**File:** `docs/architecture/PRODUCT_VISION.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Product Architecture  
**Last reviewed:** 2026-10-31  
**Related documents:**
- `docs/architecture/ARCHITECTURE_OVERVIEW.md`
- `docs/architecture/OFFLINE_KNOWLEDGE_BUNDLES.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/security/SECURITY_MODEL.md`
- `docs/distribution/KREW_PACKAGING.md`
- `docs/distribution/PIP_PACKAGING.md`
- `docs/distribution/STANDALONE_BINARIES.md`
- `docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`

---

## 1. Purpose and scope

This document defines the product vision, target users, functional scope, non-goals, measurable objectives, and release priorities for KubeCheck. It establishes the product-level contract that all downstream architecture, diagnostic, security, and distribution documents must satisfy.

KubeCheck is an offline-first command-line tool for diagnosing Kubernetes manifest problems. It accepts manifests as single files, directories, multi-document YAML streams, or standard input, and produces structured findings classified by category, severity, and confidence. It installs through Kubernetes Krew, Python pip, and standalone binaries, and its default analysis workflow requires no internet access, no Kubernetes cluster, no cloud service, no remote API, and no AI model.

This document is **product-level and normative**. It does not specify implementation internals, schema bundle formats, or diagnostic rule logic; those are delegated to the referenced documents. Where this document states a product requirement, that requirement is binding on the corresponding downstream specification.

---

## 2. Problem definition

### 2.1 The problem space

Kubernetes manifests are declarative API objects serialized as YAML. A manifest can fail in several distinct and independent dimensions:

1. **Syntactic invalidity.** The YAML cannot be parsed, uses tabs for indentation, contains duplicate mapping keys, or violates structural constraints of the YAML 1.2.2 specification.
2. **Schema invalidity.** The document is valid YAML but does not conform to the OpenAPI schema of the declared `apiVersion` and `kind` — unknown fields, wrong types, missing required fields, invalid enum values.
3. **Semantic inconsistency.** Individually valid objects conflict with each other or with Kubernetes semantics — a `Service` selector that matches no workload, a `Secret` reference that does not exist in the supplied set, a `livenessProbe` identical to a `readinessProbe`, contradictory resource requests and limits.
4. **Security posture weakness.** The manifest is valid and consistent but violates security expectations — `securityContext.privileged: true`, missing `runAsNonRoot`, `hostNetwork: true`, wildcard RBAC verbs, writable root filesystems.
5. **Operational-context defects.** The manifest is statically correct but will behave poorly at runtime — missing resource requests and limits, absent readiness probes, no `PodDisruptionBudget` for a multi-replica `Deployment`, missing `NetworkPolicy`.
6. **API version drift.** The manifest declares an `apiVersion` that has been deprecated or removed in the target Kubernetes version.

Existing tooling addresses these dimensions unevenly. Schema validators such as kubeconform focus primarily on OpenAPI conformance and offer configurable local or remote schema locations, including CRD support and offline validation capability. Linters such as kube-linter and kube-score target best practices and security concerns but operate with fixed or limited rule customization and do not perform schema validation as their primary function. A practitioner who wants syntax, schema, semantic, security, operational, and API-version coverage must typically chain multiple tools, reconcile overlapping findings, and manage separate knowledge-update mechanisms for each.

The problem KubeCheck addresses is **fragmented, partially online-dependent, and non-explainable Kubernetes manifest diagnostics**. KubeCheck provides a single offline binary or package that traverses the diagnostic dimensions in a defined order, emits findings with explicit evidence and confidence, and separates deterministic checks from context-dependent heuristics. It does not replace every existing tool; it provides a coherent, offline-first diagnostic layer whose behavior is reproducible and whose knowledge is verifiable.

### 2.2 Why offline-first matters

A manifest diagnostic tool that requires network access at analysis time has three structural weaknesses:

**Availability.** CI runners, air-gapped build environments, and restricted corporate networks cannot depend on a remote schema service being reachable. kubectl-validate’s offline mode demonstrates that local schema and CRD validation without cluster connection is achievable and useful. KubeCheck treats offline operation as the default, not a fallback mode.

**Reproducibility.** A diagnostic result that depends on a mutable remote schema registry or a remote API cannot be reproduced identically six months later. KubeCheck binds each analysis to a versioned, locally resident knowledge bundle; the same bundle version produces the same deterministic results.

**Supply-chain integrity.** Mandatory remote fetching at analysis time creates an implicit trust path that is difficult to audit. KubeCheck requires cryptographic verification before any knowledge update is activated; the default analysis path performs no network I/O.

Offline-first is therefore not merely a deployment convenience. It is a precondition for reproducible diagnostics, auditable behavior, and operation in environments where network egress is restricted or prohibited.

---

## 3. Target users and personas

### 3.1 Primary personas

| Persona ID | Persona | Context | Primary need | Success signal |
|---|---|---|---|---|
| PER-001 | **Platform engineer** | Maintains shared Kubernetes configuration libraries and Helm-equivalent bases for multiple teams. | Catch schema and semantic defects before they reach a cluster; enforce consistency across a repository. | Findings are attributable to a file and JSON path; deterministic checks do not produce noise. |
| PER-002 | **CI/CD engineer** | Owns build pipelines that must run in restricted or air-gapped runners. | Run manifest diagnosis as a pipeline step without network egress; get machine-readable output and a stable exit code. | Tool installs via pip or a standalone binary; runs offline; emits JSON with a documented exit-code contract. |
| PER-003 | **Application developer** | Writes Deployment, Service, ConfigMap, and Secret manifests for a single service. | Understand *why* a manifest is flagged, not only *that* it is flagged; receive remediation guidance. | Findings include evidence, severity, confidence basis, and a remediation description. |
| PER-004 | **Security and compliance reviewer** | Reviews manifests for privileged containers, hostPath mounts, broad RBAC, and secret exposure. | Obtain an auditable security finding set without uploading manifests to a remote service. | Security checks are documented, deterministic where claimed, and run entirely locally. |
| PER-005 | **Air-gapped operator** | Manages clusters in environments with no internet access. | Install, update knowledge bundles from predownloaded artifacts, and diagnose manifests without any network dependency. | Installation and analysis succeed with network interfaces disabled; bundle verification is cryptographic. |

### 3.2 Secondary personas

**PER-006 — Kubernetes learner** uses KubeCheck as a diagnostic aid while learning API structure; the explainability of findings is more important than throughput. **PER-007 — SRE incident responder** uses KubeCheck during post-incident review to determine whether a manifest contained a statically detectable defect that contributed to the incident; KubeCheck does not perform live-cluster forensics and must not be represented as doing so.

---

## 4. Supported inputs and invocation contexts

### 4.1 Input forms

| Input form | Requirement ID | Behavior |
|---|---|---|
| Single YAML file | INP-REQ-001 | Parse as a YAML stream; a single-document file is the degenerate case. |
| Directory (recursive) | INP-REQ-002 | Discover `*.yaml` and `*.yml` files; order of processing is deterministic (lexicographic by normalized path). |
| Multi-document YAML stream | INP-REQ-003 | Each document is analyzed independently; the stream order does not alter per-document findings. |
| Standard input (`stdin`) | INP-REQ-004 | Supported when no path arguments are given or when `-` is supplied; stdin is treated as a single stream. |
| CI invocation | INP-REQ-005 | Machine-readable output (JSON) and a documented exit-code contract are available; no interactive prompts are emitted. |

Directory traversal must not follow symbolic links outside the supplied root by default. The exact path-safety rules are normative in `docs/security/SECURITY_MODEL.md`.

### 4.2 Kubernetes resource scope

KubeCheck analyzes **built-in Kubernetes API resources** for a declared set of supported Kubernetes versions. The supported version set is a product-level parameter published with each knowledge bundle; it is not hard-coded into the engine. For **Custom Resource Definitions (CRDs)**, KubeCheck supports schema validation when a CRD schema is supplied as part of the local knowledge bundle or as an explicit local schema input. CRD support is **schema-only** by default: KubeCheck does not invent semantic or operational checks for arbitrary custom resources, because the semantics of a CRD are defined by its controller, not by Kubernetes.

The distinction is product-critical:

- **Built-in resources** are governed by upstream Kubernetes API schemas and the upstream deprecation policy. KubeCheck can make normative statements about field validity and API-version lifecycle for these resources, scoped to the supported versions.
- **CRDs** are user-defined API extensions. KubeCheck can validate structural conformance against a supplied CRD schema but must not claim semantic correctness, controller compatibility, or operational suitability for a custom resource beyond what the supplied schema states.

KubeCheck must **not** claim that a manifest is deployable merely because it passes local validation. Local validation establishes that the document conforms to the supplied schemas and the implemented diagnostic rules for the selected target version; it does not establish admission-controller acceptance, controller behavior, or cluster-state compatibility.

---

## 5. Diagnostic categories

KubeCheck organizes findings into six categories. The category taxonomy is normative; individual rule identifiers and evidence contracts are specified in `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`.

### 5.1 Syntax

Parses the input as YAML in a safe mode. Prohibits arbitrary object construction and implicit code execution. Reports parse errors with document index, line, and column where available. Syntax validity is a **deterministic** property for a given parser and YAML specification version.

### 5.2 Schema

Validates each document against the OpenAPI schema associated with its Group, Version, and Kind (GVK) for the selected target Kubernetes version. Schema conformance is **deterministic** relative to the schema bundle: for a fixed bundle version and fixed target version, the result is reproducible.

### 5.3 Semantic consistency

Detects conflicts and inconsistencies that require relating two or more documents or interpreting Kubernetes semantics. Examples include Service selectors with no matching workload in the supplied set, unresolved `configMapRef` and `secretRef` references, `livenessProbe` and `readinessProbe` identity, and resource request/limit contradictions. Semantic checks are **deterministic when the required cross-document context is fully present**; when context is incomplete, the check must emit a context-dependent warning or decline to emit a finding, and must label the limitation explicitly.

### 5.4 Security posture

Evaluates security-relevant manifest properties: privileged containers, `hostNetwork`, `hostPID`, `hostIPC`, `hostPath` volumes, missing `runAsNonRoot`, writable root filesystems, broad RBAC grants, and secret material embedded in non-Secret fields. Security checks are partitioned into **deterministic** checks (e.g., a field is literally set to `true`) and **context-dependent** checks whose severity depends on organizational policy (e.g., whether a particular capability is acceptable). Context-dependent security findings carry an explicit confidence basis and must not be reported as universal defects.

### 5.5 Maintainability

Evaluates manifest-level properties that affect long-term operability of the configuration itself: missing labels and recommended labels, absent `apiVersion`/`kind` metadata, duplicate resource identities within the analyzed set, and reliance on deprecated fields that remain schema-valid but are marked for removal. Maintainability checks are advisory in severity by default; they must not be presented as blocking errors unless a project-specific rule configuration elevates them.

### 5.6 Operational context

Evaluates properties that are statically observable but whose impact depends on runtime behavior and cluster configuration: missing resource requests and limits, absent readiness or liveness probes, missing `PodDisruptionBudget` for multi-replica workloads, absent `NetworkPolicy`, and single-replica Deployments without disruption budget. Operational-context findings are **context-dependent warnings** by default. They must carry an explicit statement that runtime impact depends on cluster configuration and workload behavior. Kube-score’s check set illustrates the class of operational best-practice checks that are statically derivable but context-sensitive: container limits, probe configuration, security context settings, and NetworkPolicy targeting are all observable in the manifest but require assumptions about operational intent.

### 5.7 API-version lifecycle

KubeCheck evaluates whether the manifest’s `apiVersion` is GA, beta, alpha, deprecated, or removed for the selected target Kubernetes version. This category depends on the Kubernetes deprecation policy and must be scoped to explicitly supported versions. The upstream policy states that GA API versions may be marked deprecated but must not be removed within a major version; beta API versions must be supported for a minimum period after deprecation; and alpha API versions may be removed without notice. KubeCheck must encode these lifecycle rules from a versioned knowledge artifact and must not rely on hard-coded release assumptions. The v1.32 removal of `flowcontrol.apiserver.k8s.io/v1beta3` illustrates the class of finding this category is designed to surface: a manifest may remain syntactically and schema-valid for an older schema while being unservable on the target version.

---

## 6. Local static analysis versus live-cluster troubleshooting

KubeCheck is a **local static analysis** tool. It does not connect to a Kubernetes cluster, read cluster state, query the API server, or perform admission simulation against a live cluster. This boundary is a product commitment, not a temporary limitation.

| Dimension | KubeCheck (local static) | Live-cluster troubleshooting |
|---|---|---|
| Input | Manifest files, directories, streams, stdin. | Cluster API state, events, logs, admission webhooks. |
| Network | None required for default analysis. | Requires API server connectivity and credentials. |
| What it can establish | YAML validity, schema conformance, cross-document consistency within the supplied set, static security properties, API-version lifecycle status for the selected target version. | Whether a manifest was admitted, whether a controller reconciled it, whether a Pod scheduled, whether probes pass, whether RBAC actually permits an action. |
| What it cannot establish | Admission-controller behavior, defaulting applied by the API server, controller reconciliation, runtime probe outcomes, cluster-specific policy effects, actual resource availability. | — |
| Reproducibility | Deterministic given a fixed bundle version and target version. | Depends on cluster state at query time. |

A manifest that passes all KubeCheck deterministic checks is **locally valid and internally consistent for the selected target version**. It is not thereby proven deployable. This distinction must be preserved in every finding summary, report header, and exit-code documentation. Tools such as clusterlint operate on live clusters and perform read-only runtime checks; KubeCheck does not overlap with that responsibility and must not be compared as a substitute.

---

## 7. Comparison criteria for adjacent tools

The following comparison is limited to **documented, publicly stated capabilities** of each tool. No comparative claim is made about relative accuracy, speed, or completeness unless a reproducible benchmark for that specific claim exists. This section defines the criteria a KubeCheck evaluation must use; it does not assert that KubeCheck outperforms any listed tool.

| Criterion | kubectl | kubeconform | kube-linter | kube-score | KubeCheck (product intent) |
|---|---|---|---|---|---|
| Primary purpose | Cluster interaction; dry-run/admission simulation against a live cluster. | Schema validation of Kubernetes manifests against OpenAPI/JSON schemas. | Static analysis for best practices and security. | Static scoring based on operational best practices. | Multi-category offline diagnosis: syntax, schema, semantic, security, maintainability, operational context, API lifecycle. |
| Offline operation | Depends on cluster connectivity for server-side validation; client-side dry-run varies. | Supports local schemas and offline validation. | Static analysis; does not require a live cluster. | Static analysis; does not require a live cluster. | Offline by default; no network I/O on the analysis path. |
| Schema validation | Server-side schema validation when connected. | Core function. | Not the primary focus. | Not the primary focus. | Yes, as one of several categories. |
| CRD schema support | Via cluster when connected. | Configurable local CRD schemas. | Policy checks, not schema validation as primary function. | Limited; not schema-validation-first. | Schema validation when CRD schema is supplied locally; no invented semantic checks. |
| Best-practice / security checks | No static check library. | No. | Yes. | Yes. | Yes, with explicit deterministic vs. context-dependent classification. |
| Multi-category in one invocation | No. | No (schema only). | Best-practice/security focus. | Best-practice/operational focus. | Yes, by design. |
| Distribution | Core binary. | Standalone binary, container images, package managers. | Standalone binary, container images. | Standalone binary. | Krew, pip, standalone binaries. |
| Machine-readable output | JSON via kubectl output flags where applicable. | JSON, YAML, etc. | JSON, SARIF, etc. | JSON. | JSON with documented schema and exit-code contract. |

**Krew distribution** is available to kubectl plugins; Krew is a plugin manager maintained by the Kubernetes SIG CLI community, and kubectl plugins are standalone executables named `kubectl-*` placed on `PATH`. KubeCheck’s Krew packaging must follow this mechanism: the installed executable must be invocable as `kubectl kubecheck` and must not depend on network access at analysis time. **pip distribution** delivers KubeCheck as a Python package; the default analysis workflow must not require optional network-dependent Python dependencies. **Standalone binaries** are published per operating system and architecture and carry the same knowledge bundle schema version as the other channels.

---

## 8. Distribution channels and equivalence requirement

| Channel | Requirement ID | Equivalence obligation |
|---|---|---|
| Krew | DIST-REQ-001 | The Krew-installed plugin must expose the same diagnostic categories, output schema, and exit-code contract as the pip and standalone channels for the same bundle version. |
| pip | DIST-REQ-002 | The pip-installed package must not require network access at analysis time; its default dependency set must be installable from a local wheelhouse. |
| Standalone binary | DIST-REQ-003 | Each published binary must embed or resolve a knowledge bundle compatible with the bundle schema version declared in its release metadata. |

All channels must deliver **equivalent diagnostic behavior** for the same knowledge bundle version and target Kubernetes version. A finding produced by one channel must be reproducible by another channel given identical inputs and bundle version. Channel-specific behavior is limited to installation mechanics and optional integrations; it must not alter the deterministic analysis result.

---

## 9. User workflows

### 9.1 Developer pre-commit workflow

A developer edits a `Deployment` manifest and runs KubeCheck against the file before committing. The tool parses the YAML, validates against the target version’s schema, checks cross-document consistency within the file, evaluates security and operational-context checks, and emits human-readable findings grouped by severity. The developer uses the evidence field to locate the JSON path and the remediation field to correct the manifest. The workflow completes without network access.

### 9.2 CI gate workflow

A CI pipeline runs KubeCheck against a manifest directory with JSON output enabled. The pipeline interprets the exit code according to the documented contract and fails the build only when deterministic error-severity findings are present, unless the project explicitly elevates context-dependent findings. The pipeline logs the bundle version and target Kubernetes version alongside the findings so the result is reproducible.

### 9.3 Air-gapped knowledge update workflow

An operator downloads a signed knowledge bundle artifact on a connected machine, transfers it to the air-gapped environment, and activates it through KubeCheck’s local bundle activation command. KubeCheck verifies the cryptographic signature and bundle manifest before activation. If verification fails, the previous bundle remains active and the update is rejected. The exact verification procedure is normative in `docs/architecture/OFFLINE_KNOWLEDGE_BUNDLES.md`.

### 9.4 CRD-aware validation workflow

A platform team supplies a local directory of CRD schemas alongside the manifests. KubeCheck validates custom resources against those schemas and reports schema findings for CRDs. It does **not** report semantic or operational-context findings for CRDs, and its output explicitly labels CRD findings as schema-scope findings.

---

## 10. Non-goals

The following are explicitly out of scope for KubeCheck. They are not deferred features; they are product boundaries.

| Non-goal ID | Non-goal | Rationale |
|---|---|---|
| NG-001 | Live-cluster connectivity, cluster state reading, or admission simulation against a live API server. | KubeCheck is a local static analysis tool. Cluster interaction is a different product category with different trust and reproducibility properties. |
| NG-002 | Cloud inference, remote AI model calls, or mandatory telemetry. | Core analysis must be offline and deterministic. Optional integrations may exist only if isolated from the core engine and disabled by default. |
| NG-003 | Mandatory remote schema fetching during analysis. | Knowledge must be resident and verifiable before analysis begins. |
| NG-004 | Claiming deployability, admission acceptance, or runtime health based on local validation. | Local validation establishes local validity, not cluster acceptance. |
| NG-005 | Inventing semantic or operational checks for arbitrary CRDs without a supplied schema contract. | CRD semantics are controller-defined; KubeCheck validates structure, not custom-resource behavior. |
| NG-006 | Replacing live-cluster troubleshooting tools. | Complementary, not substitutive. |
| NG-007 | Helm chart rendering, Kustomize build, or template expansion as part of the core analysis path. | KubeCheck analyzes rendered manifests; template expansion is an upstream concern. A future optional pre-processing hook may exist, but it must not be required for core operation. |
| NG-008 | Statistically calibrated probability scores without a reproducible benchmark. | Heuristic confidence scores must not be presented as calibrated probabilities. |

---

## 11. Measurable objectives

All objectives in this section are **targets to be validated**, not claims of achieved performance. No accuracy, latency, or size figure in this document is represented as an accomplished benchmark. Validation methods are defined here; measurement results are recorded separately in `docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`.

### 11.1 Accuracy and diagnostic quality

| Objective ID | Objective | Target | Validation method |
|---|---|---|---|
| OBJ-ACC-001 | Precision of explicitly defined, independently evaluated deterministic-check categories | ≥ 99% on the published benchmark corpus, measured per category | Independent labeling by reviewers who do not see tool output; precision computed per deterministic category with confidence intervals. |
| OBJ-ACC-002 | Recall of deterministic-check categories | Measured and reported separately per category; no aggregate recall claim without per-category data | Same corpus; recall reported alongside precision, not collapsed into a single “accuracy” figure. |
| OBJ-ACC-003 | Context-dependent warning calibration | False-positive rate for context-dependent warnings measured and published; no claim of calibrated probability | Manual adjudication of a stratified sample; results reported as observed false-positive rates with sample sizes. |

The 99% precision target applies to **deterministic checks only** and only after a reproducible benchmark supports the claim. It does not apply to semantic, security, maintainability, or operational-context heuristics as a single aggregate. Recall is never claimed as a product-wide guarantee; it is measured per category. No overall “99% accuracy” claim is permitted unless a reproducible benchmark supports that exact claim.

### 11.2 Performance

| Objective ID | Objective | Target | Validation method |
|---|---|---|---|
| OBJ-PERF-001 | Single-file analysis latency | p95 ≤ 2 s for a manifest ≤ 1 MiB on a reference machine | Timed benchmark with documented hardware profile; cold-start and warm-start reported separately. |
| OBJ-PERF-002 | Directory analysis throughput | ≥ 20 files/s for manifests of ≤ 50 KiB each, excluding I/O-bound storage | Throughput benchmark with a fixed corpus; storage type documented. |
| OBJ-PERF-003 | Memory ceiling | Peak RSS ≤ 512 MiB for the benchmark corpus and default resource limits | Measured with the resource-limit configuration active. |

These targets are provisional until the benchmark corpus and reference hardware profile are published in `docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`.

### 11.3 Security and reliability

| Objective ID | Objective | Target | Validation method |
|---|---|---|---|
| OBJ-SEC-001 | Safe YAML parsing | No arbitrary object construction, no implicit code execution, no unsafe deserialization on any input path | Parser configuration audit plus adversarial corpus tests. |
| OBJ-SEC-002 | Resource exhaustion resistance | Inputs exceeding configured size, document-count, nesting-depth, alias-expansion, time, or memory limits are rejected with a structured error, not a crash or hang | Fuzz and adversarial-input test suite covering each limit dimension. |
| OBJ-SEC-003 | Knowledge bundle integrity | Activated bundles have passed cryptographic signature verification; failed verification leaves the prior bundle active | Negative-path tests with tampered signatures, expired certificates, and malformed manifests. |
| OBJ-REL-001 | Deterministic reproducibility | Identical input + identical bundle version + identical target version → identical finding set on the same platform | Differential test across repeated runs and across distribution channels. |

### 11.4 Usability

| Objective ID | Objective | Target | Validation method |
|---|---|---|---|
| OBJ-USE-001 | Finding explainability | Every finding carries a stable rule identifier, evidence anchor, severity, confidence basis, and remediation description | Schema validation of finding records; manual review of a stratified sample. |
| OBJ-USE-002 | Exit-code contract clarity | Exit codes are documented, stable across channels, and distinguish deterministic errors from context-dependent warnings | Contract tests executed against all three distribution channels. |
| OBJ-USE-003 | Offline installation success | Installation completes on a network-disabled machine from predownloaded artifacts for pip and standalone channels; Krew installation is documented with its own connectivity requirements | Installation test matrix covering each channel and each supported platform. |

### 11.5 Distribution

| Objective ID | Objective | Target | Validation method |
|---|---|---|---|
| OBJ-DIST-001 | Cross-channel behavioral equivalence | For the same bundle version and target version, the finding set is identical across Krew, pip, and standalone channels | Differential test harness comparing normalized finding records across channels. |
| OBJ-DIST-002 | Bundle schema version compatibility | Each release declares a bundle schema version; incompatible bundles are rejected with a clear error | Compatibility matrix tests across N and N−1 bundle schema versions. |

---

## 12. Feature priorities

### 12.1 MVP

The MVP is the smallest release that delivers offline multi-category diagnosis with explicit evidence and a stable distribution surface.

| Feature ID | Feature | Priority | Acceptance criterion |
|---|---|---|---|
| FEAT-MVP-001 | YAML syntax parsing with safe parser configuration | P0 | Malformed YAML produces a structured syntax finding with document index and location; no unsafe construction occurs. |
| FEAT-MVP-002 | Schema validation for built-in resources against a versioned local schema bundle | P0 | A document with an unknown field or wrong type produces a schema finding anchored to the JSON path; a valid document produces no schema finding. |
| FEAT-MVP-003 | API-version lifecycle check for the supported target versions | P0 | A removed `apiVersion` for the selected target version produces an error-severity lifecycle finding; a GA version produces no lifecycle finding. |
| FEAT-MVP-004 | Deterministic security checks for a defined initial rule set | P0 | Each initial deterministic security rule has an independent test fixture with a known positive and negative case. |
| FEAT-MVP-005 | JSON output and documented exit-code contract | P0 | JSON output validates against the published finding schema; exit codes are identical across channels. |
| FEAT-MVP-006 | Single-file, directory, stream, and stdin input | P0 | Each input form is exercised by an acceptance test; directory traversal order is deterministic. |
| FEAT-MVP-007 | Krew, pip, and standalone binary packaging | P0 | Each channel installs and runs the MVP diagnostic set; the finding set is equivalent across channels for the same bundle version. |
| FEAT-MVP-008 | Local knowledge bundle activation with signature verification | P0 | A valid signed bundle activates; a tampered bundle is rejected and the prior bundle remains active. |

### 12.2 Subsequent releases

| Feature ID | Feature | Priority | Rationale for deferral |
|---|---|---|---|
| FEAT-R2-001 | Cross-document semantic consistency checks | P1 | Requires a defined cross-document context model; MVP establishes per-document analysis first. |
| FEAT-R2-002 | Operational-context check family | P1 | Requires explicit context-dependent labeling and false-positive rate reporting before release. |
| FEAT-R2-003 | CRD schema validation from local schema inputs | P1 | Requires schema-source prioritization and CRD-scope labeling. |
| FEAT-R2-004 | SARIF output | P2 | Useful for CI integration but not a core diagnostic capability. |
| FEAT-R2-005 | Human-readable remediation rendering with document excerpts | P2 | Depends on stable evidence-anchor format from MVP. |
| FEAT-R3-001 | Project-level rule configuration and severity elevation | P2 | Must not undermine the deterministic/context-dependent separation. |
| FEAT-R3-002 | Bundle update verification with offline rollback | P2 | Extends MVP bundle activation with rollback semantics. |

### 12.3 Out of scope

All NG-001 through NG-008 items in Section 10 are out of scope by product decision. Helm rendering, Kustomize build, and template expansion are explicitly out of scope for the core analysis path; if a pre-processing hook is introduced in a future release, it must be optional and must not be required for deterministic analysis.

---

## 13. Product requirements

The following table is the binding product-level requirement register. Requirement IDs are stable and traceable to acceptance tests.

| Req ID | Requirement | Priority | Acceptance criterion | Validation method |
|---|---|---|---|---|
| PRD-001 | KubeCheck must parse YAML in safe mode, prohibiting arbitrary object construction and implicit code execution. | P0 | Adversarial YAML with `!!python/object` tags and similar constructs produces a parse/security finding, not code execution. | Adversarial parser test suite. |
| PRD-002 | KubeCheck must support single files, directories, multi-document streams, and stdin. | P0 | Each input form produces identical findings for the same manifest content. | Input-form equivalence test. |
| PRD-003 | KubeCheck must validate against a versioned local schema bundle for built-in resources and a declared set of supported Kubernetes versions. | P0 | A schema violation produces a finding anchored to the JSON path; a schema-valid document produces no schema finding for that document. | Schema positive/negative fixture suite. |
| PRD-004 | KubeCheck must distinguish deterministic checks from context-dependent warnings in the output model. | P0 | The finding schema contains a machine-readable classification field; deterministic findings never carry a heuristic confidence label. | Finding-schema validation and manual audit. |
| PRD-005 | KubeCheck must emit a stable rule identifier, evidence anchor, severity, confidence basis, and remediation for every finding. | P0 | JSON Schema validation of the finding record; a sample audit confirms all fields are populated for every rule category. | Output schema validation; stratified sample audit. |
| PRD-006 | KubeCheck must perform no network I/O during default analysis. | P0 | Analysis succeeds with network interfaces disabled; no outbound connection is attempted. | Network-isolated acceptance test. |
| PRD-007 | KubeCheck must install through Krew, pip, and standalone binaries with equivalent diagnostic behavior for the same bundle version. | P0 | Cross-channel differential test produces an identical normalized finding set. | Cross-channel equivalence harness. |
| PRD-008 | KubeCheck must verify knowledge bundle signatures before activation and must retain the prior bundle on verification failure. | P0 | A tampered bundle is rejected; the previously active bundle remains in use; an error is reported. | Bundle verification negative-path suite. |
| PRD-009 | KubeCheck must enforce configurable limits on input size, document count, nesting depth, alias expansion, processing time, and memory, and must reject exceeded inputs with a structured error. | P0 | Each limit is exercised by a dedicated adversarial fixture; the tool exits with a structured error rather than crashing or hanging. | Limit-enforcement test matrix. |
| PRD-010 | KubeCheck must support API-version lifecycle findings for the declared supported Kubernetes versions, derived from a versioned knowledge artifact. | P0 | A removed API version for the target version produces an error finding; a GA version produces no finding; the behavior is consistent with the upstream deprecation policy for the supported versions. | Lifecycle fixture suite per supported version. |
| PRD-011 | KubeCheck must support CRD schema validation when a CRD schema is supplied locally, and must label CRD findings as schema-scope findings. | P1 | A CRD instance violating its supplied schema produces a labeled schema finding; no semantic or operational finding is emitted for the CRD. | CRD fixture suite with scope-label assertion. |
| PRD-012 | KubeCheck must not claim deployability or admission acceptance based on local validation. | P0 | No output field, summary text, or documentation asserts that a passing manifest is deployable; the limitation is stated in output headers and help text. | Output text audit; documentation review. |
| PRD-013 | KubeCheck must provide JSON output conforming to a published finding schema and a documented exit-code contract. | P0 | JSON output validates against the schema; exit codes match the contract in all supported scenarios. | Schema validation and exit-code contract tests. |
| PRD-014 | KubeCheck must provide offline installation for pip and standalone channels from predownloaded artifacts. | P0 | Pip installation from a local wheelhouse succeeds with networking disabled; standalone binary installation requires no network access. | Offline installation matrix. |
| PRD-015 | KubeCheck must not represent heuristic confidence scores as statistically calibrated probabilities. | P0 | No output label uses “probability” or a percentage confidence for heuristic findings; confidence is expressed as an ordinal basis with named evidence. | Output text audit. |
| PRD-016 | KubeCheck must separate the diagnostic engine from knowledge datasets so that rule and schema updates do not require engine binary replacement. | P1 | A bundle-only update changes supported versions or rule thresholds without changing the engine binary version. | Bundle-only update test. |

---

## 14. Assumptions, risks, constraints, and unresolved decisions

### 14.1 Assumptions

| ID | Assumption | Impact if false |
|---|---|---|
| ASM-001 | The supported Kubernetes version set can be encoded in versioned schema bundles of practical size for offline distribution. | If bundle size becomes prohibitive, version coverage must be reduced or split into per-version bundles. This is a product scoping decision, not an engine limitation. |
| ASM-002 | Deterministic checks can be evaluated with high precision on a curated corpus without requiring cluster context. | If precision targets are not met, the deterministic-check boundary must be narrowed or the affected checks reclassified as context-dependent. |
| ASM-003 | Krew’s plugin mechanism imposes no analysis-time network dependency beyond installation. | If Krew installation inherently requires connectivity, the Krew channel’s offline guarantee is limited to post-installation analysis, and documentation must state this precisely. |
| ASM-004 | Safe YAML parsing can be achieved with a parser configuration that rejects unsafe tags without breaking legitimate Kubernetes manifest constructs. | If a legitimate construct is rejected, a documented compatibility exception must be recorded in the YAML compatibility decision log. |

### 14.2 Risks

| ID | Risk | Likelihood | Impact | Mitigation direction |
|---|---|---|---|---|
| RSK-001 | Schema bundle drift from upstream Kubernetes OpenAPI releases. | Medium | Findings may be stale for newly released versions. | Bundle generation pipeline pins upstream schema commit hashes; supported-version set is published per bundle; stale bundles are rejected or warned about. |
| RSK-002 | False positives in semantic or operational checks erode trust. | Medium | Users disable context-dependent checks entirely. | Context-dependent findings are clearly labeled; false-positive rates are measured and published; deterministic and context-dependent findings are separated in output and exit codes. |
| RSK-003 | CRD semantic overreach — users interpret CRD schema findings as operational validation. | Medium | Misplaced confidence in custom resources. | CRD findings are scope-labeled; documentation states that CRD semantics are controller-defined; no semantic or operational findings are emitted for CRDs by default. |
| RSK-004 | Distribution channel divergence produces different findings for the same bundle. | Low | Violates the equivalence obligation. | Cross-channel differential test harness is a release gate; bundle schema version is checked at startup. |
| RSK-005 | Resource-exhaustion input causes out-of-memory or unbounded processing. | Low | Denial of service against CI runners. | Mandatory limits on input size, document count, nesting depth, alias expansion, time, and memory; fuzz testing. |

### 14.3 Constraints

| ID | Constraint | Source |
|---|---|---|
| CON-001 | No network I/O on the default analysis path. | Product requirement PRD-006. |
| CON-002 | No cluster connectivity, cloud inference, or mandatory telemetry. | Product non-goals NG-001, NG-002. |
| CON-003 | Knowledge updates require cryptographic verification before activation. | Product requirement PRD-008; security model. |
| CON-004 | Supported Kubernetes versions are a bundle-level parameter, not a hard-coded engine constant. | Product requirement PRD-016; architectural separation of engine and knowledge. |
| CON-005 | All findings must carry evidence and confidence basis; no opaque scoring without explanation. | Product requirement PRD-005 and PRD-015. |

### 14.4 Open decisions

| ID | Decision | Status | Owner |
|---|---|---|---|
| OPEN-001 | Exact initial supported Kubernetes version range for the first bundle. | **OPEN** — requires product scoping based on bundle size and user demand. | Product Architecture |
| OPEN-002 | Whether semantic consistency checks should operate on the full supplied input set or require an explicit `--context` scope. | **OPEN** — affects false-positive behavior and cross-document context definition. | Diagnostics Architecture |
| OPEN-003 | Whether Krew channel documentation should state a post-installation-only offline guarantee or whether Krew installation itself can be made offline-capable. | **OPEN** — depends on Krew’s installation mechanics and is not asserted here. | Distribution Engineering |
| OPEN-004 | Bundle schema versioning policy: strict N−1 compatibility versus a longer compatibility window. | **OPEN** — compatibility cost versus operational flexibility. | Platform Architecture |

---

## 15. Definition of done

This document is complete when the following conditions are satisfied.

1. The problem definition distinguishes the six diagnostic dimensions and states why offline-first is a structural requirement.
2. Target personas are enumerated with primary needs and success signals.
3. Supported input forms and the built-in-resource versus CRD boundary are stated normatively.
4. The six diagnostic categories are defined, with deterministic versus context-dependent classification assigned at category level.
5. The boundary between local static analysis and live-cluster troubleshooting is explicit, including a “cannot establish” column.
6. Comparison criteria for kubectl, kubeconform, kube-linter, and kube-score are limited to documented capabilities and do not assert unsupported comparative performance claims.
7. Distribution channels and the behavioral equivalence obligation are specified with requirement IDs.
8. User workflows cover developer pre-commit, CI, air-gapped update, and CRD-aware validation.
9. Non-goals are enumerated and justified.
10. Measurable objectives cover accuracy, performance, security, reliability, usability, and distribution, with validation methods and no claimed achieved results.
11. The product requirements table contains unique, traceable IDs with priorities, acceptance criteria, and validation methods.
12. Assumptions, risks, constraints, and open decisions are recorded with impact and ownership.
13. The document references downstream documents rather than duplicating their normative content.

---

## 16. Authoritative references

| Reference | Applicable scope | Source type | Retrieved |
|---|---|---|---|
| Kubernetes Deprecation Policy | API lifecycle rules for GA, beta, and alpha versions; minimum support periods; version skew coverage | Official Kubernetes documentation | 2026-10-31 |
| Kubernetes Deprecated API Migration Guide | Per-release removed API versions, including v1.29 and v1.32 removals | Official Kubernetes documentation | 2026-10-31 |
| Kubernetes Version Skew Policy | Supported version skew between components, relevant to the supported-version scope | Official Kubernetes documentation | 2026-10-31 |
| kubeconform project documentation | Documented schema validation behavior, local schema support, CRD support, offline validation capability | Open-source project documentation | 2026-10-31 |
| kube-linter and kube-score project documentation | Documented focus on best practices and security; static analysis without live cluster requirement | Open-source project documentation | 2026-10-31 |
| kubectl-validate project documentation | Local validation of native resources and CRDs; offline support using local schemas | SIG-CLI subproject documentation | 2026-10-31 |
| Krew documentation | Krew as a SIG CLI-maintained plugin manager; plugin installation via PATH | Official Kubernetes documentation | 2026-10-31 |
| YAML 1.2.2 specification | YAML parsing semantics and compatibility decisions | Specification | 2026-10-31 |

Licensing note: Kubernetes documentation is licensed under CC BY 4.0; kubeconform, kube-linter, and kube-score are open-source projects with their respective licenses. KubeCheck must record the license of any schema or rule artifact incorporated into a knowledge bundle. No line numbers, benchmark results, or implementation status are asserted in this document beyond what the cited sources state.

---

## 17. Consistency and completeness review

The following review was performed against the requested scope before finalization.

- **Problem definition:** Six diagnostic dimensions defined; offline-first justified on availability, reproducibility, and supply-chain integrity grounds. No claim that existing tools fail universally; comparison limited to documented capabilities.
- **User coverage:** Five primary personas plus two secondary personas; workflows cover pre-commit, CI, air-gapped update, and CRD-aware validation.
- **Input coverage:** Single file, directory, stream, stdin, CI. Built-in versus CRD boundary stated.
- **Diagnostic coverage:** Syntax, schema, semantic, security, maintainability, operational context, and API lifecycle. Deterministic versus context-dependent classification present at category level.
- **Non-goals:** Eight explicit non-goals, including live-cluster connectivity, mandatory network, deployability claims, and CRD semantic overreach.
- **Objectives:** Accuracy objectives specify per-category measurement and explicitly forbid an aggregate “99% accuracy” claim without reproducible benchmark support. Performance targets are provisional and tied to a benchmark protocol. No achieved result is claimed.
- **Requirements:** Sixteen product requirements with unique IDs, priorities, acceptance criteria, and validation methods.
- **Comparison:** Tool comparison uses only documented capabilities; no invented comparative accuracy, speed, or completeness claims.
- **Distribution:** Krew, pip, and standalone channels specified with an explicit behavioral equivalence obligation.
- **Downstream references:** Architecture, offline bundles, diagnostic taxonomy, security model, distribution packaging, and accuracy benchmark protocol are referenced rather than duplicated.

No empty boilerplate sections were added. The document is complete for its assigned responsibility.