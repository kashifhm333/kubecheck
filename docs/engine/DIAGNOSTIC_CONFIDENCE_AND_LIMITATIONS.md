# KubeCheck — Diagnostic Confidence and Limitations

**Document ID:** CONF-LIMITS-001 · **Status:** Normative — Approved · **Version:** 1.0.0 · **Owner:** Quality and Reliability Engineering · **Last reviewed:** 2026-10-31

This document defines a transparent, reproducible framework for expressing how certain each KubeCheck finding is, and for stating clearly what the tool does not know. It exists because a diagnostic tool that presents every finding as equally certain is worse than useless — it teaches users to ignore the tool — and because a tool that invents confidence numbers without calibration is actively dishonest. The framework has three objectives. First, it must let a user reading a finding understand the exact evidence supporting it without needing to read the rule source code. Second, it must let a user predict when the tool is likely to be wrong, so that the user can decide whether to act or to investigate. Third, it must make the tool's limitations visible by design rather than only through disappointing experiences.

Every requirement is identified with the prefix `CONF-LIMITS-REQ-NNN`. The document covers the distinction between deterministic errors and heuristic findings, the evidence levels and what proof each requires, the confidence labels and their meaning, the distinction between severity, confidence, and probability, the calibration methodology if numeric probabilities are ever introduced, false-positive and false-negative tracking, the handling of unknown schemas, missing CRDs, and ambiguous target versions, cluster-state-dependent conditions, admission policies and external dependencies, suppression and baseline comparisons, diagnostic provenance, benchmark categories, and confidence regression tests. It ends with the rules for downgrading confidence or returning an explicit unknown result, and with the prohibition against inventing confidence percentages.

## The governing principles

Three principles govern this document, and every requirement derives from them.

The first principle, `CONF-LIMITS-REQ-001`, states that KubeCheck never invents confidence percentages. No finding carries a numeric confidence score, a probability, or a percentage unless the tool has evidence from an independently labelled evaluation supporting that specific number for that specific check category. The default presentation is a categorical evidence class — deterministic, probable, or context-dependent — with no numeric component. This is not a temporary constraint pending better calibration; it is the correct design, because a numeric confidence score implies a calibration claim that offline static analysis cannot make without an evaluation corpus that has been independently adjudicated.

The second principle, `CONF-LIMITS-REQ-002`, states that every finding must carry an explicit confidence label and an explicit confidence basis. The label is one of the categorical evidence classes; the basis is a short, human-readable statement of what evidence supports the finding. A finding without both is a defect in the rule, not a defect in the manifest, and the rule is rejected at load time.

The third principle, `CONF-LIMITS-REQ-003`, states that when the information required to make a confident claim is unavailable, the tool returns an explicit unknown result rather than a downgraded guess. A missing CRD schema produces a "cannot validate; schema not supplied" finding with an info severity, not a schema-invalid finding with a lowered confidence. A missing target version produces a "target version not supplied; using newest bundled" warning, not a validation result with a lowered confidence. The distinction between "the tool checked and the manifest is valid," "the tool checked and the manifest is invalid," and "the tool did not check because it lacked the input" is the single most important distinction the tool must preserve, and the third case is never collapsed into either of the first two.

These three principles interact to make the tool's behaviour predictable. A user who sees a deterministic finding knows the tool is confident; a user who sees a probable finding knows the tool is providing evidence-based guidance that requires their judgment; a user who sees an info finding knows the tool is providing context rather than a defect; and a user who sees an unknown result knows exactly what additional input would resolve the unknown.

## Deterministic errors versus heuristic findings

The distinction between deterministic errors and heuristic findings is the foundational classification and it appears throughout the rest of the documentation set. A deterministic error is a finding whose verdict depends only on the manifest and on immutable reference data such as the schema bundle, the lifecycle metadata, and the feature-gate metadata. A schema violation is deterministic: for a fixed target version, a given field either conforms to the schema or does not, and the result is reproducible. A duplicate mapping key is deterministic: the YAML specification requires uniqueness, and the tool either finds a duplicate or does not. A request that exceeds its limit is deterministic: the API server rejects it, and the tool can see the constraint.

A heuristic finding is a finding whose verdict depends on information not visible in the manifest alone. A missing readiness probe on a multi-replica Deployment is heuristic because the tool cannot know whether the specific workload needs a readiness probe — some workloads tolerate unready endpoints, some do not. A Service selector that matches no Pod in the analyzed input set is heuristic because the Pod may exist in the cluster and may simply not have been supplied to the tool. A `privileged: true` container is heuristic in the sense that the tool can see the field literally, but whether the field is a defect depends on the workload's role (a CNI plugin legitimately runs privileged; an application container usually does not).

