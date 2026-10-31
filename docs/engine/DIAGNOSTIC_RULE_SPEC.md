# KubeCheck — Diagnostic Rule Specification

**Document ID:** RULE-SPEC-001  
**File:** `docs/engine/DIAGNOSTIC_RULE_SPEC.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Diagnostic Engine Architecture  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/architecture/SYSTEM_ARCHITECTURE.md`
- `docs/architecture/REQUIREMENTS_AND_NFRS.md`
- `docs/architecture/OFFLINE_KNOWLEDGE_BUNDLES.md`
- `docs/architecture/ADR/ADR-003-knowledge-bundle-format.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md`
- `docs/yaml/YAML_SECURITY_AND_PARSER_POLICY.md`
- `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`
- `docs/kubernetes/KUBERNETES_VERSION_COMPATIBILITY.md`
- `docs/output/JSON_SCHEMA.md`
- `docs/testing/DETERMINISM_TESTS.md`
- `docs/testing/ACCURACY_BENCHMARK_PROTOCOL.md`

---

## 1. Purpose and scope

This document specifies the **complete diagnostic rule format and execution contract** for KubeCheck's declarative diagnostic engine. It is the normative specification for:

- Rule identifiers and namespaces.
- Rule schema and schema versioning.
- Rule metadata fields: title, description, category, severity, confidence, evidence, remediation, references, applicability.
- Preconditions, match predicates, exclusions, and dependency declarations.
- Kubernetes version and resource-kind applicability.
- Diagnostic source locations and resource identities.
- Duplicate suppression, rule ordering, and deterministic output.
- Rule dependencies, conflict resolution, and exception handling.
- Rule deprecation, replacement, and backward compatibility.
- Safe expression evaluation without arbitrary code execution.
- Performance limits and invalid-rule behavior.
- Unit tests with positive, negative, and boundary fixtures.

### 1.1 Governing principles

> **RULE-SPEC-REQ-001 — Rules are data, not code.** A rule is a versioned, declarative artifact evaluated by a fixed engine. The engine's code does not change when a rule changes; a bundle update alone can add, modify, or remove rules.
>
> **RULE-SPEC-REQ-002 — Every rule is independently testable.** A rule must declare positive, negative, and boundary fixtures. A rule without fixtures cannot enter a bundle.
>
> **RULE-SPEC-REQ-003 — Every rule declares its evidence class.** A rule is `deterministic`, `probable`, or `context-dependent`. The engine enforces that the emitted finding's evidence class matches the rule's declared class.
>
> **RULE-SPEC-REQ-004 — Invalid rules cannot execute.** A rule that fails schema validation or safe-expression validation is rejected at bundle load time; the affected rule is disabled and a structured engine finding is emitted. The bundle remains loadable unless the manifest declares the rule as `required: true`.
>
> **RULE-SPEC-REQ-005 — Rule execution is deterministic.** Given the same NIR, inventory, relationship graph, target version, and bundle, the engine produces byte-identical findings. No wall-clock time, randomness, environment variable, or iteration-order dependence is permitted.

### 1.2 In scope

Rule format, execution contract, safe-expression language, validation, testing, determinism, deprecation, and the complete illustrative example.

### 1.3 Out of scope

- The diagnostic taxonomy's category semantics (see `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`).
- The NIR and relationship graph definitions (see `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md` §16 and `SYSTEM_ARCHITECTURE.md` §6.13).
- The bundle binary format (see ADR-003).
- Output schema field-by-field (see `docs/output/JSON_SCHEMA.md`).
- Network isolation guarantees (see `SYSTEM_ARCHITECTURE.md` §10).

### 1.4 Assumptions

- **ASSUMPTION-RULE-01:** The NIR is available to the rule engine as specified in `YAML_VALIDATION_AND_NORMALIZATION.md` §16, with every node carrying a source location.
- **ASSUMPTION-RULE-02:** The relationship graph is available as specified in `SYSTEM_ARCHITECTURE.md` §6.13, with typed edges.
- **ASSUMPTION-RULE-03:** The target Kubernetes version is supplied explicitly; the rule applicability filter has a deterministic default when it is not (`K8S-VER-REQ-040`).
- **ASSUMPTION-RULE-04:** The rule authoring format is YAML, parsed by the same safe parser as manifests (`YAML_SECURITY_AND_PARSER_POLICY.md` §4).

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **Rule** | A declarative artifact that describes a defect, its matching conditions, its evidence, and its remediation. |
| **Rule pack** | A collection of rules distributed as a versioned, signed artifact within a knowledge bundle. |
| **Rule engine** | The fixed component that loads, validates, orders, and evaluates rules. |
| **Match predicate** | A safe expression evaluated against NIR nodes, inventory entries, or relationship-graph edges to determine whether a rule applies. |
| **Precondition** | A coarse filter that determines whether a rule is applicable at all (e.g., target version in range, kind in list). |
| **Exclusion** | A condition that suppresses a finding even when the match predicate holds (e.g., an annotation opt-out). |
| **Evidence class** | `deterministic`, `probable`, or `context-dependent` (see `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`). |
| **Finding** | A single diagnostic record produced by a rule (see `SYSTEM_ARCHITECTURE.md` §6.16). |
| **Applicability** | The set of target versions and resource kinds for which a rule is evaluated. |
| **Safe expression** | A restricted expression evaluated by the engine's expression interpreter, with no host-language escape. |
| **Rule pack manifest** | The metadata file declaring a rule pack's version, rules, dependencies, and signature. |

**Requirement IDs:** `RULE-SPEC-REQ-NNN`.  
**Rule IDs** use the format defined in §3.  
**Finding IDs** are produced by rules; the rule ID is the prefix.  
**Test IDs:** `TEST-RULE-NNN`.

---

## 3. Rule identifiers and namespaces

### 3.1 Rule ID format

A rule ID is a stable, human-readable identifier composed of segments separated by hyphens:

```
<DOMAIN>-<FAMILY>-<NNN>
```

| Segment | Definition | Example |
|---|---|---|
| `<DOMAIN>` | Diagnostic domain: `YAML`, `K8S`, `SEC`, `SCHED`, `NET`, `STG`, `DEP` (deprecation), `RES` (resource governor). | `SEC` |
| `<FAMILY>` | Family within the domain. | `CTX` (security context) |
| `<NNN>` | Three-digit sequence, zero-padded. | `001` |

Example: `SEC-CTX-001` is the first rule in the security-context family.

### 3.2 Namespace ownership

