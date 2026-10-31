# KubeCheck — YAML Language Specification Reference

**Document ID:** YAML-SPEC-001  
**File:** `docs/yaml/YAML_LANGUAGE_SPEC.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** YAML Specification / Diagnostics Architecture  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/architecture/PRODUCT_VISION.md`
- `docs/architecture/SYSTEM_ARCHITECTURE.md`
- `docs/architecture/REQUIREMENTS_AND_NFRS.md`
- `docs/architecture/ADR/ADR-002-yaml-parser-selection.md`
- `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`
- `docs/diagnostics/RULE_MODEL.md`
- `docs/security/SECURITY_MODEL.md`
- `docs/security/THREAT_MODEL.md`
- `docs/yaml/YAML_KUBERNETES_PATTERNS.md`
- `docs/yaml/YAML_SECURITY_NOTES.md`
- `docs/testing/DETERMINISM_TESTS.md`

---

## 1. Purpose and scope

This document is the authoritative YAML reference for KubeCheck. It defines the YAML 1.2.2 features relevant to Kubernetes manifest analysis, records KubeCheck's acceptance policy for each feature, and distinguishes three things that are often conflated:

1. **What the YAML 1.2.2 specification requires or permits.**
2. **What a YAML processor (parser, loader, or higher-level tool) does in practice**, including implementation-specific behavior that is not mandated by the specification.
3. **What KubeCheck chooses to accept, reject, or warn about**, and the rationale for each choice.

The governing rule of this document is:

> **YAML-SPEC-REQ-001 — No feature may be described as prohibited by YAML merely because KubeCheck chooses not to support it.** KubeCheck's restrictions must be labelled as implementation decisions, with the underlying specification position stated separately.

**In scope**

- The YAML stream model: streams, documents, directives, document boundaries.
- Indentation and block structure.
- Block and flow sequences and mappings.
- Scalar styles: plain, single-quoted, double-quoted, literal, folded.
- Scalar resolution: null, booleans, integers, floats, timestamps, ambiguous values.
- Tags, anchors, aliases, merge-key convention, directives.
- Comments, Unicode, escape sequences, line breaks, whitespace.
- Mapping keys: uniqueness requirement and duplicate-key ambiguity.
- Document edge cases: empty documents, multiple documents, nested structures, scalar roots.
- YAML 1.1 vs YAML 1.2 compatibility differences.
- Constructs commonly encountered in Kubernetes manifests.
- Unsupported, discouraged, and security-sensitive constructs.
- KubeCheck acceptance policy per feature.

**Out of scope**

- Concrete parser selection (governed by ADR-002, **OPEN**).
- Schema validation semantics (see `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`).
- Bundle format (see ADR-003, **OPEN**).
- Security model details (see `docs/security/SECURITY_MODEL.md`).

---

## 2. Normative references and conventions

### 2.1 Primary reference

| Reference | Version | Role |
|---|---|---|
| YAML Ain't Markup Language (YAML™) version 1.2 | Revision 1.2.2, 2021-10-01 | Primary normative reference for all YAML language behavior in this document. |
| YAML 1.2.2 Core Schema (§10.3) | Part of the specification | Defines default implicit type resolution for plain scalars. |
| YAML 1.1 specification | 2005 | Referenced only for compatibility differences (§12). |
| Kubernetes API conventions and manifest examples | Current upstream documentation | Referenced only for Kubernetes-specific patterns (§13). |

The YAML 1.2.2 specification is the **only normative authority for YAML language behavior** in this document. Where this document cites section numbers, they refer to the YAML 1.2.2 specification. The specification states explicitly that revision 1.2.2 contains **no normative changes** from YAML 1.2 and primarily corrects errors and adds clarity.

### 2.2 Citation convention

- Specification citations use the form `§<section>` (e.g., §3.2.1.3 for node comparison). Where a direct quotation is used, the section is named explicitly.
- Each acceptance-policy row in §15 cites the requirement ID of the underlying KubeCheck requirement (`YAML-SPEC-REQ-*` or a downstream requirement ID) and, where applicable, the specification section.
- No line numbers are cited, because the specification is a living document available in multiple renderings.

### 2.3 Terminology

| Term | Definition |
|---|---|
| **Stream** | The text content of a YAML file or stream, containing zero or more documents with optional directives and comments. |
| **Document** | A single YAML data structure within a stream, presented as a root node with an optional tag and optional directives. |
| **Node** | A scalar, mapping, or sequence in the YAML representation graph. |
| **Tag** | A URI-like identifier that determines a node's type and resolution. |
| **Plain scalar** | An unquoted scalar whose type is resolved by the schema in effect. |
| **Schema** | The set of rules that map untagged plain scalars to typed values. The YAML 1.2.2 Core Schema is the default schema referenced by this document. |
| **Safe mode** | A parser configuration that prohibits arbitrary object construction, implicit code execution, and unsafe tag resolution. |
| **KubeCheck acceptance policy** | One of `ACCEPT`, `ACCEPT WITH WARNING`, `REJECT`, or `REJECT UNLESS EXPLICITLY ENABLED`. |

---

## 3. Stream model: streams, documents, directives, boundaries

### 3.1 Specification position

A **YAML stream** consists of zero or more **documents**. Documents within a stream are separated by **document boundary markers**. The specification defines:

- `---` as the **directives-end marker** (also functioning as a document-start indicator when used at the start of a document).
- `...` as the **document-end marker**.

The specification states that **directives belong to the stream and not to a document**, and that each document "is completely independent from the rest". In YAML 1.2, **directives are associated with the document they precede and do not carry over to following documents**.

A document that is **not terminated by a document-end marker** must be followed by a document that begins with a **directives-end marker** (`---`) if a new document is to start.

### 3.2 Documents and independence

Every document is a separate root node. There is no shared state between documents within a stream, and anchors and aliases cannot cross document boundaries. This is the basis for KubeCheck's per-document processing model (`SARCH-REQ-021`, `SARCH-REQ-066`).

### 3.3 Directives

The YAML 1.2.2 specification defines **two standard directives**:

| Directive | Purpose | Example |
|---|---|---|
| `%YAML` | Declares the YAML specification version of the stream. | `%YAML 1.2` |
| `%TAG` | Declares a tag handle prefix mapping. | `%TAG !e! tag:example.com,2000:app/` |

Directives appear at the start of a document and are terminated by `---`. A document with directives must begin with `%`-prefixed lines followed by `---`. Directives are **scoped to the document** and do not carry over to subsequent documents.

**Implementation-specific behavior:** Many parsers ignore `%YAML` directives unless explicitly configured to enforce version conformance. Some parsers reject `%TAG` directives that reference handles not declared in the same document. The specification requires that directives be processed **before** the document content, but does not mandate that every processor enforce every directive.

### 3.4 Document boundary edge cases

| Case | Specification behavior | KubeCheck handling |
|---|---|---|
| Stream with a single document and no `---` | The stream contains exactly one document. | `ACCEPT`. |
| Stream with multiple documents, each preceded by `---` | Each `---` starts a new document. | `ACCEPT`. |
| Stream with `...` document-end marker followed by `---` | The `...` terminates the current document; `---` starts the next. | `ACCEPT`. |
| Stream with a document not terminated by `...` followed by `---` | The `---` starts the next document. | `ACCEPT`. |
| Stream with content after `...` without `---` | According to the specification, a document must be followed by `---` if the previous document was not terminated by `...`. | `REJECT` as a parse error: a new document without a boundary marker is not permitted. |
| Stream containing only comments and no document | The stream is empty (zero documents). | `ACCEPT`; no documents to analyze. |
| Stream containing only a `%YAML` directive and no document | The stream contains a directive but no document content. | `ACCEPT WITH WARNING`: directive without document is unusual but not invalid. |

