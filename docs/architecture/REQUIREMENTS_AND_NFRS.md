# KubeCheck — Production-Grade Documentation Generation Prompt Pack

**Project:** KubeCheck  
**Purpose:** Offline-first Kubernetes manifest troubleshooting CLI  
**Documentation files:** 32  
**Primary requirements:** Accuracy, offline operation, lightweight distribution, security, reliability, explainability, maintainability, and easy knowledge updates.

---

# PART I — GLOBAL INSTRUCTIONS FOR ALL 32 DOCUMENTS

## Master instruction

You are a principal Kubernetes engineer, YAML specification expert, CLI architect, software security engineer, technical writer, and production-readiness reviewer.

Your task is to produce one complete, implementation-ready technical document for KubeCheck. The requested file is part of a larger, interconnected documentation system containing 32 core documents.

KubeCheck is an offline-first CLI tool designed to diagnose Kubernetes manifest problems. It must support installation through:

1. Kubernetes Krew.
2. Python pip.
3. Standalone binaries for supported operating systems and architectures.

All installation channels must deliver equivalent diagnostic behavior and compatible knowledge data. The default analysis workflow must work without internet access, a Kubernetes cluster, a cloud service, a remote API, or an AI model.

## Mandatory engineering principles

Every document must follow these requirements wherever applicable.

### 1. Technical accuracy

- Use authoritative specifications and official documentation as the primary sources.
- For YAML, use the YAML 1.2.2 specification and explicitly document compatibility decisions.
- For Kubernetes, use official API references, version-specific OpenAPI schemas, official API deprecation information, and relevant KEPs where appropriate.
- Verify version-sensitive claims against the versions being supported.
- Never invent APIs, fields, default values, schema properties, error messages, security guarantees, benchmarks, or source citations.
- Mark unverified information as `UNVERIFIED`, an unresolved design decision as `OPEN`, and an explicit assumption as `ASSUMPTION`.
- Separate normative specifications, implementation recommendations, and optional enhancements.
- Record the authoritative source, applicable version, retrieval or generation date, and licensing information when relevant.

### 2. Offline-first operation

- All core manifest parsing and analysis must execute locally.
- No cloud inference, remote schema fetching, mandatory telemetry, or implicit cluster access.
- Knowledge bundles must contain everything required for the checks they claim to support.
- Offline updates must be possible through predownloaded, verifiable artifacts.
- Network-dependent functionality must be optional and isolated from the core engine.
- A machine without internet access must still be able to run the installed tool.

### 3. Accuracy and honest diagnostics

- Distinguish syntax validity, schema validity, semantic consistency, security posture, and operational health.
- Do not claim that a manifest is deployable merely because it passes local validation.
- Distinguish deterministic errors from probable defects and context-dependent warnings.
- Define evidence requirements, false-positive risks, severity, confidence, and remediation for every applicable diagnostic.
- Never represent a heuristic confidence score as a statistically calibrated probability without supporting evidence.
- Target at least 99% precision for explicitly defined, independently evaluated deterministic-check categories. Measure recall separately.
- Do not claim an overall 99% accuracy rate until a reproducible benchmark supports it.

### 4. Security and resource safety

- Treat every input manifest, file path, archive, rule pack, and knowledge bundle as potentially untrusted.
- Use safe YAML parsing; prohibit arbitrary object construction and implicit code execution.
- Define limits for input size, document count, nesting depth, alias expansion, processing time, and memory consumption.
- Avoid shell execution, implicit network calls, secret leakage, unsafe file writes, and uncontrolled recursive processing.
- Require cryptographic verification of trusted knowledge updates before activation.
- Document supply-chain, dependency, integrity, rollback, and recovery risks.

### 5. Maintainability

- Separate the diagnostic engine from the knowledge datasets.
- Keep schemas, rules, source metadata, and bundle formats versioned.
- Prefer deterministic, declarative rules over opaque heuristics.
- Make every diagnostic independently testable.
- Define backward compatibility and deprecation policies.
- Avoid unnecessary dependencies and duplicated knowledge.
- Do not couple all supported Kubernetes versions to one mutable schema.

### 6. Documentation quality

Every document must contain the sections relevant to its purpose:

1. Purpose and scope.
2. Normative requirements.
3. Terminology and assumptions.
4. Detailed technical content.
5. Architecture or data-flow diagrams where useful.
6. Concrete examples and counterexamples.
7. Edge cases and failure modes.
8. Security considerations.
9. Compatibility requirements.
10. Testing and acceptance criteria.
11. Dependencies on other KubeCheck documents.
12. Authoritative references.
13. Open questions and unresolved decisions.
14. Definition of done.

Do not add empty boilerplate sections merely to satisfy the list.

### 7. Required output behavior

For every individual generation request:

- Create exactly the requested file at its specified path.
- Produce complete Markdown, not an outline or a proposal to write the document later.
- Include real examples, realistic edge cases, testable requirements, and implementation guidance.
- Use Mermaid diagrams when they materially clarify architecture or workflows.
- Use tables for matrices, compatibility rules, configuration fields, and acceptance criteria.
- Mark normative requirements using stable IDs such as `YAML-REQ-001`, `SEC-REQ-001`, or `DIST-REQ-001`.
- Make requirements uniquely identifiable and traceable to tests.
- Reference related documents by their repository-relative paths.
- Do not fabricate line numbers, citations, test results, benchmark results, or implementation status.
- If information is missing, document the uncertainty and continue with a clearly labeled provisional design.
- Do not silently change previously approved architectural decisions.
- Keep the document focused on its assigned responsibility and reference other documents rather than duplicating them.
- Before finishing, run a consistency and completeness review against the requested scope.

