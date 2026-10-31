# KubeCheck — Test Strategy

**Document ID:** TEST-STRAT-001 · **Status:** Normative — Approved · **Version:** 1.0.0 · **Owner:** Principal Quality Engineering · **Last reviewed:** 2026-10-31

This document defines the complete testing strategy for KubeCheck. It specifies every category of test the project runs, the fixture organization that supports them, the naming conventions that make them findable, the coverage expectations that bound the work, the CI stages that gate merges and releases, the flaky-test policy that keeps the suite trustworthy, and the release-blocking failures that must be resolved before any artifact ships. The strategy exists because KubeCheck is a security-sensitive CLI: a defect in the YAML parser can execute code on the user's machine, a defect in the secret redaction can leak credentials into a CI log, a defect in the signature verifier can accept a forged knowledge bundle, and a defect in the diagnostic engine can produce a false negative that lets a defective manifest reach production. The tests in this document are the mechanism by which each of those defects is caught before it reaches a user, and the strategy is written to be read continuously rather than as a checklist, because a test strategy that is read as a checklist is a test strategy that gets filled in mechanically and then ignored.

The document opens with a governing statement that is easy to state and hard to live by: **code coverage alone cannot establish diagnostic accuracy**. A test suite can achieve 100% line coverage and 100% branch coverage on the diagnostic engine and still miss every real-world defect the engine is supposed to detect, because coverage measures whether the code executed, not whether the code executed correctly. The diagnostic engine's correctness is a property of the findings it produces on labeled manifests, and that property is measured by the accuracy benchmark (`ACC-REQ-005` through `ACC-REQ-013`), not by coverage. The strategy therefore includes coverage as one signal among many, and it makes the benchmark a required gate (`GATE-BUILD-007`) that coverage cannot substitute for. Everything else in the document — the unit tests, the integration tests, the fuzzing, the metamorphic tests, the security regressions, the offline-isolation tests, the parity tests — exists to provide the evidence that the benchmark alone cannot: that the engine behaves correctly on the specific inputs that the benchmark does not cover, that the parser is robust against adversarial input, that the CLI behaves consistently across channels, and that the tool's security guarantees hold on every run.

Every requirement in this document is identified with the prefix `TEST-STRAT-REQ-NNN`, every test category with `CAT-<NAME>`, every fixture convention with `FIX-<NAME>`, every CI stage with `CI-STAGE-<NAME>`, and every release-blocking failure with `RBF-NNN`. The document is organized around the test pyramid (unit → integration → end-to-end → contract), then the specialized test categories (property-based, metamorphic, fuzz, security regression, determinism, offline isolation, cross-platform, bundle compatibility, CLI behavior, parity, performance), then the operational concerns (fixture organization, naming, coverage, CI stages, flaky policy, release-blocking failures), and it closes with the argument for why the benchmark and not coverage is the accuracy gate.

## The test pyramid and the four test categories

The strategy organizes tests into four categories, each with a different cost, a different scope, and a different role in the evidence chain.

**Unit tests** (`CAT-UNIT`) test a single component in isolation: a parser rule, a normalization step, a schema-lookup function, a rule-evaluation primitive, a path-resolution function, a hash-computation function. Unit tests are fast (milliseconds), numerous (thousands), and precise (a failure names the exact component and the exact input). They are the first line of defense against regressions in individual functions, and they are the category with the highest coverage expectation (`TEST-STRAT-REQ-001` sets the target at 90% line coverage and 85% branch coverage for the core engine, with an exemption for the parts of the code that are exercised only by integration tests and are documented in the coverage report).

**Integration tests** (`CAT-INTEG`) test two or more components together: the parser and the normalizer, the normalizer and the schema validator, the schema validator and the rule engine, the rule engine and the aggregator, the aggregator and the reporter. Integration tests are slower (tens to hundreds of milliseconds), fewer (hundreds), and broader (a failure names a component interaction). They are the mechanism by which defects in the seams between components are caught, and they are the category that exercises the NIR, the source map, the inventory, and the relationship graph as they are actually used.

**End-to-end tests** (`CAT-E2E`) test the CLI as a user invokes it: `kubecheck analyze manifest.yaml`, with a specific bundle, a specific target version, and a specific expected output. E2E tests are the slowest (seconds), the fewest (dozens), and the most user-facing. They are the mechanism by which defects in the CLI's argument parsing, output formatting, exit-code computation, and configuration resolution are caught, and they are the category that the acceptance suite is built from.

**Contract tests** (`CAT-CONTRACT`) test the boundaries that the project has promised to external consumers: the JSON output schema (`docs/output/JSON_SCHEMA.md`), the exit-code contract (`docs/cli/EXIT_CODE_CONTRACT.md`), the bundle format (`docs/knowledge/KNOWLEDGE_BUNDLE_FORMAT.md`), the rule format (`docs/engine/DIAGNOSTIC_RULE_SPEC.md`), and the CLI command surface (`docs/cli/COMMAND_REFERENCE.md`). Contract tests are fast (they validate a schema against a produced document), numerous (one per contract field), and precise. They are the mechanism by which a change that would break a downstream consumer is caught before the consumer notices, and they are the category that enforces the compatibility policy (`COMP-REQ-001` through `COMP-REQ-008`).

