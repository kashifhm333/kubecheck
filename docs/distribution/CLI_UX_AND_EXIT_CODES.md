# KubeCheck — CLI UX Specification and Exit Code Contract

**Document ID:** CLI-UX-001  
**File:** `docs/distribution/CLI_UX_AND_EXIT_CODES.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** CLI Product Design & Developer Experience Engineering  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/architecture/PRODUCT_VISION.md`
- `docs/architecture/REQUIREMENTS_AND_NFRS.md`
- `docs/quality/TEST_STRATEGY.md`
- `docs/quality/RELEASE_ACCEPTANCE_CRITERIA.md`
- `docs/security/SAFE_EXECUTION_POLICY.md`
- `docs/security/FAILURE_HANDLING_AND_RECOVERY.md`
- `docs/knowledge/OFFLINE_UPDATE_AND_SIGNING.md`
- `docs/distribution/INSTALLATION_AND_RELEASES.md`

---

## 1. Purpose and Scope

This document specifies the complete command-line interface (CLI) contract for KubeCheck. It governs the command hierarchy, flag conventions, configuration precedence, input resolution, standard stream behavior, diagnostic output formatting, terminal safety, and the exact numeric exit-code contract.

The design adheres to the **UNIX Philosophy** and modern CLI standards:
1. **Silence is golden:** Clean runs emit no unnecessary chatter on `stdout`.
2. **Channel separation:** Diagnostic and machine-readable data flows to `stdout`; human operational logs, telemetry notifications, and progress messages flow to `stderr`.
3. **Determinism:** Identical arguments and input files always yield identical output ordering and exit codes across all operating environments.
4. **Offline by design:** Analysis commands never attempt network connections.
5. **Composability:** Easy integration into UNIX shell pipelines, git pre-commit hooks, and CI/CD runners (GitHub Actions, GitLab CI, Tekton, Argo Workflows).

---

## 2. Command Hierarchy and Naming Conventions

### 2.1 Invocation Forms

KubeCheck supports two primary invocation styles:
* **Standalone Binary / Pip CLI:** `kubecheck <command> [flags] [arguments]`
* **Kubectl Plugin (via Krew or PATH):** `kubectl check <command> [flags] [arguments]`

Both forms resolve to the identical command router and engine. The executable automatically detects if it was invoked as `kubectl-check` and adjusts root help text to display `kubectl check` examples.

```mermaid
flowchart TD
    Root[kubecheck / kubectl check] --> Analyze[analyze [paths...]]
    Root --> Version[version]
    Root --> Rules[rules]
    Root --> Bundle[bundle]
    Root --> Completion[completion]

    Rules --> RulesList[rules list]
    Rules --> RulesExplain[rules explain &lt;rule-id&gt;]

    Bundle --> BundleInfo[bundle info]
    Bundle --> BundleVerify[bundle verify &lt;file&gt;]
    Bundle --> BundleImport[bundle import &lt;file&gt;]
    Bundle --> BundleExport[bundle export]
    Bundle --> BundleUpdate[bundle update]
```

### 2.2 Subcommand Summary

| Subcommand | Primary Purpose | Network Access |
|---|---|---|
| `analyze [paths...]` | Analyze Kubernetes manifests for syntax, schema, semantic, security, and compatibility defects. | **NEVER** (Offline) |
| `version` | Display engine version, embedded bundle metadata, and supported Kubernetes API versions. | **NEVER** (Offline) |
| `rules list` | Query, filter, and inspect the active catalog of diagnostic rules. | **NEVER** (Offline) |
| `rules explain <id>` | Print exhaustive documentation, rationales, and remediation examples for a rule ID. | **NEVER** (Offline) |
| `bundle info` | Inspect the currently installed knowledge bundle (schemas, rules, dates, signatures). | **NEVER** (Offline) |
| `bundle verify <path>` | Cryptographically verify a knowledge bundle archive using offline trust anchors. | **NEVER** (Offline) |
| `bundle import <path>` | Install or upgrade a local knowledge bundle from a `.tar.gz` archive. | **NEVER** (Offline) |
| `bundle export` | Export the currently active knowledge bundle to an archive file for air-gapped transport. | **NEVER** (Offline) |
| `bundle update` | Query remote update registry, download, verify, and apply the newest knowledge bundle. | **EXPLICIT HTTPS** |
| `completion <shell>` | Generate shell autocomplete scripts (bash, zsh, fish, powershell). | **NEVER** (Offline) |