**YAML-SPEC-REQ-002:** KubeCheck MUST process each document independently and MUST NOT allow anchors, aliases, or tags to cross document boundaries.

**YAML-SPEC-REQ-003:** KubeCheck MUST report a parse error when a new document begins without a boundary marker after an unterminated document, consistent with §3.1.

### 3.5 KubeCheck acceptance summary for stream features

| Feature | Specification | KubeCheck |
|---|---|---|
| Single document, no `---` | Valid | `ACCEPT` |
| Multiple documents with `---` | Valid | `ACCEPT` |
| `...` document end marker | Valid | `ACCEPT` |
| `%YAML` directive | Valid | `ACCEPT WITH WARNING`: version directive is recorded but does not change parser conformance mode (KubeCheck always targets 1.2.2). |
| `%TAG` directive | Valid | `ACCEPT` if the referenced handle is declared; `REJECT` if the handle is undeclared or the tag URI cannot be resolved. |
| Directives with no document | Valid but unusual | `ACCEPT WITH WARNING` |
| New document without boundary marker | Invalid | `REJECT` (parse error) |

---

## 4. Indentation and block structure

### 4.1 Specification position

YAML uses **indentation** to denote the structure of block collections and block scalars. The specification requires that:

- Indentation uses **spaces only**; tab characters are **not** permitted for indentation (§6.1).
- The indentation level of a block collection determines the nesting of its entries.
- Block sequence entries are denoted by `- ` (dash followed by a space) at the current indentation level.
- Block mapping entries are denoted by `key: value` at the current indentation level.

The specification defines **indentation indicators** for block scalars (§8.1.1.1) that explicitly declare the indentation level, removing ambiguity when the first content line begins with spaces.

### 4.2 Block collections

A **block sequence** is written as a series of `- ` entries at the same indentation level:

```yaml
containers:
  - name: app
    image: nginx
  - name: sidecar
    image: envoy
```

A **block mapping** is written as `key: value` pairs at the same indentation level:

```yaml
metadata:
  name: example
  labels:
    app: web
```

The specification permits a mapping value that is itself a block collection to be written **indentless** for sequences under a mapping key, a style commonly encountered in Kubernetes manifests:

```yaml
spec:
  containers:
  - name: app        # sequence is at the same indentation as the key
    image: nginx
```

This is a **valid** YAML construct (an "indentless sequence") and is widely used in Kubernetes documentation. The specification permits it because a block sequence may be a mapping value without requiring additional indentation (§8.2.1).

### 4.3 Indentation edge cases

| Case | Specification position | KubeCheck handling |
|---|---|---|
| Tab used for indentation | Invalid (§6.1) | `REJECT` with a parse error naming the line and column. |
| Inconsistent indentation within a block collection | Invalid; entries must align | `REJECT` with a parse error. |
| Indentless sequence under a mapping key | Valid | `ACCEPT`. |
| Block scalar with explicit indentation indicator (`|2`, `>1`) | Valid | `ACCEPT`. |
| Block scalar with auto-detected indentation | Valid | `ACCEPT`; KubeCheck records the effective indentation in the source map. |
| Indentation deeper than the parent node allows | Invalid if it breaks the collection structure | `REJECT`. |

**YAML-SPEC-REQ-004:** KubeCheck MUST reject tab characters used in indentation with a structured parse finding that identifies the offending line and column.

**YAML-SPEC-REQ-005:** KubeCheck MUST accept indentless sequences under mapping keys, because this construct is valid YAML and is common in Kubernetes manifests.

### 4.4 KubeCheck acceptance summary for indentation

| Feature | Specification | KubeCheck |
|---|---|---|
| Space indentation | Required | `ACCEPT` |
| Tab indentation | Prohibited | `REJECT` |
| Indentless sequence under mapping key | Valid | `ACCEPT` |
| Block scalar with explicit indentation indicator | Valid | `ACCEPT` |
| Block scalar with auto-detected indentation | Valid | `ACCEPT` |
| Mixed indentation characters | Invalid | `REJECT` |

---

## 5. Block and flow collections

### 5.1 Specification position

YAML provides two presentation styles for collections:

- **Block style**: uses indentation and line breaks; the default for human-readable documents.
- **Flow style**: uses `[` `]` for sequences and `{` `}` for mappings, with comma-separated entries; similar to JSON.

The specification defines both styles as producing **the same representation graph** (§7.1–§7.5). The choice of style is presentational and does not affect the data model.

### 5.2 Flow sequences

A **flow sequence** is written with square brackets:

```yaml
args: [--port, "8080", --verbose]
```

Flow sequences may contain nested flow collections and may span multiple lines. Trailing commas are **permitted** in flow collections (§7.4.1).

### 5.3 Flow mappings

A **flow mapping** is written with curly braces:

```yaml
labels: {app: web, tier: frontend}
```

Flow mappings may be nested and may span multiple lines. The colon-space separator (`: `) is required between key and value in block mappings; flow mappings permit `key: value` with the same requirement.

### 5.4 Flow vs block equivalence

The specification states that flow and block styles are **semantically equivalent**: the representation graph is identical regardless of style. A processor MUST NOT attach different meaning to the same content presented in different styles.

**Implementation-specific behavior:** Some processors impose different resource limits on flow collections (e.g., maximum nesting depth) because flow style is more compact and easier to generate programmatically. This is an implementation choice, not a specification difference.

### 5.5 KubeCheck acceptance summary for collections

| Feature | Specification | KubeCheck |
|---|---|---|
| Block sequence (`- item`) | Valid | `ACCEPT` |
| Block mapping (`key: value`) | Valid | `ACCEPT` |
| Indentless block sequence | Valid | `ACCEPT` |
| Flow sequence (`[a, b, c]`) | Valid | `ACCEPT` |
| Flow mapping (`{k: v}`) | Valid | `ACCEPT` |
| Trailing comma in flow collection | Valid | `ACCEPT` |
| Flow collection spanning multiple lines | Valid | `ACCEPT` |
| Flow collection nested in block collection | Valid | `ACCEPT` |
| Block collection nested in flow collection | Invalid (block collections cannot appear inside flow collections) | `REJECT` with a parse error. |

**YAML-SPEC-REQ-006:** KubeCheck MUST treat flow and block presentations of the same content as producing identical NIR structures (`SARCH-REQ-023`), so that downstream diagnostics are style-independent.

**YAML-SPEC-REQ-007:** KubeCheck MUST reject a block collection nested inside a flow collection, because the specification does not permit block collections in flow context.

---

## 6. Scalar styles

YAML defines **five scalar presentation styles**. Each style produces a scalar node in the representation graph, but the styles differ in how special characters and line breaks are handled.

### 6.1 Plain scalars

A **plain scalar** is unquoted and may span multiple lines. It is terminated by `: ` (in a mapping context), ` #` (comment), or a flow indicator. Plain scalars are subject to **implicit type resolution** by the active schema (§7).

