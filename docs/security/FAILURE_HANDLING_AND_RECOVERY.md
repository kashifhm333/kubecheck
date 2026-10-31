# KubeCheck — Failure Handling and Recovery

**Document ID:** FAIL-RECOV-001 · **Status:** Normative — Approved · **Version:** 1.0.0 · **Owner:** Reliability Engineering · **Last reviewed:** 2026-10-31

This document specifies exactly what KubeCheck does when any component fails. It is the counterpart to the safe-execution policy: that document defines what the tool must never do, and this document defines what the tool must do when something goes wrong anyway. Failures are inevitable — a manifest will contain bytes the parser cannot interpret, a bundle will be truncated by a failed download, a disk will fill up mid-write, a CI runner will be cancelled mid-run — and the difference between a tool that handles failures well and a tool that handles them badly is not whether the failures happen but how the tool responds. The governing principle of this entire document is stated once and must be read before any requirement in it: **KubeCheck never silently skips a critical security check and never labels incomplete analysis as complete.** A user who sees a report must be able to trust that every finding the tool would have emitted for the analyzed input is present, that every check the tool claims to have performed actually ran, and that every check it could not perform is explicitly reported as not performed. The failure handling in this document exists to preserve that trust.

Every requirement in this document is identified with the prefix `FAIL-RECOV-REQ-NNN`, and every failure mode is identified with a stable ID of the form `FAIL-<FAMILY>-NNN` where the family is one of `INPUT`, `SCHEMA`, `RULE`, `BUNDLE`, `STORAGE`, `OUTPUT`, `RESOURCE`, `PLATFORM`, `PARTIAL`, `RECOVERY`, or `ROLLBACK`. Every failure mode has a corresponding test identified with `FAIL-RECOV-TEST-NNN`. The document closes with a failure-mode matrix that consolidates the detection method, user-visible message, exit code, partial-result policy, recovery action, and test for every failure in one table, so that a reader can look up any specific failure without reading the whole document.

## The three-way distinction that governs every failure

Before describing individual failures, the document establishes the three-way distinction that determines how every failure is handled. A failure is classified as **fatal**, **partial**, or **recoverable**, and the classification determines the exit code, the partial-result policy, and the recovery action.

A **fatal** failure means the tool cannot produce a trustworthy result for the invocation as a whole. Examples include a missing active bundle (the tool has no knowledge to analyze with), an invalid configuration file (the tool cannot determine its own behavior), and a bundle signature verification failure (the tool cannot trust its own inputs). A fatal failure causes the tool to exit with a non-zero exit code and to emit a structured error describing the failure and the recovery action. No findings are emitted, because a partial result would be misleading — the user cannot tell whether the absence of findings reflects the manifest's quality or the tool's inability to analyze it.

A **partial** failure means the tool can produce a trustworthy result for some of the input but not all of it. Examples include a parse failure in one document of a multi-document stream (the other documents can still be analyzed), an unsupported GVK in one document (the schema validation for that document cannot be performed, but the other documents can be fully analyzed), and a resource limit exceeded in one input unit (that unit is rejected, but the other units continue). A partial failure causes the tool to emit findings for the completed portions, to emit a structured status for each document indicating whether it was fully processed, and to exit with an exit code that reflects the highest-severity issue among the findings and the failures. The report header and the per-document status are the mechanism by which the user knows which portions are complete and which are not.

A **recoverable** failure means the tool can complete the operation by retrying or by falling back to a safe alternative. Examples include a transient write error that succeeds on retry, a broken pipe on `stdout` that the tool handles by stopping output cleanly, and a cancellation that the tool handles by emitting the findings collected so far and exiting. A recoverable failure does not cause the tool to report an incomplete analysis, because the operation completes; it may cause the tool to log a warning to `stderr`, but the exit code reflects the analysis result.

`FAIL-RECOV-REQ-001` requires that every failure be classified as one of these three categories, and `FAIL-RECOV-REQ-002` requires that the classification be derivable from the failure itself and not from a user configuration. A failure that the tool classifies as recoverable when it is actually fatal is the exact failure mode this document exists to prevent — the user would see a "success" exit code and a clean report and would not know that the analysis was incomplete.

## Invalid input, parser crashes, unsupported syntax, and timeouts

The first family of failures is the input-parsing family, and it is the most common in practice because manifests are the primary input and manifests are frequently malformed.

`FAIL-INPUT-001` covers **invalid UTF-8 byte sequences**. When the decoder encounters a byte sequence that is not valid UTF-8, it cannot reliably decode the remainder of the stream, so the failure is **partial** at the stream level: documents parsed before the invalid byte are analyzed, and the invalid sequence produces a parse failure for the affected document and all subsequent documents in the stream. The detection is at the decoder stage; the user-visible message is "invalid UTF-8 byte sequence at byte offset N; documents after this offset were not analyzed"; the exit code is non-zero (the specific code is defined in `docs/cli/EXIT_CODE_CONTRACT.md`); the partial-result policy is that the completed documents produce findings and the incomplete documents are marked with a non-`processed` status; the recovery action is for the user to correct the encoding or supply a UTF-8 file; and the test is `FAIL-RECOV-TEST-001`.