---

## 3. Flag Conventions and Configuration Precedence

### 3.1 Standard Option Syntax

* Short flags use a single dash (`-f`, `-v`, `-o`, `-R`).
* Long flags use double dashes (`--file`, `--version`, `--output`, `--recursive`).
* Flags accepting arguments support both space and equals syntax (`--output json` or `--output=json`).
* Multiple boolean short flags can be chained (`-Rv` for `--recursive --verbose`).

### 3.2 Precedence Order

When an option is specified in multiple places, the effective configuration is resolved strictly in the following order:

```
[Level 1: CLI Flags]               (Highest Precedence - overrides all)
       v
[Level 2: Environment Variables]    (e.g., KUBECHECK_OUTPUT, KUBECHECK_TARGET_VERSION)
       v
[Level 3: Local Workspace Config]   (./.kubecheck.yaml or ./.kubecheck.yml)
       v
[Level 4: User Global Config]       (~/.config/kubecheck/config.yaml or $XDG_CONFIG_HOME)
       v
[Level 5: Built-in Engine Defaults] (Lowest Precedence)
```

### 3.3 Configuration File Schema Example (`.kubecheck.yaml`)

```yaml
version: 1
target_version: "1.29"
offline: true

analysis:
  recursive: true
  min_severity: "warning"
  fail_on: "error"
  max_findings: 500
  ignore_patterns:
    - "templates/tests/**"
    - "**/charts/**"
  include_categories:
    - "syntax"
    - "schema"
    - "semantic"
    - "security"
    - "compatibility"
  exclude_rules:
    - "KCHK-HEU-003" # Ignore missing liveness probe on batch jobs

output:
  format: "terminal" # terminal | json | sarif | junit | compact
  color: "auto"      # auto | always | never
  show_remediation: true
  redact_secrets: true

bundle:
  custom_path: "" # Empty defaults to ~/.local/share/kubecheck/bundles/active
```

---

## 4. Manifest Input Resolution and Stream Handling

`kubecheck analyze [paths...]` accepts flexible input targets:

### 4.1 Input Target Matrix

| Input Specification | Command Example | Resolution Behavior |
|---|---|---|
| **Single File** | `kubecheck analyze deploy.yaml` | Validates single file. |
| **Multiple Files** | `kubecheck analyze d1.yaml d2.yaml s1.yaml` | Validates multiple discrete files in given order. |
| **Directory** | `kubecheck analyze ./k8s/` | Validates all `.yaml` and `.yml` files in the directory. |
| **Recursive Directory** | `kubecheck analyze -R ./deployments/` | Recursively walks subdirectories; follows symlinks only if within root boundary. |
| **Standard Input (stdin)** | `cat manifest.yaml \| kubecheck analyze -` | Reads manifest stream from stdin. |
| **Pipeline Composition** | `helm template myapp ./chart \| kubecheck analyze -` | Directly validates generator output without disk writes. |
| **Multiple Inputs + Stdin**| `kubecheck analyze manifest.yaml -` | Validates disk files and stdin stream in a unified inventory. |

### 4.2 Multi-Document Streams

Kubernetes manifest files frequently contain multiple YAML documents separated by `---`.
* KubeCheck assigns a 0-based integer `document_index` to each document in the stream.
* If Document 0 is invalid YAML, KubeCheck isolates Document 0, reports the parse error, and continues parsing Document 1 and beyond (`EDGE-REQ-054`).
* Findings identify the document by both index and resource identity: `[file: doc #1, Service/prod/web]`.

### 4.3 Ignore Rules and File Filtering

Files are skipped if:
1. They match a pattern in `./.kubecheckignore` (gitignore-compatible pattern matching).
2. They match `--ignore-pattern <glob>`.
3. They are not UTF-8 text files or carry extensions other than `.yaml` or `.yml` (unless passed explicitly on the CLI).

---

## 5. Comprehensive Flag Specification for `kubecheck analyze`