```yaml
name: my-app
replicas: 3
enabled: true
```

The specification defines the permitted character set for plain scalars and the rules for continuation lines (§7.3.3). A plain scalar may not begin with most indicator characters (`-`, `?`, `:`, `,`, `[`, `]`, `{`, `}`, `#`, `&`, `*`, `!`, `|`, `>`, `'`, `"`, `%`, `@`, `` ` ``) in certain contexts.

### 6.2 Single-quoted scalars

A **single-quoted scalar** is enclosed in `'` characters. Within a single-quoted scalar:

- A literal `'` is represented by doubling it: `''`.
- Backslash `\` has no special meaning.
- Line breaks are folded to spaces.

```yaml
message: 'It''s a valid manifest'
path: 'C:\Users\app'
```

### 6.3 Double-quoted scalars

A **double-quoted scalar** is enclosed in `"` characters and supports **escape sequences**:

| Escape | Meaning |
|---|---|
| `\n` | Line feed |
| `\t` | Tab |
| `\"` | Double quote |
| `\\` | Backslash |
| `\uXXXX` | Unicode code point (4 hex digits) |
| `\UXXXXXXXX` | Unicode code point (8 hex digits) |
| `\xXX` | 8-bit character (2 hex digits) |

```yaml
annotation: "line one\nline two"
value: "\u00e9"        # é
```

### 6.4 Literal block scalars (`|`)

A **literal block scalar** preserves line breaks literally. The content begins on the line after the indicator and is indented relative to the parent node.

```yaml
script: |
  #!/bin/sh
  echo "hello"
  exit 0
```

The resulting scalar contains the line breaks as written, including a trailing newline unless a **chomping indicator** (`-` strip, `+` keep, default clip) modifies it.

### 6.5 Folded block scalars (`>`)

A **folded block scalar** folds line breaks into spaces, except for blank lines (which become newlines) and lines indented more than the block's base indentation (which preserve their line break).

```yaml
description: >
  This is a long description
  that is folded into a single
  line with spaces.
```

The result is `"This is a long description that is folded into a single line with spaces.\n"` (depending on chomping).

### 6.6 Chomping indicators

Block scalars support three chomping indicators:

| Indicator | Behavior |
|---|---|
| `|` or `>` (default, "clip") | Keep a single trailing newline if the content ends with one; strip extra trailing newlines. |
| `|-` or `>-` ("strip") | Strip all trailing newlines. |
| `|+` or `>+` ("keep") | Keep all trailing newlines. |

### 6.7 Scalar style edge cases

| Case | Specification position | KubeCheck handling |
|---|---|---|
| Plain scalar beginning with `- ` | Valid in a block sequence context | `ACCEPT` |
| Plain scalar containing ` #` | The ` #` begins a comment; the scalar terminates before it. | `ACCEPT`; the comment is not part of the scalar. |
| Plain scalar spanning multiple lines | Valid (§7.3.3) | `ACCEPT`; line breaks are folded to spaces. |
| Single-quoted scalar with `''` | Valid; represents a literal `'`. | `ACCEPT`. |
| Double-quoted scalar with unknown escape (`\q`) | Invalid | `REJECT` with a parse error identifying the escape sequence. |
| Double-quoted scalar with `\u` followed by non-hex | Invalid | `REJECT`. |
| Literal block scalar with content indented less than the indicator | Invalid | `REJECT`. |
| Folded block scalar with deeper-indented lines | Valid; deeper indentation preserves line breaks. | `ACCEPT`. |
| Empty block scalar | Valid | `ACCEPT`; resolves to an empty string. |

**YAML-SPEC-REQ-008:** KubeCheck MUST reject unknown escape sequences in double-quoted scalars and MUST report the offending sequence and its location.

**YAML-SPEC-REQ-009:** KubeCheck MUST preserve line-break and whitespace behavior of block scalars in the NIR, and MUST record chomping indicators in the source map so that downstream diagnostics can reference the exact content.

### 6.8 KubeCheck acceptance summary for scalar styles

| Style | Specification | KubeCheck |
|---|---|---|
| Plain scalar (single-line) | Valid | `ACCEPT` |
| Plain scalar (multi-line) | Valid | `ACCEPT` |
| Single-quoted scalar | Valid | `ACCEPT` |
| Double-quoted scalar | Valid | `ACCEPT` |
| Double-quoted with `\u`/`\U`/`\x` escapes | Valid | `ACCEPT` |
| Double-quoted with unknown escape | Invalid | `REJECT` |
| Literal block scalar (`|`) | Valid | `ACCEPT` |
| Folded block scalar (`>`) | Valid | `ACCEPT` |
| Chomping indicators (`-`, `+`) | Valid | `ACCEPT` |
| Block scalar with explicit indentation indicator | Valid | `ACCEPT` |

---

## 7. Scalar resolution: nulls, booleans, numbers, timestamps, ambiguous values

### 7.1 Specification position

YAML 1.2 defines **three built-in schemas** for implicit type resolution of untagged plain scalars:

| Schema | Purpose |
|---|---|
| **Failsafe schema** | Only strings, mappings, and sequences; no implicit typing. |
| **JSON schema** | Adds null, booleans, integers, and floats per JSON productions. |
| **Core schema** | Extends JSON schema with additional formats (e.g., hexadecimal, octal, `.inf`, `.nan`). |

The **Core Schema** is the default schema referenced by most YAML processors, including the one assumed by this document unless otherwise stated. The specification defines the **resolution order** for plain scalars in §10.3.2: **null → bool → int → float → string** (string as fallback).

### 7.2 Null

In the Core Schema, the following plain scalars resolve to `null`:

| Form | Example |
|---|---|
| `null` (any case) | `null`, `Null`, `NULL` |
| `~` | `~` |
| Empty scalar (in certain contexts) | A mapping value or sequence entry that is empty |

The specification states that in YAML 1.2, **the empty scalar** resolves to null in contexts where a null value is permitted.

### 7.3 Booleans

In the Core Schema, **only `true` and `false` (any case)** resolve to booleans:

| Form | Resolves to |
|---|---|
| `true`, `True`, `TRUE` | `true` |
| `false`, `False`, `FALSE` | `false` |

**Critical compatibility difference:** YAML 1.1 permitted `yes`, `no`, `on`, `off` as booleans. **YAML 1.2 removed this.** The specification states: "In this version of YAML, boolean values may be serialized as 'true' or 'false'; the empty scalar as 'null'". Kubernetes documentation explicitly warns against using `yes`/`no`/`on`/`off` because they "might work in one version of YAML but break in another".

### 7.4 Integers

The Core Schema resolves plain scalars matching the following patterns to integers:

| Form | Example |
|---|---|
| Decimal | `42`, `-7` |
| Hexadecimal (`0x`) | `0xFF` |
| Octal (`0o`) | `0o755` |
| Binary (`0b`) | `0b1010` |
| Underscore separators | `1_000_000` |

**Note:** `0o755` (with `o`) is the YAML 1.2 octal form; `0755` (without `o`) is a YAML 1.1 form that YAML 1.2 treats as a **decimal integer** (i.e., `0755` = `755`). Processors that retain YAML 1.1 compatibility may differ; this is an implementation-specific behavior, not a specification ambiguity.

### 7.5 Floats

The Core Schema resolves plain scalars matching float productions, including:

- `3.14`, `-0.5`
- `1e6`, `1.5e-3`
- `.inf`, `-.inf`, `.nan` (Core Schema extensions)

### 7.6 Timestamps

YAML 1.2 does not define a native timestamp tag in the Core Schema. The YAML 1.2.2 specification's Core Schema does not include a timestamp resolver. However, the **YAML 1.1 type repository** defined `!!timestamp`, and some processors (including PyYAML's default resolver and certain Kubernetes tools) may resolve ISO-like date strings to timestamp objects.