---

# PART II — THE 32 INDIVIDUAL DOCUMENT PROMPTS

## Group 1: Product and architecture

### Prompt 01 — `docs/architecture/PRODUCT_VISION.md`

Act as a principal product architect for developer tooling. Create the complete product vision and scope document for KubeCheck.

Define the problem KubeCheck solves, target users, user workflows, supported inputs, supported diagnostic categories, non-goals, and measurable success criteria.

Cover:

- Offline-first operation and why it matters.
- Single files, directories, multi-document YAML streams, stdin, and CI usage.
- Kubernetes manifests, version-specific APIs, and the distinction between built-in resources and CRDs.
- Syntax, schema, semantic, security, maintainability, and operational-context checks.
- Differences between local static analysis and live-cluster troubleshooting.
- Comparison criteria for tools such as `kubectl`, kubeconform, kube-linter, and kube-score, without inventing unsupported comparative claims.
- Krew, pip, and standalone binary distribution.
- User personas, core use cases, failure scenarios, and expected outputs.
- Measurable performance, accuracy, security, reliability, and usability objectives.
- Feature priorities: MVP, subsequent releases, and out-of-scope features.
- Assumptions, risks, constraints, and unresolved product decisions.

Include a product requirements table with unique IDs, priorities, acceptance criteria, and validation methods.

Define success without claiming that a particular accuracy, binary size, or execution time has already been achieved.

Output the complete file `docs/architecture/PRODUCT_VISION.md`.

### Prompt 02 — `docs/architecture/SYSTEM_ARCHITECTURE.md`

Act as a principal software architect specializing in Kubernetes tooling. Design the complete architecture of KubeCheck.

Specify the components, interfaces, dependencies, data structures, error propagation, and processing order.

Cover:

- CLI entry point and command routing.
- File discovery and secure input handling.
- YAML parser and source-location preservation.
- Normalized internal representation.
- Schema selection and validation.
- Kubernetes resource inventory and relationship graph.
- Declarative rule engine.
- Version-aware compatibility checks.
- Diagnostic aggregation, deduplication, confidence classification, and reporting.
- Knowledge bundle loader and integrity verification.
- Optional update/import/export subsystem.
- Logging, configuration, exit codes, and output serialization.
- Cancellation, bounded concurrency, deterministic execution, and partial-failure handling.
- Plugin or extension boundaries, if approved by the architecture.
- Separation of the core engine from Krew, pip, and binary packaging.

Provide system-context, component, and data-flow Mermaid diagrams. Define the interfaces between components and their ownership responsibilities.

Explain how the architecture avoids mandatory runtime network access, hidden mutable global state, unsafe input processing, and inconsistent behavior across packaging formats.

Include major architectural decisions, rejected alternatives, trade-offs, and ADR references. Do not choose implementation details that conflict with the approved language or dependency decisions.

Output `docs/architecture/SYSTEM_ARCHITECTURE.md`.

### Prompt 03 — `docs/architecture/REQUIREMENTS_AND_NFRS.md`

Act as a systems engineer responsible for measurable non-functional requirements. Specify the functional and non-functional contract for KubeCheck.

Cover:

- Supported operating systems, CPU architectures, runtime dependencies, and filesystem assumptions.
- Offline execution guarantees and permitted network behavior.
- Correctness, precision, recall, reproducibility, and false-positive limits.
- Startup latency, throughput, peak memory, package size, and large-manifest handling.
- Input-size, nesting-depth, document-count, and execution-time limits.
- Secure defaults, integrity verification, dependency hygiene, and secret redaction.
- Accessibility and usability of terminal output.
- Machine-readable output stability and exit-code compatibility.
- Backward compatibility, upgrade behavior, and support lifecycle.
- Testability, maintainability, observability without telemetry, and recovery from corruption.
- Build reproducibility and distribution consistency.

For every requirement, assign a unique ID, rationale, measurable target or explicitly marked provisional target, test method, priority, and owner component.

Define benchmark methodology and environmental conditions. Distinguish design targets from verified measurements.

Explain how to validate requirements on machines with no internet access and without a running Kubernetes cluster.

Output `docs/architecture/REQUIREMENTS_AND_NFRS.md`.

### Prompt 04 — `docs/architecture/ROADMAP_AND_DECISIONS.md`

Act as a technical program architect. Define KubeCheck's implementation roadmap and architectural decision process.

Cover:

- MVP scope and dependency order.
- YAML parsing and validation milestones.
- Initial Kubernetes schema and API-version coverage.
- Diagnostic engine and cross-resource analysis milestones.
- Security analysis and remediation maturity.
- Knowledge-bundle generation, signing, updates, and rollback.
- Packaging for Krew, pip, and standalone binaries.
- CI, test corpus, fuzzing, performance benchmarks, and release gates.
- Future CRD, Helm, Kustomize, policy-pack, and extension support.
- Feature dependencies, risks, exit criteria, and release-blocking defects.
- Compatibility and deprecation policies.
- Architecture Decision Record template and review process.

Create a phased roadmap with deliverables, prerequisites, acceptance criteria, and measurable completion conditions.

Do not assume that a feature is implemented simply because it appears in the roadmap.

Define how decisions are proposed, evaluated, approved, superseded, and linked to affected documentation and tests.

Output `docs/architecture/ROADMAP_AND_DECISIONS.md`.

---

## Group 2: YAML knowledge

### Prompt 05 — `docs/yaml/YAML_LANGUAGE_SPEC.md`