`CONF-LIMITS-REQ-004` requires every rule to declare its evidence class explicitly in its metadata, and `CONF-LIMITS-REQ-005` requires the engine to enforce that the emitted finding's evidence class matches the rule's declared class. A rule that declares deterministic evidence cannot emit a context-dependent finding, and a rule that declares context-dependent evidence cannot emit a deterministic finding. This enforcement is what allows a user to trust the categorical label on a finding — the label reflects the rule's classification, not a post-hoc adjustment.

`CONF-LIMITS-REQ-006` requires every rule family to document, in its specification, which of its checks are deterministic and which are heuristic, with an explanation for each. This documentation is public and versioned alongside the rules, so that a user can audit the classification rather than take it on faith. The `STATIC_ANALYSIS_RULES.md` document is the canonical example of this requirement in practice.

## Evidence levels and the proof each requires

Within the categorical classification, evidence levels describe how much proof supports a specific finding. The levels are not the same as confidence classes — a deterministic rule can produce a finding with strong or weak evidence depending on the specific input — but they are systematically related.

The first evidence level is **specification-mandated**, and it requires that the finding be supported by a normative statement in a published specification. The YAML 1.2.2 specification mandates that mapping keys be unique; the Kubernetes API conventions mandate that a Deployment's `spec.selector` match its `spec.template.metadata.labels`; the OpenAPI schema for a given GVK mandates that a required field be present. A specification-mandated finding is the strongest possible and is always `deterministic`.

The second evidence level is **schema-declared**, and it requires that the finding be supported by a schema artifact that the tool has loaded. The schema declares that `replicas` is an `integer`; the schema declares that `spec.selector` is required; the schema declares that `imagePullPolicy` is one of three enums. A schema-declared finding is `deterministic` relative to the bundle but `provisional` in the sense that a future bundle may change the schema.

The third evidence level is **cross-resource-verified**, and it requires that the finding be supported by the presence of two or more resources in the analyzed input set. A StatefulSet `volumeClaimTemplate` with no matching mount is cross-resource-verified because both the StatefulSet and the mount are in the input set. A PV/PVC access-mode mismatch is cross-resource-verified because both objects are in the input set. A cross-resource-verified finding is `deterministic` relative to the input set but `incomplete` relative to the cluster, because the input set may not be complete.

The fourth evidence level is **best-practice-derived**, and it requires that the finding be supported by a documented recommendation that is not itself a specification requirement. Missing recommended labels, missing PodDisruptionBudgets on multi-replica workloads, and missing startup probes on slow-starting workloads are all best-practice-derived. A best-practice-derived finding is `probable` or `context-dependent` and requires the user's judgment.

The fifth evidence level is **heuristic-inferred**, and it requires that the finding be supported by a pattern that is correlated with a defect but not itself the defect. A field named `password` set to a literal value is correlated with a credential leak but not proof of one. A liveness probe that targets the same endpoint as a readiness probe is correlated with a misconfiguration but not proof of one. A heuristic-inferred finding is `context-dependent` and its confidence basis must state the correlation explicitly.

`CONF-LIMITS-REQ-007` requires every rule to declare its evidence level in its metadata, and `CONF-LIMITS-REQ-008` requires the engine to enforce the constraint that a finding's evidence level is compatible with the rule's evidence class. A specification-mandated finding must be `deterministic`; a best-practice-derived finding must be `probable` or `context-dependent`; a heuristic-inferred finding must be `context-dependent`.

## Confidence labels and their meaning

The tool uses exactly three confidence labels, and they have precise meanings that are documented to the user and enforced by the engine.

The label `deterministic` means the finding's verdict depends only on the manifest and immutable reference data. If the user re-runs the tool with the same input, the same target version, and the same bundle, the finding will appear again with the same evidence. The finding is a statement about the manifest, not a statement about the cluster, and the user can act on it with the expectation that the manifest has the described defect. When the finding is at `error` severity, the API server will reject the manifest when it is applied to a cluster whose schema matches the bundle's schema.

The label `probable` means the finding is supported by strong evidence but may be a false positive in a valid configuration. The finding is a statement about the manifest, but it depends on an assumption that the tool cannot verify — for example, the assumption that the workload does not have a legitimate reason for its current configuration. A `probable` finding at `warning` severity is a recommendation that the user should evaluate in context. A `probable` finding must never be `error` severity, because the evidence does not support the certainty that `error` implies.

