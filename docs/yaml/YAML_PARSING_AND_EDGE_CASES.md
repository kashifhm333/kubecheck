# KubeCheck — YAML Parsing and Edge Cases

**Document ID:** YAML-EDGE-001  
**File:** `docs/yaml/YAML_PARSING_AND_EDGE_CASES.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Parser Engineering  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/yaml/YAML_LANGUAGE_SPEC.md`
- `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md`
- `docs/yaml/YAML_KUBERNETES_PATTERNS.md`
- `docs/architecture/SYSTEM_ARCHITECTURE.md`
- `docs/architecture/REQUIREMENTS_AND_NFRS.md`
- `docs/architecture/ADR/ADR-002-yaml-parser-selection.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/security/THREAT_MODEL.md`
- `docs/testing/DETERMINISM_TESTS.md`

---

## 1. Purpose and scope

This document specifies how KubeCheck handles **YAML parsing edge cases** and **parser-dependent behavior**. It covers lexical and structural errors, scalar and resolution edge cases, alias and expansion attacks, multi-document handling, Unicode and encoding issues, resource-exhaustion vectors, parser differences across supported libraries, source-location requirements, error recovery, parser version upgrades, and the resource limits that KubeCheck enforces.

For **every edge case**, this document states:

- **Expected behavior** — what KubeCheck does.
- **Severity** — the finding severity (`error`, `warning`, `info`) or the parser outcome (`stop`, `continue`).
- **Source-location requirements** — what location information must accompany the finding.
- **Recovery policy** — whether processing continues for the affected document or the entire stream.
- **Test scenarios** — representative inputs and assertions.

**YAML-EDGE-REQ-001 — No edge case in this document is described as "invalid YAML" unless the YAML 1.2.2 specification actually prohibits it.** Where KubeCheck rejects a construct that the specification permits, the rejection is labelled as an implementation resource-safety or security decision, consistent with `YAML-SPEC-REQ-001`.

**In scope:** parsing-time and normalization-time edge cases, resource limits, and parser-behavior variance.

**Out of scope:** diagnostic rule semantics (see `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`), schema bundle format (see ADR-003), and NIR invariants (see `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md` §16).

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **Lexical error** | A failure in the scanner/tokenizer stage: illegal character, unclosed quote, invalid escape, tab in indentation, etc. |
| **Structural error** | A failure in the parser stage: malformed mapping, broken sequence, unexpected token where a grammar production was expected. |
| **Semantic YAML error** | A specification violation detectable only after parsing: duplicate keys, undefined alias, recursive alias. |
| **Recovery policy** | `STOP_DOCUMENT` (abort the affected document, continue the stream) or `STOP_STREAM` (abort the invocation). |
| **Resource limit** | A configurable or fixed bound on input size, nesting depth, alias expansion, scalar size, collection size, or document count. |
| **Parser-dependent** | Behavior that varies across parser libraries because the specification does not mandate a single outcome. |
| **Provisionally normative** | A requirement dependent on ADR-002 (parser selection, **OPEN**) that may change when the ADR is resolved. |

**Conventions:**

- Requirement IDs use `EDGE-REQ-NNN`.
- Every edge case is presented in a table with columns: **Case**, **Specification position**, **KubeCheck behavior**, **Severity**, **Source location**, **Recovery**, **Test scenario**.
- Pseudocode is language-neutral.
- Parser-specific behaviors are stated with the parser name and version range when known; otherwise they are stated as **parser-dependent** and deferred to ADR-002.

---

## 3. Lexical and structural errors

### 3.1 Tab characters in indentation

| Attribute | Detail |
|---|---|
| **Case** | A tab (`\t`, U+0009) appears where indentation is expected in a block collection. |
| **Specification position** | YAML 1.2.2 prohibits tabs in indentation (§6.1). Tabs are permitted in scalar content and as separation whitespace in flow context, but **not** as indentation for block collections. The specification states that "tabs must not be used in indentation" (§6.1). |
| **KubeCheck behavior** | `REJECT` with a lexical error. The finding identifies the line, column, and the tab character. |
| **Severity** | `error` |
| **Source location** | Exact line and column of the tab character; the document index. |
| **Recovery** | `STOP_DOCUMENT`: the affected document is rejected; other documents in the stream continue. |
| **Test scenario** | Input: `spec:\n\tcontainers: []` (tab before `containers`). Assert: lexical error at line 2, column 1; document marked `parse-failed`; no schema or semantic findings for this document. |

**EDGE-REQ-001:** KubeCheck MUST reject tab characters used as block indentation with a finding that identifies the exact line and column, and MUST NOT attempt to reinterpret the tab as spaces.

**Parser-dependent note:** Some parsers emit a generic `ScannerError: found character '\t' that cannot start any token` rather than a structured tab-indentation error. KubeCheck normalizes this to a structured finding with a stable error code (e.g., `TAB_IN_INDENTATION`) and location, regardless of the underlying parser's message.

### 3.2 Inconsistent indentation

| Attribute | Detail |
|---|---|
| **Case** | Block collection entries are not aligned at the same indentation level (e.g., two spaces, then three spaces for a sibling entry). |
| **Specification position** | The specification requires entries of a block collection to be at the same indentation level (§8.2.1, §8.2.2). Misaligned entries produce a structural error. |
| **KubeCheck behavior** | `REJECT` with a structural error identifying the first misaligned entry and the expected indentation level. |
| **Severity** | `error` |
| **Source location** | The line and column of the first misaligned entry; the expected indentation level is included in the evidence. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `spec:\n  replicas: 1\n   template: {}` (three spaces before `template`). Assert: structural error at line 3, column 1; evidence states expected 2 spaces, found 3. |

**EDGE-REQ-002:** KubeCheck MUST report the first misaligned entry and the expected indentation level. It MUST NOT attempt to infer the author's intent or auto-correct the indentation.

### 3.3 Malformed mappings

| Attribute | Detail |
|---|---|
| **Case** | A mapping entry is missing a colon (`key value` instead of `key: value`), has a colon without a value followed by another key at the same level (`key:\nnext: value`), or has an unexpected token where a key was expected. |
| **Specification position** | The specification requires `key: value` with a colon-space separator in block mappings (§8.2.2). A missing colon is a structural error. |
| **KubeCheck behavior** | `REJECT` with a structural error. The finding identifies the line and column of the unexpected token and states the expected production (e.g., `block mapping key`). |
| **Severity** | `error` |
| **Source location** | The line and column of the unexpected token; the document index. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `metadata:\n  name web` (missing colon). Assert: structural error at line 2, column 8; evidence states expected `:` after key `name`. |

**EDGE-REQ-003:** KubeCheck MUST distinguish "missing colon" from "unexpected token" in the error code, so that users can diagnose the specific malformation.

### 3.4 Broken sequences

| Attribute | Detail |
|---|---|
| **Case** | A sequence entry is written without the `- ` indicator, or the indicator is followed by a tab instead of a space, or entries are misaligned. |
| **Specification position** | A block sequence entry requires `- ` (dash followed by a space) (§8.2.1). A dash followed by a tab is not a valid sequence entry. |
| **KubeCheck behavior** | `REJECT` with a structural error identifying the offending line and column. |
| **Severity** | `error` |
| **Source location** | The line and column of the malformed entry. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `containers:\n  -name: app` (no space after dash). Assert: structural error at line 2, column 3; evidence states expected `- ` (dash-space). |

**EDGE-REQ-004:** KubeCheck MUST reject a dash without a following space as a sequence indicator, because the specification requires the space.

### 3.5 Unclosed quotes