```
Usage:
  kubecheck analyze [paths...] [flags]

Flags:
  -f, --file stringSlice        Explicit path(s) to manifest file(s) or '-' for stdin
  -R, --recursive               Recursively scan directories for manifest files (default: false)
      --ignore-pattern strings  Glob patterns to ignore (can be specified multiple times)
      --target-version string   Target Kubernetes API version (e.g., '1.28', 'v1.30') (default: latest bundled)
      --bundle string           Path to specific knowledge bundle archive to load
      --offline                 Enforce strict offline isolation; reject any external operations (default: true)

Diagnostic Selection:
  -c, --category strings        Enable specific categories: syntax, schema, semantic, security, compatibility, heuristic
      --include-rules strings   Comma-separated list of rule IDs to include
      --exclude-rules strings   Comma-separated list of rule IDs to suppress
      --min-severity string     Minimum severity to report: error, warning, info (default: 'info')
      --fail-on string          Minimum severity threshold that triggers non-zero exit code: error, warning, info (default: 'error')

Output Configuration:
  -o, --output string           Output format: terminal, json, sarif, junit, compact (default: 'terminal')
      --color string            Control ANSI terminal coloring: auto, always, never (default: 'auto')
      --show-remediation        Display detailed remediation diffs and code suggestions (default: true)
      --redact-secrets          Mask sensitive values and secrets in source snippets (default: true)
      --max-findings int        Maximum number of findings to output before truncating (default: 500)
      --quiet                   Suppress all informative output and progress; emit only findings

General:
      --config string           Path to alternative configuration file
  -h, --help                    Help for analyze
```

---

## 6. Output Formats and Structural Contracts

KubeCheck provides five distinct output formats tailored for human developers and automated systems.

### 6.1 Terminal Human-Readable Format (Default on TTY)

Features:
* Grouped by Resource Key (`Namespace/Kind/Name`).
* Source-code context window with line numbering and caret (`^`) indicators pointing to exact offending columns.
* ANSI color coding (Red for Error, Yellow for Warning, Cyan for Info).
* Remediation guidance and recommended patches.

#### Sample Terminal Output

```
$ kubecheck analyze manifests/deploy.yaml --target-version 1.29

[KubeCheck 1.0.0 · Evaluating manifests against Kubernetes v1.29 (Bundle 2026.10)]

ERROR [KCHK-SCH-002] in manifests/deploy.yaml [Doc #0]:
  Invalid value type for field 'spec.replicas'. Expected integer, found string.

  12 | spec:
  13 |   selector:
  14 |     matchLabels:
  15 |       app: payment-processor
> 16 |   replicas: "three"
     |             ^^^^^^^
  17 |   template:
  18 |     metadata:

  Remediation: Change string value '"three"' to an unquoted integer (e.g., replicas: 3).
  Confidence: deterministic · Evidence: OpenAPI v3 schema validation failure.
  Reference: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/

WARNING [KCHK-SEC-004] in manifests/deploy.yaml [Doc #0]:
  Container 'processor' does not set 'securityContext.readOnlyRootFilesystem: true'.

  21 |     spec:
  22 |       containers:
  23 |       - name: processor
> 24 |         image: registry.internal.net/pay:v1.2.0
     |         ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

  Remediation: Add 'readOnlyRootFilesystem: true' under container securityContext.
  Confidence: probable · Evidence: Container filesystem is mutable by default.
  Reference: CIS Kubernetes Benchmark v1.8.0 §5.2.6

--------------------------------------------------------------------------------
Scan Summary:
  Target Version:  v1.29.0
  Documents:       1 parsed (0 skipped)
  Resources:       1 evaluated
  Findings:        1 Error, 1 Warning, 0 Info
  Duration:        14.2 ms
  Result:          FAILED (Found 1 error at or above fail threshold 'error')
```

### 6.2 Compact Format (`--output compact`)

Designed for UNIX text processing tools (`grep`, `awk`, `cut`, `sed`) and editor error integration:
`<file>:<doc_idx>:<line>:<col>: [<SEVERITY>] [<RULE_ID>] <MESSAGE>`

#### Sample Compact Output
```
manifests/deploy.yaml:0:16:13: [ERROR] [KCHK-SCH-002] Invalid value type for field 'spec.replicas'. Expected integer, found string.
manifests/deploy.yaml:0:24:9: [WARNING] [KCHK-SEC-004] Container 'processor' does not set 'securityContext.readOnlyRootFilesystem: true'.
```

### 6.3 Machine-Readable JSON Format (`--output json`)

Conforms strictly to the normative schema in `docs/output/JSON_SCHEMA.md`.