The label `context-dependent` means the finding's relevance depends on cluster policy, workload intent, or organizational standards. The finding is a statement about the manifest that requires the user's judgment to determine whether it is a defect. A missing readiness probe may be a defect for a web service and not a defect for a batch job. A `privileged: true` container may be a defect for an application and not a defect for a CNI plugin. A context-dependent finding at `info` severity is contextual information; at `warning` severity it is a recommendation that the user should evaluate against their specific context.

`CONF-LIMITS-REQ-009` requires every finding to carry one of these three labels, and `CONF-LIMITS-REQ-010` requires every finding to carry a `confidence_basis` field that states, in one short sentence, what evidence supports the finding. The basis must be specific to the rule and to the input, not a generic statement.

`CONF-LIMITS-REQ-011` requires the tool to expose the three labels in the output as exact string tokens: `deterministic`, `probable`, and `context-dependent`. The labels are not `high`, `medium`, `low`, not `strong`, `weak`, and not `1`, `2`, `3`. The labels are the evidence classes, and the user reads them as evidence classes.

## The distinction between severity, confidence, and probability

The three concepts are distinct and must never be conflated. Severity describes the impact of the finding if it is a defect. A schema error is `error` severity because the API server will reject the manifest; a missing readiness probe is `warning` severity because the workload will function but may misbehave under load; a missing recommended label is `info` severity because the workload will function normally. Severity is a property of the defect, not of the evidence.

Confidence describes the strength of evidence that the finding is a defect at all. A `deterministic` finding is certain given the input and the bundle; a `probable` finding is likely but not certain; a `context-dependent` finding is a recommendation that requires the user's judgment. Confidence is a property of the evidence, not of the impact.

Probability, if it is ever used, describes the probability that the finding is a true positive in the user's specific context. Probability is not the same as confidence: a `probable` finding can have any probability from 0 to 1 depending on the base rate of the defect in the population, and the tool cannot compute that probability without a labelled evaluation specific to the user's environment.

`CONF-LIMITS-REQ-012` prohibits the tool from conflating severity, confidence, and probability in its output, in its documentation, and in any internal representation. A finding carries three separate fields: `severity`, `evidence_class`, and (only if calibration evidence exists) `probability`. The fields are not derivable from each other.

`CONF-LIMITS-REQ-013` prohibits the tool from using severity as a proxy for confidence or vice versa. A `warning`-severity finding does not imply `probable` confidence, and a `deterministic` finding does not imply `error` severity. A deterministic `info` finding is perfectly legitimate: the tool is certain about a fact whose impact is informational.

`CONF-LIMITS-REQ-014` prohibits the tool from presenting the categorical labels as ordinals with numerical interpretation. The labels are categories, not a scale. "Deterministic is higher than probable" is true in the sense that deterministic evidence is stronger, but the difference is not quantifiable, and the tool must not present it as if it were.

## Calibration methodology if numeric probabilities are introduced

The tool does not, and in the MVP will not, present numeric probabilities for any finding. If a future release introduces numeric probabilities, the following methodology must be followed, and it is stated here so that the commitment is unambiguous and cannot be quietly circumvented.

The first requirement, `CONF-LIMITS-REQ-015`, is that the probability must be based on an independently labelled evaluation. The evaluation corpus must be built independently of the rule's development fixtures, labelled by reviewers who do not see the tool's output, and adjudicated by a process that records disagreements and resolutions. The evaluation corpus and the labelling procedure are the subject of the accuracy benchmark protocol in `docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`, which must be extended to cover the probability calibration process.

The second requirement, `CONF-LIMITS-REQ-016`, is that the probability must be calibrated per check category, not per tool. A tool-wide probability is meaningless because different categories have wildly different base rates of true positives. A schema rule with a 99% true-positive rate and a missing-readiness-probe rule with a 60% true-positive rate cannot be combined into a single number without losing information the user needs.

The third requirement, `CONF-LIMITS-REQ-017`, is that the probability must be accompanied by a confidence interval that reflects the sample size. A probability of 0.95 from a sample of 100 findings has a 95% confidence interval of roughly [0.89, 0.98]; a probability of 0.95 from a sample of 10,000 has an interval of roughly [0.945, 0.955]. The user needs the interval to understand how much to trust the point estimate.