| Domain | Owner | Example families |
|---|---|---|
| `YAML` | YAML parser team | `SYN` (syntax), `ESC` (escape), `ALIAS` |
| `K8S` | Core Kubernetes diagnostics | `GVK`, `SCHEMA`, `META` |
| `SEC` | Security diagnostics | `CTX`, `RBAC`, `SECRET`, `IMG`, `HOST`, `NET` |
| `SCHED` | Scheduling and capacity | `QTY`, `QOS`, `NODE`, `AFF`, `TAINT`, `PDB`, `HPA`, `QUOTA`, `LIMIT`, `STOR`, `ARCH` |
| `NET` | Networking | `SVC`, `ING`, `GW`, `NP`, `DNS`, `TLS`, `HL`, `EXT` |
| `STG` | Configuration and storage | `CM`, `SEC`, `ENV`, `VOL`, `MNT`, `PROJ`, `ED`, `CSI`, `PVC`, `PV`, `STS` |
| `DEP` | Deprecation and lifecycle | `API`, `GVK` |
| `RES` | Resource governor | `SIZE`, `DEPTH`, `ALIAS`, `TIME`, `MEM` |

**RULE-SPEC-REQ-006:** A rule ID MUST be unique within a bundle. Two rules with the same ID MUST cause bundle-load rejection.

**RULE-SPEC-REQ-007:** A rule ID MUST NOT be renumbered or reused. A rule may be superseded by a new rule with a new ID; the original ID MUST remain as a deprecated alias for the remainder of the major version (`DEP-REQ-003`).

### 3.3 Rule naming

Each rule has a human-readable `name` field (e.g., `privileged-container`). The name is a stable slug used in documentation and CLI output. The name MAY change between major versions; the rule ID MUST NOT.

---

## 4. Rule schema and schema version

### 4.1 Schema versioning

Rule packs are versioned by a **rule schema version** declared in the pack manifest:

```yaml
rule_schema_version: "1.0.0"
```

**RULE-SPEC-REQ-008:** The engine MUST declare the set of rule schema versions it supports. A rule pack declaring an unsupported schema version MUST be rejected with a structured error naming both the pack's version and the engine's supported range.

**RULE-SPEC-REQ-009:** A rule schema version MUST follow semantic versioning. A breaking change to the rule format (removing a required field, changing field semantics, changing the expression language) requires a major version increment. Adding an optional field requires a minor version increment.

**RULE-SPEC-REQ-010:** The engine MUST support at least the current and previous rule schema major versions (`COMP-REQ-002`).

### 4.2 Rule structure (top-level)

```yaml
# Rule top-level fields (all required unless marked optional)
id: <string>                    # e.g., "SEC-CTX-001"
name: <string>                  # stable slug, e.g., "privileged-container"
schema_version: <string>        # must match pack's rule_schema_version
metadata:
  title: <string>
  description: <string>
  category: <string>            # from DIAGNOSTIC_TAXONOMY
  severity: <string>            # error | warning | info
  evidence_class: <string>      # deterministic | probable | context-dependent
  confidence_basis: <string>    # short description of the evidence
  references: <list of objects> # optional
applicability:
  kubernetes_versions: <range or list>
  kinds: <list of GVK patterns>
preconditions: <list of safe expressions>  # optional
match: <safe expression>         # returns boolean
exclusions: <list of safe expressions>     # optional
evidence: <list of field bindings>         # required for finding content
remediation:
  message: <string>
  steps: <list of strings>       # optional
dependencies: <list of rule IDs>  # optional
tests:
  positive: <list of fixtures>   # required
  negative: <list of fixtures>   # required
  boundary: <list of fixtures>   # required
```

**RULE-SPEC-REQ-011:** Every top-level field in §4.2 MUST be present, except `preconditions`, `exclusions`, `dependencies`, and `remediation.steps`, which are optional.

### 4.3 Rule metadata fields

| Field | Type | Required | Constraints |
|---|---|---|---|
| `title` | string | Yes | ≤ 80 characters; no trailing period. |
| `description` | string | Yes | ≤ 2,000 characters; may contain Markdown. |
| `category` | string | Yes | Must be one of the categories in `DIAGNOSTIC_TAXONOMY.md` (e.g., `syntax`, `schema`, `semantic`, `security`, `maintainability`, `operational`, `lifecycle`, `resource`). |
| `severity` | string | Yes | One of `error`, `warning`, `info`. |
| `evidence_class` | string | Yes | One of `deterministic`, `probable`, `context-dependent`. |
| `confidence_basis` | string | Yes | ≤ 200 characters; describes the evidence supporting the finding. |
| `references` | list | No | Each entry has `url`, `title`, `section` (optional), `retrieved` (ISO-8601 date). |
| `tags` | list | No | Free-form tags for filtering (e.g., `["security", "pod-security-standards"]`). |

**RULE-SPEC-REQ-012:** The `category` field MUST match a category defined in `DIAGNOSTIC_TAXONOMY.md`. An unknown category MUST cause rule-load rejection.

**RULE-SPEC-REQ-013:** The `evidence_class` field determines the maximum severity the rule may emit (`RULE-SPEC-REQ-014`). A rule MAY emit a lower severity than the maximum; it MUST NOT emit a higher severity.

### 4.4 Evidence-class-to-severity constraint

| Evidence class | Permitted severities |
|---|---|
| `deterministic` | `error`, `warning`, `info` |
| `probable` | `warning`, `info` |
| `context-dependent` | `warning`, `info` |

**RULE-SPEC-REQ-014:** A `probable` or `context-dependent` rule MUST NOT emit an `error`-severity finding. The engine MUST reject a rule that declares `severity: error` with a non-`deterministic` evidence class.

---

## 5. Rule metadata: evidence, remediation, and references

### 5.1 Evidence binding

The `evidence` field binds NIR values, inventory fields, and graph edges to the finding's evidence object. Each evidence binding has:

| Field | Type | Required | Meaning |
|---|---|---|---|
| `name` | string | Yes | Key in the finding's evidence object. |
| `source` | enum | Yes | One of `field_path`, `resource_identity`, `graph_edge`, `computed`. |
| `path` | string | Conditional | JSON path or `CEL`-like path into the NIR (for `field_path`). |
| `expression` | string | Conditional | Safe expression producing a value (for `computed`). |
| `redact` | bool | No | If `true`, the value is redacted before being included in the finding. Default `false`. |

**RULE-SPEC-REQ-015:** An evidence binding with `source: field_path` MUST resolve to an existing NIR node. If the path does not resolve, the engine MUST emit an `ENGINE-EVIDENCE-UNRESOLVED` internal finding and MUST NOT emit the rule's finding.

**RULE-SPEC-REQ-016:** A finding that references a secret-bearing field MUST set `redact: true`. The engine MUST enforce the secret-bearing field set defined in `docs/security/THREAT_MODEL.md` and MUST reject a rule that references such a field without `redact: true` (`SEC-YAML-REQ-031`).