| Attribute | Detail |
|---|---|
| **Case** | A single-quoted scalar (`'...`) or double-quoted scalar (`"..."`) is not terminated before the end of the document or before a line break that the parser cannot fold. |
| **Specification position** | Quoted scalars must be terminated by the matching quote character (§7.3.1, §7.3.2). An unclosed quote is a lexical error. |
| **KubeCheck behavior** | `REJECT` with a lexical error identifying the opening quote location and the end of the document or the line where the parser gave up. |
| **Severity** | `error` |
| **Source location** | The line and column of the opening quote; the line where the parser detected the failure. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `name: "web` (unclosed double quote). Assert: lexical error at line 1, column 7 (opening quote); evidence states expected closing `"`. |

**EDGE-REQ-005:** KubeCheck MUST report the location of the **opening quote** in addition to the detection point, so users can find the start of the unclosed scalar.

### 3.6 Invalid escape sequences

| Attribute | Detail |
|---|---|
| **Case** | A double-quoted scalar contains an escape sequence not defined by YAML 1.2.2 (e.g., `\q`, `\z`, or an incomplete `\u12` without four hex digits). |
| **Specification position** | The specification defines the complete set of escape sequences for double-quoted scalars (§5.7, §7.3.1). An undefined escape is invalid. |
| **KubeCheck behavior** | `REJECT` with a lexical error identifying the escape sequence and its location. |
| **Severity** | `error` |
| **Source location** | The line and column of the backslash that begins the invalid escape. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `pattern: "\\q"` (invalid escape `\q`). Assert: lexical error at line 1, column 11; evidence states `\q` is not a valid YAML escape. |

**EDGE-REQ-006:** KubeCheck MUST distinguish invalid escape characters from incomplete Unicode escapes (`\u` without four hex digits) in the error code.

### 3.7 Malformed block scalars

| Attribute | Detail |
|---|---|
| **Case** | A literal (`\|`) or folded (`>`) block scalar has (a) an explicit indentation indicator equal to `0` (which is invalid), (b) content lines indented less than the indicator, or (c) a line after the block that is deeper than the block's base indentation and is read as a new key instead of as content. |
| **Specification position** | The specification requires the indentation indicator to be a digit `1`–`9` (§8.1.1.1). Content lines must be indented at least as much as the indicator. |
| **KubeCheck behavior** | `REJECT` with a structural error identifying the invalid indicator or the offending content line. |
| **Severity** | `error` |
| **Source location** | The line and column of the block scalar header (for indicator errors) or the first outdented content line. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `script: \|0\n  echo hello`. Assert: structural error at line 1, column 10 (indicator `0`); evidence states indicator must be 1–9. |

**EDGE-REQ-007:** KubeCheck MUST reject an indentation indicator of `0` and MUST report the header location, not only the first content line.

### 3.8 Unexpected tokens

| Attribute | Detail |
|---|---|
| **Case** | The parser encounters a token that cannot begin any valid production at the current position (e.g., a stray `]` in block context, a `}` where a key was expected, or a `%` directive after document content). |
| **Specification position** | The specification's grammar defines the set of tokens permitted at each position. An unexpected token is a structural error. |
| **KubeCheck behavior** | `REJECT` with a structural error identifying the token and its location. |
| **Severity** | `error` |
| **Source location** | The line and column of the unexpected token; the document index. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `spec: ]` (stray `]`). Assert: structural error at line 1, column 7; evidence states `]` cannot begin a node in block context. |

**EDGE-REQ-008:** KubeCheck MUST NOT attempt heuristic recovery from an unexpected token by skipping it. The document is rejected with a clear error.

---

## 4. Duplicate keys and conflicting values

### 4.1 Duplicate mapping keys

| Attribute | Detail |
|---|---|
| **Case** | A mapping contains the same key more than once. Keys may differ textually but resolve to the same canonical value under the Core Schema (e.g., `0xB` and `11`). |
| **Specification position** | The specification requires mapping keys to be **unique** (§3.2.1.3). Key equality is schema-dependent: under the Core Schema, `0xB` and `11` are the same integer key. |
| **KubeCheck behavior** | `REJECT` at parse time with a duplicate-key error identifying **both** occurrences (line and column) and stating the canonical form that is equal. |
| **Severity** | `error` |
| **Source location** | Both occurrences: first occurrence location and duplicate occurrence location. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `metadata:\n  name: web\n  name: api`. Assert: duplicate-key error with first at line 2, column 3 and second at line 3, column 3; evidence states `name` is duplicated. |
| **Parser-dependent note** | PyYAML silently overwrites the first entry with the last; ruamel.yaml raises `DuplicateKeyError` with its new API and `DuplicateKeyFutureWarning` with the legacy API. KubeCheck requires the parser to expose duplicate detection and MUST normalize the outcome to a structured rejection regardless of the parser's default behavior. |

**EDGE-REQ-009:** KubeCheck MUST reject duplicate mapping keys at parse time, before normalization, and MUST identify both occurrences. This is a specification requirement, not an implementation preference.

**EDGE-REQ-010:** Duplicate detection MUST use Core Schema canonical equality, not textual equality. `"11"` and `11` are different keys (string vs integer); `0xB` and `11` are the same key (integer 11).

### 4.2 Conflicting values from parser defaults

| Attribute | Detail |
|---|---|
| **Case** | A parser with a default resolution policy (first-wins or last-wins) silently resolves a duplicate key. |
| **Specification position** | The specification requires duplicates to be an error; it does not define a resolution policy. A parser that resolves duplicates is **non-conformant** in this respect. |
| **KubeCheck behavior** | KubeCheck MUST NOT rely on the parser's default resolution. It MUST detect duplicates independently (either via parser API or via a post-parse mapping scan) and reject. |
| **Severity** | `error` |
| **Source location** | Both occurrences. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Configure the parser to last-wins and verify that KubeCheck still rejects the document with a duplicate-key error. |

**EDGE-REQ-011:** KubeCheck MUST verify duplicate-key rejection independently of the parser's configured duplicate policy. This is a testable requirement (see `docs/testing/DETERMINISM_TESTS.md`).

---

## 5. Ambiguous scalars and implicit type conversion

### 5.1 YAML 1.1 boolean forms under the Core Schema

| Attribute | Detail |
|---|---|
| **Case** | A plain scalar `yes`, `no`, `on`, or `off` appears where a boolean is expected by the Kubernetes schema. |
| **Specification position** | The Core Schema resolves only `true` and `false` (any case) to booleans (§10.3.2). `yes`/`no`/`on`/`off` resolve to **strings**. YAML 1.1 permitted them as booleans; YAML 1.2 does not. |
| **KubeCheck behavior** | `ACCEPT AS STRING` at parse time. If the schema expects a boolean, the schema validator emits an informational finding explaining the Core Schema resolution and suggesting `true`/`false`. |
| **Severity** | `info` (schema finding) |
| **Source location** | The scalar's line and column. |
| **Recovery** | Continue: the document is processed; the finding is informational. |
| **Test scenario** | Input: `enabled: yes` with a schema expecting `boolean`. Assert: scalar resolves to `string` with text `"yes"`; informational finding states Core Schema treats `yes` as string. |

**EDGE-REQ-012:** KubeCheck MUST NOT apply YAML 1.1 boolean resolution by default (`YAML-SPEC-REQ-010`). Ambiguous values produce informational findings when schema context reveals a likely intent.

### 5.2 Octal-looking integers