`TEST-STRAT-REQ-002` requires that every component have at least one unit test, every component boundary have at least one integration test, every user-visible command have at least one E2E test, and every contract have at least one contract test. A component, boundary, command, or contract without a test is a gap in the strategy and must be recorded as `OPEN` in the coverage report.

## YAML parser edge cases and source-location accuracy

The YAML parser is the component with the highest security risk in the entire tool, and its tests are the most demanding. The parser tests (`CAT-YAML`) cover four areas: the edge-case corpus, the source-location accuracy suite, the adversarial tag corpus, and the resource-limit suite.

The **edge-case corpus** is the set of manifests that exercise every edge case from `YAML_PARSING_AND_EDGE_CASES.md` §15: the 32 edge cases (EC-001 through EC-032) each have a fixture, and each fixture asserts the exact error code, the source location, and the recovery policy. `TEST-STRAT-REQ-003` requires that every edge case have a positive fixture (the edge case is triggered and produces the expected result), a negative fixture (a similar but valid input does not trigger the edge case), and a boundary fixture (the edge case at the limit of the parser's behavior). The edge-case corpus is versioned alongside the parser and is run at every CI run.

The **source-location accuracy suite** asserts that every diagnostic produced by the parser resolves to the exact line, column, and byte offset in the original input. The suite is table-driven: for a set of manifests with known diagnostic locations, the suite asserts that the parser's reported location matches the known location for every diagnostic. `TEST-STRAT-REQ-004` requires the suite to cover the parser's location reporting for every node kind (scalar, mapping, sequence, alias) and for every error class (lexical, structural, semantic). A location that is off by one — a very common bug in hand-written parsers — is a failure in this suite even if the diagnostic message is otherwise correct, because a location that is wrong is a location that sends the user to the wrong line.

The **adversarial tag corpus** is the set of manifests that attempt to trigger arbitrary code execution through unsafe YAML tags: `!!python/object/apply`, `!!python/object/new`, `!!ruby/object`, `!!java/object`, `!!php/object`, and every other constructor-resolving tag the parser might recognize. `TEST-STRAT-REQ-005` requires the corpus to be executed at every CI run, and to assert that every tag is either rejected with a structured error or preserved as an opaque scalar, never resolved to a host-language object. The corpus includes the canary-file-write check: a manifest that attempts to write a canary file through a tag must not succeed, and the canary file must not exist after the test.

The **resource-limit suite** asserts that every parser limit (`LIM-REQ-001` through `LIM-REQ-008`) is enforced before the resource is allocated. `TEST-STRAT-REQ-006` requires the suite to include a bomb corpus at depths 3, 5, 8, and 10, a depth series at 255/256/257 levels, a scalar series at 9/10/11 MiB, a collection series at 99,999/100,000/100,001 entries, and a document series at 999/1,000/1,001 documents. Each fixture asserts the specific governor finding, the document status, and the recovery policy.

## Schema selection and version-specific validation

The schema validation tests (`CAT-SCHEMA`) cover three areas: the schema-selection tests, the version-specific validation tests, and the schema-coverage tests.

The **schema-selection tests** assert that the schema resolver selects the correct schema for a given GVK and target version. `TEST-STRAT-REQ-007` requires the tests to cover: a GVK present in the bundle for the target version (exact match), a GVK present for a different version (no match, `schema-unchecked`), a GVK that is a CRD with a supplied schema (match), a GVK that is a CRD without a supplied schema (`VER-013`), a GVK that is an aggregated API without a supplied schema (`VER-008`), and a GVK that is unknown (`VER-008`). Each fixture asserts the selected schema's provenance and the finding (if any).

The **version-specific validation tests** assert that a manifest that is valid for one target version and invalid for another is correctly classified for each version. `TEST-STRAT-REQ-008` requires the tests to cover: a field introduced in a later version (valid against the later version, `VER-030` against the earlier), a field removed in a later version (`VER-031` against the later, valid against the earlier), a required field added in a later version (`VER-033` against the later), a default changed between versions (`VER-034` against both), an enum value added in a later version (`VER-035` against the earlier), and a constraint tightened in a later version (`VER-036` against the later). Each fixture asserts the exact finding and the exact version-specific evidence.

The **schema-coverage tests** assert that every GVK in the bundle's `kubernetes_versions` list has a schema, that every schema's `provenance` reference resolves, and that every schema's declared scope matches the resource's actual scope. `TEST-STRAT-REQ-009` requires the coverage tests to be a gate (`GATE-BUILD-004`) that blocks the bundle if any GVK is uncovered.

## Diagnostic-rule positive and negative cases

The rule tests (`CAT-RULE`) are the largest category and the one that most directly affects the accuracy benchmark. Every rule must have positive, negative, and boundary fixtures (`RULE-SPEC-REQ-059`), and the fixtures must assert the exact finding — rule ID, category, severity, evidence class, source anchor, evidence fields — not just "a finding occurred" (`RULE-SPEC-REQ-060`).

`TEST-STRAT-REQ-010` requires every rule to have at least three positive fixtures (different manifestations of the same defect), at least three negative fixtures (different valid configurations that must not trigger the rule), and at least one boundary fixture (the rule's applicability boundary, e.g., the minimum or maximum target version, or a field value at the edge of the match). The strategy sets a higher bar than the rule spec's minimum for negative fixtures, because false positives are the failure mode that erodes user trust fastest.

The rule tests also include the **cross-rule isolation test**: for each rule, a fixture that would trigger a different rule but not the rule under test must produce only the other rule's finding. `TEST-STRAT-REQ-011` requires this test for every rule, and it is the mechanism by which a rule whose matching condition is too broad is caught: the rule under test must not trigger on the other rule's fixture.

The **rule-metadata consistency tests** assert that every rule's declared evidence class is compatible with its emitted findings (`RULE-SPEC-REQ-014`), that every rule's remediation text passes the safety scan (`RULE-SPEC-REQ-018` through `RULE-SPEC-REQ-019`), and that every rule's references are valid URLs at bundle-generation time (`RCA-REMED-REQ-049`). `TEST-STRAT-REQ-012` requires these tests at every CI run, and they are the mechanism by which a rule that violates the project's safety constraints is caught before it enters a bundle.

## Cross-resource graph correctness

The graph tests (`CAT-GRAPH`) cover five areas: node construction, edge construction, resolution results, cycle detection, and scalability.

The **node construction tests** assert that every admitted resource becomes a node with the correct `ResourceKey`, GVK, scope, and source anchor, and that a resource that fails GVK extraction does not become a node (`CROSS-RES-REQ-006`). The tests include a duplicate-key fixture that asserts the duplicate is annotated rather than silently merged (`CROSS-RES-REQ-021`).

The **edge construction tests** assert that every reference type produces the correct edge: `OwnerReference`, `WorkloadSelectorMatch`, `ServiceSelectorMatch`, `ConfigMapRef`, `SecretRef`, `ServiceAccountRef`, `PVCRef`, `StorageClassRef`, `RoleRef`, `ClusterRoleRef`, `SubjectRef`, `IngressBackendServiceRef`, `IngressTLSSecretRef`, `GatewayParentRef`, `GatewayBackendRef`, and the NetworkPolicy variants. `TEST-STRAT-REQ-013` requires a fixture for every edge type, and each fixture asserts the edge's `edge_type`, `from`, `to` (or `to_hint`), `source_anchor`, `optional`, and `scope`.

The **resolution-result tests** assert that the eight resolution kinds (`RESOLVED`, `RESOLVED_WITH_DUPLICATES`, `SELECTOR_MATCH`, `NO_MATCHING`, `UNRESOLVED`, `EXTERNAL`, `GENERATED`, `UNRESOLVABLE`) are each produced by the correct fixture. `TEST-STRAT-REQ-014` requires the tests to assert the specific resolution kind for every reference fixture, so that a change in resolution behavior is caught by the test that asserts the result.

The **cycle-detection tests** assert that an owner-reference cycle and a merge-key cycle each produce the correct finding (`XRES-OWNER-003`, `YAML-NORM-004`), and that the detection is deterministic across runs.

The **scalability tests** assert that the graph builder respects the `XRES-LIMIT-001` through `XRES-LIMIT-007` limits: a graph with more nodes than the limit is rejected, a node with more out-edges than the limit is rejected, and a traversal that exceeds the depth limit is rejected. `TEST-STRAT-REQ-015` requires the scalability tests to be run at every CI run, and to include a fixture that approaches the limit (the fixture is designed to complete within the time budget if the limit is respected and to time out if it is not).

## Property-based and metamorphic testing

Property-based and metamorphic tests (`CAT-PROPERTY`, `CAT-METAMORPHIC`) are the categories that catch defects the example-based tests miss. They are not a replacement for the example-based tests; they are a supplement.

**Property-based tests** (`CAT-PROPERTY`) generate random inputs and assert that a property holds for every input. `TEST-STRAT-REQ-016` requires at least the following properties: for every valid YAML document, parsing then re-serializing then re-parsing produces an equivalent NIR; for every invalid YAML document, the parser produces exactly one error and does not crash; for every manifest, the diagnostic set is a deterministic function of the manifest and the bundle (two runs produce the same set); for every reference in the graph, resolution is a deterministic function of the input set (two runs produce the same resolution). The property-based framework generates the random inputs, shrinks a failing input to a minimal reproduction, and records the reproduction as a regression fixture.

**Metamorphic tests** (`CAT-METAMORPHIC`) transform a manifest in a way that preserves the defect and assert that the finding persists, or transform it in a way that removes the defect and assert that the finding disappears. `TEST-STRAT-REQ-017` requires at least the following metamorphic properties: renaming a resource (name changes, defect persists) preserves every finding anchored to the resource's fields; reordering the documents in a stream preserves every finding; changing a comment or whitespace preserves every finding; changing a numeric value from `1` to `"1"` (a type change) preserves or changes the finding according to the schema (a type-mismatch finding appears); adding an unrelated valid document to the stream preserves every finding. The metamorphic tests are the mechanism by which a finding that accidentally depends on the manifest's formatting, order, or unrelated content is caught.

`TEST-STRAT-REQ-018` requires the property-based and metamorphic tests to be run at every CI run (for the properties that are fast enough) and at every release (for the properties that are slow). A metamorphic failure is a release-blocking defect, because it indicates that the tool's findings are not a deterministic function of the defect.

## Fuzzing, malformed inputs, and resource-exhaustion tests

The fuzz tests (`CAT-FUZZ`) are the mechanism by which the parser's robustness against adversarial input is established. `TEST-STRAT-REQ-019` requires the fuzz harness to be run continuously (nightly) and to run a PR-gating subset on every merge. The harness feeds random and mutated inputs to the parser, the normalizer, and the rule evaluator, and it asserts that none of them crashes, hangs, or exhausts memory beyond the configured limits.

The fuzz corpus is composed of three parts: the seed corpus (the edge-case corpus, the adversarial tag corpus, and the malformed-input corpus), the mutation engine (which generates new inputs by mutating the seed corpus), and the crash corpus (which accumulates every input that has caused a crash in the past). `TEST-STRAT-REQ-020` requires the crash corpus to be retained indefinitely and to be run at every fuzz run, so that a fixed crash cannot regress silently.

The **malformed-input corpus** is a curated set of inputs that are syntactically malformed in specific ways: truncated UTF-8, overlong UTF-8, surrogate halves, invalid escapes, unclosed quotes, duplicate keys, alias bombs, deep nesting, and oversized scalars. The corpus is executed as part of the fuzz run and as part of the CI edge-case suite. `TEST-STRAT-REQ-021` requires the corpus to be run at every CI run, and to assert that every input produces a structured finding and not a crash.

The **resource-exhaustion tests** assert that the parser and the graph builder respect the resource limits under adversarial input: a bomb that exceeds `max_aliases` is rejected before expansion, a graph that exceeds `max_edges` is rejected before construction, a document that exceeds `max_depth` is rejected at the depth limit. `TEST-STRAT-REQ-022` requires the exhaustion tests to be run at every CI run and to include a timing assertion (the rejection must complete within a documented time budget, not hang).

## Security regression testing

The security tests (`CAT-SEC`) are the tests that enforce the guarantees in the safe-execution policy and the threat model. They are the highest-priority tests in the suite, and a failure in any of them is a release-blocking defect (`RBF-001`).

`TEST-STRAT-REQ-023` requires the following security test categories to be run at every CI run:

The **secret-leakage tests** run the secret-bearing corpus at every log level and in every output format, and assert that no canary value appears in `stdout`, `stderr`, or any file the tool creates. The corpus includes `Secret.data`, `Secret.stringData`, sensitive-named fields, TLS key material, and sensitive annotations (`PRIVACY-REQ-018`). A leak in any channel is a release-blocking defect.

The **safe-loader tests** run the adversarial tag corpus and assert that no constructor-resolving tag is resolved to a host-language object (`SEC-YAML-REQ-003` through `SEC-YAML-REQ-006`).

The **shell-execution tests** run the manifest corpus with shell metacharacters in every field value and assert that no subprocess is created (`SEC-REQ-002`).

The **path-traversal tests** run the traversal corpus, the symlink corpus, and the zip-slip corpus, and assert that every escape attempt is rejected (`SEC-YAML-REQ-025` through `SEC-YAML-REQ-027`).

The **network-isolation tests** are specified in the offline tests section below.

The **signature-verification tests** run the signature negative-path suite: a tampered manifest, a tampered file, a tampered signature, a missing signature, an expired certificate, a revoked key, and an unknown key. Each fixture asserts that the bundle is rejected with the specific error and that the prior bundle remains active (`SEC-REQ-006` through `SEC-REQ-008`).

The **credential-file tests** set `KUBECONFIG` and the in-Pod ServiceAccount token path to canary files and assert that neither is read (`SAFE-EXEC-REQ-008`, `PRIVACY-REQ-023`).

`TEST-STRAT-REQ-024` requires every security test to be run on every supported platform, because a security defect that manifests only on Windows or macOS is still a security defect.

## Deterministic output and reproducibility

The determinism tests (`CAT-DETERMINISM`) are the mechanism by which the tool's reproducibility claim (`ACC-REQ-001`) is established. `TEST-STRAT-REQ-025` requires the determinism suite to run the same input through the tool at least ten times and assert that the finding set, the canonical ordering, and the rendered output are byte-identical across every run. The suite includes runs with concurrency level 1 and with the maximum configured concurrency, and asserts that the outputs are identical regardless of concurrency.

The determinism suite also runs the same input through the tool on every supported platform and asserts that the outputs are byte-identical (modulo the platform-specific error codes that are documented). `TEST-STRAT-REQ-026` requires the cross-platform determinism test to be run at every release, because a platform-specific difference in the output is a compatibility break that the user would experience as a bug.

The **reproducible-build tests** (`CAT-REPRODUCIBLE`) run the build pipeline twice on the same inputs and assert that the binary is byte-identical (modulo the documented non-deterministic metadata). `TEST-STRAT-REQ-027` requires the reproducible-build test to be run at every release and to be verified by a third party at least once per major release (`BUILD-REQ-018`).

## Offline network-isolation tests

The offline tests (`CAT-OFFLINE`) are the mechanism by which the tool's offline guarantee (`NET-REQ-001`) is established. `TEST-STRAT-REQ-028` requires the following isolation configurations to be exercised at every release:

The **network-namespace test** runs the full acceptance suite inside a Linux network namespace created with `unshare -n` and asserts that every test passes and that no outbound connection is attempted.

The **container-network-none test** runs the full acceptance suite in a container with `--network=none` and asserts the same.

The **macOS-isolation test** runs the full acceptance suite on macOS with Wi-Fi and Ethernet disabled and asserts the same.

The **Windows-isolation test** runs the full acceptance suite on Windows with all non-loopback network adapters disabled and asserts the same.

The **syscall-trace test** runs the `analyze` command under `strace -f -e trace=network` (Linux), `dtrace` (macOS), or the equivalent (Windows) and asserts that the trace contains no `connect`, `sendto`, `sendmsg`, `socket`, or DNS-related syscall.

The **static-analysis gate** rejects any use of network APIs in the core engine's packages (`NET-REQ-002`).

`TEST-STRAT-REQ-029` requires the network-isolation tests to be run on every supported platform at every release, and to be recorded in the release notes with a link to the test run artefact.

## Cross-platform and architecture testing

The cross-platform tests (`CAT-PLATFORM`) are the mechanism by which the tool's platform support (`PLAT-REQ-001` through `PLAT-REQ-003`) is established. `TEST-STRAT-REQ-030` requires the acceptance suite to be run on every combination of operating system (Linux, macOS, Windows) and architecture (amd64, arm64) that the project publishes binaries for.

The cross-platform suite includes the **path-handling tests**: paths with non-ASCII characters, paths with case differences, paths with Windows separators, paths with mixed separators, and paths that resolve to the same file under case-insensitive comparison. Each fixture asserts that the tool's behavior matches the documented platform behavior.

The cross-platform suite includes the **file-permission tests**: on POSIX, every file the tool creates has mode `0600` or `0700`; on Windows, the equivalent restrictive ACL. `TEST-STRAT-REQ-031` requires the permission tests to be run at every CI run on every platform, because a permission defect that manifests only on one platform is still a security defect.

## Knowledge-bundle compatibility and update recovery

The bundle tests (`CAT-BUNDLE`) are the mechanism by which the bundle format's guarantees (`KNOWLEDGE-REQ-001` through `KNOWLEDGE-REQ-080`) are established. `TEST-STRAT-REQ-032` requires the bundle tests to cover:

The **format-validation tests** assert that every bundle-validation stage (`KNOWLEDGE-REQ-060` through `KNOWLEDGE-REQ-064`) rejects a bundle that fails the stage, with the correct error and the correct exit code.

The **compatibility tests** assert that a bundle with an incompatible format version, an incompatible engine range, or an incompatible Kubernetes version coverage is rejected with the correct error.

The **rollback tests** assert that a bundle with a lower `bundle_version` than the active bundle is rejected (`UPDATE-SIGN-REQ-020`), that a legitimate rollback via `bundle rollback` succeeds (`UPDATE-SIGN-REQ-033`), and that a rollback attempt via the activation command is rejected.

The **revocation tests** assert that a revoked bundle is rejected with the correct error and that the prior bundle remains active (`UPDATE-SIGN-REQ-045`).

The **freshness tests** assert that a stale bundle produces a warning (`UPDATE-SIGN-REQ-025`) and that an expired bundle is rejected (`UPDATE-SIGN-REQ-027`).

The **recovery tests** assert that an interrupted activation is recovered automatically (`UPDATE-SIGN-REQ-037`), that a corrupt active bundle is recovered from `bundle.prev/` or `known-good/` (`UPDATE-SIGN-REQ-034`), and that the recovery is logged.

The **import/export tests** assert that a bundle exported from one machine and imported on another with no network access is identical to the original (`UPDATE-SIGN-REQ-039`).

## CLI exit codes and output formats

The CLI tests (`CAT-CLI`) are the mechanism by which the tool's contract with CI systems (`OUT-REQ-007` through `OUT-REQ-008`) is established. `TEST-STRAT-REQ-033` requires the CLI tests to cover:

The **exit-code matrix** asserts that every documented exit code is produced in the correct scenario: success, usage error, operational error, internal error, cancellation, and each finding-severity case. The matrix is a table of (scenario, expected exit code) and is run against every command.

The **JSON output tests** validate the output against the JSON schema (`docs/output/JSON_SCHEMA.md`), assert the canonical field ordering (`OUT-REQ-002`), and assert the presence of the required metadata fields (`OUT-REQ-005`).

The **human-readable output tests** assert that the output contains no terminal control sequences (`UX-REQ-003`), that severity and evidence class are text-tokenized (`UX-REQ-002`), and that the output is readable when `NO_COLOR` is set (`UX-REQ-001`).

The **help-text tests** assert that every command's `--help` output includes the documented sections (`UX-REQ-007`).

The **contract tests** assert that every documented CLI command, flag, and exit code exists and behaves as specified in `docs/cli/COMMAND_REFERENCE.md`.

## Installation and behavior parity across Krew, pip, and binaries

The parity tests (`CAT-PARITY`) are the mechanism by which the tool's cross-channel equivalence obligation (`DIST-REQ-001` through `DIST-REQ-003`) is established. `TEST-STRAT-REQ-034` requires the parity tests to:

Install the tool through each of the three channels (Krew, pip, standalone binary) on each supported platform, run the same manifest corpus through each installation, and assert that the normalized finding sets are identical across channels. The normalization excludes channel-specific metadata (installation path, version string format) but includes every finding, its evidence, and its source anchor.

Assert that each channel's `--version` output reports the same engine version and the same bundle ID.

Assert that each channel's behavior on every command is identical, including error messages, exit codes, and output formatting.

`TEST-STRAT-REQ-035` requires the parity tests to be run at every release and to be a release gate. A channel-specific divergence is a release-blocking defect (`RBF-002`).

## Performance, memory, and large-input benchmarks

The performance tests (`CAT-PERF`) are the mechanism by which the tool's performance targets (`PERF-REQ-001` through `PERF-REQ-014`) are measured. `TEST-STRAT-REQ-036` requires the performance tests to be run at every release and to record the results in the release notes.

The performance tests cover: cold-start latency for `kubecheck version`, warm-start latency, time-to-first-finding for `analyze`, single-file analysis latency for a manifest ≤ 1 MiB, directory analysis throughput for a corpus of manifests ≤ 50 KiB each, peak RSS for the benchmark corpus, and package sizes for each channel. The tests use the reference environment (`REF-A` from `REQUIREMENTS_AND_NFRS.md` §14.1) and the methodology in §14.2–§14.4.

The **large-input tests** cover the tool's behavior at and above the configured limits: a manifest at the maximum size, a manifest one byte above the maximum, a stream at the maximum document count, a stream one document above, a scalar at the maximum size, a scalar one byte above. `TEST-STRAT-REQ-037` requires the large-input tests to assert that the tool produces a structured governor finding, not a crash or a hang.

The **memory tests** assert that the tool's peak memory stays within the `PERF-REQ-007` target under the benchmark corpus and that it stays within the `LIM-REQ-006` limit under adversarial input. `TEST-STRAT-REQ-038` requires the memory tests to be run at every release, and to report the peak memory for every fixture.

## Fixture organization and test naming

The strategy specifies a consistent organization for fixtures and a consistent naming convention for tests, so that a test is findable from the requirement it validates and a fixture is findable from the test that uses it.

Fixtures are organized under `testdata/` with the following subdirectories: `testdata/manifests/` (the manifest corpus, organized by category), `testdata/bundles/` (the bundle fixtures, organized by bundle version), `testdata/rules/` (the rule fixtures, organized by rule family), `testdata/signatures/` (the signature test vectors), `testdata/benchmark/` (the benchmark corpus, versioned separately), and `testdata/security/` (the security corpora). `FIX-001` requires the fixtures to be checked into the repository, versioned alongside the code, and referenced by test name rather than by path, so that a fixture can be moved without breaking the test.

Tests are named according to the pattern `Test_<Component>_<Behavior>_<Case>`, where `<Component>` is the component under test, `<Behavior>` is the property or behavior being tested, and `<Case>` is the specific case (positive, negative, boundary, error). `FIX-002` requires the naming convention, and `FIX-003` requires every test to have a doc comment that names the requirement it validates (e.g., `// Validates YAML-SPEC-REQ-016.`). The doc comment is the mechanism by which the traceability matrix (`docs/testing/REQUIREMENT_TRACEABILITY.md`) is generated: the matrix is produced by scanning the test files for their requirement references, so that the matrix stays accurate as tests are added, renamed, or removed.

## Coverage expectations

`TEST-STRAT-REQ-039` sets the coverage targets: 90% line coverage and 85% branch coverage for the core engine, 95% line coverage for the parser and the signature verifier, and 80% line coverage for the CLI and the reporter. The targets are floors, not goals; a component that exceeds its floor is fine, and a component that falls below its floor is a gap that must be documented in the coverage report.

`TEST-STRAT-REQ-040` requires the coverage report to distinguish between *exercised* coverage (the line was executed) and *asserted* coverage (the line's behavior was verified by an assertion). A line that is executed but not asserted is a line whose correctness is not verified, and the coverage report must not present it as covered. The coverage tooling is configured to report both metrics, and the "asserted coverage" metric is the one the team uses to assess test quality.

`TEST-STRAT-REQ-041` requires the coverage report to be published with every release and to be reviewed at every major-version release. A reduction in coverage without a documented justification is a defect in the release.

The strategy restates the opening point as a formal requirement: `TEST-STRAT-REQ-042` — coverage alone cannot establish diagnostic accuracy. A suite that achieves 100% line coverage on the diagnostic engine but that has no fixtures for a given defect class will not detect that defect class. The accuracy of the diagnostic engine is established by the benchmark (`ACC-REQ-005` through `ACC-REQ-013`) and the fixtures (`RULE-SPEC-REQ-059` through `RULE-SPEC-REQ-063`), not by coverage. The coverage target is a floor for code health, not a ceiling for the accuracy evidence.

## CI stages

The CI pipeline has six stages (`CI-STAGE-LINT`, `CI-STAGE-UNIT`, `CI-STAGE-INTEG`, `CI-STAGE-E2E`, `CI-STAGE-SECURITY`, `CI-STAGE-RELEASE`). `TEST-STRAT-REQ-043` requires the stages to run in this order and to gate the next stage on the previous stage's success.

`CI-STAGE-LINT` runs the static analysis, the formatting check, the import-graph check, and the coverage check. A lint failure blocks the pipeline. `CI-STAGE-UNIT` runs the unit tests and the property-based tests on every supported platform. A unit failure blocks the pipeline. `CI-STAGE-INTEG` runs the integration tests, the graph tests, and the CLI tests. An integration failure blocks the pipeline. `CI-STAGE-E2E` runs the end-to-end tests and the parity tests on every supported platform. An E2E or parity failure blocks the pipeline. `CI-STAGE-SECURITY` runs the security tests, the fuzz subset, the offline-isolation tests, and the determinism tests. A security, fuzz, offline, or determinism failure blocks the release. `CI-STAGE-RELEASE` runs the full fuzz suite, the full benchmark, the full compatibility matrix, the bundle tests, and the signing tests. A release-stage failure blocks the release.

`TEST-STRAT-REQ-044` requires the CI stages to be documented in the repository's CI configuration and to be reviewed at every phase boundary. A stage that is added, removed, or reordered without a review is a defect in the CI configuration, because the gates the release depends on may have been accidentally weakened.

## Flaky-test policy

A flaky test is a test that produces different results on the same code and the same input. `TEST-STRAT-REQ-045` requires flaky tests to be treated as defects, not as noise. The policy has four rules.

The first rule is **detection**: a test that fails on a retry but passes on the original run is recorded as flaky in the CI's test results. The CI aggregates flakiness across runs and produces a flaky-test report. A test that has been flaky for more than five runs is escalated to the test's owner.

The second rule is **quarantine**: a flaky test is moved to a quarantine suite (`ci/quarantine/`) within one week of escalation, and the quarantine suite runs nightly but does not gate merges. The quarantined test is not deleted, because deletion would lose the evidence; it is marked as quarantined and the team continues to investigate it.

The third rule is **fix or delete**: a quarantined test is fixed within thirty days, or deleted with a documented rationale. A test that cannot be fixed (because it depends on external timing, for example) is deleted and replaced with a deterministic alternative. A test that is deleted without a replacement is a gap in the strategy and must be recorded in the coverage report.

The fourth rule is **release gate**: a release that has more than five quarantined tests is blocked until the quarantine is reduced, because a release with many quarantined tests has many unverified behaviors.

`TEST-STRAT-REQ-046` requires the flaky-test policy to be enforced by the CI, not by the team's discipline. A test that fails on retry is automatically recorded; a test that has been quarantined for more than thirty days is automatically escalated.

## Release-blocking failures

The strategy defines the failures that block a release regardless of any other consideration. `RBF-001` is a security test failure (any of `CAT-SEC`). `RBF-002` is a parity failure (any of `CAT-PARITY`). `RBF-003` is an offline-isolation failure (any of `CAT-OFFLINE`). `RBF-004` is a determinism failure (any of `CAT-DETERMINISM`). `RBF-005` is a bundle-verification failure (any of `CAT-BUNDLE` that rejects a valid bundle or accepts an invalid bundle). `RBF-006` is a crash or hang on any input (any of `CAT-FUZZ` or `CAT-YAML`). `RBF-007` is a schema-coverage failure (any GVK in the bundle lacks a schema, or any schema lacks a provenance reference). `RBF-008` is a benchmark failure (any deterministic category below the precision target, or any category whose metrics have regressed beyond the threshold). `RBF-009` is a reproducibility failure (the reproducible-build test does not produce byte-identical output). `RBF-010` is a coverage regression (any component below its floor without a documented justification). `RBF-011` is a flaky-test escalation (more than five quarantined tests at release). `RBF-012` is a release-metadata failure (any required field in the release metadata is missing or invalid).

`TEST-STRAT-REQ-047` requires every release-blocking failure to be resolved before the release is tagged. A release that ships with an open `RBF-NNN` is a release that has bypassed the strategy, and the release process must record the bypass and the reason.

## Why code coverage alone cannot establish diagnostic accuracy

The strategy closes with an extended argument that is worth stating in full, because the argument is the reason the benchmark is a gate and coverage is not.

Coverage measures execution: a line is covered if it was executed. Coverage does not measure whether the executed line did the right thing. A line that computes a finding's severity and always computes `warning` is fully covered by a test that checks that the line was executed, but the line's correctness — its behavior on the inputs that should produce `error`, `warning`, and `info` — is not verified by coverage. A test that asserts the line's behavior on every relevant input is what verifies correctness, and that test is a fixture, not a coverage metric.

Diagnostic accuracy is the property that the engine produces the correct finding for the correct input. The property is measured by comparing the engine's findings against a labelled ground truth: for every manifest in the benchmark corpus, the engine either produces the finding the ground truth says it should produce (a true positive), produces a finding the ground truth says it should not produce (a false positive), fails to produce a finding the ground truth says it should (a false negative), or correctly produces no finding (a true negative). The four counts, and the precision and recall derived from them, are the accuracy measurement. Coverage says nothing about these counts; it says only that the code ran.

A suite with 100% coverage and no benchmark could have 50% precision and 10% recall and would not know it, because coverage does not compare the engine's output against a labelled truth. A suite with 60% coverage and a well-designed benchmark would catch the precision and recall defects even though some code paths are never executed. The benchmark is the accuracy measurement; coverage is the code-health measurement. The strategy requires both, but it makes the benchmark a release gate and the coverage a documented target, because the release's safety depends on accuracy, not on coverage.

The corollary is that the benchmark corpus must be independent of the development fixtures (`ACC-REQ-012`). If the corpus is the same as the fixtures, the benchmark measures the engine's performance on the data it was tuned on, not its performance on data it has not seen. The strategy requires the benchmark corpus to be maintained separately, labelled independently, and versioned alongside the benchmark protocol, so that the accuracy measurement is a measurement of the engine's real-world behavior and not a measurement of its memorization of the fixtures.

## Open questions and unresolved decisions

Six open questions remain. `OPEN-TEST-01` asks whether the property-based tests will use a general-purpose framework (e.g., QuickCheck, Hypothesis, fast-check) or a project-specific generator; the current design uses a project-specific generator because the properties are manifest-specific, but a general-purpose framework could cover the parser's properties more thoroughly. `OPEN-TEST-02` asks whether the fuzz suite should be run with a coverage-guided fuzzer (which would explore the parser's code paths more effectively) or a mutation-based fuzzer (which is simpler to set up); the current design uses a mutation-based fuzzer, but a coverage-guided fuzzer is the direction if the fuzz budget increases. `OPEN-TEST-03` asks whether the benchmark corpus should be public or private; the current design makes it public to enable third-party evaluation, but a private corpus would reduce the risk of a rule being tuned to the corpus. `OPEN-TEST-04` asks whether the cross-platform tests should run on every PR or only on the main branch; the current design runs the Linux tests on every PR and the macOS and Windows tests on the main branch, but a full matrix on every PR would catch platform-specific defects earlier. `OPEN-TEST-05` asks whether the parity tests should include the Krew channel's installation-time behavior (which depends on Krew's own infrastructure) or only the post-installation behavior; the current design tests post-installation parity, but installation-time parity is a documented limitation. `OPEN-TEST-06` asks whether the reproducibility test should be run on every release or only on major releases; the current design runs it on every release, but the cost is significant and the trade-off may change.

## Definition of done

This document is complete when eleven conditions hold. The four test categories (unit, integration, E2E, contract) are defined with their scope and their coverage expectations. The YAML parser tests cover the edge-case corpus, the source-location accuracy suite, the adversarial tag corpus, and the resource-limit suite. The schema tests cover selection, version-specific validation, and coverage. The rule tests cover positive, negative, boundary, cross-rule isolation, and metadata consistency. The graph tests cover node construction, edge construction, resolution results, cycle detection, and scalability. The property-based and metamorphic tests are specified with the properties and the fixtures. The fuzz, malformed-input, and resource-exhaustion tests are specified with the corpus composition. The security regression tests are specified with the seven categories and the release-blocking status. The determinism and reproducibility tests are specified. The offline network-isolation tests are specified with the four isolation configurations. The cross-platform and architecture tests are specified. The bundle compatibility and update recovery tests are specified. The CLI exit-code and output-format tests are specified. The installation and parity tests are specified. The performance, memory, and large-input tests are specified. The fixture organization and test naming are specified. The coverage expectations are specified and the argument for why coverage cannot establish accuracy is stated. The CI stages are specified with the six stages. The flaky-test policy is specified with the four rules. The release-blocking failures are specified with twelve `RBF-NNN` identifiers. Every requirement has a stable ID (`TEST-STRAT-REQ-NNN`), every test category has a stable ID (`CAT-<NAME>`), every fixture convention has a stable ID (`FIX-<NAME>`), every CI stage has a stable ID (`CI-STAGE-<NAME>`), and every release-blocking failure has a stable ID (`RBF-NNN`). The document does not claim any implementation status, benchmark result, or coverage percentage; it specifies the tests, the gates, and the evidence each requires, and it leaves the implementation and the measurements to the project's CI and release processes.

The test strategy is not a document that can be satisfied by writing more tests. It is a document that defines what the tests must establish — that the parser is robust, the schema selection is correct, the rules are accurate, the graph is consistent, the properties hold, the security guarantees are enforced, the output is deterministic, the tool is offline, the platforms are supported, the bundles are compatible, the CLI is stable, the channels are equivalent, and the performance is bounded — and it specifies the evidence, the fixtures, the gates, and the policy by which each of those claims is verified. A test strategy that is measured by its test count is a test strategy that will grow without improving the tool; a test strategy that is measured by the claims it verifies is a test strategy that will focus on what matters. This document is the latter.