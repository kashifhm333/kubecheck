# KubeCheck — YAML Security and Parser Policy

**Document ID:** YAML-SEC-001  
**File:** `docs/yaml/YAML_SECURITY_AND_PARSER_POLICY.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Application Security Engineering  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/yaml/YAML_LANGUAGE_SPEC.md`
- `docs/yaml/YAML_PARSING_AND_EDGE_CASES.md`
- `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md`
- `docs/architecture/SYSTEM_ARCHITECTURE.md`
- `docs/architecture/REQUIREMENTS_AND_NFRS.md`
- `docs/architecture/ADR/ADR-002-yaml-parser-selection.md`
- `docs/security/SECURITY_MODEL.md`
- `docs/security/THREAT_MODEL.md`
- `docs/security/DEPENDENCY_POLICY.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/cli/CONFIGURATION.md`
- `docs/testing/DETERMINISM_TESTS.md`

---

## 1. Purpose and scope

This document defines KubeCheck's **YAML security policy**: the set of preventive controls, detection methods, resource limits, and recovery behaviors that govern how KubeCheck processes untrusted YAML input. It translates the threat model into parser-level policy and establishes safe defaults that cannot be weakened without explicit, audited override.

**In scope**

- Safe parsing and prevention of arbitrary object instantiation.
- Alias expansion, recursion, and billion-laughs mitigation.
- Resource exhaustion: nesting depth, collection sizes, scalar lengths, document counts, CPU, memory.
- Malformed UTF-8, invalid encodings, parser crashes.
- Duplicate-key ambiguity and parser differentials.
- Unsafe custom tags and unsupported extensions.
- Path traversal, symlink handling, file discovery, stdin behavior.
- Secret exposure through parser errors, logs, temporary files, debug output.
- Dependency vulnerabilities, parser upgrades, fuzzing, crash triage.
- Fail-closed versus partial-result behavior.
- Threat-to-mitigation matrix with risk, preconditions, preventive controls, detection, tests, and residual risk.
- Safe defaults and the policy for any future parser configuration override.

**Out of scope**

- Concrete parser selection (ADR-002, **OPEN**).
- Bundle format and signature scheme (ADR-003, **OPEN**).
- Diagnostic rule semantics (see `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`).
- Network isolation guarantees (see `SYSTEM_ARCHITECTURE.md` §10, `REQUIREMENTS_AND_NFRS.md` §4).

**Governing rule:**

> **SEC-YAML-REQ-001 — No parser feature or configuration is permitted to weaken the safe defaults in this document without an explicit, recorded, and audited override** (§15). Any weakening is an implementation decision, not a YAML specification requirement.

---

## 2. Normative references and conventions

| Reference | Scope | Type | Retrieved |
|---|---|---|---|
| YAML 1.2.2 specification | Safe-mode parsing, tag semantics, alias scope | Specification | 2026-10-31 |
| CWE-502: Deserialization of Untrusted Data | Arbitrary object instantiation risk | Weakness taxonomy | 2026-10-31 |
| CWE-400: Uncontrolled Resource Consumption | Exhaustion risk | Weakness taxonomy | 2026-10-31 |
| CWE-22: Path Traversal | File-discovery and archive risk | Weakness taxonomy | 2026-10-31 |
| CWE-209: Generation of Error Message Containing Sensitive Information | Secret exposure via parser errors | Weakness taxonomy | 2026-10-31 |
| OSS-Fuzz YAML parser findings | Fuzzing evidence and crash classes | Community fuzzing | 2026-10-31 |
| L4YAML security analysis (NASA JPL) | Reference parser limits and DoS/ACE taxonomy | Open-source implementation | 2026-10-31 |

**Conventions:**

- Requirement IDs use `SEC-YAML-REQ-NNN`.
- Threat IDs use `THR-YAML-NNN`.
- Test IDs use `TEST-YAML-SEC-NNN`; the mapping to executable tests is maintained in `docs/testing/REQUIREMENT_TRACEABILITY.md`.
- Severity uses KubeCheck's diagnostic severity: `error`, `warning`, `info`.
- **Fail-closed** means the affected document or invocation is rejected with a structured error and no partial results are presented as complete.
- **Fail-open** is prohibited for every control in this document.

---

## 3. Threat model summary

KubeCheck treats **every byte of input** — manifest files, directory entries, archive members, standard input, configuration files, knowledge bundles, and rule metadata — as potentially adversarial. The threat model is structured around four attacker goals:

| Goal | Description | Representative threats |
|---|---|---|
| **G1. Code execution** | Cause KubeCheck to execute attacker-controlled code during parsing or analysis. | Arbitrary object instantiation via unsafe tags; shell injection through manifest content; malicious rule pack. |
| **G2. Denial of service** | Cause KubeCheck to exhaust CPU, memory, or time, disrupting CI pipelines or developer workstations. | Billion-laughs alias bombs; deep nesting; oversized scalars; huge collections; parser quadratic behavior. |
| **G3. Information disclosure** | Cause KubeCheck to leak secret material to logs, reports, temporary files, or stderr. | Secret values echoed in parse errors; debug output containing manifest content; temporary files with world-readable permissions. |
| **G4. Integrity / policy bypass** | Cause KubeCheck to accept input it should reject, or produce divergent results across consumers. | Duplicate-key ambiguity; parser differentials; path traversal that reads files outside the intended root. |

**SEC-YAML-REQ-002:** Every preventive control in this document MUST map to at least one of G1–G4. Controls that do not map to a goal are not security controls and MUST be documented as usability or correctness features instead.

---

## 4. Safe parsing and prevention of arbitrary object instantiation

### 4.1 The risk

YAML parsers that support arbitrary object construction interpret tags such as `!!python/object/apply`, `!!python/object/new`, `!!ruby/object`, or language-specific equivalents as instructions to instantiate classes and invoke methods during deserialization. When such a parser processes untrusted input, the attacker can achieve arbitrary code execution. CVE-2026-84202 documents a concrete instance: ModelScope used PyYAML's unsafe `yaml.Loader`, and malicious model configuration files could execute arbitrary Python code through object-construction tags[reference:0]. The same class of vulnerability appears across ecosystems: Cassandra's SnakeYAML configuration loading was hardened to restrict instantiable types[reference:1], and security tooling consistently classifies `yaml.load`, `yaml.unsafe_load`, `yaml.Loader`, and `yaml.UnsafeLoader` as unsafe for untrusted input[reference:2].