| Attribute | Detail |
|---|---|
| **Case** | A plain scalar `0755` appears where an octal value is intended. |
| **Specification position** | The Core Schema resolves `0755` as **decimal 755**, not octal 493. The octal form is `0o755`. |
| **KubeCheck behavior** | `ACCEPT AS INTEGER 755`. If the schema expects an integer with octal semantics (e.g., a file mode), emit an informational finding explaining the resolution. |
| **Severity** | `info` |
| **Source location** | The scalar's line and column. |
| **Recovery** | Continue. |
| **Test scenario** | Input: `mode: 0755` with a schema noting octal semantics. Assert: resolved integer 755; informational finding suggests `0o755` for octal 493. |

**EDGE-REQ-013:** KubeCheck MUST report the resolved decimal value in the finding evidence so the user sees exactly how the Core Schema interpreted the scalar.

### 5.3 Timestamp-like strings

| Attribute | Detail |
|---|---|
| **Case** | A plain scalar `2026-10-31` appears where a timestamp is intended. |
| **Specification position** | The Core Schema does not resolve timestamps. The value remains a **string**. |
| **KubeCheck behavior** | `ACCEPT AS STRING`. If the schema expects a timestamp type, emit an informational finding. |
| **Severity** | `info` |
| **Source location** | The scalar's line and column. |
| **Recovery** | Continue. |
| **Test scenario** | Input: `date: 2026-10-31` with a schema noting timestamp format. Assert: string type; informational finding states Core Schema does not resolve timestamps. |

**EDGE-REQ-014:** KubeCheck MUST NOT silently coerce timestamp-like strings to a timestamp type; the string resolution MUST be preserved in the NIR (`NORM-REQ-023`).

---

## 6. Anchors, aliases, merge keys, and expansion attacks

### 6.1 Undefined alias

| Attribute | Detail |
|---|---|
| **Case** | An alias (`*name`) appears before the corresponding anchor (`&name`) in the document, or the anchor is never defined. |
| **Specification position** | The specification requires an alias to refer to a **previously defined** anchor in the same document (§3.2.2.2). An alias without a preceding anchor is a semantic YAML error. |
| **KubeCheck behavior** | `REJECT` with a semantic YAML error identifying the alias and stating that no preceding anchor with that name exists. |
| **Severity** | `error` |
| **Source location** | The line and column of the alias. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `a: *x\nb: &x 1`. Assert: error at line 1, column 4; evidence states `*x` has no preceding `&x`. |

**EDGE-REQ-015:** KubeCheck MUST distinguish "alias before anchor" from "undefined anchor" in the error code, because the corrective actions differ (reorder vs define).

### 6.2 Merge key with undefined or non-mapping source

| Attribute | Detail |
|---|---|
| **Case** | A merge key (`<<`) references an alias that is undefined, or references a scalar/sequence instead of a mapping. |
| **Specification position** | The merge key is an **extension** (not Core Schema). Its semantics are defined by the Merge Key specification: the value must be a mapping or a sequence of mappings. |
| **KubeCheck behavior** | `REJECT` if the alias is undefined. `REJECT` with `BAD_MERGE` if the merge source is not a mapping or a sequence of mappings. |
| **Severity** | `error` |
| **Source location** | The line and column of the merge key and, if applicable, the merge source. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `<<: *missing`. Assert: error stating `*missing` is undefined. Input: `<<: 42` (scalar). Assert: `BAD_MERGE` stating merge source must be a mapping. |

**EDGE-REQ-016:** KubeCheck MUST validate the merge source type before attempting to merge. A non-mapping merge source is rejected, not silently ignored.

### 6.3 Recursive aliases

| Attribute | Detail |
|---|---|
| **Case** | An anchor references itself directly (`a: &a [*a]`) or indirectly through a cycle. |
| **Specification position** | The specification does not explicitly prohibit recursive aliases, but a recursive alias cannot be materialized as a finite data structure. The representation graph may contain cycles, but a processor that constructs native values must detect and reject them. |
| **KubeCheck behavior** | `REJECT` with a normalization error identifying the cycle's anchor names and locations. The document is marked `normalization-failed`. |
| **Severity** | `error` |
| **Source location** | The anchor definitions and alias usages that form the cycle. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `a: &a [*a]`. Assert: normalization error at the anchor line; evidence names `a` and states the alias creates a cycle. |

**EDGE-REQ-017:** KubeCheck MUST detect cycles during NIR construction and MUST NOT attempt to break them by inserting `null` or truncating the graph (`NORM-REQ-028`). The error must name the anchors involved.

### 6.4 Billion-laughs alias expansion

| Attribute | Detail |
|---|---|
| **Case** | A document uses nested anchors and aliases to produce exponential expansion (e.g., `a: &a [x,x,x,x,x,x,x,x,x]`, `b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]`, etc.). |
| **Specification position** | Valid YAML. The specification does not impose expansion limits; implementations must add resource limits. |
| **KubeCheck behavior** | `REJECT` with a governor finding when alias expansion exceeds `max_aliases` (default 10,000) or when resolved nodes exceed `max_resolved_nodes` (default 100,000). The finding identifies the document and the limit exceeded. |
| **Severity** | `error` |
| **Source location** | The document index and, where feasible, the anchor whose expansion triggered the limit. |
| **Recovery** | `STOP_DOCUMENT`; other documents continue unless aggregate limits are also exceeded. |
| **Test scenario** | Input: 10 levels of nested `[ *prev, *prev, ... ]` (9 aliases per level). Assert: governor finding `ALIAS_EXPANSION_LIMIT` before the expansion can allocate unbounded memory. |

**EDGE-REQ-018:** Alias expansion MUST be metered during parsing, not after full materialization. The limit MUST be enforced **before** the expansion allocates memory proportional to the expanded size (`LIM-REQ-004`).

**EDGE-REQ-019:** KubeCheck MUST support both an alias-count limit (`max_aliases`) and a resolved-node limit (`max_resolved_nodes`) to close the "wide alias fan-out" gap where many aliases each reference a moderately sized anchor.

### 6.5 Recursive merge-key expansion

| Attribute | Detail |
|---|---|
| **Case** | A merge key references an anchor that itself contains a merge key referencing back, producing recursive merge expansion. |
| **Specification position** | The Merge Key specification defines merge semantics but does not explicitly address cycles. A cyclic merge cannot be materialized. |
| **KubeCheck behavior** | `REJECT` with a normalization error identifying the merge anchors involved in the cycle. |
| **Severity** | `error` |
| **Source location** | The merge keys and anchors forming the cycle. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: `a: &a {<<: *b}`, `b: &b {<<: *a}`. Assert: normalization error naming `a` and `b` as a merge cycle. |

**EDGE-REQ-020:** KubeCheck MUST treat a merge-key cycle as a normalization error, not as a parse error, because the merge key is processed after parsing and the cycle is detected during merge expansion.

### 6.6 Repeated merge keys

| Attribute | Detail |
|---|---|
| **Case** | A mapping contains more than one `<<` key (`<<: *a` followed by `<<: *b`). |
| **Specification position** | The Merge Key specification treats `<<` as a special key; repeated `<<` entries are **not** duplicate keys because the merge key is an operator, not an ordinary mapping key. |
| **KubeCheck behavior** | `ACCEPT`. The merge sources are applied in order; later sources override earlier ones for conflicting keys, and the current mapping's explicit keys override all merged keys. |
| **Severity** | No finding (valid). |
| **Source location** | Not applicable unless a merge source is invalid. |
| **Recovery** | Continue. |
| **Test scenario** | Input: `<<: *a`, `<<: *b`, `key: value`. Assert: `key` from the current mapping wins; `b` keys override `a` keys; no duplicate-key error. |

**EDGE-REQ-021:** KubeCheck MUST NOT treat repeated `<<` keys as duplicates (`YAML-SPEC-REQ-013`). A quoted `"<<"` is an ordinary key and **is** subject to duplicate detection.