```json
{
  "$schema": "https://kubecheck.io/schemas/v1/output.json",
  "engine_version": "1.0.0",
  "bundle_version": "2026.10.1",
  "target_version": "v1.29.0",
  "execution_timestamp": "2026-10-31T14:20:00Z",
  "status": "completed",
  "summary": {
    "total_files": 1,
    "total_documents": 1,
    "total_resources": 1,
    "findings_count": {
      "error": 1,
      "warning": 1,
      "info": 0
    },
    "passed": false
  },
  "findings": [
    {
      "id": "fnd_01JBMX9Z8R7Y...",
      "rule_id": "KCHK-SCH-002",
      "category": "schema",
      "severity": "error",
      "confidence": "deterministic",
      "message": "Invalid value type for field 'spec.replicas'. Expected integer, found string.",
      "location": {
        "file_path": "manifests/deploy.yaml",
        "document_index": 0,
        "start_line": 16,
        "start_column": 13,
        "end_line": 16,
        "end_column": 20,
        "byte_offset": 312
      },
      "resource": {
        "group": "apps",
        "version": "v1",
        "kind": "Deployment",
        "namespace": "default",
        "name": "payment-processor"
      },
      "remediation": {
        "summary": "Change string value '\"three\"' to an unquoted integer (e.g., replicas: 3).",
        "patch_type": "strategic-merge-patch",
        "patch": "spec:\n  replicas: 3\n"
      }
    }
  ]
}
```

### 6.4 SARIF Format (`--output sarif`)

Conforms to **OASIS SARIF v2.1.0** standard for native visualization in GitHub Advanced Security Code Scanning alerts and IDE extensions.

---

## 7. Stable Diagnostic Identifiers and Deterministic Ordering

### 7.1 Diagnostic Rule Identifier Scheme

Rule IDs follow a strict naming syntax: `KCHK-<CATEGORY_CODE>-<3_DIGIT_ID>`

| Category Code | Description | Example Rule ID |
|---|---|---|
| `SYN` | Syntax & YAML parsing | `KCHK-SYN-001` (Duplicate mapping key) |
| `SCH` | Schema & OpenAPI validation | `KCHK-SCH-002` (Type mismatch) |
| `SEM` | Semantic & Cross-resource | `KCHK-SEM-008` (Selector mismatch) |
| `SEC` | Security & Hardening | `KCHK-SEC-004` (Mutable root filesystem) |
| `CMP` | Version Compatibility | `KCHK-CMP-001` (Deprecated API removal) |
| `HEU` | Operational Heuristics | `KCHK-HEU-002` (Missing readiness probe) |

### 7.2 Deterministic Sorting Specification

To guarantee bit-for-bit identical outputs across environments, findings are ordered canonically:
1. `location.file_path` (Ascending, lexicographical by UTF-8 bytes).
2. `location.document_index` (Ascending integer: 0, 1, 2...).
3. `location.start_line` (Ascending integer).
4. `location.start_column` (Ascending integer).
5. `rule_id` (Ascending, lexicographical).

---

## 8. Authoritative Exit Codes Contract

KubeCheck uses a strict numeric exit-code contract to allow CI scripts and wrapper tools to deterministically differentiate between clean runs, policy violations, bad command usage, and operational failures.

| Exit Code | Classification | Meaning / Triggering Condition | Recommended Action in CI |
|---|---|---|---|
| **`0`** | **SUCCESS** | Analysis completed successfully. Zero findings detected at or above the `--fail-on` threshold. | Allow build pipeline to proceed. |
| **`1`** | **FINDINGS DETECTED** | One or more findings detected at or above `--fail-on` threshold (Default: 1+ Error findings). | Block deployment; output findings in PR review. |
| **`2`** | **USAGE / INVOCATION ERROR** | Malformed CLI arguments, unknown flags, invalid combination of options, syntax error in config file. | Fix CLI invocation script. |
| **`3`** | **INPUT / RESOLUTION ERROR** | Specified manifest file does not exist, permission denied, directory path unreadable, or invalid UTF-8 byte stream. | Verify file paths, permissions, and encodings. |
| **`4`** | **OPERATIONAL / ENGINE ERROR** | Internal engine error: Out-of-memory governor tripped, parser bomb recursion limit exceeded, unhandled panic. | Check system memory limits or report engine bug. |
| **`5`** | **PARTIAL / INCOMPLETE SCAN** | Analysis could not complete for all inputs: Process cancelled via `SIGINT`/`SIGTERM`, or non-recoverable parse failure in multi-document stream. | Re-run with valid documents; investigate partial results. |
| **`6`** | **BUNDLE / TRUST ERROR** | Knowledge bundle missing, corrupted archive, signature verification failed, or anti-rollback violation. | Re-import or verify knowledge bundle trust anchor. |