Act as a YAML language specification expert. Create a technically rigorous reference for the YAML features relevant to Kubernetes manifest analysis.

Use the official YAML 1.2.2 specification as the primary authority. Explicitly distinguish the specification from implementation-specific behavior.

Cover:

- Streams, documents, directives, and document boundaries.
- Indentation and block structure.
- Block and flow sequences and mappings.
- Plain, single-quoted, double-quoted, literal, and folded scalars.
- Scalar resolution, nulls, booleans, numbers, timestamps, and ambiguous values.
- Tags, anchors, aliases, merge-key conventions, and directives.
- Comments, Unicode, escape sequences, line breaks, and whitespace.
- Mapping-key requirements and duplicate-key ambiguity.
- Empty documents, multiple documents, nested structures, and scalar roots.
- YAML 1.1 versus YAML 1.2 compatibility differences.
- Constructs commonly encountered in Kubernetes manifests.
- Unsupported, discouraged, or security-sensitive constructs.

Include valid and invalid examples, expected parser interpretations, edge cases, and source references to the relevant specification sections.

Explain which YAML features KubeCheck accepts, rejects, or handles with explicit warnings. Never claim that a feature is prohibited by YAML merely because KubeCheck chooses not to support it.

Output `docs/yaml/YAML_LANGUAGE_SPEC.md`.

### Prompt 06 — `docs/yaml/YAML_PARSING_AND_EDGE_CASES.md`

Act as a parser engineer. Specify how KubeCheck must handle YAML parsing edge cases and parser-dependent behavior.

Cover:

- Tabs, inconsistent indentation, malformed mappings, and broken sequences.
- Unclosed quotes, invalid escapes, malformed block scalars, and unexpected tokens.
- Duplicate keys and conflicting values.
- Ambiguous scalars and implicit type conversion.
- Anchors, aliases, merge keys, recursive aliases, and alias-expansion attacks.
- Multiple documents, empty streams, empty documents, and document separators.
- Unicode normalization considerations, invalid byte sequences, and line-ending variants.
- Very long lines, deeply nested collections, huge scalars, and large collections.
- Parser differences across supported libraries.
- Source locations, document indexes, and error recovery.
- Parser version upgrades and regression risks.

For every edge case, specify expected behavior, severity, source-location requirements, recovery policy, and test scenarios.

Define resource limits that can be configured or fixed, with documented defaults selected through an explicit design decision.

Explain when processing must stop and when safe partial diagnostics can continue.

Output `docs/yaml/YAML_PARSING_AND_EDGE_CASES.md`.

### Prompt 07 — `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md`

Act as a compiler and data-validation engineer. Define the pipeline from raw YAML bytes to a normalized Kubernetes resource representation.

Cover:

- Input decoding, parsing, document splitting, and source tracking.
- YAML node model versus native language values.
- Mapping and sequence normalization.
- Type preservation and scalar-resolution policy.
- YAML-to-JSON conversion constraints.
- Kubernetes object-root validation.
- Duplicate-key rejection and unsupported-tag behavior.
- Source-map preservation through normalization.
- Unknown fields and unknown API versions.
- Distinction between parse errors, normalization errors, schema errors, and semantic findings.
- Handling of null, empty, absent, and defaulted fields.
- Deterministic serialization and stable resource identity.
- Error recovery without silently changing the original input.

Provide pseudocode and representative input/output examples. Define an internal normalized representation and its invariants without binding it unnecessarily to a specific implementation language.

Explain how every diagnostic maps back to the original file, document, line, and column.

Output `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md`.

### Prompt 08 — `docs/yaml/YAML_SECURITY_AND_PARSER_POLICY.md`

Act as an application-security engineer specializing in parsers and untrusted data. Define KubeCheck's YAML security policy.

Cover:

- Safe parsing and prevention of arbitrary object instantiation.
- Alias expansion and recursion limits.
- Excessive nesting, collection sizes, scalar lengths, and document counts.
- CPU and memory exhaustion.
- Malformed UTF-8, invalid encodings, and parser crashes.
- Duplicate-key ambiguity and parser differentials.
- Unsafe custom tags and unsupported extensions.
- Path traversal, symlink handling, file discovery, and stdin behavior.
- Secret exposure through parser errors, logs, temporary files, and debug output.
- Dependency vulnerabilities, parser upgrades, fuzzing, and crash triage.
- Fail-closed versus partial-result behavior.

Create a threat-to-mitigation matrix with risk, preconditions, preventive controls, detection methods, tests, and residual risk.

Specify safe defaults and a policy for any future parser configuration override.

Output `docs/yaml/YAML_SECURITY_AND_PARSER_POLICY.md`.

---

## Group 3: Kubernetes knowledge

### Prompt 09 — `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`

Act as a Kubernetes API expert. Create the foundational reference for understanding and validating Kubernetes resources offline.

Cover:

- API groups, versions, resources, kinds, and discovery concepts.
- `apiVersion`, `kind`, `metadata`, `spec`, and `status`.
- Namespaces, names, labels, selectors, annotations, and owner references.
- Built-in resources, CRDs, aggregated APIs, and custom resources.
- Namespaced versus cluster-scoped resources.
- API-server validation, schema validation, admission, defaulting, and pruning.
- Field types, required fields, enumerations, unions, lists, maps, and schema constraints.
- Unknown fields and version-specific behavior.
- Resource identity and duplicate object detection.
- Object references, lifecycle relationships, and API compatibility.
- Offline limitations when cluster discovery is unavailable.

Provide resource examples and explain which facts are derivable from local schemas versus cluster state.

