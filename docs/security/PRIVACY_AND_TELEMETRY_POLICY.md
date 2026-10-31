# KubeCheck — Privacy and Telemetry Policy

**Document ID:** PRIVACY-001 · **Status:** Normative — Approved · **Version:** 1.0.0 · **Owner:** Privacy Engineering · **Last reviewed:** 2026-10-31

This document defines the data-handling and privacy contract for KubeCheck. It specifies, in enforceable terms, what data the tool reads, what it stores, what it transmits, what it never reads, what it never stores, and what it never transmits, together with the verification methods that establish each of those claims and the boundaries of what those verification methods can and cannot prove. It is written because a tool that analyzes Kubernetes manifests is a tool that routinely touches the most sensitive configuration data in an organization — credentials, service-account tokens, internal hostnames, TLS key material, regulatory annotations — and a user has a legitimate need to know exactly what happens to that data when the tool runs. The answer in KubeCheck's case is that the data stays local, that no analysis result is transmitted anywhere, and that no telemetry is collected. But the answer must be stated with precision, because a vague privacy policy ("we respect your privacy") is worse than no policy at all: it invites users to assume protections that the tool does not actually provide and that no test can actually verify.

Every requirement in this document is identified with the prefix `PRIVACY-REQ-NNN`, every data category with `DATA-NNN`, every verification method with `PRIVACY-VERIFY-NNN`, and every test with `PRIVACY-TEST-NNN`. The document begins with the governing principles, then develops the data inventory, then the specific handling of each category of sensitive data, then the optional update and consent mechanisms, then the retention, deletion, and export behavior, then the verification boundary — which is the most important section in the document, because it states honestly what the verification can prove and what it cannot — and finally the tests that operationalize the policy.

## The governing principles

Four principles govern this document, and every requirement derives from them.

The first principle, `PRIVACY-REQ-001`, is **local processing by default**. Every byte of manifest content that KubeCheck reads is processed entirely on the machine where the tool runs, in the process where the tool runs, without being sent to any network destination. The tool does not send manifest content to a cloud service, an inference API, a schema registry, a license server, or any other remote endpoint during the analysis path. This is the same offline guarantee that `NET-REQ-001` and `SARCH-REQ-001` require for functional reasons; here it is restated as a privacy guarantee because the functional reason and the privacy reason are distinct and both must be protected.

The second principle, `PRIVACY-REQ-002`, is **zero required telemetry**. The tool does not collect telemetry, usage statistics, crash counts, feature-usage events, or any other form of analytics, and no feature of the tool requires telemetry to function. There is no opt-out because there is no opt-in; there is simply no telemetry. A user who wants to run the tool on an air-gapped machine can do so, and the tool's behavior on the air-gapped machine is identical to its behavior on a connected machine except for the optional update subsystem described below.

The third principle, `PRIVACY-REQ-003`, is **no automatic upload of anything**. The tool does not upload manifests, findings, reports, logs, configuration, bundle identifiers, engine versions, or any other data automatically. If the tool ever transmits data — and in the MVP it does not, except for the explicit and user-initiated bundle-update subsystem — the transmission is the direct result of a command the user typed, not a background process the user did not request.

The fourth principle, `PRIVACY-REQ-004`, is **no collection without justified need, and a preference for no collection at all**. Where a feature could benefit from collecting data, the default is not to collect it. A feature that requires collection must justify the collection in an approved ADR, must offer an opt-out, must document exactly what is collected, must document where it is stored, must document how long it is retained, and must be reviewed by the Privacy Engineering owner before the feature is merged. The burden of proof is on the feature, not on the user.

These four principles interact to give the user a simple statement: run KubeCheck and nothing leaves your machine. The rest of the document specifies what "nothing" means precisely, what data touches the machine's disk during a run, what data is stored persistently, what data is optionally transmitted by an explicit command, and — most importantly — what the verification can and cannot prove about those claims.

## The data inventory

The privacy policy is only meaningful if it enumerates the specific categories of data the tool touches and classifies each one as collected, stored, transmitted, or never collected. The inventory has ten categories.

`DATA-001` is **manifest content**: the YAML and JSON files, stdin streams, and archive entries that the user supplies for analysis. This data is read, processed entirely in memory, and never written to any file except through the report output the user requested. Manifest content is never transmitted by the tool.