### 8.1 Fine-Tuning the Failure Threshold (`--fail-on`)

The exit code behaviour is governed by the `--fail-on` flag:
* `--fail-on error` (Default): Returns `1` only if one or more `error` severity findings occur. Warnings and Info findings return exit code `0`.
* `--fail-on warning`: Returns `1` if any `error` or `warning` findings occur.
* `--fail-on info`: Returns `1` if any finding of any severity is detected.

---

## 9. CI/CD Integration and Pipeline Best Practices

### 9.1 Non-Interactive Terminal Detection

KubeCheck inspects its standard streams and environment variables:
* If `stdout` is redirected to a file or pipe (not a TTY), or if the `CI=true` / `GITHUB_ACTIONS=true` environment variable is set:
  * Dynamic terminal spinners, progress meters, and carriage returns (`\r`) are automatically disabled.
  * ANSI colors are disabled unless explicitly forced via `--color always`.
  * Clean findings are emitted as a single final batch.

### 9.2 GitHub Actions Example

```yaml
name: Kubernetes Manifest Validation
on: [pull_request, push]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install KubeCheck
        run: |
          curl -sSL -O https://github.com/kubecheck/kubecheck/releases/download/v1.0.0/kubecheck-linux-amd64.tar.gz
          tar -xzf kubecheck-linux-amd64.tar.gz
          sudo mv kubecheck /usr/local/bin/

      - name: Run KubeCheck SARIF Scan
        run: |
          kubecheck analyze -R ./k8s/ \
            --target-version 1.29 \
            --output sarif > kubecheck-results.sarif
        continue-on-error: true # Allow upload step to execute

      - name: Upload SARIF to GitHub Code Scanning
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: kubecheck-results.sarif

      - name: Enforce Exit Code Gate
        run: |
          kubecheck analyze -R ./k8s/ --target-version 1.29 --fail-on error
```

### 9.3 GitLab CI Example

```yaml
kubecheck_scan:
  stage: test
  image: alpine:latest
  before_script:
    - apk add --no-cache curl tar
    - curl -sSL -O https://github.com/kubecheck/kubecheck/releases/download/v1.0.0/kubecheck-linux-amd64.tar.gz
    - tar -xzf kubecheck-linux-amd64.tar.gz -C /usr/local/bin/
  script:
    - kubecheck analyze -R ./deployments/ --output junit > junit-report.xml
  artifacts:
    when: always
    reports:
      junit: junit-report.xml
```

---

## 10. Security Redaction, Terminal Safety, and Output Limits

### 10.1 Secret Redaction Policy

Per `PRIVACY-REQ-018`, KubeCheck actively protects credentials from leaking into CI logs or developer terminals:
1. Any manifest field identified as a sensitive credential (e.g., `Secret.data.*`, `Secret.stringData.*`, keys containing `password`, `token`, `secret`, `private_key`) is redacted.
2. In source snippet context windows, the literal values are masked:
   ```yaml
   data:
     api-token: [REDACTED]
   ```
3. Secret redaction cannot be disabled via CLI flags in automated CI environments (`CI=true`).

### 10.2 Terminal Sanitization (Escape Injection Defense)

To protect terminal emulators from malicious ANSI escape sequence injection (CSI injection attacks), KubeCheck filters all raw strings extracted from manifest files:
* Non-printable ASCII characters ($< \text{0x20}$) and escape codes ($\text{0x1B}$) in manifest names, annotations, and values are converted to safe unicode representations (`\u001b`) before rendering.

### 10.3 Output Size Limits

To prevent CI runner log buffer exhaustion or browser crashes when scanning thousands of malformed resources:
* `--max-findings <N>` defaults to `500`.
* If finding count exceeds the ceiling, KubeCheck truncates output and emits an explicit notice on `stderr`:
  `warning: output truncated after 500 findings (use --max-findings to increase limit)`.
* Truncation does not alter the exit code: if errors were found, exit code `1` is still returned.