Define the metadata needed to select the correct schema and avoid assuming that every API is built into Kubernetes.

Output `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`.

### Prompt 10 — `docs/kubernetes/WORKLOADS_AND_PODS.md`

Act as a Kubernetes workload troubleshooting expert. Create a detailed reference for diagnosing workload manifest defects.

Cover Pods, ReplicaSets, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs, init containers, sidecars, and ephemeral-container-related schema behavior.

Include:

- Pod templates and selector consistency.
- Containers, images, commands, arguments, environment variables, ports, and lifecycle hooks.
- Startup, readiness, and liveness probes.
- Resource requests and limits.
- Image pull policies and image reference syntax.
- Volumes, mounts, service accounts, security contexts, and scheduling fields.
- Rolling-update settings and workload-specific constraints.
- Job parallelism, completion, deadlines, and retry behavior.
- StatefulSet identity and storage relationships.
- Common configuration mistakes and their symptoms.
- Version-specific fields and deprecations.

For every diagnostic family, identify deterministic checks, heuristic checks, required evidence, likely false positives, remediation, and regression tests.

Clearly separate manifest defects from failures that require live Pod or controller status.

Output `docs/kubernetes/WORKLOADS_AND_PODS.md`.

### Prompt 11 — `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md`

Act as a Kubernetes networking specialist. Define offline analysis for service discovery, network routing, and workload connectivity.

Cover:

- Services and ClusterIP, NodePort, and LoadBalancer types.
- Selectors, Pod labels, EndpointSlices, and target ports.
- Named ports, numeric ports, protocols, and port-range constraints.
- Ingress resources, ingress classes, and controller-dependent behavior.
- Gateway API resources and versioned schemas.
- NetworkPolicy selectors and ingress/egress rules.
- DNS names and namespace-aware service references.
- Headless Services, external names, and selectorless Services.
- CNI-dependent behavior and limitations of static network analysis.
- TLS references and routing configuration where relevant.
- Common cross-resource errors and likely connectivity symptoms.

Specify the exact checks KubeCheck can perform using manifests alone and which checks require cluster state, controller knowledge, DNS resolution, or runtime networking.

Provide examples of valid and invalid configurations and tests for both positive and negative findings.

Output `docs/kubernetes/NETWORKING_AND_SERVICE_DISCOVERY.md`.

### Prompt 12 — `docs/kubernetes/CONFIGURATION_AND_STORAGE.md`

Act as a Kubernetes storage and configuration specialist. Define offline checks for configuration resources and persistent storage.

Cover ConfigMaps, Secrets, environment references, projected volumes, volume mounts, ephemeral volumes, PersistentVolumes, PersistentVolumeClaims, StorageClasses, and relevant CSI configuration fields.

Include:

- Missing references and namespace mismatches.
- Volume and mount name consistency.
- Read-only versus writable mount behavior.
- PVC binding assumptions and access modes.
- StorageClass references and versioned fields.
- Stateful workload storage relationships.
- Optional references and their actual semantics.
- Secret exposure through literals, environment variables, mounted files, and logs.
- Configuration update behavior and restart assumptions.
- Size, type, and schema constraints where authoritative.
- External storage drivers and unknown runtime conditions.

Distinguish deterministic reference errors from runtime binding, permission, attachment, and capacity problems.

Ensure remediation suggestions do not expose secret values or recommend unsafe credential handling.

Output `docs/kubernetes/CONFIGURATION_AND_STORAGE.md`.

### Prompt 13 — `docs/kubernetes/SECURITY_AND_RBAC.md`

Act as a Kubernetes security architect. Define KubeCheck's Kubernetes security knowledge and analysis boundaries.

Cover:

- ServiceAccounts, Roles, ClusterRoles, RoleBindings, and ClusterRoleBindings.
- API groups, resources, verbs, resource names, and subresources.
- Namespace and cluster-scope binding semantics.
- Pod and container security contexts.
- Privileged mode, Linux capabilities, host namespaces, hostPath, and privilege escalation.
- Non-root execution, read-only root filesystems, seccomp, and supported security fields.
- Pod Security Standards and version-dependent policy behavior.
- Secret references and exposure risks.
- Image security and supply-chain policy references.
- Host access, dangerous mounts, and overly broad permissions.
- NetworkPolicy and isolation considerations.
- Misconfigurations that create excessive privilege.

For each rule, define evidence, severity, version applicability, confidence, false-positive risks, and remediation.

Never claim that a manifest alone proves the actual effective permissions of a live identity or the security of an image.

Include secure examples, deliberately insecure examples, and regression tests.

Output `docs/kubernetes/SECURITY_AND_RBAC.md`.

### Prompt 14 — `docs/kubernetes/SCHEDULING_AND_RESOURCE_MANAGEMENT.md`

Act as a Kubernetes scheduling and capacity specialist. Define offline analysis of resource allocation and scheduling configuration.

Cover:

- CPU, memory, ephemeral storage, and extended-resource quantities.
- Requests, limits, QoS-related configuration, and resource defaults.
- Node selectors, node affinity, pod affinity, anti-affinity, and topology spread.
- Taints, tolerations, node conditions as runtime-only evidence, and priority classes.
- Pod disruption budgets.
- Horizontal Pod Autoscalers and supported metric references.
- ResourceQuota and LimitRange.
- Persistent storage and scheduling interactions.
- Architecture and operating-system scheduling constraints.
- Common pending-Pod, OOMKilled, and resource-pressure patterns.
- Version-specific scheduling fields.