The fourth requirement, `CONF-LIMITS-REQ-018`, is that the probability must be published with the corpus, the labelling procedure, the sample size, and the methodology. A probability without these artifacts is an invented number, and the tool must not present it.

The fifth requirement, `CONF-LIMITS-REQ-019`, is that the probability must not be derived from the rule's own fixtures or from the tool's own benchmark. The probability must be derived from an independent evaluation that the tool's authors did not control, and the provenance must be documented.

The sixth requirement, `CONF-LIMITS-REQ-020`, is that the probability must be presented alongside the categorical evidence class, not in place of it. The user sees "deterministic; observed true-positive rate 0.998 (95% CI [0.995, 0.999], n=12,000, corpus version 2026.10)" — the categorical label is the primary statement, and the numeric probability is a supplementary statistic. The categorical label is what the tool claims; the probability is what the evaluation observed.

The seventh requirement, `CONF-LIMITS-REQ-021`, is that the probability must be versioned and can change between releases only when the evaluation corpus changes. A probability that drifts without a corresponding corpus change is a red flag, and the release process must catch it.

## False-positive and false-negative tracking

The tool tracks false positives and false negatives continuously, and the tracking informs the confidence framework without replacing it.

A false positive is a finding that the tool emits for a manifest that does not have the described defect. A false positive is discovered through the benchmark corpus (a finding is emitted for a corpus item whose ground truth says the defect is absent), through user reports (a user supplies a manifest the tool flags and explains why the flag is incorrect), or through the tool's own development fixtures (a fixture that should be negative triggers a finding). Every false positive is recorded with the rule ID, the corpus item, the reason the finding was incorrect, and the fix (or the documented exception).

A false negative is a defect present in a manifest that the tool does not flag. A false negative is discovered through the benchmark corpus (a corpus item whose ground truth says the defect is present but the tool does not emit the finding), through user reports (a user supplies a manifest that has a defect the tool misses), and through the rule's own review (a rule author identifies a case the rule does not handle). Every false negative is recorded with the rule ID, the corpus item, the manifest excerpt, and the fix or the documented non-support.

`CONF-LIMITS-REQ-022` requires the tool to publish observed false-positive rates per rule family on the benchmark corpus. The published rates are observed rates, not calibrated probabilities, and they are labelled as such. The published rates are updated with each benchmark run and are versioned alongside the corpus.

`CONF-LIMITS-REQ-023` requires the tool to publish observed false-negative rates per rule family on the benchmark corpus, with the same caveats. A rule with a high false-negative rate is still useful if the defects it does catch are important, but the user needs to know the rate in order to calibrate their trust.

`CONF-LIMITS-REQ-024` requires the tool to publish a per-rule record of known false-positive conditions. A false-positive condition is a specific manifest pattern that a rule is known to misclassify, and the record exists so that the user can check whether their manifest matches a known condition before they act on the finding. The record is updated whenever a false positive is fixed but the underlying pattern remains difficult to distinguish in general.

`CONF-LIMITS-REQ-025` requires the tool to publish a per-rule record of known unsupported cases. An unsupported case is a manifest pattern that the rule cannot handle at all, and the record exists so that the user knows the tool's coverage boundaries. A rule with a documented unsupported case is still useful; a rule whose unsupported cases are undocumented is misleading.

## Unknown schemas, missing CRDs, and ambiguous target versions

Three specific situations require the tool to return an explicit unknown rather than a downgraded finding, and the handling of each is specified in detail.

The first situation is an unknown schema, which arises when a GVK is not present in the bundle's schema index for the target version. The tool cannot validate the object against a schema it does not have. `CONF-LIMITS-REQ-026` requires the tool to emit an `XRES-REF-002`-style finding with severity `info` and evidence class `deterministic` stating that the schema for the GVK is not in the bundle, and to exclude the object from schema validation. The finding is not a schema-invalid finding with lowered confidence; it is a distinct finding stating that the tool did not perform schema validation for this object. The user can resolve the unknown by supplying a bundle that contains the GVK, by supplying a CRD definition, or by accepting that schema validation is not available for this object.

The second situation is a missing CRD, which arises when a custom resource's GVK is present in the bundle's CRD catalog but the CRD's schema is not supplied. `CONF-LIMITS-REQ-027` requires the tool to emit a `VER-013` finding with severity `info` and evidence class `deterministic` stating that the CRD schema is not supplied, and to exclude the object from CRD-specific validation. The user can resolve the unknown by supplying the CRD definition via the appropriate input mechanism.