### 4.2 KubeCheck policy

**SEC-YAML-REQ-003 (safe loader only):** KubeCheck MUST use a safe parsing configuration that restricts node construction to the Core Schema types (`string`, `integer`, `float`, `boolean`, `null`, `mapping`, `sequence`) and opaque application-tagged scalars. It MUST NOT use any loader that can instantiate host-language objects from tags.

**SEC-YAML-REQ-004 (tag allowlist):** The parser MUST operate with an explicit tag allowlist. Tags not on the allowlist MUST be rejected with a parse error before any construction occurs. The allowlist comprises:

| Allowed tag | Purpose |
|---|---|
| `!!str`, `!!int`, `!!float`, `!!bool`, `!!null` | Core Schema scalar types |
| `!!map`, `!!seq` | Core Schema collection types |
| `!<verbatim-tag>` | Preserved as opaque; no constructor resolution |
| Local tags (`!custom`) | Preserved as opaque with a warning; no constructor resolution |

**SEC-YAML-REQ-005 (no constructor resolution):** KubeCheck MUST NOT register, enable, or fall back to any constructor that invokes host-language code. A parser that silently falls back to an unsafe loader on parse failure (as documented in CVE-2026-52903, where a module overrode `safe_load` to fall back to `unsafe_load`) is **not acceptable** for KubeCheck and MUST be rejected in ADR-002.

**SEC-YAML-REQ-006 (independent verification):** Safe-mode configuration MUST be verified independently of the parser's default behavior. A test MUST assert that a document containing `!!python/object/apply`, `!!ruby/object`, and equivalent constructor-resolving tags produces a parse error and does not execute code. This verification MUST run in CI on every parser upgrade.

### 4.3 Detection

- **Static analysis gate:** CI MUST reject any source reference to `yaml.load`, `yaml.unsafe_load`, `Loader`, `UnsafeLoader`, or equivalent APIs in the parsing module.
- **Adversarial corpus:** A corpus containing known constructor-resolving tags for Python, Ruby, Java, and .NET MUST be executed on every build and every parser upgrade. Any corpus item that produces code execution (detected by a canary file write or network call) is a release-blocking defect.
- **Runtime assertion:** The parser wrapper MUST assert at initialization that the active loader is a safe loader and MUST fail closed if the assertion fails.

### 4.4 Residual risk

A parser upgrade could introduce a new constructor path that the allowlist does not anticipate. Residual risk is mitigated by the adversarial corpus and the independent verification requirement (`SEC-YAML-REQ-006`), but is not eliminated. ADR-002 MUST record the parser's exact constructor model and any known gaps.

---

## 5. Alias expansion and recursion limits

### 5.1 The risk

YAML anchors (`&name`) and aliases (`*name`) allow a document to reference a previously defined node. A nested alias structure can expand exponentially: a document of a few hundred bytes can produce hundreds of thousands or billions of nodes when aliases are resolved. This is the **billion-laughs** attack, a denial-of-service vector that has affected PyYAML, ruamel.yaml, goccy/go-yaml, Microsoft.OpenAPI.NET (CVE-2026-72923), Symfony's YAML parser (CVE-2026-45304), and others[reference:3][reference:4]. The damage often lands **after parsing**, when a later stage walks the resolved graph as a tree and copies every alias[reference:5].

### 5.2 KubeCheck limits

| Limit ID | Limit | Default | Scope | Enforcement |
|---|---|---|---|---|
| `LIM-YAML-ALIAS-01` | `max_aliases` | 10,000 expansions | Per document | Metered during parse; rejects before expansion |
| `LIM-YAML-ALIAS-02` | `max_resolved_nodes` | 100,000 nodes | Per document | Counts every node produced by alias resolution |
| `LIM-YAML-ALIAS-03` | `max_anchor_count` | 1,000 anchors | Per document | Bounds anchor-table memory |
| `LIM-YAML-ALIAS-04` | `max_alias_depth` | 50 levels | Per document | Rejects recursive and deeply nested alias chains |

**SEC-YAML-REQ-007 (pre-expansion metering):** Alias expansion MUST be metered **before** the expanded structure is materialized. A parser that resolves aliases to native values and only then exposes the count is insufficient; the limit MUST be enforced at the point where the parser is about to allocate the expanded node.

**SEC-YAML-REQ-008 (dual limits):** KubeCheck MUST enforce both an alias-count limit and a resolved-node limit. The alias-count limit alone is insufficient because a moderate number of aliases referencing a large anchor can still produce an unbounded expansion. The resolved-node limit closes this gap.

**SEC-YAML-REQ-009 (recursive aliases):** Recursive aliases (`a: &a [*a]`) MUST be rejected with a normalization error identifying the anchor and alias locations. KubeCheck MUST NOT attempt to break the cycle by inserting `null` or truncating the graph (`NORM-REQ-028`).

### 5.3 Parser-dependent behavior

Not all parsers enforce alias limits by default. Published testing shows that PyYAML, ruamel.yaml, and goccy/go-yaml loaded alias bombs with no limit, while the `npmyaml` package, Go's `yaml.v3` and `sigs.k8s.io/yaml`, and Ruby's Psych refused them by default[reference:6]. js-yaml and SnakeYAML have limits, but a bomb built with 48 aliases reportedly passed both[reference:7].

**SEC-YAML-REQ-010:** ADR-002 MUST select a parser that either enforces alias limits natively or exposes alias-resolution metering sufficient for KubeCheck to enforce `LIM-YAML-ALIAS-01` through `LIM-YAML-ALIAS-04` independently. A parser that silently expands aliases without metering is not acceptable.

### 5.4 Detection and tests

- **Bomb corpus:** A set of alias bombs at depths 3, 5, 8, and 10 with width 9 MUST be executed on every build. Each MUST produce a governor finding before memory consumption exceeds a documented threshold.
- **Recursion corpus:** A recursive-alias fixture MUST produce a normalization error naming the anchor.
- **Wide fan-out fixture:** A document with 100 aliases each referencing a 2,000-node anchor MUST be rejected by `max_resolved_nodes` even though `max_aliases` is not exceeded.

---

## 6. Resource exhaustion: nesting, collections, scalars, documents, CPU, memory

### 6.1 Limit register