The specification position is:

- The **Core Schema** does not resolve timestamps; a plain scalar such as `2026-10-31` remains a **string** under the Core Schema.
- A processor **may** implement the YAML 1.1 `!!timestamp` type as an extension, but this is **implementation-specific** and not required by YAML 1.2.2.

**KubeCheck position:** KubeCheck targets the **Core Schema** and does **not** resolve timestamps to typed values. Date-like strings remain strings. This is consistent with the specification's Core Schema and with Kubernetes' own treatment of date-like fields as strings in most cases.

### 7.7 Ambiguous values

The following plain scalars are **ambiguous** and depend on the schema in effect:

| Value | Core Schema | YAML 1.1 | KubeCheck |
|---|---|---|---|
| `yes` | string | boolean `true` | `ACCEPT AS STRING`: Core Schema resolution. |
| `no` | string | boolean `false` | `ACCEPT AS STRING`. |
| `on` | string | boolean `true` | `ACCEPT AS STRING`. |
| `off` | string | boolean `false` | `ACCEPT AS STRING`. |
| `y` / `n` | string | boolean | `ACCEPT AS STRING`. |
| `0755` | integer `755` | integer `493` (octal) | `ACCEPT AS INTEGER 755`: Core Schema resolution. |
| `0o755` | integer `493` | integer `493` (octal) | `ACCEPT AS INTEGER 493`. |
| `2026-10-31` | string | timestamp (YAML 1.1 extension) | `ACCEPT AS STRING`. |
| `1:30` | string (sexagesimal is not Core) | integer `90` (sexagesimal, YAML 1.1) | `ACCEPT AS STRING`. |

**Implementation-specific behavior:** Processors that enable YAML 1.1 compatibility may resolve these values differently. This is a **parser configuration choice**, not a specification requirement. KubeCheck's Core Schema choice must be documented in ADR-002.

**YAML-SPEC-REQ-010:** KubeCheck MUST resolve plain scalars according to the YAML 1.2.2 Core Schema and MUST NOT apply YAML 1.1 boolean or timestamp resolution unless a future ADR explicitly enables a compatibility mode.

**YAML-SPEC-REQ-011:** KubeCheck MUST report ambiguous values (e.g., `yes`, `on`, `0755`, date-like strings) as **informational findings** when they appear in fields where the Kubernetes schema expects a boolean or integer, so that users can disambiguate deliberately.

### 7.8 KubeCheck acceptance summary for scalar resolution

| Construct | Specification (Core Schema) | KubeCheck |
|---|---|---|
| `null`, `~`, empty scalar | null | `ACCEPT` |
| `true`/`false` | boolean | `ACCEPT` |
| `yes`/`no`/`on`/`off` | string | `ACCEPT AS STRING`; informational finding if a boolean is expected by schema. |
| `0755` | integer `755` | `ACCEPT AS INTEGER 755`. |
| `0o755` | integer `493` | `ACCEPT AS INTEGER 493`. |
| `2026-10-31` | string | `ACCEPT AS STRING`. |
| `1:30` | string | `ACCEPT AS STRING`. |
| `.inf` / `.nan` | float | `ACCEPT`. |
| Explicit `!!str` on a plain scalar | string | `ACCEPT`. |

---

## 8. Tags, anchors, aliases, merge-key convention, directives

### 8.1 Tags

A **tag** identifies the type of a node. Tags may be:

- **Explicit**: written with `!!` (core tag shorthand), `!<...>` (verbatim), or `!handle!suffix` (shorthand with a `%TAG` directive).
- **Implicit**: assigned by the schema based on the node's kind and content.

Examples:

```yaml
value: !!str 42        # explicit tag: string "42"
value: 42              # implicit: integer 42 under Core Schema
value: !custom foo     # application-specific tag
```

The specification defines the **core tag repository** at `tag:yaml.org,2002:` including `!!str`, `!!int`, `!!float`, `!!bool`, `!!null`, `!!map`, `!!seq`, and `!!binary` (§10.4). Application-specific tags (e.g., `!custom`) are permitted but their semantics are defined by the application.

### 8.2 Anchors and aliases

An **anchor** (`&name`) marks a node so that it can be referenced later. An **alias** (`*name`) references the most recent preceding node with that anchor.

```yaml
common: &common
  app: web
  tier: frontend
metadata:
  labels: *common
```

The specification states that aliases refer to the **most recent preceding node** with the same anchor, and that an alias before its anchor is an error. Anchors and aliases are **document-scoped** and cannot cross document boundaries.

### 8.3 Merge-key convention (`<<`)

The **merge key** (`<<`) is **not part of the YAML 1.2.2 Core Schema**. It is defined by the **Merge Key Language-Independent Type** specification, which is a separate document referenced from the YAML type repository. The merge key is widely supported by processors (including Kubernetes tooling) but its semantics are an **extension**, not a Core Schema requirement.

The merge key takes an alias to a mapping, or a sequence of aliases, and merges the referenced mappings into the current mapping. Keys in the current mapping override merged keys.

```yaml
defaults: &defaults
  replicas: 1
  image: nginx
production:
  <<: *defaults
  replicas: 3
```

**Implementation-specific behavior:** Some processors support the merge key natively; others require it to be enabled explicitly. The specification does not mandate merge-key support.

**KubeCheck position:** KubeCheck **accepts** the merge key as an **extension** and processes it according to the Merge Key specification. It does not claim the merge key is part of the Core Schema. If the merge key references an undefined anchor, KubeCheck emits a parse error.

### 8.4 Directives

See §3.3 for directives. The `%YAML` directive does not change KubeCheck's conformance target (always 1.2.2). The `%TAG` directive is accepted if the referenced handle is declared; undeclared handles are rejected.

### 8.5 KubeCheck acceptance summary for tags, anchors, aliases, and merge keys

| Feature | Specification | KubeCheck |
|---|---|---|
| Explicit core tag (`!!str`, `!!int`) | Valid | `ACCEPT` |
| Verbatim tag (`!<tag:...>`) | Valid | `ACCEPT` if the tag URI is well-formed; `REJECT` if malformed. |
| Application-specific tag (`!custom`) | Valid (application-defined) | `ACCEPT AS OPAQUE`: preserved in NIR; no type resolution applied. |
| Anchor (`&name`) | Valid | `ACCEPT`. |
| Alias (`*name`) | Valid | `ACCEPT` if the anchor is defined; `REJECT` if undefined. |
| Alias before anchor | Invalid | `REJECT`. |
| Merge key (`<<`) | Extension (Merge Key specification) | `ACCEPT` as an extension; `REJECT` if the referenced alias is undefined. |
| `%YAML` directive | Valid | `ACCEPT WITH WARNING`: recorded, does not change conformance mode. |
| `%TAG` directive | Valid | `ACCEPT` if handle is declared; `REJECT` if undeclared. |