`FAIL-INPUT-002` covers **syntax errors** in YAML (tabs in indentation, missing colons, unclosed quotes, invalid escapes, malformed block scalars, unexpected tokens). The failure is **partial**: the affected document is rejected, other documents in the stream continue. Detection is at the parser stage; the message names the error code and the location (line, column); the exit code reflects the error severity; the partial-result policy marks the affected document `parse-failed`; the recovery action is described by the error message; the test is `FAIL-RECOV-TEST-002`.

`FAIL-INPUT-003` covers **duplicate mapping keys and unsafe tags**, which are detected during parsing and cause the affected document to be rejected as `parse-failed`. The detection and recovery are the same as `FAIL-INPUT-002`; the test is `FAIL-RECOV-TEST-003`.

`FAIL-INPUT-004` covers **unsupported YAML constructs**, such as a construct that the specification permits but the parser's safe-mode configuration rejects. The failure is **partial** and the document is marked `parse-failed` with a message that names the construct and the reason it was rejected. The test is `FAIL-RECOV-TEST-004`.

`FAIL-INPUT-005` covers **parser crashes**, which are panics or aborts inside the parser. The parser is invoked behind a crash barrier (`SEC-YAML-REQ-017`), and the barrier converts the crash into a structured finding that names the document index and the error class. The failure is **partial**: the affected document is marked `parse-failed`, other documents continue, and the crash barrier records the incident so that the crash can be triaged. A parser crash that bypasses the crash barrier and terminates the process without a structured finding is a release-blocking defect (`BLK-REQ-002`), and the test `FAIL-RECOV-TEST-005` verifies that the barrier catches the crash.

`FAIL-INPUT-006` covers **timeouts** during parsing. If the wall-clock limit (`LIM-REQ-005`) is reached during parsing, the parser is interrupted, the affected document is marked `resource-limited`, and the invocation continues with the remaining documents. If the aggregate wall-clock limit is reached, the invocation stops admitting new documents and reports the findings collected so far. The test is `FAIL-RECOV-TEST-006`.

`FAIL-INPUT-007` covers **oversized input**, which is detected by the resource governor before parsing begins. The affected document is rejected with a `resource-limited` status, and the invocation continues. The test is `FAIL-RECOV-TEST-007`.

The principle that ties the input family together is that no input failure ever causes a silent skip. A document that cannot be analyzed is marked with a non-`processed` status, and the report header states that some documents were not analyzed. A user who sees a report from a manifest set with a parse failure sees both the findings for the parsed documents and the explicit statement that some documents were not analyzed. The alternative — silently skipping the malformed document and reporting a clean run — is precisely what `FAIL-RECOV-REQ-001` and `FAIL-RECOV-REQ-002` exist to prevent.

## Missing, corrupt, incompatible, or partially readable schemas

The second family of failures is the schema family, and it arises from the knowledge bundle that provides the schemas KubeCheck uses for validation.

`FAIL-SCHEMA-001` covers a **schema that is not present for a GVK**. This is not a failure of the tool; it is a coverage gap in the bundle. The tool emits an `info`-severity finding stating that schema validation cannot be performed for the GVK, marks the document `schema-unchecked`, and continues. The failure is **partial** at the document level and the partial-result policy is that the schema-unchecked documents are excluded from schema validation but included in the other checks (semantic, security, operational) where those checks do not require a schema. The test is `FAIL-RECOV-TEST-008`.

`FAIL-SCHEMA-002` covers a **corrupt schema** in the bundle. If the schema for a GVK fails to load or validate against its own manifest, the tool treats the GVK as uncovered (same as `FAIL-SCHEMA-001`) and emits a `warning`-severity finding that names the corrupt schema. The failure is **partial**. The test is `FAIL-RECOV-TEST-009`.

`FAIL-SCHEMA-003` covers an **incompatible schema version**, which occurs when a bundle's schema version is outside the engine's supported range. This is a **fatal** failure for the invocation because the tool cannot trust the bundle's schemas. The tool rejects the bundle at load time, names the incompatibility in the error message, and exits without producing findings. The user's recovery action is to install a compatible bundle. The test is `FAIL-RECOV-TEST-010`.

`FAIL-SCHEMA-004` covers a **partially readable bundle**, which occurs when some schemas load and others do not. The tool loads the schemas that are valid, marks the affected GVKs as `schema-unchecked` with a `warning`-severity finding for each, and continues. The failure is **partial**. The test is `FAIL-RECOV-TEST-011`.

The principle that ties the schema family together is that a schema failure never causes a schema-unchecked document to be reported as schema-valid. The user must always be able to distinguish "the tool checked and the document is valid," "the tool checked and the document is invalid," and "the tool did not check because the schema was not available." The `document_status` field and the `schema-unchecked` finding are the mechanisms by which this distinction is preserved.

## Invalid rules and failed rule evaluation

The third family of failures is the rule family, and it arises from the rule packs that define what KubeCheck looks for.