**RULE-SPEC-REQ-017:** Evidence values MUST be JSON-serializable. The engine MUST reject an evidence expression that produces a non-serializable value.

### 5.2 Remediation

The `remediation` object has:

| Field | Type | Required | Meaning |
|---|---|---|---|
| `message` | string | Yes | One-paragraph description of the fix. ≤ 500 characters. |
| `steps` | list of strings | No | Ordered remediation steps. ≤ 20 steps, each ≤ 200 characters. |
| `safe_pattern` | string | No | A template the user can adapt. MUST NOT contain secrets. |

**RULE-SPEC-REQ-018:** Remediation text MUST NOT echo secret values (`SEC-REQ-014`). The engine MUST scan the remediation text for any value present in a secret-bearing field of the analyzed manifest and MUST reject the rule if a match is found.

**RULE-SPEC-REQ-019:** Remediation MUST NOT recommend a fix that itself violates a security control (e.g., "set `privileged: true` to fix a capability error"). The engine MUST scan remediation text against the security-check rules and MUST reject the rule if a recommended change matches an `error`-severity security finding.

### 5.3 References

**RULE-SPEC-REQ-020:** Every rule MUST cite at least one authoritative reference in `references` (Kubernetes documentation, YAML specification, CVE, or internal document path). A rule without a reference MUST be rejected at load time.

---

## 6. Applicability: Kubernetes versions and resource kinds

### 6.1 Applicability declaration

The `applicability` object declares the target versions and resource kinds for which the rule is evaluated:

```yaml
applicability:
  kubernetes_versions:
    min: "1.29"
    max: "1.35"
    except: ["1.30.0"]
  kinds:
    - group: "apps"
      versions: ["v1"]
      kinds: ["Deployment", "StatefulSet", "DaemonSet"]
    - group: ""
      versions: ["v1"]
      kinds: ["Pod"]
```

### 6.2 Version range semantics

**RULE-SPEC-REQ-021:** The `kubernetes_versions` field MUST support `min`, `max`, and `except`. The `min` and `max` bounds are inclusive. A target version must satisfy `min ≤ target ≤ max` and must not be in `except`.

**RULE-SPEC-REQ-022:** A target version outside the rule's range MUST cause the rule to be skipped for that version. The engine MUST NOT emit a finding stating that the rule was skipped; the skip is internal.

**RULE-SPEC-REQ-023:** If `kubernetes_versions` is omitted, the rule applies to all target versions in the bundle's supported window.

### 6.3 Kind matching

**RULE-SPEC-REQ-024:** The `kinds` field is a list of GVK patterns. A pattern may specify `group`, `versions`, and `kinds`. Wildcards (`*`) are permitted in `group` and `kinds` but not in `versions`.

**RULE-SPEC-REQ-025:** A manifest object matches a kind pattern if its GVK satisfies every specified field. The engine MUST NOT interpret a wildcard as matching an empty group (the core group is explicit: `group: ""`).

**RULE-SPEC-REQ-026:** A rule that declares no `kinds` MUST apply only to rules that operate on the document root or the NIR globally (e.g., syntax rules). The engine MUST reject a rule that declares a `match` expression referencing a specific field without declaring a `kinds` pattern.

---

## 7. Preconditions and match predicates

### 7.1 Preconditions

A **precondition** is a coarse filter evaluated before the `match` expression. Preconditions are cheap; they reduce the number of rules that need full evaluation. Each precondition is a safe expression returning a boolean.

**RULE-SPEC-REQ-027:** If any precondition returns `false`, the rule is skipped for the current object. The engine MUST NOT evaluate the `match` expression or emit any finding.

**RULE-SPEC-REQ-028:** Preconditions MUST NOT have side effects. The expression language provides no assignment, no I/O, no function calls except documented pure functions.

### 7.2 Match predicate

The `match` field is a safe expression evaluated against the current NIR node, the resource inventory entry, and the relationship graph. It MUST return a boolean.

**RULE-SPEC-REQ-029:** A rule MUST have exactly one `match` expression. Composite conditions are expressed with `&&`, `||`, `!`, and parentheses within the expression.

**RULE-SPEC-REQ-030:** The `match` expression MUST NOT contain host-language escape, object construction, or reflection. The expression language is specified in §13.

### 7.3 Exclusions

An **exclusion** is a safe expression that, when `true`, suppresses the finding even if `match` is `true`. Exclusions are used for documented opt-outs (e.g., a namespace annotation that says "this workload is intentionally privileged").

**RULE-SPEC-REQ-031:** The engine MUST evaluate all exclusions before emitting the finding. If any exclusion returns `true`, the finding MUST be suppressed and MUST NOT appear in the output.

**RULE-SPEC-REQ-032:** The engine MUST record the suppression internally for observability (log at `debug` level) but MUST NOT include suppressed findings in the output or in the exit-code computation.

**RULE-SPEC-REQ-033:** An exclusion MUST be based on manifest content, not on cluster state. An exclusion based on a cluster annotation would make the result non-reproducible offline.

---

## 8. Rule dependencies

### 8.1 Dependency declaration

A rule MAY declare `dependencies` as a list of rule IDs. A dependency means: **the dependent rule MUST NOT be evaluated if the dependency rule emitted a finding for the same object**.

**RULE-SPEC-REQ-034:** The engine MUST evaluate dependencies in topological order. A cycle in the dependency graph MUST cause bundle-load rejection.

**RULE-SPEC-REQ-035:** If a dependency rule emitted any finding for an object, the dependent rule MUST be skipped for that object. This prevents cascading findings (e.g., a schema error followed by a semantic finding that assumes schema validity).

### 8.2 Dependency example

```yaml
id: K8S-SEMANTIC-001
name: service-selector-matches-no-workload
dependencies:
  - K8S-SCHEMA-001   # skip if the Service is schema-invalid
```

If `K8S-SCHEMA-001` emits a schema error for the Service, `K8S-SEMANTIC-001` is skipped for that Service.

### 8.3 Conflict resolution

**RULE-SPEC-REQ-036:** Two rules conflict if they emit findings with the same evidence anchor and the same target, and one would suppress the other's precondition. Conflicts MUST be resolved by the following priority order:

1. A `deterministic` rule takes priority over a `probable` rule.
2. A `probable` rule takes priority over a `context-dependent` rule.
3. At the same evidence class, the rule with the lower rule ID (lexicographic) takes priority.

**RULE-SPEC-REQ-037:** The engine MUST NOT silently drop a conflicting finding. If two findings conflict, the engine emits both findings and adds a `conflict_with` field to each finding naming the other rule ID. The user can then reconcile.

---

## 9. Diagnostic source locations and resource identities

### 9.1 Source anchor