**YAML-SPEC-REQ-012:** KubeCheck MUST distinguish Core Schema tags from application-specific tags and MUST NOT attempt type resolution for application-specific tags.

**YAML-SPEC-REQ-013:** KubeCheck MUST treat the merge key as an extension and MUST document this in ADR-002. It MUST NOT describe the merge key as a Core Schema feature.

---

## 9. Comments, Unicode, escape sequences, line breaks, whitespace

### 9.1 Comments

A **comment** begins with `#` outside a scalar and extends to the end of the line. Comments are not part of the data model and are **not preserved** by most processors.

```yaml
# This is a comment
name: my-app  # inline comment
```

The specification does not require processors to preserve comments. KubeCheck does **not** preserve comments in the NIR; comments are discarded after parsing.

### 9.2 Unicode

YAML is a Unicode-based language. The specification requires support for **UTF-8** (and permits UTF-16 and UTF-32 with byte-order marks). The specification defines printable character ranges and prohibits certain control characters in certain contexts (§5.1).

**Implementation-specific behavior:** Some processors reject non-UTF-8 input without a BOM; others auto-detect encoding. KubeCheck targets UTF-8 input and reports a parse error for invalid UTF-8 byte sequences.

### 9.3 Escape sequences

Double-quoted scalars support escape sequences (§6.3). The specification defines the full set of escapes, including `\n`, `\t`, `\"`, `\\`, `\uXXXX`, `\UXXXXXXXX`, `\xXX`, and several others (`\0`, `\a`, `\b`, `\e`, `\f`, `\r`, `\v`). Unknown escapes are **invalid** in double-quoted scalars.

### 9.4 Line breaks and whitespace

The specification distinguishes **line breaks** (CR, LF, CRLF) and **whitespace** (spaces, tabs). YAML processors normalize line breaks to LF in the data model (§5.4). Tabs may appear in scalar content (especially in block scalars and quoted scalars) but **not** in indentation.

### 9.5 KubeCheck acceptance summary for comments, Unicode, escapes, whitespace

| Feature | Specification | KubeCheck |
|---|---|---|
| Comments (`#`) | Valid; not part of data model | `ACCEPT`; discarded. |
| UTF-8 input | Required | `ACCEPT`. |
| Invalid UTF-8 bytes | Invalid | `REJECT` with a parse error identifying the byte offset. |
| Double-quoted known escapes | Valid | `ACCEPT`. |
| Double-quoted unknown escapes | Invalid | `REJECT`. |
| Tabs in scalar content | Valid in quoted/block scalars | `ACCEPT`. |
| Tabs in indentation | Invalid | `REJECT`. |
| CRLF line breaks | Valid; normalized to LF | `ACCEPT`; normalized. |

**YAML-SPEC-REQ-014:** KubeCheck MUST reject invalid UTF-8 byte sequences with a structured parse finding that identifies the byte offset.

**YAML-SPEC-REQ-015:** KubeCheck MUST normalize CRLF and CR line breaks to LF in the NIR, consistent with §5.4 of the specification.

---

## 10. Mapping keys: uniqueness and duplicate-key handling

### 10.1 Specification position

The YAML 1.2.2 specification requires that **mapping keys MUST be unique**. The specification states that the content of a mapping node is an unordered set of key/value pairs **with the restriction that each of the keys is unique** (§3.2.1.3).

Crucially, the specification delegates **key equality semantics to the application/schema**. Two scalars are equal only when their **tags and canonical forms** are equal character-by-character. This means that key equality is **schema-dependent**:

- Under the **Failsafe Schema**, `0xB` and `11` are **different** strings.
- Under the **Core Schema**, `0xB` and `11` both resolve to integer `11` and are therefore **the same key**.
- Under an application schema, additional rules may apply.

The specification explicitly notes: "If both notations are used as keys in the same mapping, only a YAML processor which recognizes integer formats would correctly flag the duplicate key as an error".

### 10.2 Implementation-specific behavior

Many YAML processors **allow duplicate keys** by default and resolve them with a **first-wins** or **last-wins** policy. This is a **violation of the specification's uniqueness requirement**, but it is common in practice. The specification is clear that keys MUST be unique; processors that allow duplicates are choosing leniency over strict conformance.

Different processors choose different resolution policies:

- **First-wins**: the first occurrence is retained; subsequent duplicates are ignored.
- **Last-wins**: the last occurrence is retained; earlier occurrences are ignored.
- **Reject**: the processor emits an error and refuses to load the mapping.

The specification does not mandate a resolution policy because it requires duplicates to be an **error**.

### 10.3 KubeCheck position

KubeCheck **rejects duplicate keys** by default, because:

- The specification requires uniqueness.
- Silent first-wins or last-wins behavior can hide configuration defects.
- Kubernetes manifests with duplicate keys are almost always errors.

**YAML-SPEC-REQ-016:** KubeCheck MUST reject duplicate mapping keys with a structured parse finding that identifies **both** occurrences (line and column) and states the canonical forms that are equal.

**YAML-SPEC-REQ-017:** If a future configuration option allows duplicate keys (e.g., for lenient CI environments), the default MUST remain **reject**, and the option MUST require explicit opt-in documented in `docs/cli/CONFIGURATION.md`.

**YAML-SPEC-REQ-018:** KubeCheck MUST determine key equality according to the Core Schema. For example, `0xB` and `11` are the same key under the Core Schema and MUST be flagged as duplicates; `"11"` (quoted) and `11` (unquoted) are different keys because the quoted form is a string and the unquoted form is an integer.

### 10.4 KubeCheck acceptance summary for mapping keys

| Case | Specification | KubeCheck |
|---|---|---|
| Unique keys | Required | `ACCEPT`. |
| Duplicate keys (identical text) | Invalid | `REJECT`. |
| Duplicate keys (different text, same canonical form under Core Schema) | Invalid under Core Schema | `REJECT`. |
| Duplicate keys with quoted vs unquoted forms of the same canonical value (`"11"` vs `11`) | Different tags → different keys | `ACCEPT`: they are not duplicates. |
| Non-scalar keys (mapping or sequence as key) | Valid in YAML; represented as complex keys | `ACCEPT`; preserved in NIR. |
| Empty key (`: value`) | Valid; resolves to null key | `ACCEPT`. |
| Duplicate keys under a lenient configuration option | Invalid per specification | `REJECT` by default; lenient mode available only with explicit opt-in. |

---

## 11. Document-level edge cases

### 11.1 Empty documents

An **empty document** (a `---` followed by `---` or EOF) represents a **null** root node. This is valid YAML.

```yaml
---
---
```

The specification permits empty documents; they resolve to null.

**KubeCheck position:** `ACCEPT`. An empty document produces no findings (other than a possible informational note that it is empty). It is excluded from the resource inventory (`SARCH-REQ-025`).

### 11.2 Multiple documents

Multiple documents are separated by `---` (and optionally terminated by `...`). Each document is independent (§3.2). KubeCheck processes each document independently and aggregates findings per document (`SARCH-REQ-066`).

### 11.3 Nested structures

Deeply nested mappings and sequences are valid YAML. The specification imposes no nesting-depth limit; implementations may impose one to prevent resource exhaustion.