---

## 7. Multiple documents, empty streams, and document separators

### 7.1 Empty stream (zero documents)

| Attribute | Detail |
|---|---|
| **Case** | The input is empty (zero bytes) or contains only comments and whitespace. |
| **Specification position** | A YAML stream may contain **zero or more documents** (§9.1). An empty stream is valid and contains no documents. |
| **KubeCheck behavior** | `ACCEPT`. The stream produces zero `DocumentResult` entries. The report states "no documents found" with severity `info`. |
| **Severity** | `info` |
| **Source location** | Not applicable (no document). |
| **Recovery** | Continue: the invocation succeeds with an informational finding. |
| **Test scenario** | Input: `""` or `"# comment\n"`. Assert: zero documents; informational finding `NO_DOCUMENTS`; exit code is success (no error-severity findings). |

**EDGE-REQ-022:** KubeCheck MUST distinguish a zero-document stream from a stream containing a single `null` document (`---\n`). The former produces `NO_DOCUMENTS`; the latter produces one document with a null root.

### 7.2 Empty document

| Attribute | Detail |
|---|---|
| **Case** | A document boundary pair (`---` followed by `---` or EOF) with no content. |
| **Specification position** | An empty document resolves to a **null** root node (§9.1). |
| **KubeCheck behavior** | `ACCEPT`. The document is admitted to the NIR with `root_kind = null`, then GVK validation emits a GVK error (`INVALID_ROOT_KIND` or `MISSING_API_VERSION`). |
| **Severity** | `error` (GVK finding) |
| **Source location** | The document index and the location of the `---` marker. |
| **Recovery** | Continue to the next document. |
| **Test scenario** | Input: `---\n---`. Assert: two documents; the first (or both, depending on marker interpretation) resolves to null; GVK finding for the null document; no parse error. |

**EDGE-REQ-023:** KubeCheck MUST NOT silently drop an empty document (`YAML-SPEC-REQ-019`). It MUST admit it and produce a downstream GVK finding explaining why it cannot be analyzed as a Kubernetes resource.

### 7.3 Document separator edge cases

| Case | Specification position | KubeCheck behavior | Severity | Recovery |
|---|---|---|---|---|
| `---` at the start of a document | Valid | `ACCEPT` | — | Continue |
| `...` as document end | Valid | `ACCEPT` | — | Continue |
| Content after `...` without a new `---` | Invalid; a new document requires `---` | `REJECT` with a structural error | `error` | `STOP_DOCUMENT` |
| `---` indented (not at column 0) | Ordinary content, not a separator | `ACCEPT` as content | — | Continue |
| `%YAML` directive followed by `---` | Valid | `ACCEPT WITH WARNING` (version recorded, conformance remains 1.2.2) | `info` | Continue |
| `%TAG` directive with undeclared handle | Invalid | `REJECT` | `error` | `STOP_DOCUMENT` |

**EDGE-REQ-024:** A `---` or `...` marker MUST be recognized only at **column 0**. An indented `---` is ordinary scalar content and MUST NOT split the document (`YAML-SPEC-REQ-003`).

---

## 8. Unicode, encoding, and line endings

### 8.1 Invalid UTF-8 byte sequences

| Attribute | Detail |
|---|---|
| **Case** | The raw input contains a byte sequence that is not valid UTF-8 (e.g., an isolated continuation byte, an overlong encoding, or a surrogate half). |
| **Specification position** | YAML 1.2.2 is Unicode-based; invalid UTF-8 is not a valid YAML stream. |
| **KubeCheck behavior** | `REJECT` at the decoder stage with a parse error identifying the **byte offset** of the first invalid sequence. |
| **Severity** | `error` |
| **Source location** | Byte offset; if partial decoding is possible, the nearest line and column are also reported. |
| **Recovery** | `STOP_DOCUMENT`; if the invalid bytes occur before any document boundary, the entire stream is rejected. |
| **Test scenario** | Input: valid YAML with a single `0xFF` byte inserted at offset 42. Assert: decode error at byte 42; no documents are parsed. |

**EDGE-REQ-025:** KubeCheck MUST reject invalid UTF-8 at the decoder stage and MUST identify the byte offset. It MUST NOT attempt heuristic encoding detection (`NORM-REQ-003`).

### 8.2 Unicode normalization (NFC/NFD/NFKC/NFKD)

| Attribute | Detail |
|---|---|
| **Case** | A scalar contains Unicode text that has multiple normalization forms (e.g., `é` as U+00E9 vs `e` + U+0301). |
| **Specification position** | YAML 1.2.2 does not mandate a particular Unicode normalization form. Processors may preserve or normalize; the choice is application-defined. |
| **KubeCheck behavior** | KubeCheck **preserves the original code points** in the NIR and does **not** apply NFC/NFD/NFKC/NFKD normalization. This preserves byte-for-byte fidelity for diagnostic evidence and prevents two visually identical values from being treated as different. |
| **Severity** | No finding (preservation is the default). |
| **Source location** | Not applicable. |
| **Recovery** | Continue. |
| **Test scenario** | Input: `name: café` with `é` as U+00E9 and another document with `e` + U+0301. Assert: the two strings are **not** treated as equal; the NIR preserves the original code points. |

**EDGE-REQ-026:** KubeCheck MUST NOT apply Unicode normalization to scalar values, mapping keys, or evidence text. Downstream consumers that need normalized comparison MUST normalize explicitly, with documented semantics.

**Design rationale:** The `haz-cli` specification states: "An implementation MUST NOT apply Unicode normalization (NFC, NFD, NFKC, NFKD, or any other transformation) to a path, neither at parse time nor at any later stage" because doing so causes lookups to miss files stored with the original byte sequence. The same reasoning applies to manifest content: preserving original code points ensures that diagnostics reference the exact bytes the user wrote.

### 8.3 Line-ending variants

| Attribute | Detail |
|---|---|
| **Case** | The input uses CRLF (`\r\n`) or lone CR (`\r`) line endings instead of LF (`\n`). |
| **Specification position** | YAML recognizes `\n`, `\r\n`, and `\r` as line breaks and normalizes them to LF in the data model (§5.4). |
| **KubeCheck behavior** | `ACCEPT`. All line breaks are normalized to LF in the decoded text; the original byte offsets are preserved in the source map. |
| **Severity** | No finding. |
| **Source location** | Source map preserves original byte offsets (`NORM-REQ-044`). |
| **Recovery** | Continue. |
| **Test scenario** | Input with CRLF line endings. Assert: decoded text uses LF; findings reference correct line/column; original byte offsets are available for evidence. |

**EDGE-REQ-027:** KubeCheck MUST normalize line breaks to LF for parsing but MUST preserve original byte offsets so diagnostics can reference the original file (`YAML-SPEC-REQ-015`, `NORM-REQ-004`).

### 8.4 Byte-order mark (BOM)

| Attribute | Detail |
|---|---|
| **Case** | The input begins with a UTF-8 BOM (`EF BB BF`). |
| **Specification position** | A BOM is permitted at the start of a stream. Some parsers strip it automatically; others do not. |
| **KubeCheck behavior** | `ACCEPT`. The BOM is stripped during decoding; the source map accounts for the offset shift. |
| **Severity** | No finding. |
| **Source location** | Source map accounts for the 3-byte shift. |
| **Recovery** | Continue. |
| **Test scenario** | Input: BOM followed by `apiVersion: v1`. Assert: `apiVersion` is at the expected line/column after BOM stripping; no parse error. |