**RULE-SPEC-REQ-038:** Every finding emitted by a rule MUST carry a `source_anchor` that resolves through the source map (`SARCH-REQ-022`). The anchor contains `file`, `doc_index`, `line`, `column`, and `json_path`.

**RULE-SPEC-REQ-039:** The rule MUST declare an `anchor` field specifying where the anchor resolves:

| `anchor.type` | Meaning |
|---|---|
| `match_node` | The NIR node that the `match` expression evaluated as `true`. |
| `field_path` | A specific field path declared in `anchor.path`. |
| `resource_root` | The root of the analyzed object. |
| `graph_edge` | The edge that triggered the match. |

**RULE-SPEC-REQ-040:** If `anchor.type` is `field_path` but the path does not resolve, the engine MUST fall back to `resource_root` and MUST include a `anchor_fallback` note in the finding's evidence.

### 9.2 Resource identity

**RULE-SPEC-REQ-041:** Every finding MUST carry the `resource_identity` of the analyzed object: `(group, version, kind, namespace, name)` (`NORM-REQ-048`). If the identity is unresolved (e.g., `metadata.name` absent), the finding MUST carry `resource_identity_unresolved: true`.

---

## 10. Duplicate suppression and deterministic output

### 10.1 Duplicate suppression

**RULE-SPEC-REQ-042:** The engine MUST deduplicate findings that are logically identical: same `rule_id`, same `source_anchor` (file, doc_index, line, column, json_path), same `resource_identity`. The deduplication is performed by the aggregator (`SARCH-REQ-043`), not by individual rules.

**RULE-SPEC-REQ-043:** A rule MUST NOT emit the same finding twice for the same anchor. If a rule's `match` expression could match multiple times (e.g., iterating over containers), the rule MUST emit one finding per matched sub-anchor, not one finding per container with a combined evidence list. The rule's `anchor.type` MUST be `field_path` with the sub-anchor path in this case.

### 10.2 Deterministic output

**RULE-SPEC-REQ-044:** Findings MUST be emitted in canonical order: `(origin.logical_path, doc_index, line, column, category_rank, rule_id, evidence_class_rank)` (`SARCH-REQ-043`).

**RULE-SPEC-REQ-045:** Rule evaluation order MUST be deterministic: the engine evaluates rules in `(domain, family, NNN)` lexicographic order, subject to dependency topology. The engine MUST NOT depend on the order of rules in the pack manifest.

**RULE-SPEC-REQ-046:** For a fixed NIR, inventory, graph, target version, and bundle, repeated runs MUST produce byte-identical output (`ACC-REQ-001`).

---

## 11. Rule deprecation, replacement, and compatibility

### 11.1 Deprecation

A rule MAY be deprecated by adding a `deprecation` object:

```yaml
deprecation:
  deprecated_in: "v0.3.0"
  removed_in: "v1.0.0"
  reason: "Superseded by SEC-CTX-042"
  replacement: "SEC-CTX-042"
```

**RULE-SPEC-REQ-047:** A deprecated rule MUST remain evaluable until `removed_in`. When it emits a finding, the finding MUST include `deprecated: true` and `replacement: <rule-id>`.

**RULE-SPEC-REQ-048:** A rule MUST NOT be removed within a KubeCheck major version unless it was first deprecated in a prior minor release (`DEP-REQ-003`).

### 11.2 Replacement

**RULE-SPEC-REQ-049:** A replacement rule MUST cover at least the same cases as the deprecated rule, as verified by the deprecated rule's positive fixtures. The bundle verifier MUST run the deprecated rule's fixtures against the replacement rule and MUST block the bundle if any fixture fails.

### 11.3 Compatibility

**RULE-SPEC-REQ-050:** The rule schema version MUST remain stable within a KubeCheck major version. A rule schema change that would invalidate existing rules requires a KubeCheck major-version release and a migration note.

---

## 12. Safe expression evaluation

### 12.1 Design goals

The expression language must allow rule authors to express matching conditions without risking arbitrary code execution. The language is deliberately restricted:

- **No host-language escape.** No reflection, no class loading, no eval, no FFI.
- **No I/O.** No file access, network access, environment variables, or system clock.
- **No mutation.** No assignment, no side effects.
- **Pure functions only.** Every function is deterministic and side-effect-free.
- **Bounded evaluation.** Every expression has a step limit and a recursion limit.

### 12.2 Expression language grammar

```
expression    := or_expr
or_expr       := and_expr ( "||" and_expr )*
and_expr      := not_expr ( "&&" not_expr )*
not_expr      := "!" not_expr | comparison
comparison    := additive ( ("==" | "!=" | "<" | "<=" | ">" | ">=" | "in" | "matches") additive )?
additive      := multiplicative ( ("+" | "-") multiplicative )*
multiplicative := unary ( ("*" | "/" | "%") unary )*
unary         := "!" unary | primary
primary       := literal | path | function_call | "(" expression ")"
path          := identifier ( "." identifier | "[" literal "]" )*
function_call := identifier "(" ( expression ( "," expression )* )? ")"
literal       := string | number | boolean | null | list | map
```

### 12.3 Available paths