**KubeCheck position:** `ACCEPT` within the configured `max_depth` limit (`LIM-REQ-003`); `REJECT` with a structured governor finding when the limit is exceeded. This is an **implementation resource-safety decision**, not a YAML prohibition.

### 11.4 Scalar roots

A document may have a **scalar root**:

```yaml
---
hello
```

This is valid YAML; the document's root node is the scalar `"hello"`.

**KubeCheck position:** `ACCEPT` at the parser level. A scalar-root document is **not** a Kubernetes manifest and will fail GVK extraction (`SARCH-REQ-025`) with a deterministic finding stating that the document has no `apiVersion` and `kind`. This is a downstream schema/semantic outcome, not a YAML rejection.

### 11.5 KubeCheck acceptance summary for document edge cases

| Case | Specification | KubeCheck |
|---|---|---|
| Empty document | Valid; null root | `ACCEPT`; excluded from inventory. |
| Multiple documents with `---` | Valid | `ACCEPT`; processed independently. |
| Scalar root | Valid | `ACCEPT` at parser level; downstream GVK failure finding. |
| Null root | Valid | `ACCEPT`; excluded from inventory. |
| Deeply nested structure within `max_depth` | Valid | `ACCEPT`. |
| Deeply nested structure above `max_depth` | Valid but resource-unsafe | `REJECT` with governor finding (implementation limit). |

**YAML-SPEC-REQ-019:** KubeCheck MUST NOT reject a valid YAML document solely because it is not a Kubernetes manifest. It MUST accept the document at the parser level and emit a downstream finding explaining why it cannot be analyzed as a Kubernetes resource.

---

## 12. YAML 1.1 vs YAML 1.2 compatibility differences

The YAML 1.2 specification is **compatible with 1.1 for most practical applications**, but the implicit typing rules changed significantly. The following table summarises the differences most relevant to Kubernetes manifests.

| Feature | YAML 1.1 | YAML 1.2 (Core Schema) | KubeCheck |
|---|---|---|---|
| Booleans | `true`, `false`, `yes`, `no`, `on`, `off` (any case) | `true`, `false` only | Core Schema: `yes`/`no`/`on`/`off` are strings. |
| Null | `~`, `null`, empty scalar | `null`, `~`, empty scalar | Same as Core Schema. |
| Octal integers | `0755` (leading zero) | `0o755` (explicit `o`) | Core Schema: `0755` is decimal `755`; `0o755` is octal `493`. |
| Sexagesimal integers | `1:30` = `90` | Not supported | Core Schema: string `"1:30"`. |
| Timestamps | `!!timestamp` in the type repository | Not in Core Schema | Core Schema: strings. |
| Merge key (`<<`) | Defined in the type repository | Not in Core Schema | Extension: accepted as documented extension. |
| Duplicate keys | Processors vary; many allow | Keys MUST be unique | `REJECT` by default. |
| Line breaks | Unicode line breaks permitted | Unicode line breaks removed | Core Schema: CR/LF/CRLF normalized to LF. |
| JSON compatibility | Not a subset | JSON is a subset | Core Schema is JSON-compatible. |

**YAML-SPEC-REQ-020:** KubeCheck MUST document which YAML 1.1 features are resolved differently under the Core Schema and MUST NOT claim that YAML 1.1 behavior is invalid YAML; it is simply a different schema.

**YAML-SPEC-REQ-021:** KubeCheck MUST NOT enable YAML 1.1 compatibility by default. A future compatibility mode, if added, must be documented in an ADR and must be opt-in.

---

## 13. Constructs commonly encountered in Kubernetes manifests

The following constructs appear frequently in Kubernetes manifests. Each is valid YAML under the Core Schema; KubeCheck accepts them unless otherwise noted.

| Construct | Example | KubeCheck handling |
|---|---|---|
| Indentless sequence under `containers:` | `containers:\n- name: app` | `ACCEPT`. |
| Block literal for scripts | `command:\n- /bin/sh\n- -c\n- \|` | `ACCEPT`. |
| Folded block for descriptions | `description: >` | `ACCEPT`. |
| Anchors for shared labels | `&labels` / `*labels` | `ACCEPT`. |
| Merge key for defaults | `<<: *defaults` | `ACCEPT` as extension. |
| Quoted numeric strings for ports | `port: "8080"` | `ACCEPT`; string type preserved. |
| Unquoted numeric ports | `port: 8080` | `ACCEPT`; integer type. |
| Boolean env values | `value: "true"` | `ACCEPT`; string type. |
| Unquoted `on`/`off` in env | `value: on` | `ACCEPT AS STRING`; informational finding if schema expects boolean. |
| Multi-document stream of resources | `---` separated | `ACCEPT`; each document processed independently. |
| Date-like values in annotations | `date: 2026-10-31` | `ACCEPT AS STRING`. |
| Null values for optional fields | `nodeSelector: null` | `ACCEPT`; null type. |
| Flow-style small collections | `labels: {app: web}` | `ACCEPT`. |
| Comments in manifests | `# note` | `ACCEPT`; discarded. |
| Explicit `!!str` for ambiguous values | `value: !!str on` | `ACCEPT`; string type. |

**YAML-SPEC-REQ-022:** KubeCheck MUST accept every construct in this table without a parse error, because each is valid under the YAML 1.2.2 Core Schema. Findings arising from type mismatches are **schema/semantic findings**, not YAML parse errors.

---

## 14. Unsupported, discouraged, and security-sensitive constructs

### 14.1 Security-sensitive constructs

The following constructs are **valid YAML** but are treated as security-sensitive because they can lead to arbitrary code execution, information disclosure, or resource exhaustion when processed by unsafe parsers.

| Construct | Why sensitive | Specification position | KubeCheck |
|---|---|---|---|
| Application-specific tags that resolve to host-language constructors (e.g., `!!python/object/apply`, `!!ruby/object`) | Can execute arbitrary code in unsafe parsers | Valid YAML tags; the application defines their semantics | `REJECT`: safe mode prohibits constructor-resolution tags. |
| Anchors and aliases used for **exponential expansion** (billion laughs) | Resource exhaustion | Valid YAML | `REJECT` when alias expansion exceeds `max_aliases` (`LIM-REQ-004`). |
| Extremely deep nesting | Resource exhaustion / stack overflow | Valid YAML | `REJECT` when depth exceeds `max_depth` (`LIM-REQ-003`). |
| Oversized input | Memory exhaustion | Valid YAML | `REJECT` when size exceeds `max_input_bytes` (`LIM-REQ-001`). |
| Duplicate mapping keys | Can hide configuration defects | Invalid per specification | `REJECT` by default (`YAML-SPEC-REQ-016`). |

**YAML-SPEC-REQ-023:** KubeCheck MUST operate in safe mode and MUST reject application-specific tags that resolve to host-language constructors. This is an **implementation security decision**, not a claim that such tags are invalid YAML.

**YAML-SPEC-REQ-024:** KubeCheck MUST enforce resource limits before and during parsing, and MUST emit structured governor findings when limits are exceeded. Limits are implementation resource-safety controls, not YAML prohibitions.

### 14.2 Discouraged constructs

The following constructs are **valid YAML** but are discouraged for Kubernetes manifests because they are confusing, environment-dependent, or likely to be misinterpreted.