| Limit ID | Limit | Default | Enforced by | Rationale |
|---|---|---|---|---|
| `LIM-YAML-DEPTH-01` | `max_depth` | 256 levels | Parser / Governor | Prevents stack overflow and runaway recursion. |
| `LIM-YAML-COLL-01` | `max_mapping_pairs` | 100,000 per mapping | Parser / Governor | Bounds mapping memory. |
| `LIM-YAML-COLL-02` | `max_sequence_items` | 100,000 per sequence | Parser / Governor | Bounds sequence memory. |
| `LIM-YAML-SCALAR-01` | `max_scalar_bytes` | 10 MiB per scalar | Parser / Governor | Bounds single-value memory; accommodates large ConfigMaps and certificates. |
| `LIM-YAML-DOC-01` | `max_documents` | 1,000 per invocation | Resource Governor | Bounds aggregate work. |
| `LIM-YAML-DOC-02` | `max_document_bytes` | 50 MiB per document | Governor | Bounds per-document memory distinct from per-input-unit. |
| `LIM-YAML-INPUT-01` | `max_input_bytes` | 10 MiB per input unit | Resource Governor | Bounds decode and parse time. |
| `LIM-YAML-TIME-01` | `max_time_ms` | 300,000 ms (300 s) | Governor | Prevents runaway processing in CI. |
| `LIM-YAML-MEM-01` | `max_memory_mb` | 1,024 MiB | Governor | Bounds aggregate memory. |

**SEC-YAML-REQ-011 (pre-allocation enforcement):** Every limit in §6.1 MUST be enforced **before** the resource is allocated, not after. A limit that is checked only after the parser has already allocated the structure does not prevent exhaustion.

**SEC-YAML-REQ-012 (structured failure):** Exceeding any limit MUST produce a structured governor finding with the limit ID, the observed value, the configured limit, and the document index. The affected document MUST be marked `resource-limited` and the invocation MUST continue for other documents unless an aggregate limit (`LIM-YAML-MEM-01`, `LIM-YAML-TIME-01`) is reached.

### 6.2 CPU exhaustion and parser complexity

Some YAML constructs cause **quadratic or worse** behavior in specific parsers. A documented example is js-yaml's `!!omap` handling, where `omapTag.addItem()` performs a linear duplicate-key scan on every insertion, resulting in O(n²) CPU consumption for a document with n `omap` entries; a small crafted document can trigger a multi-second CPU stall[reference:8]. SnakeYAML has been reported vulnerable to stack-overflow denial of service when parsing untrusted input[reference:9].

**SEC-YAML-REQ-013:** ADR-002 MUST evaluate the candidate parser for known quadratic or stack-exhaustion behaviors on adversarial constructs (`!!omap`, deeply nested flow collections, large simple keys). A parser with an unresolved quadratic-complexity advisory for a construct KubeCheck accepts is not acceptable unless KubeCheck can reject the construct at the allowlist layer.

**SEC-YAML-REQ-014:** The wall-clock limit (`LIM-YAML-TIME-01`) MUST be enforced at a granularity sufficient to interrupt a parser that has entered a long-running loop. The governor MUST check the deadline at least every 100 ms of consumed time or after every N parse events, whichever is sooner.

### 6.3 Tests

- **Depth series:** Documents at depth 255, 256, and 257. The 257-level document MUST produce `DEPTH_LIMIT`.
- **Collection series:** Mappings and sequences at 99,999 / 100,000 / 100,001 entries.
- **Scalar series:** Scalars at 9 MiB, 10 MiB, and 11 MiB.
- **Document-count series:** Streams at 999 / 1,000 / 1,001 documents.
- **Quadratic fixture:** An `!!omap` document with 50,000 entries MUST complete within the documented time budget or be rejected by a construct-level restriction.

---

## 7. Malformed UTF-8, invalid encodings, and parser crashes

### 7.1 Encoding policy

**SEC-YAML-REQ-015 (UTF-8 only):** KubeCheck MUST attempt UTF-8 decoding of every input unit. A leading UTF-8 BOM MUST be stripped. If the byte sequence is not valid UTF-8, KubeCheck MUST reject the input with a parse error identifying the byte offset of the first invalid sequence and MUST NOT attempt heuristic encoding detection (`NORM-REQ-003`).

**SEC-YAML-REQ-016 (no silent fallback):** KubeCheck MUST NOT fall back to a permissive decoder, a locale-dependent decoder, or a lossy replacement policy (`U+FFFD`) on invalid UTF-8. Rejection is required so that the user can correct the encoding.

### 7.2 Parser crash classes

OSS-Fuzz and similar programs have found heap buffer overflows, stack overflows, and null-dereferences in YAML parsers across languages. A representative finding is OpenCV's `parseKey()` heap buffer over-read on an empty key, found by fixing an upstream fuzzer that had its core API call commented out[reference:10]. SnakeYAML has documented stack-overflow DoS issues[reference:11].

**SEC-YAML-REQ-017 (crash containment):** A parser crash (panic, abort, unhandled exception, or segmentation fault) MUST NOT terminate the invocation without a structured finding. The parser MUST be invoked behind a crash barrier that converts a crash into a structured `PARSER_CRASH` finding with the document index and, where available, the last parsed location.

**SEC-YAML-REQ-018 (fuzzing):** The selected parser MUST be fuzzed continuously in KubeCheck's CI against KubeCheck's safe-mode configuration. A crash in the fuzz harness is a release-blocking defect (`BLK-REQ-002`). The fuzz corpus MUST include the adversarial constructs from `YAML_PARSING_AND_EDGE_CASES.md` §15.

### 7.3 Crash triage

| Crash class | Triage action | Release impact |
|---|---|---|
| Panic / abort on malformed input | Reproduce with minimal fixture; report upstream if parser-internal; add fixture to regression suite; verify crash barrier produces structured finding. | Release-blocking if crash barrier is bypassed. |
| Segfault / memory corruption | Halt release; report upstream; evaluate alternative parser; do not ship until fixed or parser replaced. | Release-blocking. |
| Stack overflow | Check `max_depth` and `max_alias_depth` enforcement; if the parser overflows below the limit, lower the limit or replace the parser. | Release-blocking if the limit cannot prevent the overflow. |
| Hang / infinite loop | Check wall-clock limit enforcement; add fixture; verify governor interrupts. | Release-blocking if the governor cannot interrupt. |

---

## 8. Duplicate-key ambiguity and parser differentials