| Path prefix | Meaning |
|---|---|
| `doc.*` | The NIR root of the current document. |
| `node.*` | The current NIR node (resolved from the rule's anchor). |
| `inventory.*` | The resource inventory keyed by `ResourceKey` (`SARCH-REQ-031`). |
| `graph.*` | The relationship graph (`SARCH-REQ-032`). |
| `target_version` | The target Kubernetes minor version. |
| `env.*` | Documented, read-only manifest metadata: `env.kind`, `env.apiVersion`, `env.namespace`, `env.name`. |

### 12.4 Available functions

| Function | Signature | Semantics |
|---|---|---|
| `exists(path)` | `path -> bool` | Whether the path resolves to a non-`null` node. |
| `has(path)` | `path -> bool` | Whether the path resolves to any node, including `null`. |
| `length(value)` | `list or string -> int` | Number of elements / characters. |
| `to_string(value)` | `any -> string` | Canonical string form. |
| `to_int(value)` | `any -> int` | Canonical integer form; error if not representable. |
| `to_bool(value)` | `any -> bool` | `true` only for `true`, `"true"`, `1`; `false` otherwise. |
| `matches(string, regex)` | `string, string -> bool` | Anchored regular expression match; the regex is restricted to the RE2 subset. |
| `contains(list, value)` | `list, any -> bool` | Membership test. |
| `starts_with(string, prefix)` | `string, string -> bool` | Prefix test. |
| `is_null(value)` | `any -> bool` | True if the value is `null` or absent. |
| `key_count(map)` | `map -> int` | Number of keys. |
| `sum(list_of_int)` | `list -> int` | Integer sum; overflow returns an error. |
| `path_of(value)` | `any -> string` | The JSON path of the value, if resolvable. |

**RULE-SPEC-REQ-051:** The expression language MUST provide exactly the functions in §12.4. Additional functions require a rule schema version increment.

**RULE-SPEC-REQ-052:** The `matches` function MUST use the RE2 subset (no backreferences, no lookaround). A regex that uses an unsupported construct MUST cause rule-load rejection.

### 12.5 Evaluation limits

| Limit | Default | Rationale |
|---|---|---|
| Max expression length | 4 KiB | Prevents pathological regex or deep nesting. |
| Max AST depth | 32 | Prevents stack exhaustion. |
| Max evaluation steps per expression | 10,000 | Prevents quadratic blow-up. |
| Max evaluation time per expression | 50 ms | Prevents long-running expressions. |
| Max `matches` regex length | 1 KiB | Prevents regex-based DoS. |
| Max `length` on a string | 1 MiB | Aligns with the scalar limit. |

**RULE-SPEC-REQ-053:** Exceeding any evaluation limit MUST cause the rule to be skipped for the current object with an `ENGINE-EXPR-LIMIT` internal finding. The rule MUST NOT be disabled for the whole invocation; the limit is per-object.

### 12.6 Evaluation pseudocode

```pseudo
function evaluate(expression, context) -> Value | EvalError:
    if expression.length > MAX_EXPRESSION_LENGTH:
        return EvalError(kind="EXPR_TOO_LONG")
    ast = parse_expression(expression)      # rejects unsupported constructs
    if ast.depth > MAX_AST_DEPTH:
        return EvalError(kind="AST_TOO_DEEP")
    state = EvalState(steps=0, deadline=now() + MAX_EVAL_TIME)
    try:
        return eval_ast(ast, context, state)
    except StepLimitExceeded:
        return EvalError(kind="STEP_LIMIT")
    except DeadlineExceeded:
        return EvalError(kind="DEADLINE")
    except RegexError as e:
        return EvalError(kind="REGEX_UNSUPPORTED", detail=e.message)

function eval_ast(ast, context, state):
    state.steps += 1
    if state.steps > MAX_STEPS:
        raise StepLimitExceeded()
    if now() > state.deadline:
        raise DeadlineExceeded()
    match ast.kind:
        case Literal(v): return v
        case Path(p): return resolve_path(p, context)
        case Call(name, args):
            fn = FUNCTION_TABLE[name] or raise EvalError("UNKNOWN_FUNCTION")
            return fn([eval_ast(a, context, state) for a in args])
        case Binary(op, l, r):
            lv = eval_ast(l, context, state)
            rv = eval_ast(r, context, state)
            return apply_binary(op, lv, rv)
        case Unary(op, e):
            return apply_unary(op, eval_ast(e, context, state))
```

### 12.7 Why not JSONPath or CEL directly

JSONPath is expressive but has no evaluation limits and no function restriction; some implementations allow code evaluation through extension points. CEL (Common Expression Language) is a strong candidate and is used by Kubernetes admission policies, but CEL's runtime must itself be vetted for resource limits. KubeCheck's expression language is a **subset** of CEL's core operators, with a fixed function table and explicit limits. This is a deliberate design decision recorded in ADR-009 (open).

---

## 13. Performance limits

### 13.1 Per-rule limits

| Limit | Default | Rationale |
|---|---|---|
| Max rules per bundle | 2,000 | Bounds bundle size and evaluation time. |
| Max rules evaluated per object | 500 | Bounds per-object work; rules whose preconditions fail are not counted. |
| Max findings per rule per object | 100 | Prevents a single rule from flooding the output. |
| Max total findings per invocation | 10,000 | Bounds output size. |
| Max evidence bindings per rule | 20 | Bounds evidence complexity. |
| Max remediation steps | 20 | Bounds output size per finding. |
| Max dependencies per rule | 10 | Bounds topological sort complexity. |

**RULE-SPEC-REQ-054:** Exceeding a per-rule limit MUST cause the rule to be skipped for the current object with an `ENGINE-RULE-LIMIT` internal finding. The rule MUST NOT abort the invocation.

**RULE-SPEC-REQ-055:** Exceeding the total-findings limit MUST cause the engine to stop emitting findings and to emit a single `ENGINE-OUTPUT-LIMIT` finding stating that the output was truncated. The exit code MUST reflect this (`OUT-REQ-007`).

### 13.2 Invalid-rule behavior

**RULE-SPEC-REQ-056:** A rule that fails schema validation MUST be disabled, and the engine MUST emit a single `ENGINE-RULE-INVALID` finding per invalid rule. The finding MUST name the rule ID, the validation failure, and the rule's location in the pack.

**RULE-SPEC-REQ-057:** A rule pack manifest MAY declare a rule as `required: true`. If a required rule is invalid, the entire bundle MUST be rejected (`SARCH-REQ-050`).

**RULE-SPEC-REQ-058:** An invalid rule MUST NOT emit findings. The engine MUST NOT attempt a best-effort interpretation.

---

## 14. Testing requirements

### 14.1 Mandatory fixtures

**RULE-SPEC-REQ-059:** Every rule MUST have at least:

- **3 positive fixtures:** manifests that MUST trigger the rule.
- **3 negative fixtures:** manifests that MUST NOT trigger the rule.
- **1 boundary fixture:** a manifest at the rule's applicability boundary (e.g., the minimum or maximum target version, or a field value at the edge of the match).

**RULE-SPEC-REQ-060:** A positive fixture MUST assert the exact finding: rule ID, category, severity, evidence class, source anchor (line, column, json_path), and evidence fields. A fixture that only asserts "a finding occurred" is insufficient.

**RULE-SPEC-REQ-061:** A negative fixture MUST assert that no finding is produced by the rule, even if other rules may fire.

### 14.2 Fixture format

Each fixture is a YAML document with:

```yaml
fixture_id: <string>
description: <string>
input: <list of manifest documents>
target_version: <string>
expected_findings: <list of finding assertions>
```

Finding assertions use the same evidence-binding names as the rule's `evidence` field.

### 14.3 Fixture independence

**RULE-SPEC-REQ-062:** Fixtures MUST be independent of other rules. A fixture that triggers a different rule is not a valid fixture for the rule under test.

**RULE-SPEC-REQ-063:** Fixtures MUST NOT depend on the target Kubernetes version beyond the rule's declared `applicability.kubernetes_versions`. A fixture that requires a specific version MUST declare it in `target_version` and MUST be skipped if the version is outside the rule's range.

---

## 15. Complete illustrative rule

### 15.1 Rule: `SEC-CTX-001` — Privileged container

```yaml
id: SEC-CTX-001
name: privileged-container
schema_version: "1.0.0"

metadata:
  title: Privileged container detected
  description: >
    A container is running with `securityContext.privileged: true`. Privileged
    containers receive nearly all host capabilities and effectively remove the
    isolation boundary between the container and the node. This is appropriate
    only for a small set of infrastructure workloads (CNI plugins, storage
    drivers, kubelet-adjacent tooling) and requires explicit justification.
  category: security
  severity: warning
  evidence_class: deterministic
  confidence_basis: The `privileged` field is literally `true` in the manifest.
  tags: ["pod-security", "baseline", "container-escape"]
  references:
    - url: "https://kubernetes.io/docs/concepts/security/pod-security-standards/"
      title: "Pod Security Standards"
      retrieved: "2026-10-31"
    - url: "https://kubernetes.io/docs/tasks/configure-pod-container/security-context/"
      title: "Configure a Security Context for a Pod or Container"
      retrieved: "2026-10-31"

applicability:
  kubernetes_versions:
    min: "1.20"
    max: "1.99"
  kinds:
    - group: ""
      versions: ["v1"]
      kinds: ["Pod"]
    - group: "apps"
      versions: ["v1"]
      kinds: ["Deployment", "StatefulSet", "DaemonSet", "ReplicaSet"]
    - group: "batch"
      versions: ["v1"]
      kinds: ["Job", "CronJob"]

preconditions:
  - "exists(doc.spec)"
  - "exists(doc.spec.containers) || exists(doc.spec.template.spec.containers)"

match: >
  any(
    exists(doc.spec.containers) &&
      exists(doc.spec.containers[_].securityContext.privileged) &&
      doc.spec.containers[_].securityContext.privileged == true,
    exists(doc.spec.template.spec.containers) &&
      exists(doc.spec.template.spec.containers[_].securityContext.privileged) &&
      doc.spec.template.spec.containers[_].securityContext.privileged == true
  )

exclusions:
  - "exists(doc.metadata.annotations) && doc.metadata.annotations['kubecheck.example.com/allow-privileged'] == 'true'"
  - "env.namespace == 'kube-system'"

anchor:
  type: field_path
  path: "doc.spec.containers[_].securityContext.privileged"
  fallback: "doc.spec.template.spec.containers[_].securityContext.privileged"

evidence:
  - name: container_name
    source: field_path
    path: "doc.spec.containers[_].name"
    redact: false
  - name: template_container_name
    source: field_path
    path: "doc.spec.template.spec.containers[_].name"
    redact: false
  - name: privileged
    source: field_path
    path: "doc.spec.containers[_].securityContext.privileged"
    redact: false

remediation:
  message: >
    Replace `privileged: true` with the minimum set of Linux capabilities the
    workload needs. Start with `capabilities.drop: ["ALL"]` and add only the
    capabilities the workload demonstrably requires (for example,
    `NET_ADMIN` for a network tooling workload). If the workload truly requires
    privileged mode, document the justification in an annotation and add a
    `kubecheck.example.com/allow-privileged: "true"` annotation so that the
    finding is suppressed with an audit trail.
  steps:
    - "Identify the capabilities the workload needs (e.g., NET_ADMIN, SYS_TIME)."
    - "Set capabilities.drop: [\"ALL\"] and capabilities.add to the required set."
    - "Remove privileged: true from the securityContext."
    - "If privileged mode is required, add the justification annotation."

dependencies: []

tests:
  positive:
    - fixture_id: "SEC-CTX-001-pos-01"
      description: "A bare Pod with a privileged container."
      input:
        - |
          apiVersion: v1
          kind: Pod
          metadata:
            name: test
          spec:
            containers:
            - name: app
              image: nginx
              securityContext:
                privileged: true
      target_version: "1.34"
      expected_findings:
        - rule_id: SEC-CTX-001
          category: security
          severity: warning
          evidence_class: deterministic
          anchor:
            json_path: "/spec/containers/0/securityContext/privileged"
          evidence:
            container_name: "app"
            privileged: true

    - fixture_id: "SEC-CTX-001-pos-02"
      description: "A Deployment template with a privileged container."
      input:
        - |
          apiVersion: apps/v1
          kind: Deployment
          metadata:
            name: test
          spec:
            selector:
              matchLabels: {app: test}
            template:
              metadata:
                labels: {app: test}
              spec:
                containers:
                - name: app
                  image: nginx
                  securityContext:
                    privileged: true
      target_version: "1.34"
      expected_findings:
        - rule_id: SEC-CTX-001
          severity: warning
          evidence_class: deterministic
          evidence:
            template_container_name: "app"

    - fixture_id: "SEC-CTX-001-pos-03"
      description: "A DaemonSet with a privileged container (infrastructure workload)."
      input:
        - |
          apiVersion: apps/v1
          kind: DaemonSet
          metadata:
            name: cni
          spec:
            selector:
              matchLabels: {app: cni}
            template:
              metadata:
                labels: {app: cni}
              spec:
                containers:
                - name: cni
                  image: example.com/cni
                  securityContext:
                    privileged: true
      target_version: "1.34"
      expected_findings:
        - rule_id: SEC-CTX-001
          severity: warning

  negative:
    - fixture_id: "SEC-CTX-001-neg-01"
      description: "A Pod with no securityContext."
      input:
        - |
          apiVersion: v1
          kind: Pod
          metadata:
            name: test
          spec:
            containers:
            - name: app
              image: nginx
      target_version: "1.34"
      expected_findings: []

    - fixture_id: "SEC-CTX-001-neg-02"
      description: "A Pod with securityContext but privileged not set."
      input:
        - |
          apiVersion: v1
          kind: Pod
          metadata:
            name: test
          spec:
            containers:
            - name: app
              image: nginx
              securityContext:
                runAsNonRoot: true
      target_version: "1.34"
      expected_findings: []

    - fixture_id: "SEC-CTX-001-neg-03"
      description: "A Pod with privileged: false explicitly set."
      input:
        - |
          apiVersion: v1
          kind: Pod
          metadata:
            name: test
          spec:
            containers:
            - name: app
              image: nginx
              securityContext:
                privileged: false
      target_version: "1.34"
      expected_findings: []

  boundary:
    - fixture_id: "SEC-CTX-001-bnd-01"
      description: "Target version at the minimum of the applicability range (1.20)."
      input:
        - |
          apiVersion: v1
          kind: Pod
          metadata:
            name: test
          spec:
            containers:
            - name: app
              image: nginx
              securityContext:
                privileged: true
      target_version: "1.20"
      expected_findings:
        - rule_id: SEC-CTX-001
          severity: warning
```

### 15.2 Rule execution trace

For the first positive fixture, the engine performs:

1. **Applicability check:** Target `1.34` is in `[1.20, 1.99]`; kind `v1/Pod` matches. Rule applies.
2. **Precondition:** `exists(doc.spec)` → true; `exists(doc.spec.containers)` → true. Preconditions pass.
3. **Match:** `any(...)` evaluates the first clause. `exists(doc.spec.containers[_].securityContext.privileged)` → true (one container); `[...] == true` → true. `any` returns true.
4. **Exclusions:** No annotation; namespace is not `kube-system`. No exclusion applies.
5. **Anchor resolution:** `anchor.type: field_path`; path `doc.spec.containers[0].securityContext.privileged` resolves to line 11, column 17, json_path `/spec/containers/0/securityContext/privileged`.
6. **Evidence binding:** `container_name` → `"app"`; `privileged` → `true`; `template_container_name` → unresolved (documented as absent in the evidence).
7. **Finding emission:** The finding is assembled with the rule metadata, source anchor, resource identity (`""/v1/Pod/`/`test`), and evidence.
8. **Aggregator:** The finding is added to the ordered finding set; canonical ordering is applied by the aggregator.

### 15.3 Expected finding (JSON)

```json
{
  "rule_id": "SEC-CTX-001",
  "rule_name": "privileged-container",
  "category": "security",
  "severity": "warning",
  "evidence_class": "deterministic",
  "confidence_basis": "The `privileged` field is literally `true` in the manifest.",
  "source_anchor": {
    "file": "manifest.yaml",
    "doc_index": 0,
    "line": 11,
    "column": 17,
    "json_path": "/spec/containers/0/securityContext/privileged"
  },
  "resource_identity": {
    "group": "",
    "version": "v1",
    "kind": "Pod",
    "namespace": "",
    "name": "test"
  },
  "evidence": {
    "container_name": "app",
    "privileged": true
  },
  "remediation": {
    "message": "Replace `privileged: true` with the minimum set of Linux capabilities the workload needs.",
    "steps": [
      "Identify the capabilities the workload needs (e.g., NET_ADMIN, SYS_TIME).",
      "Set capabilities.drop: [\"ALL\"] and capabilities.add to the required set.",
      "Remove privileged: true from the securityContext.",
      "If privileged mode is required, add the justification annotation."
    ]
  },
  "references": [
    {
      "url": "https://kubernetes.io/docs/concepts/security/pod-security-standards/",
      "title": "Pod Security Standards",
      "retrieved": "2026-10-31"
    },
    {
      "url": "https://kubernetes.io/docs/tasks/configure-pod-container/security-context/",
      "title": "Configure a Security Context for a Pod or Container",
      "retrieved": "2026-10-31"
    }
  ],
  "deprecated": false,
  "conflict_with": []
}
```

---

## 16. Rule pack validation

### 16.1 Validation stages

Before activation, a rule pack passes four stages:

| Stage | Checks |
|---|---|
| **1. Schema validation** | The pack manifest and every rule match the declared rule schema version. |
| **2. Semantic validation** | Rule IDs are unique; dependencies form a DAG; categories exist; evidence bindings resolve structurally; remediation text passes safety scans (§5.2). |
| **3. Safe-expression validation** | Every expression parses under the §12 grammar; every function is in the §12.4 table; every regex uses the RE2 subset. |
| **4. Fixture execution** | Every rule's positive, negative, and boundary fixtures execute and produce the expected results. |

**RULE-SPEC-REQ-064:** A rule pack that fails any stage MUST be rejected before activation. The rejection MUST name the failing rule and the failing check.

### 16.2 Pack manifest

```yaml
rule_pack:
  id: "kubecheck-standard-rules"
  version: "1.4.0"
  rule_schema_version: "1.0.0"
  engine_version_range: ">=0.1.0,<1.0.0"
  rules:
    - SEC-CTX-001
    - SEC-CTX-002
    - K8S-SCHEMA-001
  required: false
```

**RULE-SPEC-REQ-065:** The pack manifest's `engine_version_range` MUST be checked against the running engine's version. A mismatch MUST reject the pack with a structured error.

**RULE-SPEC-REQ-066:** The pack manifest MUST be signed with the same key as the containing bundle. The signature covers the manifest and all rule files.

### 16.3 Invalid-rule-pack rejection pseudocode

```pseudo
function validate_rule_pack(pack) -> ValidationResult:
    # Stage 1: schema
    for rule in pack.rules:
        if not schema_valid(rule, pack.rule_schema_version):
            return Reject(rule=rule.id, stage="schema", detail=schema_errors(rule))

    # Stage 2: semantic
    ids = set()
    for rule in pack.rules:
        if rule.id in ids:
            return Reject(rule=rule.id, stage="semantic", detail="duplicate rule id")
        ids.add(rule.id)
        if not category_exists(rule.metadata.category):
            return Reject(rule=rule.id, stage="semantic", detail="unknown category")
        if not evidence_bindings_resolve(rule):
            return Reject(rule=rule.id, stage="semantic", detail="unresolved evidence path")
        if not remediation_safe(rule.remediation):
            return Reject(rule=rule.id, stage="semantic", detail="unsafe remediation text")
    if not is_dag(pack.rules):
        return Reject(stage="semantic", detail="dependency cycle")

    # Stage 3: expression
    for rule in pack.rules:
        for expr in rule.all_expressions():
            if not expression_safe(expr):
                return Reject(rule=rule.id, stage="expression", detail=expr_errors(expr))

    # Stage 4: fixtures
    for rule in pack.rules:
        for fixture in rule.tests.all():
            if not fixture_passes(rule, fixture):
                return Reject(rule=rule.id, stage="fixture", detail=fixture.id)

    return Accept()
```

---

## 17. Consolidated rule schema reference

The following table is the authoritative summary of every rule field. A rule that omits a required field or violates a constraint is rejected at load time.

| Field | Type | Required | Constraint | Requirement |
|---|---|---|---|---|
| `id` | string | Yes | Matches `<DOMAIN>-<FAMILY>-<NNN>` | `RULE-SPEC-REQ-006` |
| `name` | string | Yes | Lowercase slug | `RULE-SPEC-REQ-007` |
| `schema_version` | string | Yes | Equals pack's version | `RULE-SPEC-REQ-008` |
| `metadata.title` | string | Yes | ≤ 80 chars | §4.3 |
| `metadata.description` | string | Yes | ≤ 2,000 chars | §4.3 |
| `metadata.category` | string | Yes | From taxonomy | `RULE-SPEC-REQ-012` |
| `metadata.severity` | string | Yes | `error`/`warning`/`info` | §4.3 |
| `metadata.evidence_class` | string | Yes | `deterministic`/`probable`/`context-dependent` | §4.3 |
| `metadata.confidence_basis` | string | Yes | ≤ 200 chars | §4.3 |
| `metadata.tags` | list | No | Free-form | §4.3 |
| `metadata.references` | list | Yes | ≥ 1 entry with `url`, `title` | `RULE-SPEC-REQ-020` |
| `applicability.kubernetes_versions` | object | No | `min`, `max`, `except` | `RULE-SPEC-REQ-021` |
| `applicability.kinds` | list | Conditional | Required if `match` references kind fields | `RULE-SPEC-REQ-026` |
| `preconditions` | list of expressions | No | Pure, side-effect-free | `RULE-SPEC-REQ-027` |
| `match` | expression | Yes | Returns boolean | `RULE-SPEC-REQ-029` |
| `exclusions` | list of expressions | No | Returns boolean | `RULE-SPEC-REQ-031` |
| `anchor` | object | Yes | `type` and optional `path`/`fallback` | `RULE-SPEC-REQ-039` |
| `evidence` | list of bindings | Yes | ≥ 1 binding | `RULE-SPEC-REQ-015` |
| `remediation.message` | string | Yes | ≤ 500 chars | §5.2 |
| `remediation.steps` | list | No | ≤ 20 steps, each ≤ 200 chars | §5.2 |
| `dependencies` | list of rule IDs | No | DAG | `RULE-SPEC-REQ-034` |
| `deprecation` | object | No | `deprecated_in`, `removed_in`, `reason`, `replacement` | `RULE-SPEC-REQ-047` |
| `tests.positive` | list | Yes | ≥ 3 fixtures | `RULE-SPEC-REQ-059` |
| `tests.negative` | list | Yes | ≥ 3 fixtures | `RULE-SPEC-REQ-059` |
| `tests.boundary` | list | Yes | ≥ 1 fixture | `RULE-SPEC-REQ-059` |

---

## 18. Authoritative references

| Reference | Scope | Type | Retrieved |
|---|---|---|---|
| Kubernetes Documentation — Pod Security Standards | Privileged mode, capabilities, security context fields | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Configure a Security Context | Field semantics for `privileged`, `capabilities`, `runAsNonRoot` | Official documentation | 2026-10-31 |
| Kubernetes Deprecation Policy | Rule deprecation analogue for API lifecycle | Official documentation | 2026-10-31 |
| RE2 regular expression syntax | Supported regex subset for the `matches` function | Open-source documentation | 2026-10-31 |
| Common Expression Language (CEL) specification | Reference for the restricted expression language | Specification | 2026-10-31 |
| `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md` (internal) | Category definitions and evidence-class semantics | Internal | 2026-10-31 |
| `docs/yaml/YAML_SECURITY_AND_PARSER_POLICY.md` (internal) | Safe parsing, secret redaction, and expression limits | Internal | 2026-10-31 |
| `docs/architecture/SYSTEM_ARCHITECTURE.md` (internal) | Rule engine interface (`IF-RULES-001`), canonical ordering | Internal | 2026-10-31 |

**Licensing note:** Kubernetes documentation is licensed under CC BY 4.0. RE2 and CEL specifications are subject to their respective licenses. No line numbers, benchmark results, or implementation status are asserted beyond what the cited sources state.

---

## 19. Open questions and unresolved decisions

| ID | Question | Status | Owner |
|---|---|---|---|
| `OPEN-RULE-01` | Whether the expression language will be a CEL subset or a custom language; the current specification describes a CEL-like grammar. | **OPEN** | Software Architecture |
| `OPEN-RULE-02` | Exact rule schema versioning policy for the first release; whether `1.0.0` is the MVP schema version. | **OPEN** | Diagnostic Engine Architecture |
| `OPEN-RULE-03` | Whether rule packs will be distributed as separate artifacts from the schema bundle or as part of the same bundle. | **OPEN** | Bundle maintainers |
| `OPEN-RULE-04` | Whether the `matches` function will support named capture groups or only boolean matching. | **OPEN** | Diagnostic Engine Architecture |
| `OPEN-RULE-05` | Whether rules will be able to declare a `weight` for the diagnostic aggregator's priority ordering, or whether evidence class alone determines ordering. | **OPEN** | Diagnostic Engine Architecture |
| `OPEN-RULE-06` | Whether the `deprecation.replacement` rule must cover all fixtures of the deprecated rule, or whether a documented gap list is acceptable. | **OPEN** | Diagnostic Engine Architecture |

---

## 20. Definition of done

This document is complete when the following conditions hold:

1. Rule IDs and namespaces are defined with ownership and stability requirements.
2. The rule schema and schema versioning policy are stated, including backward-compatibility rules.
3. Title, description, category, severity, confidence, evidence, remediation, and references are defined with constraints.
4. Preconditions, match predicates, exclusions, and dependency declarations are specified with safety requirements.
5. Kubernetes version and resource-kind applicability is specified.
6. Diagnostic source locations and resource identities are defined, including fallback behavior.
7. Duplicate suppression, rule ordering, and deterministic output are specified with requirement IDs.
8. Rule dependencies, conflict resolution, and exception handling are specified.
9. Rule deprecation, replacement, and compatibility are specified.
10. Safe expression evaluation is specified with grammar, available functions, and evaluation limits.
11. Performance limits and invalid-rule behavior are specified.
12. Unit tests with positive, negative, and boundary fixtures are mandatory.
13. A complete illustrative rule is provided with its YAML form, validated data structure, expected diagnostic, and test cases.
14. Rule pack validation is specified with a four-stage pipeline and a rejection pseudocode.
15. A consolidated schema reference summarizes every rule field.

---

## 21. Consistency and completeness review

A review was performed against the requested scope before finalization.

- **Stable rule IDs and namespaces:** §3.
- **Rule schema and schema version:** §4.
- **Title, description, category, severity, confidence, evidence, remediation, references, applicability:** §4.3–§5, §6.
- **Preconditions, match predicates, exclusions, dependency requirements:** §7–§8.
- **Kubernetes versions and resource kinds affected:** §6.
- **Diagnostic source locations and resource identities:** §9.
- **Duplicate suppression, rule ordering, deterministic output:** §10.
- **Rule dependencies, conflict resolution, exception handling:** §8, §13.2.
- **Rule deprecation, replacement, compatibility:** §11.
- **Safe expression evaluation without arbitrary code execution:** §12.
- **Performance limits and invalid-rule behavior:** §13.
- **Unit tests with positive, negative, boundary fixtures:** §14.
- **Complete illustrative rule with YAML, data structure, expected diagnostic, test cases:** §15.
- **Versioned machine-readable schema and invalid-rule-pack rejection:** §16, §17.

No empty boilerplate section was added solely to satisfy a checklist. The document is complete for its assigned responsibility.