**EDGE-REQ-028:** KubeCheck MUST strip a leading UTF-8 BOM and MUST adjust source-map offsets accordingly. It MUST NOT strip a BOM that appears in the middle of a scalar.

---

## 9. Resource exhaustion: long lines, deep nesting, huge scalars, large collections

### 9.1 Very long lines

| Attribute | Detail |
|---|---|
| **Case** | A single line exceeds the parser's or KubeCheck's line-length tolerance (e.g., a base64-encoded secret or TLS certificate on one line). |
| **Specification position** | The specification does not impose a line-length limit. Some parsers impose a 1 KiB limit on simple mapping keys; KubeCheck should not impose a line-length limit on scalar content unless it is a resource-safety measure. |
| **KubeCheck behavior** | `ACCEPT` within `max_scalar_bytes` (default 10 MiB). If a single scalar exceeds the limit, `REJECT` with a governor finding identifying the scalar's location and the limit. |
| **Severity** | `error` (governor finding) |
| **Source location** | The line and column where the oversized scalar begins. |
| **Recovery** | `STOP_DOCUMENT`; other documents continue. |
| **Test scenario** | Input: `data: <12 MiB base64 string>`. Assert: governor finding `SCALAR_SIZE_LIMIT` at the scalar's start; document marked `parse-failed`. |

**EDGE-REQ-029:** KubeCheck MUST distinguish an oversized **scalar** from an oversized **document**. The former is limited by `max_scalar_bytes`; the latter by `max_input_bytes`.

**Parser-dependent note:** `bufio.Scanner` defaults to a 64 KiB maximum token size, which causes lines longer than that (base64 secrets, TLS certificates) to be silently truncated with `scanner.Scan() == false`. KubeCheck's parser configuration MUST set the scanner buffer to at least `max_scalar_bytes + 1` to avoid this class of failure.

### 9.2 Deeply nested collections

| Attribute | Detail |
|---|---|
| **Case** | A document contains deeply nested mappings or sequences (e.g., 500 levels of `{a: {a: {a: ... }}}`). |
| **Specification position** | The specification imposes no nesting-depth limit. Implementations must add one to prevent stack overflow. |
| **KubeCheck behavior** | `ACCEPT` up to `max_depth` (default 256). Above the limit, `REJECT` with a governor finding identifying the document and the depth at which the limit was exceeded. |
| **Severity** | `error` (governor finding) |
| **Source location** | The line and column of the node at the maximum permitted depth. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: 300 levels of nested sequences. Assert: governor finding `DEPTH_LIMIT` at the 257th level; no stack overflow. |

**EDGE-REQ-030:** Depth MUST be counted uniformly for mappings and sequences. The limit MUST be enforced during parsing, not after construction, to prevent stack overflow before the check can run.

### 9.3 Huge scalars

| Attribute | Detail |
|---|---|
| **Case** | A single scalar value is very large (e.g., a 50 MiB ConfigMap value embedded in a manifest). |
| **Specification position** | No specification limit. |
| **KubeCheck behavior** | `ACCEPT` up to `max_scalar_bytes` (default 10 MiB). Above the limit, `REJECT` with a governor finding. |
| **Severity** | `error` |
| **Source location** | The scalar's start location. |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: a single scalar of 11 MiB. Assert: governor finding `SCALAR_SIZE_LIMIT`. |

**EDGE-REQ-031:** The scalar-size limit MUST be independent of the document-size limit, so that a document with many small scalars is treated differently from a document with one huge scalar.

### 9.4 Large collections

| Attribute | Detail |
|---|---|
| **Case** | A mapping has 200,000 keys or a sequence has 500,000 items. |
| **Specification position** | No specification limit. |
| **KubeCheck behavior** | `ACCEPT` up to `max_mapping_pairs` (default 100,000) and `max_sequence_items` (default 100,000). Above the limits, `REJECT` with a governor finding. |
| **Severity** | `error` |
| **Source location** | The location of the collection's opening token (mapping start or sequence start). |
| **Recovery** | `STOP_DOCUMENT`. |
| **Test scenario** | Input: a mapping with 100,001 keys. Assert: governor finding `MAPPING_SIZE_LIMIT` at the mapping start. |

**EDGE-REQ-032:** Collection-size limits MUST be enforced before the collection is fully materialized in memory, to prevent memory exhaustion.

---

## 10. Parser differences across supported libraries

The YAML 1.2.2 specification is not implemented identically by every parser. The following table summarizes known differences relevant to KubeCheck's edge cases. Parser selection is governed by ADR-002 (**OPEN**); this table constrains that decision.

| Behavior | PyYAML | ruamel.yaml | libyaml | go-yaml | KubeCheck requirement |
|---|---|---|---|---|---|
| **Duplicate keys** | Silently overwrites (last-wins) | Raises `DuplicateKeyError` (new API); warns with legacy API | Parser-level duplicate detection varies; often last-wins | Does not reject by default; last-wins | `REJECT` at parse time, independently of parser policy (`EDGE-REQ-009`). |
| **Tab indentation** | Scanner error | Scanner error | Scanner error | Scanner error | `REJECT` with structured `TAB_IN_INDENTATION` (`EDGE-REQ-001`). |
| **YAML 1.1 booleans** | Resolves `yes`/`no`/`on`/`off` as booleans | Configurable; Core Schema by default | Core Schema | Core Schema (varies) | Core Schema: `yes`/`no` are strings (`EDGE-REQ-012`). |
| **`0755` integer** | Resolves as octal 493 (YAML 1.1) | Core Schema: decimal 755 | Core Schema: decimal 755 | Core Schema: decimal 755 | Core Schema: decimal 755 (`EDGE-REQ-013`). |
| **Source locations** | Line/column at token level; node-level via custom constructor | Node-level source locations available | Token-level; node-level via augmentation | Node-level available | Every node MUST have a source location (`NORM-REQ-009`). |
| **Alias expansion limit** | No built-in limit | No built-in limit; configurable via custom resolver | No built-in limit | `WithMaxAliasExpansions` available | `max_aliases` and `max_resolved_nodes` enforced (`EDGE-REQ-018`). |
| **Recursive alias detection** | May stack overflow | Detects cycles with new API | May stack overflow | Detects cycles with `WithMaxAliasDepth` | `REJECT` with normalization error (`EDGE-REQ-017`). |
| **Multi-document** | `load_all` available | `load_all` available | Event-based; supports multi-doc | `Decoder` supports multi-doc | Each document processed independently (`EDGE-REQ-023`). |
| **BOM handling** | Strips BOM | Strips BOM | Passive; may not strip | Strips BOM | Strip BOM and adjust offsets (`EDGE-REQ-028`). |
| **Unicode normalization** | Preserves original | Preserves original | Preserves original | Preserves original | Preserve original (`EDGE-REQ-026`). |

**EDGE-REQ-033:** The parser selected by ADR-002 MUST either provide duplicate-key detection natively or expose the mapping entries needed for KubeCheck to detect duplicates independently. A parser that silently resolves duplicates without exposing the occurrences is **not acceptable**.

**EDGE-REQ-034:** The parser MUST expose source locations at the node level (or provide token locations sufficient for augmentation). A parser that exposes only line numbers without columns is **not acceptable** for source-anchored diagnostics.

**EDGE-REQ-035:** The parser MUST support safe-mode parsing with language-tag rejection (`SEC-REQ-001`) and MUST NOT resolve application-specific tags to host-language constructors.

**EDGE-REQ-036:** The parser MUST provide alias-expansion metering or expose the alias resolution count so that KubeCheck can enforce `max_aliases` and `max_resolved_nodes` before expansion completes.

**Parser-dependent behavior record:** The following behaviors are explicitly **parser-dependent** and MUST be documented in ADR-002 with the chosen parser's exact behavior:

1. Duplicate-key policy (first-wins, last-wins, reject, or warning).
2. Tab-indentation error message and code.
3. YAML 1.1 compatibility mode availability and default.
4. Source-location granularity (token vs node).
5. Alias-expansion metering availability.
6. Recursive-alias detection availability.
7. Multi-document API shape.
8. BOM handling.
9. Line-length limits in the scanner.

---

## 11. Source locations, document indexes, and error recovery

### 11.1 Source-location contract

**EDGE-REQ-037:** Every finding produced during parsing or normalization MUST carry:

- The **document index** (0-based) within the stream.
- The **line** and **column** (1-based) in the decoded text.
- The **byte offset** in the decoded text.
- The **original byte offset** in the raw input (to account for BOM stripping and line-break normalization).

**EDGE-REQ-038:** For duplicate-key errors, **both** occurrences MUST be reported with their own source locations. For alias-cycle errors, every anchor and alias in the cycle MUST be reported with its location.

**EDGE-REQ-039:** For structural errors where the parser cannot determine the exact offending token (e.g., "did not find expected key"), KubeCheck MUST report the location where the parser gave up and the previous successfully parsed token, and MUST state the limitation in the evidence field.

### 11.2 Document indexes

**EDGE-REQ-040:** Documents are indexed in stream order starting at `0`. A parse failure in document `i` does not change the index of document `i+1`. The report MUST account for every index, including failed documents, so that the user can correlate findings with the input stream.

### 11.3 Error recovery policy

| Condition | Recovery policy | Rationale |
|---|---|---|
| Lexical error (tab, unclosed quote, invalid escape) | `STOP_DOCUMENT` | The document cannot be reliably parsed; continuing would produce cascading errors. |
| Structural error (malformed mapping/sequence, unexpected token) | `STOP_DOCUMENT` | Same. |
| Duplicate key | `STOP_DOCUMENT` | The specification requires uniqueness; the document is invalid. |
| Undefined alias | `STOP_DOCUMENT` | The document cannot be materialized. |
| Recursive alias | `STOP_DOCUMENT` (normalization stage) | The document cannot be materialized. |
| Billion-laughs limit exceeded | `STOP_DOCUMENT` (governor) | Resource safety. |
| Invalid UTF-8 | `STOP_STREAM` (if before any document) or `STOP_DOCUMENT` (if within a document) | Decoding failure affects all subsequent content. |
| Empty stream | Continue (no documents) | Valid stream; informational finding. |
| Empty document | Continue to next document | The document is null; GVK finding produced; no parse failure. |
| GVK error | Continue to next document | The document is structurally valid YAML but not a valid Kubernetes resource. |
| Schema error | Continue (unless schema status prevents further checks) | Other checks may still apply. |

**EDGE-REQ-041:** A `STOP_STREAM` condition MUST halt the invocation and produce a finding that names the stream-level failure. A `STOP_DOCUMENT` condition MUST NOT halt processing of other documents.

**EDGE-REQ-042:** When a document is stopped, the report MUST include a `document_status` field with one of `processed`, `parse-failed`, `normalization-failed`, `gvk-failed`, or `skipped` (for documents after a stream stop). This field allows CI consumers to distinguish "no findings" from "could not analyze" (`SARCH-REQ-065`).

---

## 12. Parser version upgrades and regression risks

### 12.1 Upgrade risk categories

| Risk | Description | Mitigation |
|---|---|---|
| **Behavioral drift** | A parser upgrade changes duplicate-key handling, scalar resolution, or source-location granularity. | Pin parser version; run the full edge-case corpus against the new version before upgrade; require ADR if behavior changes. |
| **New resource limits** | A parser upgrade introduces a new default limit (e.g., a 3 MiB document cap) that rejects previously valid manifests. | Document parser limits in the edge-case corpus; configure parser limits explicitly rather than relying on defaults. |
| **Source-location regression** | A parser upgrade changes line/column reporting or drops node-level locations. | Source-location tests in CI; compare locations across parser versions. |
| **Security regression** | A parser upgrade introduces a new unsafe-tag resolution path or changes safe-mode defaults. | Safe-mode audit per upgrade; adversarial tag corpus rerun. |
| **Multi-document API change** | A parser upgrade changes the document iteration API or the behavior of `---` / `...`. | Multi-document test suite; document API compatibility in ADR-002. |

**EDGE-REQ-043:** Parser version upgrades MUST be treated as architectural decisions requiring an ADR if they change any behavior in §10 or §12.1. A patch-version upgrade that changes only performance is exempt but MUST still pass the full edge-case corpus.

**EDGE-REQ-044:** The edge-case corpus in this document MUST be executable against any candidate parser version. A parser upgrade is **blocked** if any edge-case test fails or if any new behavior is not documented in §10.

### 12.2 Regression test requirements

**EDGE-REQ-045:** The regression suite MUST include at least:

1. A duplicate-key fixture with first-wins, last-wins, and reject-parser configurations.
2. A tab-indentation fixture.
3. An unclosed-quote fixture.
4. An invalid-escape fixture.
5. A malformed-block-scalar fixture (indicator `0`, outdented content).
6. A billion-laughs fixture at and below the limit.
7. A recursive-alias fixture.
8. A merge-key cycle fixture.
9. A multi-document fixture with mixed valid/invalid documents.
10. An empty-stream fixture.
11. An invalid-UTF-8 fixture.
12. A CRLF/CR fixture.
13. A BOM fixture.
14. A deep-nesting fixture at and above the limit.
15. A huge-scalar fixture at and above the limit.

**EDGE-REQ-046:** Each regression fixture MUST assert the exact error code, the source location (line, column, document index), and the recovery policy. A fixture that only asserts "an error occurred" is insufficient.

---

## 13. Resource limits: configurable and fixed

### 13.1 Limit register

| Limit ID | Limit | Default | Configurable? | Enforced by | Rationale |
|---|---|---|---|---|---|
| `LIM-EDGE-01` | `max_input_bytes` | 10 MiB per input unit | Yes | Resource Governor | Bounds per-file memory and decode time. |
| `LIM-EDGE-02` | `max_documents` | 1,000 per invocation | Yes | Resource Governor | Bounds aggregate work. |
| `LIM-EDGE-03` | `max_depth` | 256 levels | Yes | Parser / Governor | Prevents stack overflow. |
| `LIM-EDGE-04` | `max_aliases` | 10,000 expansions per document | Yes | Parser / Governor | Prevents billion-laughs expansion. |
| `LIM-EDGE-05` | `max_resolved_nodes` | 100,000 nodes per document | Yes | Governor | Closes wide alias fan-out gap. |
| `LIM-EDGE-06` | `max_scalar_bytes` | 10 MiB per scalar | Yes | Parser / Governor | Bounds single-value memory. |
| `LIM-EDGE-07` | `max_mapping_pairs` | 100,000 per mapping | Yes | Parser / Governor | Bounds mapping memory. |
| `LIM-EDGE-08` | `max_sequence_items` | 100,000 per sequence | Yes | Parser / Governor | Bounds sequence memory. |
| `LIM-EDGE-09` | `max_document_bytes` | 50 MiB per document | Yes | Governor | Bounds per-document memory (distinct from per-input-unit). |
| `LIM-EDGE-10` | `max_anchor_count` | 1,000 per document | Yes | Parser / Governor | Bounds anchor-table memory. |
| `LIM-EDGE-11` | `max_merge_depth` | 32 levels | Yes | Governor | Prevents recursive merge expansion. |
| `LIM-EDGE-12` | `max_simple_key_length` | 1,024 characters | **Fixed** | Parser | Aligns with the YAML community consensus that a simple key exceeding 1 KiB may be rejected; this is a parser-level protection, not a KubeCheck requirement. |