Distinguish impossible or inconsistent declared configurations from merely restrictive configurations whose feasibility depends on actual cluster nodes and capacity.

Define units, quantity parsing, overflow handling, and comparisons using authoritative Kubernetes quantity semantics.

Output `docs/kubernetes/SCHEDULING_AND_RESOURCE_MANAGEMENT.md`.

### Prompt 15 — `docs/kubernetes/KUBERNETES_VERSION_COMPATIBILITY.md`

Act as a Kubernetes API lifecycle and compatibility specialist. Define how KubeCheck supports multiple Kubernetes minor versions without confusing schemas or deprecation policies.

Cover:

- Supported-version policy and end-of-support criteria.
- Schema provenance and version pinning.
- API additions, deprecations, removals, and replacement APIs.
- Version-specific required fields, types, defaults, and constraints.
- Feature gates and fields whose validation depends on configuration.
- CRD and operator version compatibility.
- Unknown or ambiguous target-version behavior.
- Multi-version analysis and diagnostic differences.
- Schema bundle generation, verification, and regression tests.
- Release compatibility and migration guidance.

Provide a compatibility matrix template and a deterministic algorithm for choosing a schema when the target version is known or unknown.

Never infer a precise target cluster version from a manifest unless authoritative evidence establishes it.

Output `docs/kubernetes/KUBERNETES_VERSION_COMPATIBILITY.md`.

---

## Group 4: Diagnostic engine

### Prompt 16 — `docs/engine/DIAGNOSTIC_RULE_SPEC.md`

Act as the lead engineer of a deterministic static-analysis engine. Specify the complete KubeCheck diagnostic rule format and execution contract.

Define:

- Stable rule IDs and namespaces.
- Rule schema and schema version.
- Title, description, category, severity, confidence, evidence, remediation, references, and applicability.
- Preconditions, match predicates, exclusions, and dependency requirements.
- Kubernetes versions and resource kinds affected.
- Diagnostic source locations and resource identities.
- Duplicate suppression, rule ordering, and deterministic output.
- Rule dependencies, conflict resolution, and exception handling.
- Rule deprecation, replacement, and compatibility.
- Safe expression evaluation without arbitrary code execution.
- Performance limits and invalid-rule behavior.
- Unit tests and positive, negative, and boundary test fixtures.

Include a complete illustrative rule, its validated data structure, expected diagnostic, and test cases.

If YAML is chosen for rule authoring, define a versioned machine-readable schema and reject invalid rule packs before use.

Output `docs/engine/DIAGNOSTIC_RULE_SPEC.md`.

### Prompt 17 — `docs/engine/STATIC_ANALYSIS_RULES.md`

Act as a Kubernetes static-analysis rule author and reviewer. Build a comprehensive taxonomy of local checks KubeCheck should implement.

Organize checks into:

- YAML and normalized-structure defects.
- Schema violations and unknown fields.
- Required, missing, mistyped, and invalid values.
- API deprecations and version incompatibility.
- Workload configuration defects.
- Probe and lifecycle issues.
- Resource request and limit problems.
- Service and networking configuration.
- Storage and configuration references.
- Security and RBAC risks.
- Reliability and availability risks.
- Maintainability and best-practice warnings.

For every rule family, specify the matching conditions, counterexamples, severity, confidence, applicability, false-positive traps, remediation, and test cases.

Separate deterministic validation from heuristic best-practice recommendations. Avoid blanket rules that misclassify valid optional configurations.

Provide an initial rule inventory with stable proposed IDs, but do not invent claims of completed implementation or verified detection rates.

Output `docs/engine/STATIC_ANALYSIS_RULES.md`.

### Prompt 18 — `docs/engine/CROSS_RESOURCE_ANALYSIS.md`

Act as a Kubernetes object-graph analysis specialist. Define how KubeCheck identifies defects that cannot be detected by validating resources individually.

Cover:

- Resource identity, scope, namespace, and duplicate definitions.
- Pod-template and workload selector relationships.
- Service selector-to-Pod label matching.
- ConfigMap, Secret, ServiceAccount, PVC, and StorageClass references.
- RBAC subject and role references.
- Ingress and Gateway backend references.
- Owner references and controller relationships.
- NetworkPolicy and workload selector relationships.
- Resource ordering versus actual dependency semantics.
- Optional, external, generated, and unresolved references.
- CRDs, custom resources, and incomplete knowledge.
- Duplicate, ambiguous, and conflicting references.

Define a graph model, edge types, indexing strategy, traversal limits, and deterministic reference-resolution algorithm.

Explicitly distinguish “no matching resource found in the analyzed input” from “resource does not exist in the cluster.”

Include graph examples, diagnostic rules, scalability constraints, and tests for false positives.

Output `docs/engine/CROSS_RESOURCE_ANALYSIS.md`.

### Prompt 19 — `docs/engine/ROOT_CAUSE_AND_REMEDIATION.md`

Act as a senior incident-analysis engineer. Define how KubeCheck groups findings into understandable explanations and safe remediation suggestions.

Cover:

- Relationship between symptoms, causes, and contributing factors.
- Rule dependencies and evidence-based root-cause ranking.
- Duplicate and cascading diagnostics.
- Confidence and severity aggregation.
- Minimal correction suggestions and context preservation.
- Patch generation versus advisory-only recommendations.
- Handling of multiple valid corrections.
- Uncertain, conflicting, or incomplete evidence.
- Source references and links to official documentation.
- Secret-safe explanations and redaction.
- Prevention of unsupported claims about runtime behavior.
- Explanation of why a recommendation applies.