### 8.1 The risk

The YAML 1.2.2 specification requires mapping keys to be **unique** (§3.2.1.3). Many parsers nevertheless allow duplicate keys and resolve them with first-wins or last-wins semantics. This creates a **parser differential**: two services parsing the same input can observe different values for the same key. The Trail of Bits analysis of Go parsers documents duplicate-key handling as a first-class parser differential, noting that JSON and YAML parsers commonly accept duplicates with last-wins while others reject or behave differently[reference:12]. L4YAML explicitly separates duplicate-key acceptance from resource limits, configuring it via a `DuplicateKeyPolicy` rather than through `ParserLimits`[reference:13].

### 8.2 KubeCheck policy

**SEC-YAML-REQ-019 (reject duplicates):** KubeCheck MUST reject duplicate mapping keys at parse time, before normalization, with a finding that identifies **both** occurrences and the canonical form that is equal under the Core Schema (`YAML-SPEC-REQ-016`, `EDGE-REQ-009`).

**SEC-YAML-REQ-020 (parser-policy independence):** Duplicate-key detection MUST be verified independently of the parser's configured duplicate policy. A parser configured for last-wins MUST still produce a KubeCheck duplicate-key rejection. This is a testable requirement (`EDGE-REQ-011`).

**SEC-YAML-REQ-021 (Core Schema equality):** Duplicate detection MUST use Core Schema canonical equality, not textual equality. `0xB` and `11` are the same integer key and MUST be flagged; `"11"` and `11` are different keys and MUST NOT be flagged (`YAML-SPEC-REQ-018`).

### 8.3 Detection and tests

- **Dual-configuration test:** Run the duplicate-key fixture against a parser configured for first-wins and one configured for last-wins. Both MUST produce `DUPLICATE_KEY`.
- **Canonical-equality test:** `0xB` and `11` MUST be flagged; `"11"` and `11` MUST NOT be flagged.
- **Parser-differential canary:** A fixture designed to produce different values under first-wins and last-wins MUST be rejected, so that KubeCheck cannot silently adopt either parser's resolution.

---

## 9. Unsafe custom tags and unsupported extensions

### 9.1 Tag policy

| Tag class | Example | KubeCheck policy | Rationale |
|---|---|---|---|
| Core Schema tags | `!!str`, `!!int`, `!!map` | **ACCEPT** | Required for Kubernetes manifests. |
| Verbatim tags | `!<tag:example.com,2026:foo>` | **ACCEPT AS OPAQUE** | Preserved in NIR; no constructor resolution. |
| Local tags | `!custom` | **ACCEPT AS OPAQUE WITH WARNING** | Preserved; no constructor resolution. |
| Constructor-resolving tags | `!!python/object/apply`, `!!ruby/object` | **REJECT** | Safe-mode requirement (`SEC-YAML-REQ-003`). |
| Directives with undeclared handles | `%TAG !e! ...` with no matching use | **REJECT** | Prevents tag-handle confusion. |

**SEC-YAML-REQ-022 (opaque preservation):** Application-specific and local tags MUST be preserved as opaque scalars in the NIR with their original tag string. KubeCheck MUST NOT attempt to resolve them to a native type, and MUST NOT interpret them as constructor instructions.

**SEC-YAML-REQ-023 (warning on local tags):** A local tag (`!custom`) that appears in a document MUST produce a `warning`-severity finding stating that the tag is preserved as opaque and that KubeCheck cannot validate its semantics. The finding MUST include the tag string and its location.

### 9.2 Unsupported extensions

The **merge key** (`<<`) is an extension, not part of the YAML 1.2.2 Core Schema. KubeCheck accepts it as an extension and documents this explicitly (`YAML-SPEC-REQ-013`). An undefined merge source, or a merge source that is not a mapping or sequence of mappings, MUST be rejected with `BAD_MERGE` (`EDGE-REQ-016`). Recursive merge expansion MUST be rejected with `MERGE_CYCLE` (`EDGE-REQ-020`).

**SEC-YAML-REQ-024:** Extensions accepted by KubeCheck MUST be enumerated in ADR-002 with their semantic contract. An extension not enumerated in ADR-002 MUST be treated as an opaque tag and MUST NOT be silently interpreted.

---

## 10. Path traversal, symlinks, file discovery, stdin

### 10.1 The risk

Path traversal and symlink attacks against Kubernetes-adjacent tooling are well documented. Argo CD's Jsonnet importer did not confine `import`, `importstr`, or `importbin` to the repository root; absolute paths, parent-directory traversal, and repository symlinks could open `/proc/self/environ`, service-account tokens, and private keys[reference:14]. Podman's `kube play` was found to overwrite host files via symlinks in ConfigMap and Secret volume mounts (CVE-2025-9566, GHSA-wp3j-xq48-xpjw)[reference:15][reference:16]. Helm had a path traversal and symlink issue via crafted `Chart.yaml` and `Chart.lock` files (CVE-2025-53547)[reference:17].

### 10.2 KubeCheck controls

**SEC-YAML-REQ-025 (root confinement):** File discovery MUST resolve every admitted file to a path that is a descendant of an explicitly supplied root. A path that escapes the root after symbolic-link resolution MUST be rejected with a `PATH_ESCAPE` finding and MUST NOT be read.

**SEC-YAML-REQ-026 (symlink policy):** Symbolic links MUST NOT be followed by default. Following symlinks requires an explicit opt-in flag documented in `docs/cli/COMMAND_REFERENCE.md`. When following is enabled, each resolved target MUST be re-checked against the root confinement rule.

**SEC-YAML-REQ-027 (archive safety):** Archive members (if archives are supported) MUST be rejected if their normalized path escapes the extraction root (zip-slip), if they are absolute paths, if they contain `..` segments after normalization, or if they are device files, FIFOs, or other non-regular-file entries.

**SEC-YAML-REQ-028 (stdin):** Standard input MUST be treated as a single stream with no path. It MUST NOT be used to resolve relative paths for other inputs, and it MUST be subject to the same size and resource limits as file input.

**SEC-YAML-REQ-029 (no ambient file access):** KubeCheck MUST NOT read any file not explicitly supplied by the user or not part of the documented configuration and bundle discovery. In particular, it MUST NOT read `KUBECONFIG`, cloud credential files, `/proc/self/environ`, or home-directory secret files during analysis.