The third situation is an ambiguous target version, which arises when the user has not supplied `--kubernetes-version` and the tool has no authoritative evidence of the target. `CONF-LIMITS-REQ-028` requires the tool to use the newest minor version in the active bundle as the schema-selection version, to emit a `VER-006` finding with severity `warning` and evidence class `deterministic` stating that the target version was not supplied, and to include the selected version in the report header. The finding is not a schema-invalid finding with lowered confidence; it is a distinct finding that informs the user that the analysis was performed against a default target. The user can resolve the unknown by supplying `--kubernetes-version`.

In all three situations, `CONF-LIMITS-REQ-029` requires the tool to present the situation as an explicit unknown rather than as a valid or invalid result. The output says "cannot validate; schema not supplied" or "cannot validate; target version defaulted" or "cannot validate; CRD schema not supplied" — never "schema-invalid" with a lowered confidence and never "schema-valid" with a lowered confidence.

`CONF-LIMITS-REQ-030` requires the tool to record unknown results in the report's per-document status, so that a CI consumer can distinguish "the tool checked and the manifest is valid," "the tool checked and the manifest is invalid," and "the tool did not check because it lacked the input." The `document_status` field described in `EDGE-REQ-042` includes the values `processed`, `parse-failed`, `normalization-failed`, `gvk-failed`, `resource-limited`, and `skipped`, and the schema-unchecked case is a distinct value `schema-unchecked` that must appear for every document where schema validation was not performed.

## Cluster-state-dependent conditions

Many findings depend on cluster state that the tool cannot observe. The most important categories are cluster-scoped resources (StorageClasses, IngressClasses, RuntimeClasses, PriorityClasses, CSIDrivers), object existence (whether a referenced Service, Secret, ConfigMap, or PVC exists in the cluster), object state (whether a Pod is Ready, whether a PVC is Bound, whether a Deployment has available replicas), and admission outcomes (whether an admission webhook would accept or reject the object).

`CONF-LIMITS-REQ-031` requires every finding that depends on cluster state to be classified as `context-dependent` or as a distinct `cluster-dependent` result, and to state explicitly what cluster state would be required to resolve the dependency. A Service selector that matches no supplied Pod is a `context-dependent` finding stating that the selector matches no Pod in the analyzed input set. A reference to a StorageClass that is not in the input set is a `context-dependent` finding stating that the StorageClass existence in the cluster is required to complete the check. The `info` severity is the default for these findings, and `warning` severity is used only when the tool has evidence suggesting a defect.

`CONF-LIMITS-REQ-032` requires the tool to distinguish "the referenced object is not in the input set" from "the referenced object is invalid." A Secret that is not in the input set produces an `info` finding; a Secret that is in the input set but missing a required key produces an `error` finding. The two situations are different and the tool must not collapse them.

`CONF-LIMITS-REQ-033` requires the tool to publish a catalog of every cluster-state-dependent check, with an explanation of what cluster state would be required to resolve the dependency. The catalog is part of the user-facing documentation, and it lets a user understand in advance which of the tool's findings are deterministic and which are not.

## Admission policies, controllers, and external dependencies

Admission policies, admission webhooks, controller behavior, and external dependencies are outside the tool's offline analysis scope, and findings that depend on them must be handled with the same discipline as cluster-state-dependent findings.

`CONF-LIMITS-REQ-034` requires the tool to state explicitly, in every finding that depends on admission behavior, that admission is outside scope. A finding about a Pod Security Standards violation does not claim that the Pod will be rejected; it claims that the Pod's manifest fields are inconsistent with a named profile and that the actual admission outcome depends on the cluster's PSA configuration.

`CONF-LIMITS-REQ-035` requires the tool to state explicitly, in every finding that depends on controller behavior, that the controller's reconciliation is outside scope. A finding about a StatefulSet's `volumeClaimTemplates` does not claim that the PVCs will be created; it claims that the template's structure is valid or invalid and that the StatefulSet controller's behavior is a cluster concern.

`CONF-LIMITS-REQ-036` requires the tool to state explicitly, in every finding that depends on an external dependency (a registry, a DNS server, a CNI plugin, a CSI driver, an admission webhook), that the dependency's availability and behavior are outside scope. A finding about an image reference does not claim that the image exists; a finding about a NetworkPolicy does not claim that the CNI enforces it; a finding about a CSI volume does not claim that the driver supports the requested mode.

The common thread is that the tool must be explicit about what it does not claim. A user who understands that admission, controller, and external dependencies are out of scope will not misinterpret a `warning`-severity finding as a "the cluster will reject this" finding, and the tool's credibility is preserved.