Provide examples involving a Service selector mismatch, missing configuration references, invalid API versions, and resource scheduling constraints.

For every remediation type, define preconditions, expected changes, validation steps, and risks. Do not automatically apply changes unless a separately approved and explicitly invoked editing feature is designed.

Output `docs/engine/ROOT_CAUSE_AND_REMEDIATION.md`.

### Prompt 20 — `docs/engine/DIAGNOSTIC_CONFIDENCE_AND_LIMITATIONS.md`

Act as a quality and reliability engineer. Define a transparent framework for expressing the certainty and limitations of KubeCheck findings.

Cover:

- Deterministic errors versus heuristic findings.
- Evidence levels and required proof.
- Confidence labels and their meaning.
- Distinction between severity, confidence, and probability.
- Calibration methodology if numeric probabilities are introduced.
- False-positive and false-negative tracking.
- Unknown schemas, missing CRDs, and ambiguous target versions.
- Cluster-state-dependent conditions.
- Admission policies, controllers, and external dependencies.
- Suppression, exceptions, and baseline comparisons.
- Diagnostic provenance and user-facing limitations.
- Benchmark categories and confidence regression tests.

Specify rules for downgrading confidence or returning an explicit unknown result when required information is unavailable.

Never invent confidence percentages. Require independent labeled evaluation before publishing calibration claims.

Output `docs/engine/DIAGNOSTIC_CONFIDENCE_AND_LIMITATIONS.md`.

---

## Group 5: Security and reliability

### Prompt 21 — `docs/security/THREAT_MODEL.md`

Act as a product security architect. Produce a formal threat model for KubeCheck using a structured methodology such as STRIDE where applicable.

Identify assets, trust boundaries, threat actors, attack surfaces, attack trees, abuse cases, and security assumptions.

Cover:

- Malicious YAML and pathological parser inputs.
- Malicious filenames, symlinks, path traversal, and directory traversal.
- Oversized manifests and resource-exhaustion attacks.
- Compromised dependencies and build infrastructure.
- Malicious diagnostic rules and poisoned knowledge bundles.
- Tampered update archives and signature-verification failures.
- Rollback attacks and stale knowledge.
- Secret exposure in logs, output, temporary files, and crash reports.
- Unintended network access or shell execution.
- Supply-chain attacks against Krew, pip, and binary releases.
- Cross-platform filesystem differences.
- Denial of service and privilege boundaries.

Create a risk register containing likelihood, impact, severity, mitigation, verification method, residual risk, and ownership.

Define release-blocking security findings and an incident-response procedure.

Output `docs/security/THREAT_MODEL.md`.

### Prompt 22 — `docs/security/SAFE_EXECUTION_POLICY.md`

Act as a secure CLI implementation specialist. Define the safe execution contract for KubeCheck.

Cover:

- Read-only default behavior.
- No implicit Kubernetes API access or kubeconfig use.
- No shell commands derived from manifest content.
- No arbitrary code execution through YAML tags, rules, templates, or plugins.
- Safe file traversal, symlink policy, path validation, and stdin handling.
- Resource limits and cancellation.
- Safe temporary-file creation and cleanup.
- Output escaping for terminals, JSON, and logs.
- Secret redaction and error-message hygiene.
- File permissions and platform-specific behavior.
- Optional future features that require explicit user authorization.
- Safe behavior when configuration or knowledge data is malformed.

For each policy, define an enforceable requirement and a corresponding test.

Include prohibited behaviors, approved exceptions, and a process for reviewing changes that weaken safety guarantees.

Output `docs/security/SAFE_EXECUTION_POLICY.md`.

### Prompt 23 — `docs/security/FAILURE_HANDLING_AND_RECOVERY.md`

Act as a reliability engineer. Specify KubeCheck's behavior when any component fails.

Cover:

- Invalid input, parser crashes, unsupported syntax, and timeouts.
- Missing, corrupt, incompatible, or partially readable schemas.
- Invalid rules and failed rule evaluation.
- Knowledge-bundle checksum or signature failures.
- Interrupted updates and storage errors.
- Output write failures, broken pipes, and cancellation.
- Memory pressure and resource-limit violations.
- Unsupported platforms and dependency failures.
- Partial analysis and independent document failures.
- Recovery from interrupted bundle replacement.
- Safe rollback and preservation of last-known-good data.

Create a failure-mode matrix with detection, user-visible message, exit code, partial-result policy, recovery action, and test.

Never silently skip a critical security check or label incomplete analysis as complete.

Output `docs/security/FAILURE_HANDLING_AND_RECOVERY.md`.

### Prompt 24 — `docs/security/PRIVACY_AND_TELEMETRY_POLICY.md`

Act as a privacy engineer. Define KubeCheck's data-handling and privacy contract.

Cover:

- Local processing and zero required telemetry.
- No automatic upload of manifests or diagnostics.
- Handling of credentials, Secret resources, and sensitive annotations.
- Log and debug-output redaction.
- Temporary files and cleanup.
- Crash reports and support bundles.
- Optional update checks and network destinations, if supported.
- Consent requirements for future data sharing.
- Retention, deletion, and export behavior for locally stored data.
- Dependency behavior and unexpected outbound traffic.
- Privacy tests and verification on offline machines.

Define which data is collected, stored, transmitted, or never collected. Prefer no collection unless a clearly justified feature requires it.

Do not claim that every operating-system or third-party dependency is incapable of network access; document the actual guarantees and verification boundaries.

Output `docs/security/PRIVACY_AND_TELEMETRY_POLICY.md`.

---

## Group 6: Offline knowledge and updates