### 10.3 Tests

- **Traversal corpus:** Paths containing `../`, absolute paths, and Windows drive-prefixed paths MUST be rejected or normalized and re-checked.
- **Symlink corpus:** A symlink inside the root pointing outside the root MUST NOT be followed by default.
- **Zip-slip corpus:** An archive member named `../../etc/passwd` MUST be rejected.
- **`KUBECONFIG` canary:** A test that sets `KUBECONFIG` to a nonexistent path MUST pass and MUST NOT consult the path (`BENCH-REQ-006`).

---

## 11. Secret exposure through errors, logs, temporary files, debug output

### 11.1 The risk

Manifests frequently contain secret material in `Secret.data`, `Secret.stringData`, `ConfigMap` values, environment variables, and annotations. A tool that echoes manifest content in parse errors, logs, or debug output can leak those values into CI logs, artifact stores, or third-party services. A documented pattern is the online YAML linter that "silently logs the AWS access key embedded in your ConfigMap, the database credentials in your Helm values, and the JWT secret in your deployment spec" while responding with a helpful parse tree[reference:18]. Argo CD has had secret-exposure vulnerabilities in patch errors and diff views when syncing invalid Secret resources[reference:19].

### 11.2 KubeCheck controls

**SEC-YAML-REQ-030 (no content in errors):** Parser and normalization errors MUST NOT include the text of the offending scalar. An error MUST identify the location (line, column, JSON path), the error code, and a structural description (e.g., "expected mapping value, found sequence entry"), but MUST NOT echo the value.

**SEC-YAML-REQ-031 (secret-field redaction):** When a finding must reference a secret-bearing field — for example, a schema error on `Secret.stringData.password` — the evidence field MUST identify the field path and the structural fact (present/absent, wrong type), and MUST NOT include the value. The secret-bearing field set is defined in `docs/security/THREAT_MODEL.md` and includes at minimum `Secret.data`, `Secret.stringData`, and fields matching the documented sensitive-name patterns.

**SEC-YAML-REQ-032 (no manifest content in logs by default):** The logging sink MUST NOT emit manifest scalar values at any log level by default. Debug mode MAY emit structural paths and node identifiers but MUST NOT emit scalar values from secret-bearing fields (`SEC-REQ-013`). Any future option to emit scalar values MUST require dual opt-in and MUST redact secret-bearing fields.

**SEC-YAML-REQ-033 (no temporary files with manifest content):** KubeCheck MUST NOT write manifest content to temporary files. If a future feature requires intermediate serialization, it MUST use an in-memory buffer or a file created with restrictive permissions and deleted on exit. The write set remains as specified in `PLAT-REQ-012`.

**SEC-YAML-REQ-034 (stream separation):** Report output (`stdout`) and diagnostic logs (`stderr`) MUST be separated so that machine consumers of the report are not exposed to log content (`SEC-REQ-015`).

### 11.3 Detection and tests

- **Secret-bearing corpus:** A corpus containing canary secret values in `Secret.data`, `Secret.stringData`, and sensitive-named fields MUST be analyzed at every log level and in every output format. The canary values MUST NOT appear in `stdout`, `stderr`, or any temporary file.
- **Error-message audit:** Every parse and normalization error MUST be reviewed to confirm it contains no scalar content.
- **Debug-mode audit:** Debug output MUST be inspected over the secret-bearing corpus to confirm redaction.

---

## 12. Dependency vulnerabilities, parser upgrades, fuzzing, crash triage

### 12.1 Dependency policy

**SEC-YAML-REQ-035 (pinning):** The YAML parser and all transitive dependencies MUST be pinned to exact versions in the build manifest (`SEC-REQ-010`). The parser version and its vulnerability status MUST be recorded in the release notes.

**SEC-YAML-REQ-036 (vulnerability scanning):** The release pipeline MUST run a dependency vulnerability scan. A parser with a known exploitable vulnerability at or above the documented severity threshold MUST block release (`SEC-REQ-011`).

**SEC-YAML-REQ-037 (upgrade as ADR):** A parser version upgrade that changes any behavior in `YAML_PARSING_AND_EDGE_CASES.md` §10 MUST be treated as an architectural decision requiring an ADR (`EDGE-REQ-043`). A patch upgrade that changes only performance is exempt but MUST pass the full edge-case and security corpus.

### 12.2 Fuzzing

**SEC-YAML-REQ-038 (continuous fuzzing):** The parser MUST be fuzzed continuously in CI against KubeCheck's safe-mode configuration, using a harness that exercises the full decode → parse → normalize path. The fuzz corpus MUST include the adversarial constructs from `YAML_PARSING_AND_EDGE_CASES.md` §15 and the constructor-resolving tag corpus from §4.3.

**SEC-YAML-REQ-039 (OSS-Fuzz participation):** If the selected parser participates in OSS-Fuzz or an equivalent program, KubeCheck MUST monitor its advisories and incorporate new crash classes into its regression corpus. The OpenCV `parseKey()` finding illustrates the value of this monitoring: the crash was found only after an upstream fuzzer was fixed to actually call the parser API[reference:20].

### 12.3 Crash triage process

1. **Detect:** Fuzz harness, adversarial corpus, or production crash report produces a fixture.
2. **Minimize:** Reduce the fixture to the smallest input that reproduces the crash.
3. **Classify:** Panic, abort, segfault, stack overflow, or hang (§7.3).
4. **Contain:** Verify the crash barrier produces a structured finding rather than terminating the invocation.
5. **Report:** If the crash is parser-internal, report upstream with the minimized fixture.
6. **Regress:** Add the fixture to the regression suite and the fuzz corpus.
7. **Gate:** If the crash bypasses the crash barrier or a limit, block release until fixed or until a construct-level restriction is added.

---

## 13. Fail-closed versus partial-result behavior

### 13.1 Fail-closed conditions

**SEC-YAML-REQ-040:** KubeCheck MUST fail closed — reject the affected scope and produce no result that could be mistaken for a successful analysis — under the following conditions:

| Condition | Scope | Rationale |
|---|---|---|
| Safe-loader assertion fails at startup | Entire invocation | The parser is not in safe mode; no input can be trusted. |
| Active bundle signature verification fails | Entire invocation | An unverified bundle cannot be used (`SEC-REQ-008`). |
| Aggregate memory limit reached | Entire invocation | Further parsing could exhaust the host. |
| Aggregate wall-clock limit reached | Entire invocation | Further parsing could stall CI. |
| Invalid UTF-8 before any document boundary | Entire stream | The remainder cannot be decoded reliably. |