`DATA-002` is **diagnostic findings**: the reports the tool produces. These are written to `stdout` (and to the user's chosen output path if `--output` is used). They may be stored by the user's shell, CI system, or ticket system, but KubeCheck itself does not store them beyond the duration of the invocation. Diagnostic findings are never transmitted by the tool.

`DATA-003` is **logs and debug output**: the content the tool writes to `stderr`. Logs are written only to `stderr`, are subject to the redaction rules below, and are never transmitted by the tool. Logs are not persisted by KubeCheck; they are persisted only by whatever captures `stderr` in the user's environment.

`DATA-004` is **configuration**: the tool's configuration file and command-line flags. Configuration is read from the user's specified locations and is not stored by the tool. Configuration is never transmitted.

`DATA-005` is **knowledge bundles**: the signed schema and rule artifacts the tool uses. Bundles are read from the bundle store, verified, and used. They are not modified by the analysis path. They are never transmitted by the analysis path. The optional update subsystem may download a bundle on explicit user command, and it may send the request that asks the server for the bundle, but it does not send manifest content, diagnostic findings, or any user-specific data.

`DATA-006` is **bundle store metadata**: the tool's local record of which bundles are installed, their signatures, their sequence numbers, and their activation timestamps. This data is stored locally in the bundle store with user-only permissions. It is never transmitted.

`DATA-007` is **crash reports**: the structured error records the tool produces when it encounters an internal error. These are written to `stderr` and, if the user directs them, to a user-specified file. They are never transmitted automatically and never uploaded to a crash-collection service.

`DATA-008` is **support bundles**: the diagnostic artifacts a user might create manually (e.g., via a `kubecheck support-bundle` command, if one is added) to share with the project for troubleshooting. Support bundles are created only on explicit user command, are written to a user-specified path, and are never transmitted by the tool. The user chooses whether to share them, and with whom.

`DATA-009` is **credentials and secrets**: passwords, tokens, API keys, TLS private keys, and any other sensitive value that appears in the manifest. This data is read as part of the manifest, is processed in memory, and is subject to the strictest redaction rules. It is never stored, never logged, and never transmitted.

`DATA-010` is **user identity**: usernames, email addresses, machine hostnames, IP addresses, and other identifying information. KubeCheck does not read any of these. It does not read `$USER`, it does not read `hostname`, it does not read `/etc/passwd`, and it does not read network interface addresses for any analysis purpose. The only identity-related value the tool may read is the bundle-store path, which follows the platform convention and therefore may contain the user's home directory path — but this value is never transmitted.

`PRIVACY-REQ-005` requires that every data category be classified in the inventory and that the classification be enforced by tests. `PRIVACY-REQ-006` requires that any future feature that would add, remove, or reclassify a data category be reviewed as a privacy-relevant change under the process in the final section.

The inventory is consolidated in this table:

| Data category | Read | Stored by KubeCheck | Transmitted by KubeCheck | Classification |
|---|---|---|---|---|
| Manifest content | Yes | No (in memory only) | No | Sensitive; never transmitted |
| Diagnostic findings | N/A (produced) | No (user's output only) | No | May contain derived sensitive data; never transmitted |
| Logs / debug output | N/A (produced) | No | No | May contain derived sensitive data; never transmitted |
| Configuration | Yes | No | No | May contain paths; never transmitted |
| Knowledge bundles | Yes | Yes (bundle store) | Only on explicit update command | Non-sensitive artifact; signed |
| Bundle store metadata | N/A (produced) | Yes (bundle store) | No | Non-sensitive artifact metadata |
| Crash reports | N/A (produced) | No | No | Contains location and error class, never content |
| Support bundles | N/A (produced, user-initiated) | Only at user-specified path | Only if user sends them | User-controlled |
| Credentials and secrets | Yes (as part of manifest) | No | No | Strictly prohibited from storage or transmission |
| User identity | No | No | No | Never read |

`PRIVACY-REQ-007` requires that this table be updated whenever any data category changes, and that the release notes reference the update.

## Local processing and zero required telemetry

The local-processing guarantee is the foundation of the privacy contract, and it must be stated with enough precision that a user can reason about it.

`PRIVACY-REQ-008` requires that the `analyze` command process every byte of manifest content in the same process that the user invoked, without creating a subprocess, without invoking a network API, and without writing the content to any file outside the report the user requested. `PRIVACY-REQ-009` requires that the `analyze` command not resolve any hostname, not read any environment variable that names a network endpoint, and not consult any network-related configuration. `PRIVACY-REQ-010` requires that the `version`, `explain`, and `config` commands behave the same way — they read only the inputs they need (the bundle metadata, the config file, the engine version) and never transmit anything.

The zero-telemetry guarantee is equally precise. `PRIVACY-REQ-011` requires that the tool not implement any telemetry collection, telemetry buffering, telemetry upload, or telemetry-adjacent feature such as crash reporting to a remote service, usage analytics, feature flags that phone home, or update-ping endpoints. `PRIVACY-REQ-012` requires that the tool not have a hidden "phone home" mechanism activated by a default configuration. `PRIVACY-REQ-013` requires that the tool's behavior on a machine with no internet access be identical to its behavior on a connected machine for every command except the optional update subcommands described below.

The verification for these guarantees is important because the guarantees are claims about the tool's behavior and a claim is only as good as its verification. `PRIVACY-VERIFY-001` is network-namespace isolation: the tool runs in a Linux network namespace created with `unshare -n` (or in a container with `--network=none`), the full acceptance suite runs, and every test passes. `PRIVACY-VERIFY-002` is syscall tracing: `strace` on Linux, `dtrace` on macOS, and the equivalent on Windows trace every `connect`, `sendto`, `sendmsg`, `socket`, and DNS-related syscall, and the trace is asserted to contain no outbound connection during the `analyze` command. `PRIVACY-VERIFY-003` is static analysis: the build pipeline rejects any use of network APIs (`net.Dial`, `socket.connect`, `requests.get`, `urllib`, `http.Client`, `fetch`, and equivalents) in the packages that make up the core engine.

The syscall-tracing verification is the strongest of the three, but it is not unlimited — see the verification boundary section below for the specific limits.

## No automatic upload of manifests or diagnostics

The "no automatic upload" guarantee is a corollary of the local-processing and zero-telemetry guarantees, but it is stated separately because the two most common privacy failures in developer tooling are automatic manifest upload and automatic diagnostic upload.

`PRIVACY-REQ-014` requires that the tool never upload manifest content automatically. There is no exception for "anonymized manifest content," "manifest excerpts," "manifest hashes," or "manifest metadata." The tool does not compute a manifest hash for transmission; it computes content hashes for bundle verification and for internal deduplication, and those hashes are never transmitted.

`PRIVACY-REQ-015` requires that the tool never upload diagnostic findings automatically. There is no exception for "aggregate finding counts," "finding IDs only," "finding categories only," or "anonymized findings." If a user wants to share a finding with the project — for example, to report a false positive — the user shares it themselves via the project's issue tracker or security-disclosure channel, using their own communication tooling.

`PRIVACY-REQ-016` requires that the tool never upload the report, the logs, the crash reports, the configuration, or any derived data automatically. The tool's `stderr` output is intended for the user's terminal, and the user's CI system may capture it, but KubeCheck does not send it anywhere.

`PRIVACY-REQ-017` requires that the tool not implement any "send feedback," "report a problem," "share anonymous usage," or similar feature that, when invoked, would upload data. A future feature that helps users file a bug report must do so by preparing a local file for the user to review and share, not by uploading anything.

## Handling of credentials, Secret resources, and sensitive annotations

The most sensitive data the tool touches is the class of data that appears in Kubernetes Secrets, ServiceAccounts, and sensitive annotations. The handling of this data is specified in the threat model and the safe-execution policy, but it is restated here because the privacy policy is the document a user reads when deciding whether to trust the tool with their secrets.

`PRIVACY-REQ-018` requires that the tool identify the following fields as secret-bearing: `Secret.data.*`, `Secret.stringData.*`, `Secret` values in any form, ServiceAccount tokens, TLS key material (`tls.key`), the values of any field matching the documented sensitive-name patterns (`password`, `token`, `apiKey`, `secret`, `credential`, `passwd`, `pwd`, `privateKey`, `auth`), and any field that the manifest explicitly marks as sensitive via the documented annotation.

`PRIVACY-REQ-019` requires that secret-bearing values never be written to any log, finding, error message, crash report, or temporary file, in any output format, at any log level, including debug mode. The safe-execution policy requires this for the report and logs (`SAFE-EXEC-REQ-041` through `SAFE-EXEC-REQ-046`); the privacy policy restates it as a data-protection guarantee.

`PRIVACY-REQ-020` requires that the tool not compute any derived value from a secret-bearing field that would allow the value to be reconstructed or compared across invocations. Content hashes of secret-bearing values are prohibited, because a hash is a derived value and, for low-entropy secrets (short passwords, known token formats), a hash can be attacked offline. The tool does not need to hash secrets for any function; if a future feature proposes to hash secrets, the proposal must be reviewed as a privacy-relevant change and must justify why a non-hashable approach is insufficient.

`PRIVACY-REQ-021` requires that sensitive annotations be treated the same as Secret fields. A user who marks an annotation as sensitive (for example, with a documented `kubecheck.example.com/sensitive: "true"` annotation) is telling the tool that the annotation's value must be redacted. The tool must honor the marker and must treat the value as secret-bearing in every output channel.

`PRIVACY-REQ-022` requires that the tool not enumerate secret-bearing fields for reporting purposes. A finding that "the Secret contains 5 keys" is a derived fact that reveals information about the Secret; a finding that "the Secret is missing key `DB_PASSWORD`" reveals the key name but not the value, and the key name may itself be sensitive. The tool must report key names only when the finding's remediation requires the user to know which key is missing, and must not report key names in general-purpose summaries.

`PRIVACY-REQ-023` requires that the tool not read the ServiceAccount token file from `/var/run/secrets/kubernetes.io/serviceaccount/` even when the tool runs inside a Pod. The safe-execution policy already prohibits reading `KUBECONFIG` and credential files (`SAFE-EXEC-REQ-008`); the privacy policy extends the prohibition to the in-Pod ServiceAccount token, which is a credential file that the tool does not need.

## Log and debug-output redaction

The redaction rules are the operational mechanism by which the secret-handling guarantees are enforced, and they must be precise.

`PRIVACY-REQ-024` requires that the default log level emit no manifest scalar values at all. A log line at the default level names the component, the stage, and the operation, and may include file paths, document indices, and rule identifiers, but never includes field values. A log line such as "parsing document 3 of manifest.yaml" is permitted; a log line such as "parsing document 3: found value 'my-password'" is prohibited.

`PRIVACY-REQ-025` requires that the debug log level emit structural paths and node identifiers but not scalar values. A debug line may say "evaluating rule SEC-CTX-001 on /spec/containers/0/securityContext/privileged," but it may not say "the field's value is true." The prohibition on scalar values at debug level is unconditional; there is no log level at which secret-bearing scalar values are emitted.

`PRIVACY-REQ-026` requires that the `--debug` flag emit a warning to `stderr` on activation, stating that debug output may include structural paths from the manifest and that the user should review the output before sharing it. The warning is required even though the redaction rules still apply, because a structural path can itself be sensitive (a field path such as `/spec/template/spec/containers/2/env/3/valueFrom/secretKeyRef/name` reveals the name of a secret key, which may be sensitive).

`PRIVACY-REQ-027` requires that the tool's error messages never include the value of an offending scalar, and that the tool's own error-handling code path never format a manifest value into an error string. The safe-execution policy requires this for internal errors (`SAFE-EXEC-REQ-043`); the privacy policy restates it as a data-protection requirement and extends it to every error path, including the crash barrier and the internal-error reporting path.

`PRIVACY-REQ-028` requires that the tool's log output be scanned at rule-authoring time for any value that matches a secret-bearing pattern, and that a rule whose remediation or evidence text contains such a value be rejected before the rule enters a bundle. This is the same check as `RCA-REMED-REQ-052` and `SEC-YAML-REQ-030`, restated as a privacy guarantee.

## Temporary files and cleanup

The temporary-file policy is specified in full in `SAFE-EXEC-REQ-031` through `SAFE-EXEC-REQ-034`, and the privacy policy restates the data-protection aspects.

`PRIVACY-REQ-029` requires that the `analyze` command create no temporary file. All intermediate data is held in memory. A temporary file that contains manifest content is a data-at-rest exposure that the tool must not create.

`PRIVACY-REQ-030` requires that any temporary file created by the bundle-management commands contain no manifest content and no secret-bearing value. The bundle-management commands may create temporary files for downloading, verifying, and atomically replacing bundles; those files contain bundle content, which is a signed, public artifact, not user data.

`PRIVACY-REQ-031` requires that any temporary file created by the tool be created with restrictive permissions (`0600` on POSIX) and be deleted on exit, including on abnormal exit. The safe-execution policy specifies the mechanism (`O_CREAT|O_EXCL` with a random suffix, `defer` or `finally` cleanup); the privacy policy specifies the data-protection objective.

`PRIVACY-REQ-032` requires that the tool not use the system temporary directory (`/tmp`, `TMPDIR`, `%TEMP%`) for any file that contains user data. The bundle store's `tmp/` directory is the only permitted temporary location, and it is created with `0700` permissions.

## Crash reports and support bundles

Crash reports and support bundles are the two artifacts a user might share with the project, and they must be designed so that the user can share them safely.

`PRIVACY-REQ-033` requires that the tool's crash reports contain only the following: the engine version, the bundle ID, the target Kubernetes version (if supplied), the error class (e.g., `PARSE_CRASH`, `NORMALIZATION_CRASH`, `INTERNAL_ERROR`), the document index and location where the crash occurred, and the last successfully parsed stage. The crash report must not contain manifest content, NIR nodes, scalar values, or file paths beyond the file name. The safe-execution policy specifies the same boundary (`SAFE-EXEC-REQ-045`); the privacy policy restates it and adds that the crash report must be reviewable by the user before sharing.

`PRIVACY-REQ-034` requires that the crash report be written to `stderr` by default and to a user-specified file only when the user directs it (`--crash-report <path>`). The tool must not write a crash report to a location the user did not specify.

`PRIVACY-REQ-035` requires that a support bundle, if the tool provides a `support-bundle` command in a future release, be created only on explicit user command, be written only to a user-specified path, and contain only the following: the engine version, the bundle ID and metadata, the resolved configuration with secret-bearing values redacted, the last N log lines with scalar values redacted, and the crash reports (if any) with the same content boundary as `PRIVACY-REQ-033`. The support bundle must not contain manifest content, diagnostic findings with evidence, or any secret-bearing value. The tool must not upload the support bundle.

`PRIVACY-REQ-036` requires that the support-bundle command, when invoked, print a summary to `stderr` of what the bundle contains and what it does not contain, so the user can make an informed decision before sharing it.

## Optional update checks and network destinations

The only command that may perform network I/O is the bundle-update subsystem, and its network behavior is the subject of the most scrutiny under the privacy policy because it is the one place where the tool's data could leave the machine.

`PRIVACY-REQ-037` requires that the tool not perform automatic update checks in the background. The user must invoke the update command explicitly (`kubecheck bundle update` or equivalent). The tool must not check for updates on `analyze`, on `version`, on `--help`, or on any command the user did not explicitly invoke for that purpose.

`PRIVACY-REQ-038` requires that the update subsystem contact only the update source the user has configured or the default official update source. The tool must not contact multiple sources without the user's knowledge, must not contact analytics services, must not contact CDNs the user did not authorize, and must not contact any source the user has not been told about in the update command's help text.

`PRIVACY-REQ-039` requires that the update request sent to the update source contain only the following: the request path (e.g., `/bundles/<bundle-id>`), the HTTP `User-Agent` header identifying the tool and its version (so the source can serve compatible bundles), and standard HTTP headers (`Host`, `Accept`, `Accept-Encoding`). The request must not contain the user's manifest content, the user's diagnostic findings, the user's configuration, the user's identity, or any unique identifier tied to the user's machine. A request containing a stable per-machine identifier is prohibited even if the identifier is described as "anonymous."

`PRIVACY-REQ-040` requires that the update subsystem, after downloading a bundle, verify the bundle's signature against the local trust anchor before activating it. The signature verification is the mechanism by which a compromised update source cannot substitute a malicious bundle. The privacy policy restates this because a compromised update source is also a potential privacy compromise (it could correlate requests, if the requests were unique).

`PRIVACY-REQ-041` requires that the update subsystem support an offline mode in which the user supplies the bundle from a locally transferred file. The offline mode is the mechanism by which a user in an air-gapped environment can update without any network request. The offline mode must accept the same bundle format and apply the same signature verification as the online mode.

`PRIVACY-REQ-042` requires that the update command's help text and documentation state precisely: what data the request contains, what data the request does not contain, what source is contacted, and what happens if the request fails. The user must be able to make an informed decision about whether to run the update.

## Consent requirements for future data sharing

The tool does not currently share any data with any third party beyond the optional update request described above. If a future feature proposes to share data — for example, a crash-collection service, a feature-usage analytics service, a benchmark submission service, or a "share finding" service — the feature must be reviewed under the following requirements.

`PRIVACY-REQ-043` requires that any future data-sharing feature be gated behind an explicit, documented opt-in. The opt-in must be a command the user types, a configuration key the user sets, or a flag the user passes. A default-on opt-out is prohibited. An "opt-out of telemetry" mechanism is prohibited if telemetry is enabled by default, because the default is what most users will experience.

`PRIVACY-REQ-044` requires that any future data-sharing feature document the following before the feature ships: what data is collected, where it is sent, who can access it, how long it is retained, whether it is linked to the user's identity, whether it is linked to the user's machine, whether it is shared with third parties, and what happens to it if the user later withdraws consent.

`PRIVACY-REQ-045` requires that any future data-sharing feature provide a way for the user to withdraw consent and, when the consent is withdrawn, to request deletion of previously shared data. The mechanism for requesting deletion must be documented and must be honored.

`PRIVACY-REQ-046` requires that any future data-sharing feature be reviewed by the Privacy Engineering owner and approved by an ADR. The review must include a Data Protection Impact Assessment (DPIA) if the data could be linked to an identifiable person or if the data is subject to a regulatory framework (GDPR, CCPA, HIPAA, or equivalent).

`PRIVACY-REQ-047` requires that any future data-sharing feature be reported in the release notes and in the user-facing documentation, and that the release notes state clearly what the feature does and why it exists.

## Retention, deletion, and export behavior for locally stored data

The tool stores bundle files and bundle-store metadata on the user's machine, and the privacy policy specifies how the user can inspect, export, and delete that data.

`PRIVACY-REQ-048` requires that the tool provide a command (`kubecheck bundle list` and `kubecheck bundle info <id>`) that lists every bundle in the local store with its ID, sequence number, signature, activation timestamp, and status. The user must be able to see exactly what the tool has stored.

`PRIVACY-REQ-049` requires that the tool provide a command (`kubecheck bundle export <id> --output <path>`) that exports a stored bundle to a user-specified path, so the user can move it to another machine or archive it. The export must not include the bundle-store metadata, which is machine-specific.

`PRIVACY-REQ-050` requires that the tool provide a command (`kubecheck bundle delete <id>`) that deletes a stored bundle and its metadata. The command must refuse to delete the active bundle unless the user passes an explicit flag (`--force`) and the command must warn that deleting the active bundle will leave the tool unable to analyze until a new bundle is activated. The deletion must be a real deletion (the file is removed and, on filesystems that support it, overwritten); the tool must not move the bundle to a trash directory.

`PRIVACY-REQ-051` requires that the tool provide a command (`kubecheck bundle purge`) that deletes every bundle and all bundle-store metadata, returning the tool to its uninstalled state. The command must require explicit confirmation and must warn that it will delete every stored bundle.

`PRIVACY-REQ-052` requires that the tool not store any other data beyond the bundle store. There is no cache, no history file, no recent-files list, no last-used-configuration file, and no telemetry spool. The bundle store is the only persistent state.

`PRIVACY-REQ-053` requires that the tool document the exact location of the bundle store on each platform (`$XDG_DATA_HOME/kubecheck/` on Linux, `~/Library/Application Support/kubecheck/` on macOS, `%LOCALAPPDATA%\kubecheck\` on Windows) and the exact permissions (`0700` for directories, `0600` for files).

## Dependency behavior and unexpected outbound traffic

The privacy policy cannot claim that KubeCheck is the only software on the user's machine that could make a network call, and it must not make that claim. The tool's own behavior is specified above; the behavior of the operating system, the runtime, and the dependencies is outside the tool's control, and the privacy policy must be honest about this boundary.

`PRIVACY-REQ-054` requires that the tool's third-party dependencies be pinned to exact versions, recorded in a machine-readable manifest with license identifiers, and scanned for known vulnerabilities at every release. The pinning and scanning reduce the risk that a dependency is malicious or compromised; they do not prove that a dependency is incapable of network access.

`PRIVACY-REQ-055` requires that the tool's dependency review process include a privacy review for every new dependency: what network access does the dependency request, does it contact any service at import time, does it phone home for updates or license checks, does it collect any telemetry, and does it read any credential file. A dependency that does any of these things must be rejected or its behavior must be explicitly documented as a known and accepted risk.

`PRIVACY-REQ-056` requires that the tool not bundle any dependency whose documented behavior includes contacting a remote service at import time. A dependency that makes a network call when imported is incompatible with the offline guarantee, regardless of whether the call is "just a version check."

`PRIVACY-REQ-057` requires that the tool's tests verify that the dependencies do not make unexpected network calls during the `analyze` command, using the same network-namespace and syscall-tracing methods as the tool's own behavior verification. A dependency that makes a network call during `analyze` is a release-blocking defect.

The honest statement of the dependency boundary is this: the tool's dependencies are pinned, scanned, and reviewed, but the tool cannot prove that a dependency is incapable of network access. The tool can prove that, in its tested configurations and with its pinned dependency versions, the dependency does not make a network call during the `analyze` command. The user's protection against a malicious dependency is the pinning, the scanning, and the review, not a mathematical proof of incapability. The privacy policy must say this explicitly, because a user who believes the tool's dependencies are provably safe is a user who has been misled.

## The verification boundary — what the verification can and cannot prove

The most important section in this document is the one that states honestly what the verification can and cannot prove, because the rest of the document is a set of claims and a claim without a verified boundary is a claim that will eventually be overtrusted.

The tool can prove the following: that its own source code does not contain network API calls in the core engine (static analysis), that its own binary does not make outbound connections during the tested commands (syscall tracing), that its own behavior is identical on a network-isolated machine and on a connected machine for the `analyze` command (namespace isolation), and that its own report and logs do not contain secret-bearing values for the tested corpus (secret-bearing corpus test).

The tool cannot prove the following: that the operating system on which it runs does not make network calls (the OS may check for updates, synchronize time, resolve DNS names, or contact telemetry endpoints independent of KubeCheck); that a third-party dependency does not make network calls in a code path the tests do not exercise (the tests exercise the tested code paths, not every possible code path); that a future version of a dependency will not introduce network calls (the tests are run at release time on the pinned versions); that a malicious dependency would be detected (the tests detect known network APIs, not arbitrary obfuscated network code); and that the user's environment is what the user believes it to be (the tool cannot verify the machine's network configuration).

`PRIVACY-REQ-058` requires that these limitations be documented in the tool's user-facing privacy documentation, not only in the internal threat model. The user must be able to read, in plain language, what the tool's privacy guarantees cover and what they do not cover. The privacy documentation must state that the tool does not control the operating system's network behavior, does not control the dependencies' behavior beyond the pinned and tested versions, and cannot verify that a dependency is provably incapable of network access.

`PRIVACY-REQ-059` requires that the tool's `--version` output and the report header include a short privacy statement that refers the user to the privacy documentation. The statement must not make claims beyond what the verification supports; it must be a pointer to the documentation, not a substitute for it.

`PRIVACY-REQ-060` requires that the tool's release notes, when they change any privacy-relevant behavior, include a section that states what changed, what the change means for the privacy guarantees, and what the user should do (if anything) in response. A privacy-relevant change that is not reported in the release notes is a defect.

## Privacy tests and verification on offline machines

The privacy policy is enforced by a set of tests that are run at every release, and the tests are the operationalization of the requirements in this document.

`PRIVACY-TEST-001` runs the full acceptance suite in a Linux network namespace created with `unshare -n` and asserts that every test passes and that no outbound connection is attempted. `PRIVACY-TEST-002` runs the full acceptance suite in a container with `--network=none` and asserts the same. `PRIVACY-TEST-003` runs the full acceptance suite on macOS with Wi-Fi and Ethernet disabled and asserts the same. `PRIVACY-TEST-004` runs the full acceptance suite on Windows with all non-loopback network adapters disabled and asserts the same.

`PRIVACY-TEST-005` runs the `analyze` command under `strace -f -e trace=network` and asserts that the trace contains no `connect`, `sendto`, `sendmsg`, or DNS-related syscall. `PRIVACY-TEST-006` runs the same on macOS with `dtrace` and on Windows with the equivalent tracing tool.

`PRIVACY-TEST-007` runs the full acceptance suite with a `KUBECONFIG` environment variable pointing at a canary file containing a fake credential, and asserts that the credential does not appear in `stdout`, `stderr`, or any file the tool creates. `PRIVACY-TEST-008` runs the full acceptance suite with a fake ServiceAccount token file at `/var/run/secrets/kubernetes.io/serviceaccount/token` and asserts that the token is not read.

`PRIVACY-TEST-009` runs the secret-bearing corpus at every log level and in every output format and asserts that no canary secret value appears in any output. `PRIVACY-TEST-010` runs the same corpus with the `--debug` flag and asserts that the canary values do not appear even at debug level.

`PRIVACY-TEST-011` audits every error message produced by the malformed corpus and asserts that no error message contains a scalar value. `PRIVACY-TEST-012` audits every crash report produced by the crash corpus and asserts that no crash report contains manifest content.

`PRIVACY-TEST-013` runs the `bundle update` command with the official update source replaced by a local mock server and asserts that the request contains only the permitted headers and no user-specific data. `PRIVACY-TEST-014` runs the same with the update source returning a bundle and asserts that the bundle is verified before activation.

`PRIVACY-TEST-015` runs the `bundle list`, `bundle info`, `bundle export`, `bundle delete`, and `bundle purge` commands and asserts that each behaves as specified. `PRIVACY-TEST-016` audits the bundle store's file permissions and asserts that the directory is `0700` and the files are `0600`.

`PRIVACY-TEST-017` runs the full acceptance suite under a syscall tracer that records every file the tool opens for writing and asserts that the set matches the documented write set. `PRIVACY-TEST-018` runs the same with every command and asserts that no temporary file in the system temporary directory is created.

`PRIVACY-TEST-019` audits the dependency manifest and asserts that every dependency is pinned to an exact version and has a license identifier. `PRIVACY-TEST-020` audits the dependency manifest against a documented list of dependencies that are known to make network calls at import time and asserts that none of them are present.

`PRIVACY-TEST-021` runs the full acceptance suite on each supported platform and asserts that the tool's privacy-relevant behavior (no network, no temp files, no secret leakage) is identical across platforms.

`PRIVACY-REQ-061` requires that every test above be run at every release and that a failure be a release-blocking defect. `PRIVACY-REQ-062` requires that the results be recorded in the release notes with a link to the test run artefact, so the user can verify that the tests were run for the version they are installing.

## Open questions and unresolved decisions

Six open questions remain. `OPEN-PRIVACY-01` asks whether the tool will ever support a "share finding with the project" feature that prepares a local file for the user to share; the current design does not, but a well-designed feature that never transmits data automatically would be consistent with the principles. `OPEN-PRIVACY-02` asks whether the `--version` privacy statement should be a short string or a link to a URL; the current design uses a short string plus a documentation path, but the exact format is open. `OPEN-PRIVACY-03` asks whether the support-bundle command will be implemented in the MVP or in a later phase; the current design reserves the name but does not implement the command. `OPEN-PRIVACY-04` asks whether the bundle store should be relocatable via a `--bundle-store` flag or only via the platform convention; the current design uses the platform convention and does not provide a relocation flag, but a relocation flag may be needed for shared environments. `OPEN-PRIVACY-05` asks whether the tool should offer a `--no-network` flag that asserts the offline guarantee at runtime, or whether the existing behavior is sufficient; the current design relies on the existing behavior and does not provide a flag. `OPEN-PRIVACY-06` asks whether the privacy documentation should be embedded in the binary (accessible via `kubecheck privacy` or `kubecheck --privacy`) or only available online; the current design assumes the documentation is available online and does not embed it, but embedding would help air-gapped users.

## Definition of done

This document is complete when ten conditions hold. The four governing principles (local processing, zero required telemetry, no automatic upload, no collection without justified need) are stated and derive every requirement. The data inventory enumerates ten data categories with read/stored/transmitted classification. The local-processing and zero-telemetry guarantees are specified with three independent verification methods. The no-automatic-upload guarantee is specified for manifests, diagnostics, and all other data. The handling of credentials, Secrets, and sensitive annotations is specified with the prohibition on hashing and on key enumeration. The log and debug-output redaction rules are specified at both default and debug levels. The temporary-file policy is specified with the restrictive-permission requirement. The crash-report and support-bundle boundaries are specified with a content allowlist. The optional update checks are specified with a strict request allowlist and an offline mode. The consent requirements for future data sharing are specified with a DPIA requirement and an ADR approval process. The retention, deletion, and export behavior is specified with five commands. The dependency behavior is specified with pinning, scanning, and an explicit acknowledgment that the tool cannot prove a dependency is incapable of network access. The verification boundary is stated explicitly, including the five things the tool cannot prove. The privacy tests are enumerated and required at every release. Every requirement has a stable ID (`PRIVACY-REQ-NNN`), every data category has a stable ID (`DATA-NNN`), every verification method has a stable ID (`PRIVACY-VERIFY-NNN`), and every test has a stable ID (`PRIVACY-TEST-NNN`). The document does not claim that the operating system, the runtime, or the dependencies are incapable of network access; it documents the tool's actual guarantees, the verification methods, and the boundaries of what those methods can prove. The document does not collect any data by default, and it does not transmit any data by default. It is written to be read continuously rather than as a checklist, because a privacy policy that is read as a checklist is a privacy policy that gets filled in mechanically and then ignored. A user who reads this document should finish it with a clear understanding of what the tool does with their data, what it does not do, and how the tool's claims are verified — and, equally importantly, what the verification cannot establish and where the user's own responsibility begins.