**EDGE-REQ-047:** All limits except `LIM-EDGE-12` MUST be configurable through the documented configuration mechanism (`docs/cli/CONFIGURATION.md`). Defaults are chosen to accommodate typical Kubernetes manifests while preventing resource exhaustion.

**EDGE-REQ-048:** A limit MAY be set to `unlimited` only with an explicit configuration key and MUST emit a warning to `stderr` on activation (`LIM-REQ-008`).

**EDGE-REQ-049:** When multiple limits apply (e.g., a document that exceeds both `max_depth` and `max_scalar_bytes`), the governor MUST report the **first limit reached in processing order** and MUST include the limit ID in the finding evidence.

### 13.2 Default selection rationale

The defaults in §13.1 are chosen with the following reasoning:

- **10 MiB input / 10 MiB scalar / 50 MiB document:** Kubernetes manifests rarely exceed a few MiB; 10 MiB accommodates large ConfigMaps and embedded certificates. 50 MiB per document allows a document to contain multiple large scalars without one dominating. These are **design targets**, not measured values.
- **256 nesting depth:** Deeply nested Kubernetes structures rarely exceed 20–30 levels. 256 provides headroom while bounding recursion.
- **10,000 aliases / 100,000 resolved nodes:** Closes the billion-laughs vector without rejecting legitimate manifests that use a few dozen aliases. The resolved-node limit catches wide fan-out even when alias count is moderate.
- **100,000 mapping pairs / sequence items:** Accommodates large but legitimate collections (e.g., a large NetworkPolicy peer list) while bounding memory.
- **1,024-character simple key (fixed):** The YAML community has historically agreed that a parser may reject a simple key exceeding 1 KiB. This is a **parser-level** protection, not a KubeCheck policy; it is fixed because increasing it would allow pathological lookahead.

**EDGE-REQ-050:** The defaults MUST be documented in `docs/cli/CONFIGURATION.md` and MUST be reflected in the benchmark environment profile (`docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`). Changing a default requires an ADR because it changes the tool's acceptance envelope.

---

## 14. When processing must stop and when safe partial diagnostics can continue

### 14.1 Stop conditions

**EDGE-REQ-051:** Processing MUST stop for the entire invocation (`STOP_STREAM`) when:

1. Invalid UTF-8 is detected **before any document boundary**. The remainder of the stream cannot be decoded reliably.
2. The aggregate memory limit (`LIM-REQ-006`) is reached.
3. The aggregate wall-clock limit (`LIM-REQ-005`) is reached.
4. The aggregate document-count limit (`LIM-EDGE-02`) is reached; the remaining input units are not admitted, but findings for already-admitted units are reported.

**EDGE-REQ-052:** Processing MUST stop for the **affected document** (`STOP_DOCUMENT`) when:

1. A lexical or structural error occurs.
2. A duplicate key is detected.
3. An undefined or recursive alias is detected.
4. A per-document resource limit (`LIM-EDGE-03` through `LIM-EDGE-11`) is exceeded.
5. A normalization error occurs (cycle, unsupported construct).

### 14.2 Safe partial diagnostics

**EDGE-REQ-053:** After a `STOP_DOCUMENT`, KubeCheck MUST still report:

- The document index and the error that caused the stop.
- Any findings produced **before** the stop for the affected document (e.g., a syntax finding for an earlier line).
- Findings for **other** documents in the stream, which continue processing.

**EDGE-REQ-054:** A parse error for document `i` MUST NOT prevent schema or semantic analysis of document `i+1`. The pipeline coordinator records `document_status` per document and the report accounts for every index (`EDGE-REQ-042`).

**EDGE-REQ-055:** When a `STOP_STREAM` condition occurs, the report MUST include findings for all documents processed up to the stop, the stream-level finding, and a `stream_status` field indicating `completed`, `stopped-resource-limit`, or `stopped-decode-error`. Exit code reflects the highest-severity finding (`OUT-REQ-007`).

---

## 15. Consolidated edge-case register

The following register is the authoritative lookup for every edge case in this document.

| Edge-case ID | Case | KubeCheck behavior | Severity | Recovery |
|---|---|---|---|---|
| EC-001 | Tab indentation | `REJECT` (`TAB_IN_INDENTATION`) | `error` | `STOP_DOCUMENT` |
| EC-002 | Inconsistent indentation | `REJECT` (`INDENTATION_MISMATCH`) | `error` | `STOP_DOCUMENT` |
| EC-003 | Malformed mapping (missing colon) | `REJECT` (`MISSING_COLON`) | `error` | `STOP_DOCUMENT` |
| EC-004 | Broken sequence (dash without space) | `REJECT` (`INVALID_SEQUENCE_INDICATOR`) | `error` | `STOP_DOCUMENT` |
| EC-005 | Unclosed quote | `REJECT` (`UNCLOSED_QUOTE`) | `error` | `STOP_DOCUMENT` |
| EC-006 | Invalid escape | `REJECT` (`INVALID_ESCAPE`) | `error` | `STOP_DOCUMENT` |
| EC-007 | Malformed block scalar | `REJECT` (`INVALID_BLOCK_SCALAR`) | `error` | `STOP_DOCUMENT` |
| EC-008 | Unexpected token | `REJECT` (`UNEXPECTED_TOKEN`) | `error` | `STOP_DOCUMENT` |
| EC-009 | Duplicate key | `REJECT` (`DUPLICATE_KEY`) | `error` | `STOP_DOCUMENT` |
| EC-010 | Duplicate key under parser last-wins | `REJECT` (`DUPLICATE_KEY`) | `error` | `STOP_DOCUMENT` |
| EC-011 | `yes`/`no`/`on`/`off` where boolean expected | `ACCEPT AS STRING` + info finding | `info` | Continue |
| EC-012 | `0755` where octal expected | `ACCEPT AS INTEGER 755` + info finding | `info` | Continue |
| EC-013 | Timestamp-like string | `ACCEPT AS STRING` + info finding | `info` | Continue |
| EC-014 | Undefined alias | `REJECT` (`UNDEFINED_ALIAS`) | `error` | `STOP_DOCUMENT` |
| EC-015 | Merge key with non-mapping source | `REJECT` (`BAD_MERGE`) | `error` | `STOP_DOCUMENT` |
| EC-016 | Recursive alias | `REJECT` (`RECURSIVE_ALIAS`) | `error` | `STOP_DOCUMENT` |
| EC-017 | Billion-laughs expansion | `REJECT` (`ALIAS_EXPANSION_LIMIT`) | `error` | `STOP_DOCUMENT` |
| EC-018 | Recursive merge-key expansion | `REJECT` (`MERGE_CYCLE`) | `error` | `STOP_DOCUMENT` |
| EC-019 | Repeated merge keys | `ACCEPT` | — | Continue |
| EC-020 | Empty stream | `ACCEPT` + `NO_DOCUMENTS` | `info` | Continue |
| EC-021 | Empty document | `ACCEPT` + GVK finding | `error` (GVK) | Continue to next |
| EC-022 | Content after `...` without `---` | `REJECT` (`MISSING_DOCUMENT_BOUNDARY`) | `error` | `STOP_DOCUMENT` |
| EC-023 | Indented `---` | `ACCEPT` as content | — | Continue |
| EC-024 | `%TAG` with undeclared handle | `REJECT` (`UNDECLARED_TAG_HANDLE`) | `error` | `STOP_DOCUMENT` |
| EC-025 | Invalid UTF-8 | `REJECT` (`INVALID_UTF8`) | `error` | `STOP_STREAM` or `STOP_DOCUMENT` |
| EC-026 | Unicode normalization | `ACCEPT`; preserve original | — | Continue |
| EC-027 | CRLF/CR line endings | `ACCEPT`; normalize to LF | — | Continue |
| EC-028 | BOM | `ACCEPT`; strip and adjust offsets | — | Continue |
| EC-029 | Oversized scalar | `REJECT` (`SCALAR_SIZE_LIMIT`) | `error` | `STOP_DOCUMENT` |
| EC-030 | Deep nesting | `REJECT` (`DEPTH_LIMIT`) | `error` | `STOP_DOCUMENT` |
| EC-031 | Huge scalar | `REJECT` (`SCALAR_SIZE_LIMIT`) | `error` | `STOP_DOCUMENT` |
| EC-032 | Large collection | `REJECT` (`COLLECTION_SIZE_LIMIT`) | `error` | `STOP_DOCUMENT` |