### 13.2 Safe partial results

**SEC-YAML-REQ-041:** KubeCheck MAY produce partial results — findings for completed documents plus a structured status for incomplete documents — under the following conditions, provided every incomplete document is explicitly marked with a non-`processed` status:

| Condition | Scope | Marking |
|---|---|---|
| Lexical or structural error in one document | That document | `parse-failed` |
| Duplicate key in one document | That document | `parse-failed` |
| Unsafe tag in one document | That document | `parse-failed` |
| Recursive alias or merge cycle in one document | That document | `normalization-failed` |
| Per-document resource limit exceeded | That document | `resource-limited` |
| GVK error in one document | That document | `gvk-failed` |

**SEC-YAML-REQ-042 (no silent completeness):** A partial result MUST NOT be presented as a complete analysis. The report MUST include a `document_status` per document and a `stream_status` indicating whether any document was skipped or failed (`EDGE-REQ-042`, `EDGE-REQ-055`).

**SEC-YAML-REQ-043 (exit code reflects partiality):** The exit code MUST reflect the highest-severity finding, including governor and parse findings, so that a CI pipeline cannot mistake a partial result for a clean pass (`OUT-REQ-007`).

---

## 14. Threat-to-mitigation matrix

| Threat ID | Risk | Preconditions | Preventive controls | Detection methods | Tests | Residual risk |
|---|---|---|---|---|---|---|
| **THR-YAML-001** | Arbitrary code execution via constructor-resolving tags | Attacker controls manifest input; parser uses unsafe loader | Safe-loader only (`SEC-YAML-REQ-003`); tag allowlist (`SEC-YAML-REQ-004`); no constructor resolution (`SEC-YAML-REQ-005`) | Static-analysis gate; adversarial tag corpus; safe-loader assertion | `TEST-YAML-SEC-001` | Parser upgrade introduces new constructor path; mitigated by independent verification. |
| **THR-YAML-002** | Billion-laughs alias expansion DoS | Attacker controls manifest; parser expands aliases without metering | `max_aliases`, `max_resolved_nodes`, `max_anchor_count`, `max_alias_depth` (`SEC-YAML-REQ-007`–`009`) | Bomb corpus; pre-expansion metering assertion | `TEST-YAML-SEC-002` | Wide fan-out below alias limit; mitigated by resolved-node limit. |
| **THR-YAML-003** | Stack overflow via deep nesting | Attacker controls manifest; parser recurses | `max_depth` (`SEC-YAML-REQ-011`); crash barrier (`SEC-YAML-REQ-017`) | Depth series; crash triage | `TEST-YAML-SEC-003` | Parser overflows below configured limit; mitigated by lowering limit or replacing parser. |
| **THR-YAML-004** | CPU exhaustion via quadratic parser behavior | Attacker controls manifest; parser has O(n²) construct | Construct allowlist; wall-clock limit (`SEC-YAML-REQ-014`) | Quadratic fixture; time-limit test | `TEST-YAML-SEC-004` | Undocumented quadratic behavior in a permitted construct; mitigated by fuzzing and upstream monitoring. |
| **THR-YAML-005** | Memory exhaustion via oversized scalar or collection | Attacker controls manifest | `max_scalar_bytes`, `max_mapping_pairs`, `max_sequence_items`, `max_document_bytes` (`SEC-YAML-REQ-011`) | Limit series; memory sampling | `TEST-YAML-SEC-005` | Limit defaults too permissive for a specific environment; configurable but safe by default. |
| **THR-YAML-006** | Duplicate-key ambiguity leading to divergent values | Attacker controls manifest; downstream consumers use different parsers | Reject duplicates (`SEC-YAML-REQ-019`); parser-policy independence (`SEC-YAML-REQ-020`); Core Schema equality (`SEC-YAML-REQ-021`) | Dual-configuration test; canonical-equality test | `TEST-YAML-SEC-006` | A downstream consumer accepts duplicates; KubeCheck's rejection surfaces the issue before deployment. |
| **THR-YAML-007** | Path traversal or symlink escape reading arbitrary files | Attacker controls file paths or archive contents | Root confinement (`SEC-YAML-REQ-025`); symlink policy (`SEC-YAML-REQ-026`); archive safety (`SEC-YAML-REQ-027`); no ambient file access (`SEC-YAML-REQ-029`) | Traversal corpus; symlink corpus; zip-slip corpus; `KUBECONFIG` canary | `TEST-YAML-SEC-007` | Platform-specific symlink semantics; mitigated by re-checking resolved targets. |
| **THR-YAML-008** | Secret exposure via error messages, logs, or debug output | Attacker (or careless user) supplies manifest with secret; tool echoes content | No content in errors (`SEC-YAML-REQ-030`); secret-field redaction (`SEC-YAML-REQ-031`); no content in logs (`SEC-YAML-REQ-032`); no temp files (`SEC-YAML-REQ-033`) | Secret-bearing corpus; error-message audit; debug-mode audit | `TEST-YAML-SEC-008` | A future feature adds a new output channel without redaction; mitigated by the redaction test suite. |
| **THR-YAML-009** | Parser crash terminating invocation without structured finding | Malformed input triggers parser defect | Crash barrier (`SEC-YAML-REQ-017`); continuous fuzzing (`SEC-YAML-REQ-038`) | Fuzz harness; crash triage | `TEST-YAML-SEC-009` | Crash in a path not covered by the barrier; mitigated by barrier placement at every parser entry point. |
| **THR-YAML-010** | Dependency vulnerability in parser or transitive library | Upstream vulnerability disclosed | Version pinning (`SEC-YAML-REQ-035`); vulnerability scanning (`SEC-YAML-REQ-036`); upgrade as ADR (`SEC-YAML-REQ-037`) | Scan report per release; advisory monitoring | `TEST-YAML-SEC-010` | Zero-day without an advisory; mitigated by fuzzing and least-privilege execution. |
| **THR-YAML-011** | Unsupported extension misinterpreted as a supported construct | Manifest uses an extension KubeCheck does not model | Extension enumeration in ADR-002 (`SEC-YAML-REQ-024`); opaque-tag policy (`SEC-YAML-REQ-022`); local-tag warning (`SEC-YAML-REQ-023`) | Extension fixture suite; opaque-tag assertion | `TEST-YAML-SEC-011` | A future extension is added without updating ADR-002; mitigated by the enumeration requirement. |
| **THR-YAML-012** | Partial result mistaken for complete analysis | One document fails while others succeed | Fail-closed conditions (`SEC-YAML-REQ-040`); explicit status marking (`SEC-YAML-REQ-041`); no silent completeness (`SEC-YAML-REQ-042`); exit code reflects partiality (`SEC-YAML-REQ-043`) | Status-field validation; exit-code matrix | `TEST-YAML-SEC-012` | A downstream consumer ignores `document_status`; mitigated by exit-code contract and documentation. |