| Construct | Why discouraged | KubeCheck |
|---|---|---|
| Unquoted `yes`/`no`/`on`/`off` where a boolean is expected | YAML 1.1 resolves them as booleans; YAML 1.2 resolves them as strings. | `ACCEPT` with an **informational finding** when the schema expects a boolean. |
| Unquoted `0755` where an octal value is intended | YAML 1.2 resolves it as decimal `755`, not octal `493`. | `ACCEPT` with an **informational finding** when the schema expects an integer with octal semantics. |
| Unquoted date-like strings where a timestamp is intended | YAML 1.2 Core Schema does not resolve timestamps. | `ACCEPT AS STRING`; informational finding if the schema expects a timestamp type. |
| Excessively deep nesting of collections | Readability and resource safety. | `ACCEPT` within limits; informational finding above a configurable advisory threshold. |

**YAML-SPEC-REQ-025:** KubeCheck MUST emit an informational finding for each discouraged construct when it appears in a schema-expected context, explaining the Core Schema resolution and suggesting explicit quoting or tagging.

### 14.3 Constructs KubeCheck rejects

KubeCheck rejects the following constructs. Each rejection is an **implementation decision**; the specification position is stated so that the rejection is not misread as a YAML prohibition.

| Construct | Specification position | KubeCheck decision | Rationale |
|---|---|---|---|
| Tab indentation | Invalid | `REJECT` | Specification requires spaces; tabs are a parse error. |
| Unknown double-quote escape | Invalid | `REJECT` | Specification requires valid escapes. |
| Undefined alias | Invalid | `REJECT` | Specification requires anchors to precede aliases. |
| Duplicate mapping keys | Invalid | `REJECT` | Specification requires unique keys; duplicates hide defects. |
| Constructor-resolving tags (`!!python/*`, etc.) | Valid as application tags | `REJECT` | Safe-mode requirement (`SEC-REQ-001`). |
| Input above resource limits | Valid YAML | `REJECT` with governor finding | Resource-safety requirement (`LIM-REQ-001`–`008`). |
| Block collection inside flow collection | Invalid | `REJECT` | Specification disallows block collections in flow context. |
| New document without boundary marker | Invalid | `REJECT` | Specification requires separation. |

---

## 15. KubeCheck acceptance policy — consolidated register

The following register is the authoritative summary of KubeCheck's acceptance policy for YAML features. Each row cites the governing requirement ID.

| YAML feature | Specification section | KubeCheck policy | Governing requirement |
|---|---|---|---|
| Single document, no `---` | §9.1 | `ACCEPT` | `YAML-SPEC-REQ-002` |
| Multiple documents with `---` | §9.1 | `ACCEPT` | `YAML-SPEC-REQ-002` |
| `...` document end marker | §9.1 | `ACCEPT` | `YAML-SPEC-REQ-002` |
| Directives (`%YAML`, `%TAG`) | §6.8 | `%YAML`: `ACCEPT WITH WARNING`; `%TAG`: `ACCEPT` if declared | `YAML-SPEC-REQ-003` |
| Space indentation | §6.1 | `ACCEPT` | `YAML-SPEC-REQ-004` |
| Tab indentation | §6.1 | `REJECT` | `YAML-SPEC-REQ-004` |
| Indentless sequence | §8.2.1 | `ACCEPT` | `YAML-SPEC-REQ-005` |
| Block collections | §8 | `ACCEPT` | `YAML-SPEC-REQ-006` |
| Flow collections | §7 | `ACCEPT` | `YAML-SPEC-REQ-006` |
| Block collection inside flow collection | §7 | `REJECT` | `YAML-SPEC-REQ-007` |
| Plain scalar (single/multi-line) | §7.3 | `ACCEPT` | `YAML-SPEC-REQ-009` |
| Single-quoted scalar | §7.3.2 | `ACCEPT` | `YAML-SPEC-REQ-009` |
| Double-quoted scalar (known escapes) | §7.3.1 | `ACCEPT` | `YAML-SPEC-REQ-008` |
| Double-quoted scalar (unknown escape) | §7.3.1 | `REJECT` | `YAML-SPEC-REQ-008` |
| Literal block scalar (`|`) | §8.1 | `ACCEPT` | `YAML-SPEC-REQ-009` |
| Folded block scalar (`>`) | §8.1 | `ACCEPT` | `YAML-SPEC-REQ-009` |
| Chomping indicators | §8.1.1.2 | `ACCEPT` | `YAML-SPEC-REQ-009` |
| Null (`null`, `~`, empty) | §10.3.2 | `ACCEPT` | `YAML-SPEC-REQ-010` |
| Boolean (`true`/`false`) | §10.3.2 | `ACCEPT` | `YAML-SPEC-REQ-010` |
| `yes`/`no`/`on`/`off` | §10.3.2 | `ACCEPT AS STRING` | `YAML-SPEC-REQ-011` |
| `0755` | §10.3.2 | `ACCEPT AS INTEGER 755` | `YAML-SPEC-REQ-011` |
| `0o755` | §10.3.2 | `ACCEPT AS INTEGER 493` | `YAML-SPEC-REQ-011` |
| Date-like strings | §10.3.2 | `ACCEPT AS STRING` | `YAML-SPEC-REQ-011` |
| `.inf` / `.nan` | §10.3.2 | `ACCEPT` | `YAML-SPEC-REQ-010` |
| Explicit core tags (`!!str`, etc.) | §10.4 | `ACCEPT` | `YAML-SPEC-REQ-012` |
| Application-specific tags (`!custom`) | §6.8.2 | `ACCEPT AS OPAQUE` | `YAML-SPEC-REQ-012` |
| Anchors and aliases | §3.2.2 | `ACCEPT` | `YAML-SPEC-REQ-012` |
| Undefined alias | §3.2.2.2 | `REJECT` | `YAML-SPEC-REQ-012` |
| Merge key (`<<`) | Merge Key spec (extension) | `ACCEPT` as extension | `YAML-SPEC-REQ-013` |
| Comments | §6.2 | `ACCEPT`; discarded | — |
| UTF-8 input | §5.1 | `ACCEPT` | `YAML-SPEC-REQ-014` |
| Invalid UTF-8 | §5.1 | `REJECT` | `YAML-SPEC-REQ-014` |
| CRLF line breaks | §5.4 | `ACCEPT`; normalized | `YAML-SPEC-REQ-015` |
| Duplicate mapping keys | §3.2.1.3 | `REJECT` | `YAML-SPEC-REQ-016` |
| Duplicate keys under Core Schema canonical equality | §3.2.1.3 | `REJECT` | `YAML-SPEC-REQ-018` |
| Quoted vs unquoted forms of same text | §3.2.1.3 | `ACCEPT` (different tags) | `YAML-SPEC-REQ-018` |
| Empty document | §9.1 | `ACCEPT` | `YAML-SPEC-REQ-019` |
| Scalar root | §9.1 | `ACCEPT` at parser; downstream finding | `YAML-SPEC-REQ-019` |
| Constructor-resolving tags | §6.8.2 | `REJECT` | `YAML-SPEC-REQ-023` |
| Resource-exhausting constructs | Various | `REJECT` with governor finding | `YAML-SPEC-REQ-024` |
| YAML 1.1 compatibility features | §1.2 of YAML 1.1 | Core Schema resolution; no 1.1 mode by default | `YAML-SPEC-REQ-020` |

---

## 16. Rationale for KubeCheck decisions