`FAIL-RULE-001` covers an **invalid rule** in a rule pack. The rule engine validates every rule against the rule schema before activation (`RULE-SPEC-REQ-064`); an invalid rule is disabled and produces a single `ENGINE-RULE-INVALID` finding that names the rule and the validation failure. The other rules in the pack continue to execute. The failure is **partial**. If the rule is declared `required: true` in the pack manifest, the failure becomes **fatal** and the bundle is rejected. The test is `FAIL-RECOV-TEST-012`.

`FAIL-RULE-002` covers a **rule evaluation error**, which occurs when a rule's expression throws during evaluation (a path that does not resolve, a type mismatch, a step-limit exceedance). The rule is skipped for the affected object, the engine emits an `ENGINE-EXPR-ERROR` finding naming the rule and the object, and the other rules continue. The failure is **partial** at the object level. The test is `FAIL-RECOV-TEST-013`.

`FAIL-RULE-003` covers a **rule that exceeds its evaluation limits** (step limit, deadline, regex complexity). The rule is skipped for the affected object, an `ENGINE-EXPR-LIMIT` finding is emitted, and the invocation continues. The failure is **partial** at the object level. The test is `FAIL-RECOV-TEST-014`.

`FAIL-RULE-004` covers a **rule that emits an invalid finding** (a finding whose evidence class contradicts the rule's metadata, a finding whose evidence binding does not resolve, a finding whose remediation violates the safety constraints). The engine rejects the finding, emits an `ENGINE-FINDING-INVALID` finding naming the rule and the constraint violated, and continues. The failure is **partial** at the object level, and if the rule's metadata itself is invalid, the failure escalates to `FAIL-RULE-001`. The test is `FAIL-RECOV-TEST-015`.

The principle that ties the rule family together is that a rule failure never suppresses the findings of other rules. A single broken rule does not prevent the tool from finding every other defect in the manifest. The user sees the broken rule's error and the other rules' findings, so they know both what was checked and what could not be checked.

## Knowledge-bundle checksum or signature failures

The fourth family of failures is the bundle-integrity family, and it is where the tool's fail-closed behavior is most important because a bad bundle is worse than no bundle.

`FAIL-BUNDLE-001` covers a **checksum mismatch** in the bundle. The bundle loader computes the content hash of every file and compares it against the manifest; a mismatch causes the bundle to be rejected with a **fatal** error naming the file and the expected and actual hashes. The user's recovery action is to re-download the bundle from the official source. The test is `FAIL-RECOV-TEST-016`.

`FAIL-BUNDLE-002` covers a **signature verification failure**, which occurs when the bundle's signature does not verify against the local trust anchors. The bundle is rejected with a **fatal** error naming the signer and the verification failure. The prior bundle (if any) remains active; the user's recovery action is to verify that the bundle came from the official source and, if it did, to report the failure. The test is `FAIL-RECOV-TEST-017`.

`FAIL-BUNDLE-003` covers a **missing signature**, which is the case where the bundle has no signature file. The bundle is rejected with a **fatal** error. The test is `FAIL-RECOV-TEST-018`.

`FAIL-BUNDLE-004` covers a **bundle that is older than the active bundle** (a rollback attempt). The bundle is rejected with a **fatal** error naming the active bundle's sequence number and the attempted bundle's sequence number. The user's recovery action is to use the active bundle or a newer one. The test is `FAIL-RECOV-TEST-019`.

`FAIL-BUNDLE-005` covers a **bundle that has expired**. The loader emits a `warning`-severity finding stating that the bundle is past its expiry date and may be missing recent security findings. The analysis proceeds; the failure is **recoverable** because the user can continue with the stale bundle knowingly. The test is `FAIL-RECOV-TEST-020`.

The principle that ties the bundle family together is the fail-closed behavior: a bundle that fails verification never becomes active, and the prior bundle (if one exists) remains in place. The user is never asked to choose between "a bundle that might be malicious" and "no analysis"; the tool always chooses the prior verified bundle or refuses to run.

## Interrupted updates and storage errors

The fifth family of failures is the storage family, and it arises from the tool's own write operations during bundle management.

`FAIL-STORAGE-001` covers an **interrupted bundle activation**, which occurs when the process is killed between writing the new bundle and committing the activation marker. The activation is transactional: the new bundle is written to a temporary directory, verified, and then atomically renamed into place; if the process is killed before the rename, the previous bundle remains active. On the next run, the loader finds the temporary directory and cleans it up. The failure is **recoverable**: the previous bundle remains active, and the user can retry the activation. The test is `FAIL-RECOV-TEST-021`.

`FAIL-STORAGE-002` covers a **write failure** during bundle activation (disk full, permission denied, I/O error). The activation is aborted, the previous bundle remains active, and the error is reported with the specific cause. The failure is **recoverable**. The test is `FAIL-RECOV-TEST-022`.

`FAIL-STORAGE-003` covers a **corrupt active bundle at load time** (a bundle that was valid at activation but has since been corrupted on disk). The loader detects the corruption, refuses to load the bundle, and reports the corruption with the recovery procedure. The failure is **fatal** for the invocation, because the tool has no trustworthy bundle to analyze with. The user's recovery action is to run `kubecheck bundle verify` to confirm the corruption and then to activate a known-good bundle. The test is `FAIL-RECOV-TEST-023`.

`FAIL-STORAGE-004` covers a **partially readable bundle**, where some files are readable and others are not. The loader loads the readable files, rejects the unreadable ones, and emits a `warning`-severity finding for each. The failure is **partial**. The test is `FAIL-RECOV-TEST-024`.

The principle that ties the storage family together is that the last-known-good bundle is always preserved. A failure during activation never destroys the active bundle, a failure during load never silently uses a corrupted bundle, and a failure during verification never promotes an unverified bundle.

## Output write failures, broken pipes, and cancellation

The sixth family of failures is the output family, and it arises from the tool's interaction with its consumers.

`FAIL-OUTPUT-001` covers a **write failure on `stdout`** (the consumer closed the pipe, the disk is full, the consumer is not reading). The tool treats `SIGPIPE` and `EPIPE` as normal termination: it stops writing, logs a warning to `stderr`, and exits with the exit code that reflects the analysis result. A write failure that is not a broken pipe (a disk-full error on a redirected `stdout`) is reported as a **fatal** error because the report was not produced. The test is `FAIL-RECOV-TEST-025`.

`FAIL-OUTPUT-002` covers a **write failure on `stderr`**. The tool cannot reliably report the failure because the reporting channel is broken, so it exits with the exit code that reflects the analysis result and, if the process is on a terminal, the terminal will show the broken pipe. The failure is **recoverable**: the exit code is still produced. The test is `FAIL-RECOV-TEST-026`.

`FAIL-OUTPUT-003` covers a **cancellation** (SIGINT or SIGTERM). The tool stops the pipeline, emits the findings collected so far, and exits with the documented cancellation exit code. The partial-result policy is that the report header states that the invocation was cancelled and that the findings are partial. The test is `FAIL-RECOV-TEST-027`.

`FAIL-OUTPUT-004` covers a **JSON serialization failure**, which occurs if a finding contains a value that cannot be serialized. This is an internal error (the schema is supposed to prevent it), and it produces a **fatal** error naming the offending finding. The test is `FAIL-RECOV-TEST-028`.

The principle that ties the output family together is that the exit code always reflects the truth of the invocation. A cancelled invocation is never reported as a successful analysis; a write failure is never reported as a successful write; a broken pipe is not reported as an error because the analysis completed.

## Memory pressure and resource-limit violations

The seventh family of failures is the resource family, and it arises from the tool's own resource consumption.

`FAIL-RESOURCE-001` covers a **resource-limit violation** (an input exceeds the size, depth, alias, document, or scalar limits). The affected input unit is rejected, the document is marked `resource-limited`, and a structured governor finding is emitted. The failure is **partial** at the unit level and becomes **fatal** only if the aggregate memory or wall-clock limit is reached. The test is `FAIL-RECOV-TEST-029`.

`FAIL-RESOURCE-002` covers **memory pressure** (the process approaches its `max_memory_mb` limit). The governor samples memory at a documented interval and terminates the invocation cleanly when the limit is reached, emitting a `ENGINE-OUTPUT-LIMIT`-style finding that states the limit and the amount consumed. The failure is **fatal** for the invocation but the findings collected so far are emitted. The test is `FAIL-RECOV-TEST-030`.

`FAIL-RESOURCE-003` covers a **wall-clock limit violation** (the invocation exceeds `max_time_ms`). The governor terminates the invocation cleanly, emits the findings collected so far, and reports the limit and the elapsed time. The failure is **fatal** for the invocation. The test is `FAIL-RECOV-TEST-031`.

The principle that ties the resource family together is that the tool terminates cleanly under resource pressure rather than being killed by the OS. A clean termination produces a structured finding and a meaningful exit code; an OS kill produces an unhelpful crash. The resource limits are the mechanism that ensures the tool terminates cleanly.

## Unsupported platforms and dependency failures

The eighth family of failures is the platform family, and it arises from the tool running in an environment it did not expect.

`FAIL-PLATFORM-001` covers an **unsupported operating system or architecture**. The tool detects the platform at startup; if the platform is not in the supported matrix (`PLAT-REQ-001` through `PLAT-REQ-003`), the tool refuses to start with a **fatal** error naming the platform and the supported matrix. The test is `FAIL-RECOV-TEST-032`.

`FAIL-PLATFORM-002` covers a **missing runtime dependency** (a shared library, a locale, a minimum kernel version). The tool detects the missing dependency at startup or at the first use; if the dependency is required for safe operation, the tool refuses to proceed with a **fatal** error naming the dependency and the version required. The test is `FAIL-RECOV-TEST-033`.

`FAIL-PLATFORM-003` covers a **filesystem that does not support the required operations** (POSIX permissions, atomic rename, exclusive create). The tool detects the missing capability at startup and either falls back to a safe alternative (if one exists) or refuses to proceed with a **fatal** error. The test is `FAIL-RECOV-TEST-034`.

The principle that ties the platform family together is that the tool refuses to run in an environment where it cannot guarantee its safety guarantees. A tool that runs on an unsupported platform and silently violates its guarantees is worse than a tool that refuses to start.

## Partial analysis and independent document failures

The ninth family is the partial-analysis family, and it describes the semantics of partial results across the whole pipeline.

`FAIL-PARTIAL-001` covers **independent document failures**, where one or more documents in a multi-document stream fail while others succeed. Each document is processed independently; the failed documents are marked `parse-failed` or `schema-unchecked` or `resource-limited` depending on the failure, and the successful documents are fully analyzed. The report header states the counts: total documents, documents processed, documents with failures. The exit code reflects the highest-severity issue among the findings and the failures. The test is `FAIL-RECOV-TEST-035`.

`FAIL-PARTIAL-002` covers **independent rule failures**, where one rule fails for one object but succeeds for other objects and other rules succeed for the failing object. The rule failure produces an `ENGINE-*` finding for the affected object, and the other findings are unaffected. The test is `FAIL-RECOV-TEST-036`.

`FAIL-PARTIAL-003` covers **independent graph failures**, where the relationship graph cannot be built for one namespace or one resource kind. The affected resources are excluded from cross-resource analysis, a structured finding is emitted for each, and the rest of the analysis proceeds. The test is `FAIL-RECOV-TEST-037`.

The principle that ties the partial family together is the same as the principle of the whole document: an incomplete analysis is never labeled as complete. The user sees the partial status, the specific failures, and the specific findings, and the exit code reflects the true state of the invocation.

## Recovery from interrupted bundle replacement

The tenth family is the recovery family, and it describes how the tool recovers from an interrupted bundle replacement.

`FAIL-RECOVERY-001` covers **recovery from an interrupted activation**. The bundle store uses a transactional pattern: the new bundle is written to `bundle.new/`, verified, and then atomically renamed to `bundle/`; the previous bundle is renamed to `bundle.prev/` before the new bundle is promoted. If the process is interrupted at any point, the next run detects the incomplete state and restores the last-known-good state. The recovery is automatic and does not require user intervention, though the user is warned that a recovery occurred. The test is `FAIL-RECOV-TEST-038`.

`FAIL-RECOVERY-002` covers **recovery from a corrupted active bundle**. If the active bundle is found to be corrupt at load time, the loader checks for `bundle.prev/` and, if it exists and verifies, promotes it back to `bundle/`. The user is warned that the corruption was detected and the previous bundle was restored. If `bundle.prev/` does not exist or also fails verification, the tool reports a **fatal** error with the recovery procedure (re-download a known-good bundle). The test is `FAIL-RECOV-TEST-039`.

`FAIL-RECOVERY-003` covers **recovery from a failed verification during activation**. If a new bundle fails verification, the new bundle is deleted, the previous bundle remains active, and the user is notified with the specific verification failure. The test is `FAIL-RECOV-TEST-040`.

The principle that ties the recovery family together is that recovery is automatic where it can be, and explicit where it cannot. The user is never left in a state where the tool silently uses a bad bundle or silently refuses to run; the tool either recovers to a known-good state or reports the specific failure with the specific recovery action.

## Safe rollback and preservation of last-known-good data

The eleventh family is the rollback family, and it describes how the tool preserves and restores the last-known-good data.

`FAIL-ROLLBACK-001` covers **rollback of a bundle activation** that the user has determined to be unsuitable. The `bundle activate --rollback` command restores `bundle.prev/` to `bundle/` after verifying it, so the user can return to a prior known-good bundle without network access and without re-downloading. The rollback is transactional and reports the restored bundle ID. The test is `FAIL-RECOV-TEST-041`.

`FAIL-ROLLBACK-002` covers **rollback after a bundle rollback attack** (an attacker attempts to activate an older bundle that is still signed but is known to be vulnerable). The loader rejects the rollback attempt with a **fatal** error naming the active bundle's sequence number and the attempted bundle's sequence number, and the active bundle remains in place. The test is `FAIL-RECOV-TEST-042`.

`FAIL-ROLLBACK-003` covers **preservation of last-known-good data**. The bundle store retains the previous bundle as `bundle.prev/` for at least one activation cycle, so that rollback is always possible. If the previous bundle is deleted by the user or by the retention policy, the tool reports the absence and refuses to roll back. The test is `FAIL-RECOV-TEST-043`.

The principle that ties the rollback family together is that the user can always return to a known-good state. Rollback is not a fallback; it is a first-class operation with its own guarantees and its own tests. A tool that cannot roll back is a tool that a user will not upgrade, and a tool that a user will not upgrade is a tool that cannot deliver security fixes.

## The failure-mode matrix

The failure-mode matrix consolidates the eleven families in one table. The columns are: the failure ID, the failure name, the detection method, the user-visible message, the exit code, the partial-result policy, the recovery action, and the test.

| ID | Failure | Detection | User-visible message | Exit code | Partial-result policy | Recovery action | Test |
|---|---|---|---|---|---|---|---|
| FAIL-INPUT-001 | Invalid UTF-8 | Decoder byte-offset check | "invalid UTF-8 byte sequence at byte offset N; documents after this offset were not analyzed" | Non-zero | Completed docs analyzed; incomplete marked `parse-failed` | Correct the encoding or supply UTF-8 | FAIL-RECOV-TEST-001 |
| FAIL-INPUT-002 | YAML syntax error | Parser | Error code + location | Non-zero | Doc marked `parse-failed`; others continue | Fix the manifest | FAIL-RECOV-TEST-002 |
| FAIL-INPUT-003 | Duplicate key / unsafe tag | Parser | Error code + location + both occurrences | Non-zero | Doc marked `parse-failed` | Fix the manifest | FAIL-RECOV-TEST-003 |
| FAIL-INPUT-004 | Unsupported construct | Parser | Construct name + reason | Non-zero | Doc marked `parse-failed` | Remove or rewrite the construct | FAIL-RECOV-TEST-004 |
| FAIL-INPUT-005 | Parser crash | Crash barrier | Document index + error class | Non-zero | Doc marked `parse-failed`; crash triaged | Re-run; report the crash | FAIL-RECOV-TEST-005 |
| FAIL-INPUT-006 | Parse timeout | Governor deadline | "wall-clock limit reached during parsing" | Non-zero | Doc marked `resource-limited` | Increase limit or reduce input | FAIL-RECOV-TEST-006 |
| FAIL-INPUT-007 | Oversized input | Governor pre-check | "input exceeds max_input_bytes" | Non-zero | Doc marked `resource-limited` | Reduce input or increase limit | FAIL-RECOV-TEST-007 |
| FAIL-SCHEMA-001 | Schema not present | Bundle index lookup | "schema for GVK not in bundle" | Non-zero | Doc marked `schema-unchecked`; other checks proceed | Supply a bundle or CRD | FAIL-RECOV-TEST-008 |
| FAIL-SCHEMA-002 | Corrupt schema | Schema validation on load | "schema for GVK failed validation" | Non-zero | Doc marked `schema-unchecked` | Re-download the bundle | FAIL-RECOV-TEST-009 |
| FAIL-SCHEMA-003 | Incompatible schema version | Bundle loader | "bundle schema version N not supported by engine" | Non-zero | Fatal; no findings emitted | Install a compatible bundle | FAIL-RECOV-TEST-010 |
| FAIL-SCHEMA-004 | Partially readable bundle | Per-file load check | "N of M schemas loaded; failures listed" | Non-zero | Affected GVKs marked `schema-unchecked` | Re-download the bundle | FAIL-RECOV-TEST-011 |
| FAIL-RULE-001 | Invalid rule | Rule validation on load | "rule X invalid: reason" | Non-zero | Other rules continue; rule disabled | Fix or replace the rule pack | FAIL-RECOV-TEST-012 |
| FAIL-RULE-002 | Rule evaluation error | Expression interpreter | "rule X failed on object Y: reason" | Non-zero | Other rules continue | Fix the rule | FAIL-RECOV-TEST-013 |
| FAIL-RULE-003 | Rule limit exceedance | Expression interpreter | "rule X exceeded its step/deadline limit on object Y" | Non-zero | Other rules continue | Fix the rule | FAIL-RECOV-TEST-014 |
| FAIL-RULE-004 | Invalid finding emitted | Finding validator | "rule X emitted an invalid finding: constraint" | Non-zero | Other findings continue | Fix the rule | FAIL-RECOV-TEST-015 |
| FAIL-BUNDLE-001 | Checksum mismatch | Content hash check | "checksum mismatch for file F: expected E, got A" | Non-zero | Fatal; no findings emitted | Re-download the bundle | FAIL-RECOV-TEST-016 |
| FAIL-BUNDLE-002 | Signature failure | Signature verifier | "signature verification failed for signer S" | Non-zero | Fatal; prior bundle retained | Verify source; report the failure | FAIL-RECOV-TEST-017 |
| FAIL-BUNDLE-003 | Missing signature | Signature verifier | "bundle has no signature" | Non-zero | Fatal | Use a signed bundle | FAIL-RECOV-TEST-018 |
| FAIL-BUNDLE-004 | Rollback attempt | Sequence number check | "bundle sequence N < active sequence M" | Non-zero | Fatal; active bundle retained | Use the active bundle or newer | FAIL-RECOV-TEST-019 |
| FAIL-BUNDLE-005 | Bundle expired | Expiry check | "bundle expired on DATE; may be missing recent findings" | Analysis exit code | Analysis proceeds | Update the bundle | FAIL-RECOV-TEST-020 |
| FAIL-STORAGE-001 | Interrupted activation | Transaction marker check | "previous activation was interrupted; previous bundle retained" | Analysis exit code | Analysis proceeds | Retry the activation | FAIL-RECOV-TEST-021 |
| FAIL-STORAGE-002 | Write failure | I/O error | "activation failed: reason" | Non-zero | Previous bundle retained | Free space / fix permissions | FAIL-RECOV-TEST-022 |
| FAIL-STORAGE-003 | Corrupt active bundle | Bundle load check | "active bundle corrupt: reason; recovery procedure follows" | Non-zero | Fatal | Run `bundle verify`; activate a known-good bundle | FAIL-RECOV-TEST-023 |
| FAIL-STORAGE-004 | Partially readable bundle | Per-file load check | "N of M files loaded" | Non-zero | Loaded files used; failures reported | Re-download the bundle | FAIL-RECOV-TEST-024 |
| FAIL-OUTPUT-001 | stdout write failure | I/O error | (Broken pipe: silent; disk full: "write to stdout failed: reason") | Non-zero for disk full; analysis code for broken pipe | Analysis result preserved | Redirect or free space | FAIL-RECOV-TEST-025 |
| FAIL-OUTPUT-002 | stderr write failure | I/O error | (Silent; terminal shows broken pipe) | Analysis exit code | Analysis result preserved | Fix stderr | FAIL-RECOV-TEST-026 |
| FAIL-OUTPUT-003 | Cancellation | SIGINT/SIGTERM handler | "invocation cancelled; findings are partial" | Cancellation code | Partial findings emitted; header states cancellation | Re-run if needed | FAIL-RECOV-TEST-027 |
| FAIL-OUTPUT-004 | JSON serialization failure | Serializer | "finding X cannot be serialized: reason" | Non-zero | Fatal | Fix the finding (internal error) | FAIL-RECOV-TEST-028 |
| FAIL-RESOURCE-001 | Resource-limit violation | Governor | "limit L exceeded: value V > max M" | Non-zero | Doc marked `resource-limited` | Reduce input or increase limit | FAIL-RECOV-TEST-029 |
| FAIL-RESOURCE-002 | Memory pressure | Governor sampler | "memory limit reached: used U of M" | Non-zero | Collected findings emitted | Increase limit or reduce input | FAIL-RECOV-TEST-030 |
| FAIL-RESOURCE-003 | Wall-clock limit | Governor deadline | "wall-clock limit reached: elapsed E of L" | Non-zero | Collected findings emitted | Increase limit or reduce input | FAIL-RECOV-TEST-031 |
| FAIL-PLATFORM-001 | Unsupported platform | Startup check | "unsupported platform: OS/arch" | Non-zero | Fatal | Run on a supported platform | FAIL-RECOV-TEST-032 |
| FAIL-PLATFORM-002 | Missing runtime dependency | Startup check | "missing dependency: name, required version" | Non-zero | Fatal | Install the dependency | FAIL-RECOV-TEST-033 |
| FAIL-PLATFORM-003 | Filesystem limitation | Startup check | "filesystem does not support required operation: operation" | Non-zero | Fatal or safe fallback | Use a supported filesystem | FAIL-RECOV-TEST-034 |
| FAIL-PARTIAL-001 | Independent doc failures | Per-doc status | "N documents processed; M failed" | Non-zero | Failed docs marked; successful docs analyzed | Fix the failed docs | FAIL-RECOV-TEST-035 |
| FAIL-PARTIAL-002 | Independent rule failures | Per-rule status | "rule X failed on object Y; other rules continue" | Non-zero | Other findings continue | Fix the rule | FAIL-RECOV-TEST-036 |
| FAIL-PARTIAL-003 | Independent graph failures | Per-namespace status | "graph not built for namespace N; reason" | Non-zero | Other namespaces analyzed | Fix the namespace's manifests | FAIL-RECOV-TEST-037 |
| FAIL-RECOVERY-001 | Interrupted activation | Transaction marker | "interrupted activation recovered; previous bundle restored" | Analysis exit code | Analysis proceeds | None; automatic | FAIL-RECOV-TEST-038 |
| FAIL-RECOVERY-002 | Corrupt active bundle | Load check | "corruption detected; previous bundle restored" | Analysis exit code | Analysis proceeds with previous bundle | None; automatic | FAIL-RECOV-TEST-039 |
| FAIL-RECOVERY-003 | Failed verification during activation | Verifier | "verification failed; previous bundle retained" | Non-zero | Analysis proceeds with previous bundle | Retry or report | FAIL-RECOV-TEST-040 |
| FAIL-ROLLBACK-001 | User-initiated rollback | `bundle activate --rollback` | "rolled back to bundle B" | Analysis exit code | Analysis proceeds with restored bundle | None | FAIL-RECOV-TEST-041 |
| FAIL-ROLLBACK-002 | Rollback attack | Sequence check | "rollback attempt rejected: sequence N < M" | Non-zero | Fatal; active bundle retained | Use the active bundle | FAIL-RECOV-TEST-042 |
| FAIL-ROLLBACK-003 | Previous bundle unavailable | Store check | "no previous bundle available for rollback" | Non-zero | Fatal | Re-download a known-good bundle | FAIL-RECOV-TEST-043 |

`FAIL-RECOV-REQ-003` requires that every failure mode in the matrix be linked to its test, and `FAIL-RECOV-REQ-004` requires that the exit code column be consistent with `docs/cli/EXIT_CODE_CONTRACT.md`. A failure mode that is not in the matrix is a gap in this document; a failure mode whose test does not exist is a policy statement, not an implementation.

## Never silently skip a critical security check

The most important requirement in this document is `FAIL-RECOV-REQ-005`, which states that KubeCheck never silently skips a critical security check. A "critical security check" is any check whose omission would cause a manifest to be reported as valid when it is actually exploitable. The concrete list includes: the safe-loader assertion at startup (`SEC-YAML-REQ-003`), the signature verification of the active bundle (`SEC-REQ-006`), the duplicate-key rejection (`YAML-SPEC-REQ-016`), the unsafe-tag rejection (`SEC-YAML-REQ-023`), the shell-execution prohibition (`SEC-REQ-002`), the path-traversal check (`SEC-YAML-REQ-025`), the secret-redaction check (`SEC-YAML-REQ-031`), and the privilege-escalation checks in the security context analysis.

If any of these checks cannot be performed — because the bundle is corrupt, the parser is misconfigured, the rule engine fails to load the rule, the security context analysis errors out — the tool must treat the situation as a **fatal** failure and refuse to produce a "success" result. A manifest that passes every check except the ones that could not be performed must be reported as "analysis incomplete: security checks not performed: [list]." The user must be able to trust that a "success" result means every critical security check ran and passed.

`FAIL-RECOV-REQ-006` requires that a non-critical check that fails be reported as `not-performed` with an `info` severity, and that the report header include a count of the checks not performed. `FAIL-RECOV-REQ-007` requires that the exit code reflect the highest severity among (a) the findings emitted, (b) the fatal failures, and (c) any critical security check that was not performed. A run that skipped a critical security check must not exit with code `0` under any configuration.

## Never label incomplete analysis as complete

The second most important requirement is `FAIL-RECOV-REQ-008`, which states that KubeCheck never labels an incomplete analysis as complete. The "complete" label is the tool's claim that every finding it would have emitted for the analyzed input is present in the report. If any document was not analyzed, any rule was not evaluated, any check was not performed, or any stage was skipped, the report is incomplete and must be labeled as such.

`FAIL-RECOV-REQ-009` requires that the report header contain a `completeness` field with one of the values `complete`, `partial`, `fatal`, or `cancelled`. The `complete` value is used only when every document was fully processed, every rule was fully evaluated, and every check was fully performed. The `partial` value is used when some inputs were processed but some were not. The `fatal` value is used when the invocation could not produce a trustworthy result. The `cancelled` value is used when the invocation was interrupted.

`FAIL-RECOV-REQ-010` requires that the `completeness` field be present in every output format, that it be the first field a consumer reads (or the first line of the human-readable output), and that it not be suppressible by configuration. A user who suppresses the field is suppressing the tool's own honesty mechanism, and the tool must not allow that.

## Open questions and unresolved decisions

Six open questions remain. `OPEN-FAIL-01` asks whether the `completeness` field should also include a list of the specific checks not performed, or only the summary value; the current design includes the summary value in the header and the per-document status in the body, but the per-check list may be added if there is demand. `OPEN-FAIL-02` asks whether a `bundle verify` command should be added to validate the active bundle on demand, or whether the automatic load-time verification is sufficient; the current design has load-time verification, but a verify command would help with recovery. `OPEN-FAIL-03` asks whether the rollback transaction should keep more than one previous bundle, or only one; the current design keeps one, but more may be needed for organizations that activate multiple bundles. `OPEN-FAIL-04` asks whether the tool should fail closed on a bundle-expiry check, or warn and continue; the current design warns and continues, but a future policy may require fail-closed for expired bundles. `OPEN-FAIL-05` asks whether the platform check should be a compile-time check (the binary refuses to run if the platform is not in the supported matrix) or a runtime check (the binary runs and reports the mismatch); the current design uses a runtime check for portability, but a compile-time check is possible for statically linked binaries. `OPEN-FAIL-06` asks whether the partial-analysis report should include the specific manifest paths of the failed documents in the header, or only in the body; the current design includes them in the body, but the header may be extended if there is demand.

## Definition of done

This document is complete when eleven conditions hold. The three-way distinction between fatal, partial, and recoverable failures is defined and governs every failure in the document. Every family of failures (input, schema, rule, bundle, storage, output, resource, platform, partial, recovery, rollback) is specified with the specific detection method, user-visible message, exit code, partial-result policy, recovery action, and test. The failure-mode matrix consolidates all fifty failure modes in one table with all seven columns. The never-silently-skip rule is specified as `FAIL-RECOV-REQ-005` and `FAIL-RECOV-REQ-006`, and the never-label-incomplete-as-complete rule is specified as `FAIL-RECOV-REQ-008` through `FAIL-RECOV-REQ-010`. The completeness field is required in every output format and is not suppressible. The recovery family specifies automatic recovery from interrupted activation and corrupt bundles. The rollback family specifies safe rollback and preservation of last-known-good data. The open questions are recorded with ownership. Every requirement has a stable ID (`FAIL-RECOV-REQ-NNN`), every failure mode has a stable ID (`FAIL-<FAMILY>-NNN`), and every test has a stable ID (`FAIL-RECOV-TEST-NNN`). The document does not claim any implementation status, and it does not promise that any failure is fully handled — it specifies what handling means and what the tests must prove.

The failure handling and recovery policy is the counterpart to the safe-execution policy. The safe-execution policy says what the tool must never do; this document says what the tool must do when the input it receives or the environment it runs in prevents it from following the safe-execution policy. Both documents are enforced by tests, and both documents are reviewed at every phase boundary and every major-version release. A tool that claims to be safe but cannot handle failure gracefully is a tool that will fail unsafely, and this document is what prevents that.