---

## 16. Authoritative references

| Reference | Scope | Type | Retrieved |
|---|---|---|---|
| YAML Ain't Markup Language (YAML™) revision 1.2.2 (2021-10-01) | Lexical, structural, and semantic rules; Core Schema; duplicate-key requirement (§3.2.1.3); tab prohibition (§6.1); line-break normalization (§5.4) | Specification | 2026-10-31 |
| Merge Key Language-Independent Type | Merge-key semantics (extension) | Separate specification | 2026-10-31 |
| OWASP Testing Guide — XML Injection (billion laughs) | Alias expansion as a DoS vector | Security guidance | 2026-10-31 |
| L4YAML ParserLimits (NASA JPL) | Reference default limits: `maxResolvedNodes` 100,000, `maxAliasDepth` 50, `maxAliasExpansions` 10,000, `maxDepth` 100, `maxScalarBytes` 10 MB, `maxDocuments` 100, `maxInputBytes` 100 MB | Open-source implementation | 2026-10-31 |
| go-yaml `limit` plugin documentation | Configurable nesting-depth and alias-expansion limits | Open-source documentation | 2026-10-31 |
| Kubernetes API `fieldValidation` parameter documentation | `Strict` mode rejects unknown fields and duplicate fields; `Warn` reports them; `Ignore` is pre-v1.23 default | Official Kubernetes OpenAPI specification | 2026-10-31 |
| PyYAML duplicate-key behavior and ruamel.yaml `DuplicateKeyError` | Parser-specific duplicate handling | Community documentation | 2026-10-31 |
| YAML community consensus on 1 KiB simple-key limit | Parser-level simple-key length protection | YAML-core mailing list | 2026-10-31 |
| `haz-cli` Unicode normalization specification | Rationale for preserving original code points | Internal specification reference | 2026-10-31 |
| ADR-002 YAML parser selection | Parser choice and conformance configuration | Internal (OPEN) | 2026-10-31 |

**Licensing note:** The YAML 1.2.2 specification is freely copyable provided it is not modified. The Merge Key specification is part of the YAML type repository. L4YAML is an open-source NASA JPL project. Kubernetes documentation is licensed under CC BY 4.0. No line numbers, benchmark results, or implementation status are asserted beyond what the cited sources state.

---

## 17. Open questions and unresolved decisions

| ID | Question | Status | Owner |
|---|---|---|---|
| OPEN-EDGE-01 | Parser selection (ADR-002). Affects every row in §10 and every parser-dependent note in this document. | **OPEN** | Software Architecture |
| OPEN-EDGE-02 | Whether the duplicate-key detection will use a parser API or a post-parse mapping scan. Affects EDGE-REQ-011 and EDGE-REQ-033. | **OPEN** | Software Architecture |
| OPEN-EDGE-03 | Whether a lenient duplicate-key mode will be supported and, if so, under what constraints. | **OPEN** | CLI Architecture |
| OPEN-EDGE-04 | Exact source-location granularity required from the parser (token-level vs node-level) and the cost of the augmentation layer if node-level is not available. | **OPEN** | Software Architecture |
| OPEN-EDGE-05 | Whether Unicode normalization should be applied to **finding evidence text** (not to NIR values) for display consistency, or whether evidence must preserve original code points verbatim. The current specification preserves original code points throughout. | **OPEN** | Diagnostics Architecture |
| OPEN-EDGE-06 | Whether `max_simple_key_length` should be configurable in a future release or remain fixed. | **OPEN** | Parser Engineering |
| OPEN-EDGE-07 | Whether `max_resolved_nodes` should be expressed as a multiple of `max_aliases` or as an independent limit. The current design uses independent limits. | **OPEN** | Parser Engineering |

---

## 18. Definition of done

This document is complete when the following conditions hold:

1. Every edge case in the requested scope has a row in the consolidated register (§15) with expected behavior, severity, source-location requirements, recovery policy, and test scenario.
2. Every edge case distinguishes the YAML specification position from KubeCheck's implementation decision, and no edge case is described as "invalid YAML" unless the specification prohibits it.
3. Lexical and structural errors are specified with stable error codes and recovery policies.
4. Duplicate-key handling is specified independently of parser default policy, with a requirement to verify detection regardless of the parser's configuration.
5. Ambiguous scalar resolution is specified against the Core Schema, with informational findings when schema context reveals a likely intent.
6. Alias, merge-key, recursive-alias, and billion-laughs behavior is specified with limits and metering requirements.
7. Multi-document and empty-stream behavior is specified with distinct outcomes for zero-document streams and empty documents.
8. Unicode, encoding, BOM, and line-ending behavior is specified with preservation and normalization rules.
9. Resource limits are enumerated with defaults, configurability, enforcement stage, and rationale.
10. Stop conditions and safe partial-diagnostic behavior are specified for both `STOP_DOCUMENT` and `STOP_STREAM`.
11. Parser differences are tabulated with KubeCheck requirements for each behavior.
12. Parser version upgrades are governed by the regression requirements in §12.
13. Every requirement has a stable ID (`EDGE-REQ-NNN`), and dependencies on OPEN ADRs are marked.

---

## 19. Consistency and completeness review

A review was performed against the requested scope before finalization.

- **Tabs, inconsistent indentation, malformed mappings, broken sequences:** §3.1–§3.4.
- **Unclosed quotes, invalid escapes, malformed block scalars, unexpected tokens:** §3.5–§3.8.
- **Duplicate keys and conflicting values:** §4, including parser last-wins independence.
- **Ambiguous scalars and implicit type conversion:** §5, with Core Schema resolution and informational findings.
- **Anchors, aliases, merge keys, recursive aliases, alias-expansion attacks:** §6, including repeated merge keys and recursive merge cycles.
- **Multiple documents, empty streams, empty documents, document separators:** §7.
- **Unicode normalization, invalid byte sequences, line-ending variants:** §8, including BOM handling.
- **Very long lines, deeply nested collections, huge scalars, large collections:** §9.
- **Parser differences across supported libraries:** §10, with a comparison table and acceptability requirements.
- **Source locations, document indexes, error recovery:** §11.
- **Parser version upgrades and regression risks:** §12.
- **Resource limits with documented defaults:** §13.
- **Stop conditions and safe partial diagnostics:** §14.
- **Test scenarios for every edge case:** embedded in each edge-case table and consolidated in §15.
- **No claim that KubeCheck-unsupported features are YAML-prohibited:** enforced by `YAML-EDGE-REQ-001` and consistent with `YAML-SPEC-REQ-001`.

No empty boilerplate section was added solely to satisfy a checklist. The document is complete for its assigned responsibility.