**SEC-YAML-REQ-044:** Every threat in §14 MUST have at least one preventive control, one detection method, and one test. A threat without a test is an unverified control and MUST be recorded as `OPEN` in the traceability matrix.

---

## 15. Safe defaults and parser configuration override policy

### 15.1 Safe defaults

The defaults in §5.2 and §6.1 are the **normative safe defaults**. They are chosen to accommodate typical Kubernetes manifests while preventing the threats in §14. They are recorded here as a single register.

| Default | Value | Rationale |
|---|---|---|
| Safe loader | Enabled, non-overridable | `SEC-YAML-REQ-003` |
| Tag allowlist | Core Schema + opaque extensions | `SEC-YAML-REQ-004` |
| `max_aliases` | 10,000 | Billion-laughs mitigation; aligns with community defaults (`9†L19-L20`). |
| `max_resolved_nodes` | 100,000 | Wide fan-out mitigation. |
| `max_anchor_count` | 1,000 | Anchor-table memory bound. |
| `max_alias_depth` | 50 | Recursive and deep-chain alias bound. |
| `max_depth` | 256 | Stack-overflow mitigation. |
| `max_scalar_bytes` | 10 MiB | Large ConfigMap/certificate accommodation. |
| `max_mapping_pairs` | 100,000 | Mapping memory bound. |
| `max_sequence_items` | 100,000 | Sequence memory bound. |
| `max_documents` | 1,000 | Aggregate work bound. |
| `max_document_bytes` | 50 MiB | Per-document memory bound. |
| `max_input_bytes` | 10 MiB | Per-input-unit bound. |
| `max_time_ms` | 300,000 | CI wall-clock bound. |
| `max_memory_mb` | 1,024 | Aggregate memory bound. |
| Duplicate keys | Reject | Specification requirement and parser-differential mitigation. |
| Symlinks | Not followed | Path-traversal mitigation. |
| Secret redaction | Always on | `SEC-YAML-REQ-031`. |

**SEC-YAML-REQ-045:** The safe defaults MUST be documented in `docs/cli/CONFIGURATION.md` and MUST be reflected in the benchmark environment profile. Changing a default requires an ADR because it changes KubeCheck's security envelope.

### 15.2 Override policy

**SEC-YAML-REQ-046 (overridable vs non-overridable):** The following controls are **non-overridable** in any configuration:

| Control | Reason |
|---|---|
| Safe loader | No configuration may enable an unsafe loader. |
| Constructor-resolving tag rejection | No configuration may allow constructor tags. |
| Secret redaction | No configuration may disable redaction of secret-bearing fields. |
| Duplicate-key rejection | No configuration may enable last-wins or first-wins resolution. |
| Crash barrier | No configuration may disable crash containment. |

**SEC-YAML-REQ-047 (overridable with audit):** The following controls MAY be overridden only through an explicit configuration key, with a warning emitted to `stderr` on activation, and with the override recorded in the report metadata:

| Control | Override conditions |
|---|---|
| Resource limits (`LIM-YAML-*`) | Explicit key; `unlimited` requires a separate key and emits a warning (`LIM-REQ-008`). |
| Symlink following | Explicit opt-in flag; resolved targets re-checked against root confinement. |
| Debug output of structural paths | Explicit flag; secret-bearing fields remain redacted. |

**SEC-YAML-REQ-048 (no environment-variable overrides):** Security-relevant overrides MUST NOT be settable through environment variables alone. Environment variables are inherited implicitly and can be set by an attacker who controls the CI environment; overrides must be explicit in the invocation or the configuration file, and must be visible in `--print-effective-config` output.

**SEC-YAML-REQ-049 (override provenance):** When an override is active, the report header MUST include the override key, its value, and a warning that the safe default was changed. This allows a CI consumer to detect that a run was performed under non-default security settings.

### 15.3 Future parser configuration override

If a future release permits parser-level configuration (e.g., a different alias limit implementation, a custom tag handler, or a parser plugin), the following constraints MUST hold:

1. The configuration MUST be validated against a schema; unknown keys are rejected.
2. The configuration MUST NOT be able to enable an unsafe loader, disable duplicate-key rejection, or disable secret redaction (`SEC-YAML-REQ-046`).
3. The configuration MUST be recorded in the report metadata and MUST be reproducible from the report (`OUT-REQ-005`).
4. A configuration that changes the parser's behavior in a way that affects `YAML_PARSING_AND_EDGE_CASES.md` §10 MUST be treated as an ADR-level decision.
5. The default configuration MUST remain the safe defaults of §15.1; a non-default configuration MUST require explicit opt-in and MUST emit a warning.

**SEC-YAML-REQ-050:** A parser configuration override that cannot satisfy constraints 1–5 MUST be rejected at startup with a structured usage error, not silently ignored.

---

## 16. Authoritative references