### Prompt 25 — `docs/knowledge/KNOWLEDGE_BUNDLE_FORMAT.md`

Act as a data-format and package-format architect. Specify the complete, versioned KubeCheck knowledge-bundle format.

Define:

- Bundle manifest and format version.
- Engine compatibility range.
- Supported Kubernetes versions.
- Schema files and schema metadata.
- Diagnostic rule packs.
- Resource relationship definitions.
- Diagnostic catalog and source references.
- Provenance, licensing, generation timestamp, and source versions.
- Cryptographic hashes and signature metadata.
- Indexes, compression, and optional data partitions.
- Resource limits and maximum supported bundle size.
- Unknown fields and forward-compatibility policy.
- Validation, loading, caching, and integrity verification.
- Backward compatibility and deprecation.
- Invalid, incomplete, stale, or unsupported bundle handling.

Provide a complete illustrative bundle manifest and directory layout. Specify a machine-readable schema for the bundle metadata.

Explain how bundles can be generated and imported on a connected machine and then transported securely to an isolated machine.

Output `docs/knowledge/KNOWLEDGE_BUNDLE_FORMAT.md`.

### Prompt 26 — `docs/knowledge/OFFLINE_UPDATE_AND_SIGNING.md`

Act as a software supply-chain security engineer. Design the offline-compatible knowledge update and signing mechanism.

Cover:

- Trusted signing-key distribution and key rotation.
- Digital signatures and authenticated metadata.
- Hash verification and archive integrity.
- Compatibility checks before installation.
- Version ordering and anti-rollback controls.
- Freshness policies and intentionally permitted offline exceptions.
- Atomic activation and last-known-good bundle preservation.
- Interrupted updates and crash recovery.
- Manual import/export through removable media.
- Revocation and emergency replacement of compromised bundles.
- Update provenance and audit logs.
- No mandatory network access.
- Failure behavior and clear user messaging.

Specify a cryptographic verification workflow, key-management boundaries, and test vectors using non-production example keys.

Distinguish a checksum, which detects corruption relative to a trusted digest, from a digital signature, which authenticates signed metadata under a trusted key.

Never hardcode private signing keys into the repository, package, or client binary.

Output `docs/knowledge/OFFLINE_UPDATE_AND_SIGNING.md`.

### Prompt 27 — `docs/knowledge/KNOWLEDGE_BUILD_PIPELINE.md`

Act as a knowledge-engineering and CI/CD architect. Define the complete pipeline that produces KubeCheck's schemas, rules, indexes, and release bundles.

Cover:

- Authoritative source acquisition and pinned source versions.
- Kubernetes OpenAPI schema extraction and normalization.
- Rule authoring and schema validation.
- Provenance and license verification.
- Deterministic generation and reproducible outputs.
- Duplicate detection and conflict handling.
- Static checks, unit tests, regression tests, fuzzing, and compatibility tests.
- False-positive evaluation and benchmark gates.
- Bundle integrity and signing.
- Artifact publishing and release metadata.
- Rollback, emergency fixes, and source updates.
- Dependency pinning and isolated build environments.
- Auditability and reproducibility.

Define each pipeline stage, its inputs, outputs, validation rules, failure conditions, and release gates.

Ensure the connected build pipeline never becomes a runtime dependency for the offline CLI.

Output `docs/knowledge/KNOWLEDGE_BUILD_PIPELINE.md`.

---

## Group 7: Testing and quality

### Prompt 28 — `docs/quality/TEST_STRATEGY.md`

Act as a principal quality engineer for a security-sensitive CLI. Define the complete KubeCheck testing strategy.

Cover:

- Unit, integration, end-to-end, and contract testing.
- YAML parser edge cases and source-location accuracy.
- Schema selection and version-specific validation.
- Diagnostic-rule positive and negative cases.
- Cross-resource graph correctness.
- Property-based and metamorphic testing.
- Fuzzing, malformed inputs, and resource-exhaustion tests.
- Security regression testing.
- Deterministic output and reproducibility.
- Offline network-isolation tests.
- Cross-platform and architecture testing.
- Knowledge-bundle compatibility and update recovery.
- CLI exit codes and output formats.
- Installation and behavior parity across Krew, pip, and binaries.
- Performance, memory, and large-input benchmarks.

Define fixture organization, test naming, coverage expectations, CI stages, flaky-test policy, and release-blocking failures.

Explain why code coverage alone cannot establish diagnostic accuracy.

Output `docs/quality/TEST_STRATEGY.md`.

### Prompt 29 — `docs/quality/ACCURACY_BENCHMARKS.md`

Act as a statistical evaluation and static-analysis specialist. Define a reproducible benchmark for measuring KubeCheck's diagnostic quality.

Cover:

- Independently labeled valid and invalid manifests.
- Syntax, schema, semantic, security, compatibility, and heuristic categories.
- Positive cases, negative cases, and misleading near-matches.
- Version-specific fixtures and CRD scenarios.
- Precision, recall, false-positive rate, false-negative rate, and F1 where appropriate.
- Severity-weighted metrics, clearly distinguished from ordinary accuracy.
- Confidence calibration when numeric probabilities are used.
- Dataset provenance, licensing, deduplication, and contamination risks.
- Train/test separation if any learned component is introduced.
- Human review and disagreement resolution.
- Benchmark versioning and regression tracking.
- Performance and resource-consumption measurement.

Define formulas, evaluation procedures, minimum sample considerations, confidence intervals where appropriate, and acceptance thresholds that require explicit approval.

Specify how the 99% precision target is measured for defined check categories without implying universal correctness.