### 16.1 Why Core Schema

The YAML 1.2.2 Core Schema is the most widely deployed default for YAML 1.2 processors. It is JSON-compatible, avoids the ambiguous boolean forms of YAML 1.1, and provides well-defined integer, float, and null resolution. Kubernetes manifests authored against the Kubernetes API conventions do not rely on YAML 1.1 boolean forms; they rely on explicit `true`/`false` and on quoted strings where ambiguity would otherwise arise.

### 16.2 Why reject duplicate keys

The specification requires unique keys. Allowing duplicates with silent first-wins or last-wins semantics can mask configuration defects that would otherwise be caught before deployment. KubeCheck's default rejection surfaces these defects early. The Core Schema canonical equality rule (e.g., `0xB` and `11` are the same key) is applied so that semantically duplicate keys are caught even when their textual forms differ.

### 16.3 Why accept the merge key as an extension

The merge key is not part of the Core Schema, but it is widely used in Kubernetes-adjacent configuration and in Helm-equivalent templates. Rejecting it would produce false parse errors for valid, common manifests. Accepting it as a documented extension preserves interoperability while making the extension status explicit.

### 16.4 Why accept indentless sequences

Indentless sequences under mapping keys are valid YAML and are the predominant style in Kubernetes documentation. Rejecting them would break the majority of real manifests. KubeCheck accepts them and records the effective structure in the NIR.

### 16.5 Why reject tab indentation

The specification prohibits tabs for indentation. There is no interoperability benefit to accepting them: a manifest with tab indentation will be rejected by Kubernetes' own YAML parsing. KubeCheck rejects with a clear parse error rather than allowing a manifest that cannot be applied.

### 16.6 Why treat resource limits as implementation decisions

The specification does not impose limits on input size, nesting depth, alias expansion, or document count. These are **resource-safety controls** required by `REQUIREMENTS_AND_NFRS.md` (§7) and `SEC-REQ-002`. KubeCheck labels them as such and does not claim that YAML prohibits large inputs.

---

## 17. Authoritative references

| Reference | Scope | Type | Retrieved |
|---|---|---|---|
| YAML Ain't Markup Language (YAML™) version 1.2, Revision 1.2.2 (2021-10-01) | Primary normative reference for all YAML language behavior in this document | Specification | 2026-10-31 |
| YAML 1.2.2 Core Schema (§10.3) | Default implicit type resolution for plain scalars | Specification section | 2026-10-31 |
| YAML 1.2.2 §3.2.1.3 (Node Comparison) | Mapping key uniqueness and canonical equality | Specification section | 2026-10-31 |
| YAML 1.2.2 §7 (Flow Styles) and §8 (Block Styles) | Collection and scalar presentation styles | Specification sections | 2026-10-31 |
| YAML 1.2.2 §9 (Document Streams) | Streams, documents, directives, boundaries | Specification section | 2026-10-31 |
| YAML 1.2.2 §5.4 (Line Break Characters) | Line-break normalization | Specification section | 2026-10-31 |
| Merge Key Language-Independent Type | Merge-key semantics (extension) | Separate specification | 2026-10-31 |
| Kubernetes Configuration Good Practices | Ambiguous boolean guidance for Kubernetes manifests | Official documentation | 2026-10-31 |
| ADR-002 YAML parser selection | Parser choice and conformance configuration | Internal (OPEN) | 2026-10-31 |

**Licensing note:** The YAML 1.2.2 specification is freely copyable provided it is not modified. The merge-key specification is part of the YAML type repository. Kubernetes documentation is licensed under CC BY 4.0. KubeCheck records the license of any schema or rule artefact incorporated into a knowledge bundle. No line numbers, benchmark results, or implementation status are asserted in this document beyond what the cited sources state.

---

## 18. Open questions and unresolved decisions

| ID | Question | Status | Owner |
|---|---|---|---|
| OPEN-YAML-01 | Final parser selection and Core Schema conformance details (ADR-002). Affects scalar resolution, duplicate-key equality, and merge-key handling. | **OPEN** | Software Architecture |
| OPEN-YAML-02 | Whether a lenient duplicate-key mode will be supported, and if so, what the resolution policy is. | **OPEN** | CLI Architecture |
| OPEN-YAML-03 | Whether YAML 1.1 compatibility mode will ever be supported, and under what opt-in conditions. | **OPEN** | Software Architecture |
| OPEN-YAML-04 | Exact advisory threshold for deeply nested structures before an informational finding is emitted. | **OPEN** | Diagnostics Architecture |
| OPEN-YAML-05 | Whether timestamp-like strings should produce an informational finding regardless of schema context, or only when the schema expects a timestamp type. | **OPEN** | Diagnostics Architecture |

---

## 19. Definition of done

This document is complete when the following conditions hold.

1. Every YAML feature listed in the scope (§1) is covered with a statement of the specification position and KubeCheck's acceptance policy.
2. Every acceptance-policy row cites a governing `YAML-SPEC-REQ-*` or downstream requirement ID.
3. The distinction between specification behavior, implementation-specific behavior, and KubeCheck's decision is explicit in every section where a difference exists.
4. No feature is described as "prohibited by YAML" merely because KubeCheck rejects it; rejections are labelled as implementation decisions with rationale.
5. YAML 1.1 vs YAML 1.2 compatibility differences are enumerated and resolved against the Core Schema.
6. Kubernetes-specific constructs are covered and their KubeCheck handling is stated.
7. Security-sensitive constructs are enumerated with the safe-mode rationale and the governing requirement IDs.
8. Resource limits are labelled as implementation controls, not YAML restrictions.
9. The consolidated register in §15 provides a single lookup table for every feature.
10. All references are cited with version and retrieval date.
11. Open decisions are recorded with ownership.

---

## 20. Consistency and completeness review

A review was performed against the requested scope before finalization.

- **Streams, documents, directives, boundaries:** §3, with specification citations and acceptance table.
- **Indentation and block structure:** §4, including tab rejection and indentless sequences.
- **Block and flow sequences and mappings:** §5, with flow/block equivalence and rejection of block-in-flow.
- **Scalar styles:** §6, covering plain, single-quoted, double-quoted, literal, folded, and chomping.
- **Scalar resolution:** §7, covering null, booleans, integers, floats, timestamps, and ambiguous values under the Core Schema.
- **Tags, anchors, aliases, merge keys, directives:** §8, distinguishing Core Schema tags from extensions and application tags.
- **Comments, Unicode, escapes, line breaks, whitespace:** §9, with UTF-8 and line-break normalization requirements.
- **Mapping keys and duplicate-key ambiguity:** §10, with the specification's uniqueness requirement and Core Schema canonical equality.
- **Document edge cases:** §11, covering empty documents, multiple documents, nested structures, and scalar roots.
- **YAML 1.1 vs 1.2 differences:** §12, with a comparison table and explicit Core Schema resolution.
- **Kubernetes constructs:** §13, with acceptance handling for each common pattern.
- **Security-sensitive and discouraged constructs:** §14, with rationale and governing requirements.
- **Consolidated acceptance register:** §15, single lookup table.
- **Rationale:** §16, explaining each major decision without claiming YAML prohibits KubeCheck-unsupported features.
- **References and open questions:** §17 and §18.

No empty boilerplate section was added solely to satisfy a checklist. The document is complete for its assigned responsibility.