## Suppression, exceptions, and baseline comparisons

Suppression is the mechanism by which a user tells the tool to stop emitting a finding for a specific resource or a specific rule. Suppression is legitimate — some findings are irrelevant in some contexts, and forcing users to fix a false positive they have already investigated is worse than letting them suppress it — but suppression must be visible, versioned, and auditable.

`CONF-LIMITS-REQ-037` requires every suppression to be declared in the manifest (via an annotation) or in a configuration file that the tool reads explicitly. The suppression must be visible in the finding's suppressed state so that a user reviewing the output can see what was suppressed and why. The suppression must not be a hidden global setting.

`CONF-LIMITS-REQ-038` requires the tool to distinguish "suppressed finding" from "finding that does not apply." A finding that does not apply (because the rule's applicability filter excludes it) is not suppressed; it is simply not evaluated. A finding that applies and is then suppressed is evaluated and then suppressed. The distinction matters because a rule with many suppressed findings is a rule that users are actively fighting, which is a signal that the rule's evidence class or severity may be miscalibrated.

`CONF-LIMITS-REQ-039` requires the tool to publish the count of suppressed findings per rule in the report's metadata. The count is not itself a finding, but it is useful context: a user seeing that a rule has been suppressed 500 times in the current analysis can investigate whether the rule is appropriate for their environment.

`CONF-LIMITS-REQ-040` requires the tool to enforce that suppressions cannot suppress `error`-severity deterministic findings. An `error`-severity deterministic finding corresponds to an API-server rejection or a specification violation, and suppressing it does not change the fact that the manifest is invalid. The user can choose not to act on the finding, but the tool must still emit it.

`CONF-LIMITS-REQ-041` requires the tool to support baseline comparison as a distinct feature from suppression. A baseline is a previously generated finding set that the user has reviewed and accepted; new findings are those that appear in the current run but not in the baseline. The tool does not "forget" the baseline findings; it distinguishes them so that the user sees what changed. The baseline comparison is not suppression — the baseline findings are still present and visible, they are just marked as baseline.

## Diagnostic provenance and user-facing limitations

Diagnostic provenance is the set of metadata that accompanies a finding and allows the user to understand its origin and its constraints. `CONF-LIMITS-REQ-042` requires every finding to carry the following provenance fields.

The `rule_id` identifies the rule that produced the finding. The `rule_version` identifies the version of the rule. The `bundle_id` and `bundle_schema_version` identify the knowledge bundle that was active during the analysis. The `engine_version` identifies the tool binary. The `target_version` identifies the Kubernetes target version that was used for schema selection. The `evidence_class` and `evidence_level` identify the evidence classification. The `confidence_basis` is the human-readable evidence statement. The `source_anchor` identifies the exact location in the manifest. The `resource_identity` identifies the resource. The `limitations` field lists the categories of limitation that apply to the finding — cluster-state-dependent, admission-dependent, controller-dependent, external-dependency-dependent, unknown-schema, missing-crd, ambiguous-target.

`CONF-LIMITS-REQ-043` requires the tool to publish a user-facing limitations document that describes, in non-technical language, what the tool does not do. The document covers: the tool does not connect to a cluster; the tool does not verify image contents; the tool does not verify admission outcomes; the tool does not simulate controller behavior; the tool does not verify external dependencies; the tool does not verify CRD semantics for CRDs whose schema is not supplied; the tool does not resolve ambiguities in the target Kubernetes version. The document is part of the tool's documentation, is linked from the CLI help, and is included in the report header so that every report reminds the user of the boundaries.

`CONF-LIMITS-REQ-044` requires the tool to repeat the limitations in the report header and in the `--help` output. Repetition is intentional: users read the report more often than they read the documentation, and a limitation that is only in the documentation is a limitation that will be forgotten.

## Benchmark categories and confidence regression tests

The benchmark corpus is organised into categories, and each category has a specific role in calibrating confidence and detecting regressions.

The first category is the `deterministic-schema` category, containing manifests that are valid or invalid against the schema for the target version. The category is used to measure schema-rule precision and recall, and to detect regressions when the schema bundle or the schema validator changes.

The second category is the `deterministic-lifecycle` category, containing manifests that use GA, Beta, Alpha, Deprecated, or Removed GVKs. The category is used to measure lifecycle-rule precision and recall, and to detect regressions when the lifecycle metadata or the lifecycle engine changes.

The third category is the `deterministic-cross-resource` category, containing manifests with cross-resource references that are resolvable within the input set. The category is used to measure cross-resource-rule precision and recall, and to detect regressions when the relationship graph or the cross-resource rules change.

The fourth category is the `probable-best-practice` category, containing manifests that either do or do not trigger best-practice recommendations. The category is used to measure the observed false-positive rate for probable findings, and to detect regressions that would increase the false-positive rate.

The fifth category is the `context-dependent` category, containing manifests that either do or do not trigger context-dependent findings. The category is used to measure the observed false-positive rate for context-dependent findings, and to detect regressions.

The sixth category is the `false-positive-adversarial` category, containing manifests that are deliberately similar to trigger conditions but that are valid, designed to catch rules whose matching conditions are too broad.

The seventh category is the `false-negative-adversarial` category, containing manifests with subtle defects that a careless rule would miss, designed to catch rules whose matching conditions are too narrow.

The eighth category is the `regression` category, containing manifests from every past benchmark run that caused a regression when first introduced. The category grows over time and is never pruned.

`CONF-LIMITS-REQ-045` requires the benchmark to be run for every release, and the results to be published with the release. The published results include per-category precision, recall, false-positive rate, and (for deterministic categories only) observed true-positive rate with confidence intervals.

`CONF-LIMITS-REQ-046` requires a **confidence regression test** to be run for every release. The regression test compares the current release's per-category confidence statistics (precision, recall, false-positive rate) to the previous release's, and fails the release if any category's precision has dropped by more than a documented threshold (for deterministic categories) or if any category's false-positive rate has increased by more than a documented threshold (for probable and context-dependent categories). The thresholds are published in the benchmark protocol.

`CONF-LIMITS-REQ-047` requires the tool to publish, per release, a "confidence changelog" that lists every rule whose evidence class, severity, confidence basis, or observed false-positive rate changed. The changelog lets users understand which findings they should re-evaluate after an upgrade.

## Downgrading confidence and returning explicit unknowns

The rules for downgrading confidence are narrow and specific. The general principle is that confidence is not a continuous quantity to be adjusted; it is a categorical classification that is set by the rule's design and can only be changed by an explicit decision, not by an automatic adjustment.

`CONF-LIMITS-REQ-048` permits confidence to be downgraded in exactly three situations. The first is when the required context is partially missing: a rule that is normally `deterministic` may be downgraded to `probable` when the target version is unknown and the schema selection is defaulted, because the rule's verdict now depends on an assumption. The second is when the analyzed input set is known to be incomplete: a cross-resource rule that is normally `deterministic` may be downgraded to `probable` when the referenced object is not in the input set, because the rule's verdict now depends on the object's absence from the input. The third is when a documented false-positive condition applies: a rule that is normally `deterministic` may be downgraded to `probable` when the manifest matches a known false-positive condition, because the rule's verdict is now dependent on an ambiguity the tool cannot resolve.

`CONF-LIMITS-REQ-049` prohibits confidence from being upgraded by an automatic adjustment. A `probable` rule does not become `deterministic` because the manifest matches the rule strongly; a `context-dependent` rule does not become `probable` because the tool is confident. Upgrading confidence requires an explicit change to the rule's metadata, a benchmark validation, and a release, and it is not a runtime adjustment.

`CONF-LIMITS-REQ-050` requires the tool to return an explicit unknown result when the information required to make a confident claim is entirely unavailable. The specific conditions are: the schema for a GVK is not in the bundle (return `schema-unchecked` with an `info` finding); the CRD schema for a custom resource is not supplied (return `crd-schema-unchecked` with an `info` finding); the target Kubernetes version is not supplied and no authoritative evidence exists (return the analysis against the newest bundled version with a `VER-006` warning); the cluster state required to resolve a reference is not observable offline (return `context-dependent` with an `info` finding stating what cluster state would be required); the admission policy or controller behavior required to resolve a finding is not observable offline (return `context-dependent` with an `info` finding stating what would be required).

`CONF-LIMITS-REQ-051` requires the explicit-unknown result to be distinguishable in the output from a valid result and from an invalid result. The `document_status` field and the finding's `evidence_class` and `evidence_level` fields jointly encode the distinction, and the report header explains the encoding.

`CONF-LIMITS-REQ-052` prohibits the tool from returning a downgraded guess in place of an explicit unknown. A finding that would be `deterministic` if the schema were available does not become a `probable` finding with an imagined schema; it becomes a `schema-unchecked` result. A finding that would be `deterministic` if the cluster state were observable does not become a `probable` finding with an imagined cluster; it becomes a `context-dependent` finding with an explicit cluster-state dependency.

## The prohibition against invented confidence percentages

The most important requirement in this document is `CONF-LIMITS-REQ-053`, which is the prohibition against invented confidence percentages. The tool must not present, in any output format, any numeric confidence score, probability, or percentage for a finding unless the calibration methodology in the section on numeric probabilities has been satisfied for that specific rule and that specific evaluation corpus. The categorical evidence class is the only confidence representation the tool uses in the MVP, and the MVP has no numeric probabilities for any rule.

`CONF-LIMITS-REQ-054` requires the tool to reject any rule that attempts to declare a numeric confidence in its metadata, in its evidence basis, or in its remediation text. A rule whose confidence basis is "95% confident" is rejected at load time with an `ENGINE-REMED-UNSAFE`-style finding, because the number is uncalibrated and the rule's authors do not have the evaluation to support it.

`CONF-LIMITS-REQ-055` requires the tool to reject any output field that would present a numeric confidence. The finding schema in `docs/output/JSON_SCHEMA.md` does not have a `confidence_score` or `probability` field, and any attempt to add one without the calibration methodology is a schema violation.

`CONF-LIMITS-REQ-056` requires the tool's documentation, its help text, and its website to avoid any claim of "high confidence," "99% accurate," or similar phrasing for any category unless the claim is backed by a published evaluation with the sample size and the confidence interval. The default documentation describes the categorical labels and their meanings, not numeric claims.

The prohibition is not a temporary state pending calibration. It is the correct permanent design for a tool whose claims must remain honest as its knowledge evolves. The categorical labels are sufficient to communicate confidence, and the numeric labels, if ever added, must be backed by evidence that the tool has not yet generated.

## Open questions and definition of done

Six open questions remain unresolved, and they are recorded here so that future contributors can resolve them without re-litigating the design.

The first question, `OPEN-CONF-01`, is whether the benchmark corpus for confidence calibration will be maintained separately from the accuracy benchmark corpus or whether the two will be unified. The current design assumes they will be unified, but the exact governance is open.

The second question, `OPEN-CONF-02`, is whether the tool will support user-supplied calibration data (e.g., a user's own labelled corpus) in addition to the tool's published corpus. The current design does not support this, but it may be added if there is demand.

The third question, `OPEN-CONF-03`, is whether the tool will support per-environment calibration, in which the user's specific cluster configuration is used to refine the observed true-positive rate. The current design assumes no, because the tool has no way to observe the user's cluster.

The fourth question, `OPEN-CONF-04`, is whether the `limitations` field in the finding schema will be a fixed enumeration or an extensible list. The current design uses a fixed enumeration, but extensibility may be needed as new limitation categories are discovered.

The fifth question, `OPEN-CONF-05`, is whether the confidence regression test thresholds will be uniform across categories or category-specific. The current design uses category-specific thresholds, but the exact values are open pending the first benchmark run.

The sixth question, `OPEN-CONF-06`, is whether the tool will ever support numeric probabilities for any category, and if so, whether the calibration methodology in this document is sufficient or whether it needs to be extended with additional statistical requirements.

The document is complete when nine conditions hold. First, deterministic errors and heuristic findings are distinguished with an explicit evidence-level taxonomy. Second, confidence labels and their meanings are specified precisely, with the three labels used consistently. Third, severity, confidence, and probability are distinguished, and the tool is prohibited from conflating them. Fourth, the calibration methodology for numeric probabilities is specified as a commitment that constrains any future numeric presentation. Fifth, false-positive and false-negative tracking is specified with per-rule records and published rates. Sixth, unknown schemas, missing CRDs, and ambiguous target versions are handled with explicit unknown results rather than downgraded guesses. Seventh, cluster-state-dependent conditions, admission policies, controller behavior, and external dependencies are each handled with explicit boundary statements. Eighth, suppression, exceptions, and baseline comparisons are specified with visibility and auditability requirements. Ninth, diagnostic provenance and user-facing limitations are specified with a mandatory limitations document and repeated reporting. The benchmark categories, the confidence regression tests, the downgrade rules, the explicit-unknown rules, and the prohibition against invented confidence percentages are the operative requirements that implement these conditions.

The document is self-contained, uses stable requirement identifiers, and is written to be read continuously rather than as a reference manual. It does not claim any implementation status, benchmark result, or verified detection rate, and it does not promise that any finding will be more accurate than the categorical evidence class it carries.