Output `docs/quality/ACCURACY_BENCHMARKS.md`.

### Prompt 30 — `docs/quality/RELEASE_ACCEPTANCE_CRITERIA.md`

Act as a release engineering lead. Define the mandatory criteria for releasing KubeCheck engine versions and knowledge bundles.

Cover:

- Required unit, integration, regression, and security test results.
- Benchmark precision and recall gates.
- Supported-version compatibility.
- Performance and memory regression limits.
- Offline operation and network-isolation verification.
- Dependency and vulnerability scanning.
- SBOM generation and license compliance.
- Reproducible build verification.
- Artifact checksums, signing, and provenance.
- Cross-platform binary verification.
- Krew and pip packaging checks.
- Knowledge-bundle compatibility and recovery tests.
- Release notes, known limitations, and migration guidance.
- Rollback, emergency release, and release-approval procedures.

Define explicit pass/fail criteria, evidence required for approval, and conditions that block publication.

Distinguish mandatory security gates from optional quality improvements. Never report a gate as passed without actual evidence.

Output `docs/quality/RELEASE_ACCEPTANCE_CRITERIA.md`.

---

## Group 8: CLI and distribution

### Prompt 31 — `docs/distribution/CLI_UX_AND_EXIT_CODES.md`

Act as a CLI product designer and developer-experience engineer. Define KubeCheck's complete command-line contract.

Cover:

- Command hierarchy and naming conventions.
- Help output and examples.
- Single-file, directory, recursive, stdin, and multi-document inputs.
- Target Kubernetes version selection.
- Rule selection, exclusions, configuration files, and severity thresholds.
- Offline mode and explicit update/import/export commands.
- Human-readable, JSON, and other justified output formats.
- Stable diagnostic IDs, file paths, document indexes, and line/column locations.
- Exit codes for clean results, findings, invalid input, operational errors, and incomplete analysis.
- CI integration and pipeline behavior.
- Color, terminal detection, machine-readable output, and localization considerations.
- Secret redaction, safe terminal output, and output-size limits.
- Configuration precedence and backward compatibility.
- Deterministic ordering and stable machine-readable schemas.

Provide concrete command examples, sample output, configuration examples, an exit-code table, and compatibility requirements.

Do not overload the CLI with flags that duplicate configuration without a clear precedence rule.

Output `docs/distribution/CLI_UX_AND_EXIT_CODES.md`.

### Prompt 32 — `docs/distribution/INSTALLATION_AND_RELEASES.md`

Act as a cross-platform release engineer. Specify production-grade packaging and releases for KubeCheck's three installation channels.

Cover:

- Krew plugin manifest requirements and executable naming.
- pip distribution, package metadata, dependencies, and entry points.
- Standalone binary targets and platform-specific behavior.
- A canonical engine and knowledge-bundle compatibility contract.
- Version numbering and release channels.
- SHA-256 checksums, artifact signatures, and build provenance.
- Reproducible builds and dependency pinning.
- SBOM generation and license compliance.
- Installation and upgrades on connected and isolated machines.
- Prepackaged offline installation artifacts.
- Knowledge-bundle installation and updates.
- Platform support matrix and end-of-support policy.
- Smoke tests, package parity, and release verification.
- Rollback and recovery from failed upgrades.
- Documentation and changelog requirements.

Explain any architectural differences required if the chosen implementation language does not naturally support all three channels.

Define exact release artifacts, naming conventions, metadata, and acceptance checks without inventing actual release URLs or claiming compatibility that has not been tested.

Output `docs/distribution/INSTALLATION_AND_RELEASES.md`.

---

# PART III — FINAL REPOSITORY VALIDATION PROMPT

After all 32 documents have been generated, use this separate prompt.

You are the principal technical reviewer responsible for the consistency and production readiness of the entire KubeCheck documentation repository.

Review all 32 documents together. Do not rewrite everything immediately. First produce an audit report that identifies missing requirements, contradictory decisions, duplicated responsibilities, unresolved assumptions, incorrect technical claims, and gaps between documentation and the proposed implementation.

Verify the following:

1. All 32 required files exist at the specified paths.
2. All referenced internal documents exist.
3. Every requirement has a unique, stable ID.
4. Every mandatory requirement maps to an acceptance test.
5. YAML syntax rules are consistent with the stated YAML specification.
6. Kubernetes schemas and compatibility claims are version-specific and traceable to authoritative sources.
7. Deterministic checks are separated from heuristics and runtime-dependent findings.
8. Offline operation does not depend on remote APIs, cloud services, or external inference.
9. Krew, pip, and binary releases share the same diagnostic contract.
10. Security requirements cover hostile inputs, dependencies, updates, and supply-chain threats.
11. Knowledge bundles have defined compatibility, integrity, provenance, and recovery mechanisms.
12. Accuracy targets have reproducible measurement procedures.
13. Resource limits and failure behavior are defined.
14. CLI outputs and exit codes are stable and documented.
15. No implementation, test result, benchmark, or security guarantee is falsely claimed as verified.

Produce:

- A document completeness matrix.
- A requirements traceability matrix.
- A cross-document consistency report.
- A list of unresolved decisions, ranked by impact.
- A list of unsupported technical claims requiring verification.
- A security and reliability gap report.
- A prioritized correction plan.

Only after presenting the audit findings should you correct the documents. Preserve existing valid decisions, avoid unnecessary rewrites, and rerun the consistency checks after changes.

The final result must be an auditable, internally consistent specification for implementing KubeCheck—not merely a large collection of documentation files.

# END OF PROMPT PACK