| Reference | Scope | Type | Retrieved |
|---|---|---|---|
| YAML 1.2.2 specification | Safe-mode parsing, tag semantics, duplicate-key uniqueness, alias scope | Specification | 2026-10-31 |
| CWE-502 | Deserialization of untrusted data | Weakness taxonomy | 2026-10-31 |
| CWE-400 | Uncontrolled resource consumption | Weakness taxonomy | 2026-10-31 |
| CWE-22 | Path traversal | Weakness taxonomy | 2026-10-31 |
| CWE-209 | Sensitive information in error messages | Weakness taxonomy | 2026-10-31 |
| CVE-2026-84202 (ModelScope / PyYAML unsafe loader) | Arbitrary code execution via object-construction tags | CVE | 2026-10-31 |
| CVE-2026-72923 (Microsoft.OpenAPI.NET alias bomb) | Billion-laughs alias expansion | CVE | 2026-10-31 |
| CVE-2026-45304 (Symfony YAML alias bomb) | Alias-expansion denial of service | CVE | 2026-10-31 |
| CVE-2025-9566 / GHSA-wp3j-xq48-xpjw (Podman `kube play` symlink) | Symlink-based host file overwrite | CVE / advisory | 2026-10-31 |
| CVE-2025-53547 (Helm path traversal) | Symlink and path traversal via chart files | CVE | 2026-10-31 |
| CVE-2026-113672 (Argo CD Jsonnet path traversal) | Path traversal and symlink escape | CVE | 2026-10-31 |
| Trail of Bits, "Unexpected security footguns in Go's parsers" | Parser differentials and duplicate-key handling | Security research | 2026-10-31 |
| L4YAML security analysis (NASA JPL) | Reference DoS/ACE taxonomy and parser limits | Open-source implementation | 2026-10-31 |
| OSS-Fuzz OpenCV YAML parser finding (#28619) | Heap buffer overflow on empty key | Fuzzing finding | 2026-10-31 |
| `docs/yaml/YAML_PARSING_AND_EDGE_CASES.md` (internal) | Edge-case behavior and resource-limit register | Internal | 2026-10-31 |
| `docs/security/THREAT_MODEL.md` (internal) | Secret-bearing field set and threat taxonomy | Internal | 2026-10-31 |

**Licensing note:** The YAML 1.2.2 specification is freely copyable provided it is not modified. CVE records are public. L4YAML is an open-source NASA JPL project. Kubernetes documentation is licensed under CC BY 4.0. No line numbers, benchmark results, or implementation status are asserted beyond what the cited sources state.

---

## 17. Open questions and unresolved decisions

| ID | Question | Status | Owner |
|---|---|---|---|
| OPEN-YAML-SEC-01 | Parser selection (ADR-002). Affects safe-loader model, alias-limit metering, source-location granularity, and quadratic-complexity profile. | **OPEN** | Software Architecture |
| OPEN-YAML-SEC-02 | Whether `max_time_ms` should be enforced by a watchdog thread, a signal, or cooperative checks; the choice affects interruptibility of a parser in a tight loop. | **OPEN** | Software Architecture |
| OPEN-YAML-SEC-03 | Exact secret-bearing field set and sensitive-name pattern list; currently referenced to `docs/security/THREAT_MODEL.md`, which is maintained separately. | **OPEN** | Security Architecture |
| OPEN-YAML-SEC-04 | Whether `--print-effective-config` will include override provenance by default or only with a flag. | **OPEN** | CLI Architecture |
| OPEN-YAML-SEC-05 | Whether a future parser configuration override will be permitted at all, and under what governance. | **OPEN** | Software Architecture |
| OPEN-YAML-SEC-06 | Crash-barrier placement: process-level, thread-level, or both, and the interaction with the language runtime's abort behavior. | **OPEN** | Software Architecture |

---

## 18. Definition of done

This document is complete when the following conditions hold:

1. Every threat in §14 has a risk statement, preconditions, preventive controls, detection methods, tests, and residual risk.
2. Every preventive control cites a `SEC-YAML-REQ-*` requirement and maps to at least one attacker goal (G1–G4).
3. Safe parsing and constructor-tag rejection are specified with static-analysis, corpus, and runtime verification.
4. Alias expansion is metered before materialization, with dual limits and recursion rejection.
5. Resource limits are enumerated with defaults, enforcement stage, and structured failure behavior.
6. Encoding policy requires UTF-8 with no silent fallback and identifies invalid byte offsets.
7. Crash containment converts parser crashes into structured findings and is backed by continuous fuzzing.
8. Duplicate-key rejection is independent of parser policy and uses Core Schema canonical equality.
9. Unsafe tags and extensions are governed by an allowlist and an opaque-preservation policy.
10. Path traversal, symlinks, archives, and stdin are governed by root confinement and explicit opt-in.
11. Secret exposure is prevented across errors, logs, temporary files, and debug output, with a secret-bearing corpus test.
12. Dependency pinning, vulnerability scanning, upgrade governance, and fuzzing are specified.
13. Fail-closed and partial-result behaviors are explicitly distinguished, with per-document status marking and exit-code reflection.
14. Safe defaults are recorded in a single register, and non-overridable versus overridable controls are explicitly separated.
15. The future parser configuration override policy states the constraints that any override must satisfy.

---

## 19. Consistency and completeness review

A review was performed against the requested scope before finalization.

- **Safe parsing and arbitrary object instantiation prevention:** §4, with safe-loader requirement, tag allowlist, independent verification, and residual-risk note.
- **Alias expansion and recursion limits:** §5, with dual limits, pre-expansion metering, recursion rejection, and parser-dependent behavior.
- **Excessive nesting, collection sizes, scalar lengths, document counts:** §6, with limit register and pre-allocation enforcement.
- **CPU and memory exhaustion:** §6.2, with quadratic-complexity evaluation and wall-clock enforcement.
- **Malformed UTF-8, invalid encodings, parser crashes:** §7, with UTF-8-only policy, crash containment, fuzzing, and crash triage.
- **Duplicate-key ambiguity and parser differentials:** §8, with reject policy, parser-policy independence, and canonical-equality tests.
- **Unsafe custom tags and unsupported extensions:** §9, with tag policy, opaque preservation, and extension enumeration.
- **Path traversal, symlinks, file discovery, stdin:** §10, with root confinement, symlink policy, archive safety, and no-ambient-access.
- **Secret exposure:** §11, with no-content-in-errors, secret-field redaction, no-log-content, no-temp-files, and stream separation.
- **Dependency vulnerabilities, parser upgrades, fuzzing, crash triage:** §12, with pinning, scanning, ADR-governed upgrades, continuous fuzzing, and triage process.
- **Fail-closed versus partial-result:** §13, with explicit conditions and status marking.
- **Threat-to-mitigation matrix:** §14, with all required columns.
- **Safe defaults and parser override policy:** §15, with non-overridable controls, audited overrides, and future-override constraints.
- **No claim that KubeCheck-unsupported features are YAML-prohibited:** the governing rule `SEC-YAML-REQ-001` and the distinction between implementation decisions and specification requirements are maintained throughout.

No empty boilerplate section was added solely to satisfy a checklist. The document is complete for its assigned responsibility.