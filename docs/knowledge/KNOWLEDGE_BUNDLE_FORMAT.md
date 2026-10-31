# KubeCheck — Knowledge Bundle Format

**Document ID:** KNOWLEDGE-FMT-001 · **Status:** Normative — Approved · **Version:** 1.0.0 · **Owner:** Data-Format and Package-Format Architecture · **Last reviewed:** 2026-10-31

This document specifies the complete, versioned KubeCheck knowledge-bundle format. A knowledge bundle is the sole source of KubeCheck's knowledge about Kubernetes — the schemas it validates against, the diagnostic rules it evaluates, the lifecycle metadata it consults, the feature-gate metadata it respects, the relationship definitions it uses for cross-resource analysis, and the provenance it cites. The engine binary contains the interpreter, the parser, the graph builder, and the aggregator; the bundle contains everything else. This separation is not a packaging convenience — it is the architectural mechanism that allows a rule to be added, a schema to be corrected, or a Kubernetes target version to be supported without replacing the engine binary, and it is the mechanism that makes the offline-first guarantee achievable because the bundle is a single signed artifact that can be carried across an air gap on removable media and verified on the isolated machine without any network access.

Every requirement in this document is identified with the prefix `KNOWLEDGE-REQ-NNN`, every bundle-level structure with `DS-BUNDLE-<NAME>`, every validation check with `VALIDATE-<NAME>-NNN`, and every test with `KNOWLEDGE-TEST-NNN`. The document develops the bundle manifest and format version, the engine compatibility range, the supported Kubernetes versions, the schema files and their metadata, the rule packs, the relationship definitions, the diagnostic catalog and source references, the provenance and licensing information, the cryptographic hashes and signature metadata, the indexes and compression strategy, the optional data partitions, the resource limits, the unknown-field and forward-compatibility policy, the validation, loading, caching, and integrity verification pipeline, the backward compatibility and deprecation policy, the handling of invalid, incomplete, stale, and unsupported bundles, a complete illustrative bundle manifest and directory layout, a machine-readable schema for the bundle metadata, and the workflow for generating a bundle on a connected machine and transporting it securely to an isolated machine.

## The governing principles

Five principles govern the bundle format, and every requirement in this document derives from them.

The first principle, `KNOWLEDGE-REQ-001`, is **the bundle is self-sufficient**. Everything the engine needs to evaluate every rule it will evaluate must be present in the bundle. There is no fallback to a network source, no lazy fetch of a missing schema, no implicit dependency on a service, and no "if the bundle is missing X, use a built-in default." A bundle that is missing a component it declares is an invalid bundle, not a partially-functional bundle.

The second principle, `KNOWLEDGE-REQ-002`, is **the bundle is signed as a whole**. The signature covers the manifest and every file in the bundle, not just the manifest. A bundle whose signature covers only the manifest is vulnerable to payload substitution (`THR-005`), and the format must make that vulnerability impossible by construction.

The third principle, `KNOWLEDGE-REQ-003`, is **the format is forward-compatible by explicit versioning, not by silent tolerance**. A bundle declares its format version, and an engine that does not support that version rejects the bundle with a specific error. An engine does not attempt to interpret a bundle whose format version it does not know, because a partial interpretation of an unknown format is a security risk (the format may have added a field with security-relevant semantics that the older engine will ignore).

The fourth principle, `KNOWLEDGE-REQ-004`, is **the bundle's provenance is cryptographically recorded and independently verifiable**. Every schema, every rule, every lifecycle entry, and every feature-gate entry cites its source document, the source's version, the retrieval date, and — where possible — a content hash of the source. The user can verify that the bundle's knowledge corresponds to the sources it claims to have been derived from.

The fifth principle, `KNOWLEDGE-REQ-005`, is **the bundle is transportable across an air gap without modification**. A bundle generated on a connected machine is the same bytes when it arrives on an isolated machine. There is no "re-sign" step, no "re-verify against a remote server" step, and no modification on transfer. The trust anchor is the same on both machines, and the verification is the same on both machines.

These five principles interact to give the bundle format its essential property: a bundle is a single, self-contained, signed, versioned artifact that carries everything needed to reproduce the tool's knowledge for a specific set of Kubernetes versions, and it can be verified and used on a machine that has never been connected to the internet.

## Bundle manifest and format version

The bundle manifest is a JSON document at the root of the bundle directory, named `manifest.json`. It is the first file the loader reads, and it determines how the rest of the bundle is interpreted. The manifest has the following top-level fields.

`format_version` is a semantic version string (`"1.0.0"`) that identifies the bundle format. `KNOWLEDGE-REQ-006` requires that this field be present and be a valid semantic version. `KNOWLEDGE-REQ-007` requires that the engine declare the set of format versions it supports and reject a bundle whose format version is outside that set, with an error that names both the bundle's version and the engine's supported range. The format version and the bundle content version are distinct: `format_version` describes how the bundle is structured, while `bundle_version` (below) describes what knowledge the bundle contains.

`bundle_version` is a monotonically increasing integer that is used for rollback protection (`THR-024`). `KNOWLEDGE-REQ-008` requires that this field be present, be a positive integer, and be greater than the previous bundle's `bundle_version` for the same signer. The loader rejects a bundle whose `bundle_version` is not greater than the active bundle's `bundle_version`.

`bundle_id` is a human-readable identifier (`"kubecheck-standard-2026.10.1"`) used in reports and logs. `KNOWLEDGE-REQ-009` requires that this field be present, be unique across the signer's bundles, and be reproduced in the report header.

`engine_compatibility` is an object with `min_version` and `max_version` fields (both semantic version strings) describing the range of KubeCheck engine versions that can load the bundle. `KNOWLEDGE-REQ-010` requires that this field be present, that `min_version` be less than or equal to `max_version`, and that the engine reject a bundle whose range does not include the engine's own version.

`kubernetes_versions` is a list of objects, each with `minor` (e.g., `"1.34"`), `patch_pin` (e.g., `"1.34.2"`), `schema_version` (a per-version schema format version), `status` (`"supported"` or `"deprecated"`), and `supported_until` (an ISO-8601 date, for deprecated versions). `KNOWLEDGE-REQ-011` requires that at least one Kubernetes version be listed, that every version in the list be covered by schemas and lifecycle metadata, and that no version be listed twice.

`generation` is an object with `timestamp` (ISO-8601), `generator_version`, `source_commit` (the KubeCheck commit that produced the bundle), and `reproducible` (a boolean indicating whether the bundle is reproducible from the recorded inputs). `KNOWLEDGE-REQ-012` requires that this object be present and that `source_commit` be a full git commit hash.

`provenance` is a list of objects, each with `source_id`, `source_type` (`"kubernetes-docs"`, `"kubernetes-source"`, `"yaml-spec"`, `"cve"`, `"internal"`), `source_url` or `source_path`, `source_version`, `retrieved_at` (ISO-8601), and `content_hash` (a SHA-256 hash of the source content, where the source is a file or a deterministic document). `KNOWLEDGE-REQ-013` requires that every schema, rule, lifecycle entry, and feature-gate entry reference at least one `source_id` from this list.

`files` is a list of objects, each with `path` (a relative path from the bundle root), `sha256` (the file's content hash), `size` (in bytes), and `kind` (`"schema"`, `"rule"`, `"lifecycle"`, `"feature-gate"`, `"relationship"`, `"catalog"`, `"index"`). `KNOWLEDGE-REQ-014` requires that every file in the bundle (except the manifest and the signature) be listed in `files`, that every listed file exist, and that every listed file's hash match the computed hash at load time. A file that is present but not listed is an invalid bundle; a file that is listed but not present is an invalid bundle.

`signature` is an object with `algorithm` (`"ed25519"` in the MVP), `key_id` (the identifier of the signing key), `value` (the base64-encoded signature), and `signed_manifest_hash` (the hash of the manifest with the signature field zeroed out). `KNOWLEDGE-REQ-015` requires that the signature cover the manifest hash and the concatenated hash of every file listed in `files`, in the order they appear. The signature verification procedure is specified in the validation section.

`partitions` is an optional list of objects, each with `name`, `description`, `version`, and `files` (paths into the `files` list). Partitions allow a bundle to declare logical groupings of files so that future features can selectively load or skip a partition. `KNOWLEDGE-REQ-016` requires that partitions be additive — a bundle without partitions is valid, and a bundle with partitions must be usable by an engine that ignores partitions.

`limits` is an object with `max_total_size`, `max_files`, `max_schema_size`, `max_rule_pack_size`, `max_index_entries`, and `max_decompressed_size`. These are the limits that the generator enforces when producing the bundle; the loader re-checks them against the bundle's declared limits to detect a bundle that has been tampered with after generation. `KNOWLEDGE-REQ-017` requires that the limits be present, be positive integers, and not exceed the engine's hard limits (specified in the resource limits section below).

`KNOWLEDGE-REQ-018` requires that the manifest be valid JSON, that every field be of the declared type, that no unknown fields be present (the strict-mode policy is specified in the forward-compatibility section), and that the manifest be no larger than 64 KiB (the manifest is small because it is the entry point; the knowledge is in the files it lists).

## Engine compatibility range

The engine compatibility range is the mechanism that prevents an old engine from loading a new bundle that relies on knowledge the engine does not understand, and that prevents a new engine from loading an old bundle whose format it no longer supports.

The `min_version` and `max_version` in `engine_compatibility` are semantic version bounds. `KNOWLEDGE-REQ-019` requires that the engine compare its own version against the range using semantic-version precedence, treating a version outside the range as an incompatible bundle. The engine must use the same comparison logic across all platforms, so that a bundle accepted on Linux is accepted on Windows and macOS for the same engine version.

`KNOWLEDGE-REQ-020` requires that the compatibility range be as wide as the bundle's features permit. A bundle that uses only features present in engine `0.1.0` should declare `min_version: "0.1.0"` even if it was generated by engine `0.4.0`, so that users on older engines can benefit from the knowledge. A bundle that uses a feature introduced in engine `0.4.0` must declare `min_version: "0.4.0"` and must not be generated with a lower `min_version`.

`KNOWLEDGE-REQ-021` requires that the engine's supported format version set be documented in the engine's `version` output, so that a user can check compatibility before installing a bundle. The output must list the supported format versions and the supported engine versions.

The interaction between format version and engine compatibility is important. The format version describes the bundle's structural schema; the engine compatibility describes the engine versions that can interpret that structure. A bundle with format version `1.0.0` and engine compatibility `>=0.1.0, <1.0.0` is loadable by any engine in the `0.x` series that supports format version `1.0.0`. A future engine `1.0.0` may or may not support format version `1.0.0` depending on the engine's own supported-format set; if it does not, it rejects the bundle with a specific error.

## Supported Kubernetes versions

The `kubernetes_versions` list declares which Kubernetes minor versions the bundle covers, which patch release is pinned for each, and which versions are deprecated. This list is the authoritative statement of the bundle's Kubernetes coverage, and it must be complete: every schema file, rule applicability range, and lifecycle entry must be consistent with the list.

`KNOWLEDGE-REQ-022` requires that every `minor` be a valid Kubernetes minor version (`"1.x"`) and that every `patch_pin` be a valid patch release within that minor. The patch pin is the exact release whose schemas are bundled, and it is reported in every report header so that a user can reproduce the analysis.

`KNOWLEDGE-REQ-023` requires that the `schema_version` for each minor be a version that the engine's schema validator supports. A schema version that the engine does not support is an incompatible bundle.

`KNOWLEDGE-REQ-024` requires that a `deprecated` version remain loadable for at least one engine minor release (`K8S-VER-REQ-005`) and that the `supported_until` date be in the future at bundle generation time. A bundle whose deprecated version is past `supported_until` at generation time is a defect in the bundle, not in the engine.

`KNOWLEDGE-REQ-025` requires that the list contain at least one `supported` version. A bundle with only deprecated versions is an invalid bundle, because it provides no currently-supported analysis target.

## Schema files and schema metadata

Schema files are the bundle's largest component and the component most directly tied to the Kubernetes release. The format supports two schema layouts: a **per-GVK layout** in which each GVK's schema is a separate file, and a **bundled layout** in which all schemas for a Kubernetes minor version are in one file. The per-GVK layout is the default because it allows schema updates to be delta-sized (only the changed GVKs need to be regenerated), while the bundled layout is available for bundles where file-count is a constraint.

The per-GVK layout places schemas at `schemas/<minor>/<group>/<version>/<kind>.json`, where `<group>` is the API group (`""` for the core group, serialized as `_core`), `<version>` is the API version, and `<kind>` is the Kind. Each schema file is a JSON object with the following fields: `gvk` (the group, version, kind tuple), `schema` (the OpenAPI v3 structural schema), `scope` (`"Namespaced"` or `"Cluster"`), `short_names`, `plural`, `singular`, `categories`, `subresources`, and `provenance` (a reference to the `provenance` entry from the manifest).

`KNOWLEDGE-REQ-026` requires that every schema file be valid JSON, be valid OpenAPI v3, and conform to the engine's schema format. A schema file that fails validation causes the affected GVK to be treated as uncovered (`FAIL-SCHEMA-002`) and produces a `warning`-severity finding, but does not invalidate the rest of the bundle unless the schema is declared `required: true` in the schema metadata.

`KNOWLEDGE-REQ-027` requires that every schema file's `provenance` reference resolve to an entry in the manifest's `provenance` list. A schema without a provenance reference is an invalid schema.

Schema metadata is stored in a separate `schemas/index.json` file that lists every schema file with its GVK, its hash, and its size. `KNOWLEDGE-REQ-028` requires that this index be present, that every schema file be listed, and that the index itself be listed in the manifest's `files` list. The index is used by the loader to build the in-memory schema lookup without reading every schema file eagerly.

## Diagnostic rule packs

Rule packs are the bundle's behavioral component. Each rule pack is a directory under `rules/<pack_id>/` containing the pack manifest (`pack.json`), the rule files (`rules/<rule_id>.yaml`), and the fixture files (`fixtures/<rule_id>/<fixture_id>.yaml`). The pack manifest declares the pack's version, the rule schema version it conforms to, the engine compatibility range for the pack, and the list of rules it contains.

`KNOWLEDGE-REQ-029` requires that every rule pack be valid according to the rule schema defined in `docs/engine/DIAGNOSTIC_RULE_SPEC.md` §17. A rule pack that fails validation causes its rules to be disabled (`FAIL-RULE-001`) and produces a `warning`-severity finding, unless the pack is declared `required: true` in the bundle manifest, in which case the bundle is rejected.

`KNOWLEDGE-REQ-030` requires that every rule pack be signed as part of the bundle (the bundle signature covers the pack's files). An unsigned rule pack that appears in the bundle directory is an invalid bundle, and the loader rejects it.

`KNOWLEDGE-REQ-031` requires that every rule's fixtures be present in the bundle. The fixtures are the basis for the rule's unit tests, and a rule without fixtures is a rule that cannot be verified. The loader runs the fixtures at bundle-load time (in a lightweight mode) to confirm that the rule behaves as expected for the bundle's own content, and a rule whose fixtures fail is disabled.

The pack manifest is the authoritative declaration of a pack's contents, and the rule files are the implementation. `KNOWLEDGE-REQ-032` requires that every rule listed in the pack manifest exist in the pack directory, and every rule file in the pack directory be listed in the pack manifest. An orphan rule file or a missing rule file is an invalid pack.

Rule packs can be partitioned by category (`rules/security/`, `rules/scheduling/`, `rules/networking/`, and so on) for organizational clarity, but the format does not require a specific partition scheme. `KNOWLEDGE-REQ-033` requires that the partition scheme, if used, be documented in the bundle manifest's `partitions` list.

## Resource relationship definitions

The relationship definitions are the bundle's contribution to the cross-resource analysis. They describe, for each edge type, the source resource kinds, the target resource kinds, the fields that establish the edge, and the semantics of the resolution (direct reference, selector match, generated reference, external reference).

Relationship definitions are stored at `relationships/edges.json` and have the following structure: a list of edge definitions, each with `edge_id`, `edge_type`, `source_gvks`, `target_gvks`, `source_fields` (the JSON paths that establish the edge), `resolution_kind` (`"direct"`, `"selector"`, `"generated"`, `"external"`), `optional_field` (the JSON path of the optional flag, if any), and `provenance` (a reference to the manifest's provenance list).

`KNOWLEDGE-REQ-034` requires that every edge type used by any rule be defined in the relationship definitions. A rule that declares a dependency on an edge type that is not in the definitions is an invalid rule, and the rule is disabled.

`KNOWLEDGE-REQ-035` requires that every relationship definition be consistent with the schemas it references: the source and target GVKs must exist in the schema index for every supported Kubernetes version, and the source fields must resolve to fields declared in the schema. A relationship definition that references a nonexistent GVK or field is an invalid definition, and the affected edges are not built.

## Diagnostic catalog and source references

The diagnostic catalog is the bundle's user-facing index of every rule and its metadata. It exists so that the `kubecheck explain <rule-id>` command can present a rule's description, category, evidence class, severity, examples, and references without loading the rule's implementation.

The catalog is stored at `catalog/rules.json` and contains a list of objects, each with `rule_id`, `title`, `description`, `category`, `evidence_class`, `severity_default`, `references` (a list of URLs and titles), `examples` (a list of short manifest snippets, without secrets), and `provenance` (a reference to the manifest's provenance list).

`KNOWLEDGE-REQ-036` requires that every rule in a rule pack be represented in the catalog. A rule without a catalog entry is an invalid rule, and the loader rejects the pack.

`KNOWLEDGE-REQ-037` requires that every catalog entry's references be valid URLs (or internal paths for provenance references) and that the references be checked at bundle-generation time (`RCA-REMED-REQ-049`). A catalog entry with a broken reference is a defect in the bundle, and the bundle is rejected at release time.

## Provenance, licensing, generation timestamp, and source versions

The provenance and licensing information is the bundle's claim to have been derived from authoritative sources. It is stored in the manifest's `provenance` list and in a separate `LICENSES/` directory containing the full text of every license under which any bundled artifact is distributed.

`KNOWLEDGE-REQ-038` requires that the `provenance` list contain an entry for every distinct source used by any schema, rule, relationship, or catalog entry. An entry with a `source_url` must have a `retrieved_at` date no later than the bundle's `generation.timestamp`, so that a user can verify that the source was current at generation time.

`KNOWLEDGE-REQ-039` requires that the `LICENSES/` directory contain the full text of every license referenced by any artifact in the bundle, named as `<SPDX-identifier>.txt` (e.g., `Apache-2.0.txt`, `CC-BY-4.0.txt`). The bundle's `licenses` field in the manifest lists the SPDX identifiers of every bundled license. A bundle that redistributes an artifact under a license whose text is not present is an invalid bundle.

`KNOWLEDGE-REQ-040` requires that the `generation.timestamp` be an ISO-8601 timestamp in UTC, and that the `generator_version` be a semantic version string matching the tool that produced the bundle. The `reproducible` field is `true` only if the generator has been verified to produce byte-identical output from identical inputs; a bundle that declares `reproducible: true` but is not reproducible is a defect, and the release process rejects it.

## Cryptographic hashes and signature metadata

The cryptographic hashes and signature metadata are the bundle's integrity mechanism. The `files` list contains a SHA-256 hash of every file, and the `signature` object contains the signature over the manifest hash and the file hashes.

`KNOWLEDGE-REQ-041` requires that every file's `sha256` be computed over the file's exact bytes as stored in the bundle, with no normalization (no line-ending conversion, no whitespace trimming, no Unicode normalization). The loader recomputes the hash and compares it to the manifest's value; a mismatch is a bundle-corruption error (`FAIL-BUNDLE-001`).

`KNOWLEDGE-REQ-042` requires that the signature be computed over a canonical serialization of the manifest and the file list. The canonical serialization is defined as: the manifest JSON with the `signature` field omitted, serialized with sorted keys and no whitespace, concatenated with the sorted list of `(path, sha256)` pairs serialized as `<path>:<sha256>\n`. The signature algorithm in the MVP is Ed25519, and the signing key is the project's release key whose public key is distributed with the engine.

`KNOWLEDGE-REQ-043` requires that the signature be verified using a locally resident trust anchor. The trust anchor is a public key that ships with the engine or is installed by the user via a documented mechanism (`kubecheck trust add <key-id> <public-key>`). The verification must not require network access, and must use the same anchor on connected and isolated machines.

`KNOWLEDGE-REQ-044` requires that signature verification failure cause the bundle to be rejected with a `fatal` error (`FAIL-BUNDLE-002`) and that the prior bundle (if any) remain active. A bundle whose signature is valid but whose signer is not in the trust anchor set is a `warning`-severity situation in an environment that permits warning-level signers, but the MVP treats it as `fatal` because unsigned bundles are not permitted.

## Indexes, compression, and optional data partitions

The bundle format supports indexes to accelerate loading and compression to reduce size.

The primary index is `schemas/index.json`, which lists every schema file with its GVK and hash. The loader reads this index first, builds an in-memory map from GVK to schema file path, and loads individual schema files lazily (on first use). `KNOWLEDGE-REQ-045` requires that the primary index be present and be listed in the manifest's `files` list.

The secondary index is `rules/index.json`, which lists every rule pack and every rule within it. `KNOWLEDGE-REQ-046` requires that the secondary index be present and that it be regenerable from the pack manifests (the index is an optimization, not a source of truth; a discrepancy between the index and the pack manifests is a defect in the bundle, and the pack manifests take precedence).

The catalog index is `catalog/index.json`, which lists every rule's catalog entry. `KNOWLEDGE-REQ-047` requires that the catalog index be present and consistent with `catalog/rules.json`.

Compression is applied to individual files, not to the bundle as a whole. A file can be stored compressed (`<file>.gz`) or uncompressed. The manifest's `files` list records the compression (`compression: "none"` or `compression: "gzip"`) and the decompressed size (`decompressed_size`). `KNOWLEDGE-REQ-048` requires that a compressed file's `sha256` be computed over the **decompressed** bytes, so that the hash is stable regardless of the compression level. The loader decompresses, then computes the hash, then verifies.

`KNOWLEDGE-REQ-049` requires that the decompressed size of every file be bounded by the manifest's `limits.max_decompressed_size`, so that a compression-bomb bundle is rejected before decompression completes. The loader checks the declared `decompressed_size` against the limit and, during decompression, stops if the actual decompressed size exceeds the declared size.

Data partitions allow a bundle to declare logical groupings of files. A partition has a `name`, a `description`, a `version`, and a `files` list. Partitions are used in future releases to allow selective loading: an engine that only needs the security rules can load only the `security` partition and skip the `scheduling` partition. The MVP engine loads all partitions because it does not yet support selective loading; the format is defined now so that future engines can use it without a format-version bump.

`KNOWLEDGE-REQ-050` requires that partitions be additive and non-overlapping: every file is in at most one partition, and a file not in any partition is loaded unconditionally. A bundle with overlapping partitions is an invalid bundle.

## Resource limits and maximum supported bundle size

The bundle format imposes limits on its own size, file count, and decompressed size so that a bundle cannot be a denial-of-service vector and so that the loader's memory footprint is bounded.

`KNOWLEDGE-REQ-051` requires that the manifest's `limits.max_total_size` not exceed the engine's hard limit of 512 MiB. The MVP bundles are expected to be under 50 MiB; the hard limit is set high to accommodate future expansion but low enough that a bundle cannot exhaust a typical CI runner's disk.

`KNOWLEDGE-REQ-052` requires that the manifest's `limits.max_files` not exceed the engine's hard limit of 100,000. A bundle with more files is rejected because the loader's per-file bookkeeping (open file descriptors, hash computations) would become the bottleneck.

`KNOWLEDGE-REQ-053` requires that the manifest's `limits.max_schema_size` not exceed 16 MiB per schema file and that `limits.max_rule_pack_size` not exceed 32 MiB per pack. These limits accommodate the largest Kubernetes schemas (the `CustomResourceDefinition` and `Pod` schemas) with headroom.

`KNOWLEDGE-REQ-054` requires that the manifest's `limits.max_decompressed_size` not exceed the engine's hard limit of 2 GiB total decompressed across all files. This is the bound on the loader's peak memory footprint during decompression.

`KNOWLEDGE-REQ-055` requires that the loader check the bundle's declared limits against the engine's hard limits at load time, and reject a bundle whose declared limits exceed the engine's hard limits (which would indicate tampering or a bug in the generator).

## Unknown fields and forward-compatibility policy

The bundle format's forward-compatibility policy is strict: unknown fields are rejected, and the engine does not attempt to interpret a bundle whose format it does not fully understand. This is the opposite of the "tolerant reader" pattern that is common in JSON APIs, and it is deliberate.

`KNOWLEDGE-REQ-056` requires that the manifest parser reject any field not declared in the manifest schema. A bundle with an unknown manifest field is an invalid bundle. The rejection must name the unknown field and the manifest's location.

`KNOWLEDGE-REQ-057` requires that the same strict policy apply to schema files, rule packs, relationship definitions, and catalog entries: any field not declared in the corresponding schema is a rejection. A rule pack with an unknown field in a rule is a rule-pack validation failure (`FAIL-RULE-001`), not a silent acceptance.

The rationale for strictness is that the bundle is a security boundary. A tolerant reader that ignores unknown fields would allow a future format to add a field with security-relevant semantics (e.g., a `required: true` field, a `signature` field, a `trust` field) that the older engine would ignore, which could allow a bundle to bypass a check the newer engine would enforce. Strict rejection means an old engine cannot be tricked by a new bundle, and a new engine cannot be tricked by an old bundle that happens to have a field name it recognizes.

`KNOWLEDGE-REQ-058` requires that any change to the bundle format that adds, removes, or renames a manifest field be accompanied by a major-version bump to `format_version`. Any change that adds an optional field to a nested object be accompanied by a minor-version bump. Any change that does not affect the schema (e.g., a documentation update) be accompanied by a patch-version bump.

`KNOWLEDGE-REQ-059` requires that the engine support at least the current and previous format-version major versions (`COMP-REQ-002`). A bundle with format `2.0.0` must be loadable by an engine that supports `1.x` and `2.x`; a bundle with format `3.0.0` is not loadable by an engine that supports only `1.x` and `2.x`.

## Validation, loading, caching, and integrity verification

The loader performs five stages of validation and processing, in this order.

**Stage 1: Manifest read and parse.** The loader reads `manifest.json`, validates it against the manifest schema (a JSON Schema document shipped with the engine), and rejects the bundle if the schema validation fails or if any unknown field is present. `KNOWLEDGE-REQ-060` requires this stage.

**Stage 2: Compatibility check.** The loader checks the bundle's `format_version` against the engine's supported set, the `engine_compatibility` range against the engine's version, and the `kubernetes_versions` list against the engine's supported Kubernetes version set. A bundle that fails any of these checks is rejected with a specific error naming the failure. `KNOWLEDGE-REQ-061` requires this stage.

**Stage 3: File list verification.** The loader iterates over the manifest's `files` list and verifies that every listed file exists, that its size matches the declared `size`, and that its SHA-256 matches the declared `sha256`. A bundle with a missing file, a size mismatch, or a hash mismatch is rejected with a `fatal` error naming the file and the mismatch. `KNOWLEDGE-REQ-062` requires this stage.

**Stage 4: Signature verification.** The loader computes the canonical serialization of the manifest and the file list, verifies the signature against the trust anchor, and rejects the bundle if verification fails. A bundle whose signature is missing or invalid is rejected with a `fatal` error naming the signer and the failure. `KNOWLEDGE-REQ-063` requires this stage.

**Stage 5: Content validation.** The loader loads the index files, the schema files, the rule packs, the relationship definitions, and the catalog, and validates each against its respective schema. A bundle with an invalid schema file, an invalid rule pack, an invalid relationship definition, or an invalid catalog entry is rejected with a specific error naming the affected component, unless the component is declared `required: false` (in which case the component is disabled and a `warning`-severity finding is produced, and the bundle remains loadable). `KNOWLEDGE-REQ-064` requires this stage.

Caching is the loader's mechanism for amortizing the validation cost across invocations. The loader writes a cache entry at `cache/<bundle_id>/<manifest_hash>` after successful validation, containing the validated file list, the index structures, and the computed in-memory representations. On subsequent invocations, the loader reads the cache entry if the manifest hash matches and skips stages 1–4 (the signature was already verified when the cache was written). `KNOWLEDGE-REQ-065` requires that the cache be authenticated (a tampered cache entry must be detected) and that the cache be rebuilt if the manifest hash does not match or if the cache entry's integrity check fails. A cache miss or a cache-integrity failure is not an error; it causes the full validation to re-run.

`KNOWLEDGE-REQ-066` requires that the cache be stored in the bundle store with the same restrictive permissions as the bundle itself (`0700` for directories, `0600` for files), and that the cache be deleted when the corresponding bundle is deleted.

## Backward compatibility and deprecation

The bundle format's backward compatibility policy has three tiers.

The **engine backward compatibility** tier covers the ability of an older engine to load a newer bundle. `KNOWLEDGE-REQ-067` requires that a bundle with format version `1.x` be loadable by any engine that supports format version `1.x`, regardless of the bundle's patch or minor version. A newer engine must load an older bundle from the same major format version, and an older engine must load a newer bundle from the same major format version if the bundle's `engine_compatibility` range includes the older engine's version.

The **bundle backward compatibility** tier covers the ability of a newer engine to load an older bundle. `KNOWLEDGE-REQ-068` requires that the newer engine load the older bundle if the older bundle's format version is in the newer engine's supported set. The engine must not require a newer bundle, and must not refuse to load an older bundle solely because a newer version exists.

The **format deprecation** tier covers the eventual removal of a format version. `KNOWLEDGE-REQ-069` requires that removing support for a format version be accompanied by a major engine version bump and a migration note that specifies the last engine version supporting the removed format. The migration note must be published at least one engine minor release before the removal.

`KNOWLEDGE-REQ-070` requires that the bundle's `format_version` be the only mechanism for format evolution. The engine must not infer a format from the bundle's content, from the manifest's fields, or from any other heuristic. The format version is authoritative.

## Invalid, incomplete, stale, or unsupported bundle handling

The loader's handling of problematic bundles is specified by four categories, each of which has a distinct user-visible message and a distinct exit code.

An **invalid bundle** is a bundle that fails any of the five validation stages. It is rejected with a `fatal` error naming the stage and the failure. The prior bundle, if any, remains active. The user's recovery action is to re-download the bundle from the official source or to activate a known-good bundle. `KNOWLEDGE-REQ-071` requires this behavior.

An **incomplete bundle** is a bundle that is missing a component it declares (a listed file is absent, a rule's fixtures are absent, a schema's provenance reference is missing). It is rejected with a `fatal` error naming the missing component. The prior bundle remains active. The user's recovery action is to re-download. `KNOWLEDGE-REQ-072` requires this behavior.

A **stale bundle** is a bundle whose `generation.timestamp` is older than the engine's `stale_after` threshold (default: 180 days) or whose `kubernetes_versions` list does not include the target version the user requested. A stale bundle is **not** rejected, because the user may legitimately need to analyze manifests against an older Kubernetes version that the current bundle does not cover. Instead, the loader emits a `warning`-severity finding stating that the bundle is stale and naming the reason. `KNOWLEDGE-REQ-073` requires this behavior.

An **unsupported bundle** is a bundle whose `format_version`, `engine_compatibility`, or `kubernetes_versions` does not match the engine's capabilities. It is rejected with a `fatal` error naming the mismatch and the engine's supported range. The prior bundle remains active. The user's recovery action is to install a bundle that the engine supports, or to upgrade the engine. `KNOWLEDGE-REQ-074` requires this behavior.

The distinction between invalid, incomplete, stale, and unsupported is important because the recovery actions differ. An invalid bundle is a distribution problem; an incomplete bundle is a distribution problem; a stale bundle is a knowledge-freshness problem; an unsupported bundle is a version-compatibility problem. Collapsing these into a single "bundle rejected" message would obscure the specific action the user needs to take.

## Illustrative bundle manifest and directory layout

The following is a complete illustrative bundle manifest for a fictional bundle `kubecheck-standard-2026.10.1` covering Kubernetes `1.33`, `1.34`, and `1.35`. The manifest is shown as pretty-printed JSON for readability; the stored manifest is canonicalized and signed as described in the signature section.

```json
{
  "format_version": "1.0.0",
  "bundle_version": 42,
  "bundle_id": "kubecheck-standard-2026.10.1",
  "engine_compatibility": {
    "min_version": "0.1.0",
    "max_version": "0.99.99"
  },
  "kubernetes_versions": [
    {
      "minor": "1.35",
      "patch_pin": "1.35.1",
      "schema_version": "1.0.0",
      "status": "supported"
    },
    {
      "minor": "1.34",
      "patch_pin": "1.34.2",
      "schema_version": "1.0.0",
      "status": "supported"
    },
    {
      "minor": "1.33",
      "patch_pin": "1.33.4",
      "schema_version": "1.0.0",
      "status": "supported"
    },
    {
      "minor": "1.32",
      "patch_pin": "1.32.10",
      "schema_version": "1.0.0",
      "status": "deprecated",
      "supported_until": "2026-12-31"
    }
  ],
  "generation": {
    "timestamp": "2026-10-31T14:00:00Z",
    "generator_version": "0.4.2",
    "source_commit": "8f3a2b9c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a",
    "reproducible": true
  },
  "provenance": [
    {
      "source_id": "k8s-1.35-openapi",
      "source_type": "kubernetes-source",
      "source_path": "tag:v1.35.1",
      "source_version": "v1.35.1",
      "retrieved_at": "2026-10-30T00:00:00Z",
      "content_hash": "sha256:3f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a"
    },
    {
      "source_id": "k8s-1.34-openapi",
      "source_type": "kubernetes-source",
      "source_path": "tag:v1.34.2",
      "source_version": "v1.34.2",
      "retrieved_at": "2026-10-30T00:00:00Z",
      "content_hash": "sha256:1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b"
    },
    {
      "source_id": "k8s-1.33-openapi",
      "source_type": "kubernetes-source",
      "source_path": "tag:v1.33.4",
      "source_version": "v1.33.4",
      "retrieved_at": "2026-10-30T00:00:00Z",
      "content_hash": "sha256:7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d"
    },
    {
      "source_id": "k8s-deprecation-guide",
      "source_type": "kubernetes-docs",
      "source_url": "https://kubernetes.io/docs/reference/using-api/deprecation-guide/",
      "source_version": "2026-10-30",
      "retrieved_at": "2026-10-30T00:00:00Z",
      "content_hash": "sha256:9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
    },
    {
      "source_id": "k8s-feature-gates",
      "source_type": "kubernetes-docs",
      "source_url": "https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/",
      "source_version": "2026-10-30",
      "retrieved_at": "2026-10-30T00:00:00Z",
      "content_hash": "sha256:5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b"
    },
    {
      "source_id": "yaml-1.2.2",
      "source_type": "yaml-spec",
      "source_url": "https://yaml.org/spec/1.2.2/",
      "source_version": "1.2.2",
      "retrieved_at": "2026-10-30T00:00:00Z",
      "content_hash": "sha256:2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c"
    }
  ],
  "files": [
    {
      "path": "schemas/index.json",
      "sha256": "sha256:4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e",
      "size": 48213,
      "kind": "index",
      "compression": "gzip",
      "decompressed_size": 182301
    },
    {
      "path": "schemas/1.34/_core/v1/Pod.json",
      "sha256": "sha256:6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a",
      "size": 28411,
      "kind": "schema",
      "compression": "gzip",
      "decompressed_size": 102847
    },
    {
      "path": "schemas/1.34/apps/v1/Deployment.json",
      "sha256": "sha256:8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c",
      "size": 32140,
      "kind": "schema",
      "compression": "gzip",
      "decompressed_size": 128410
    },
    {
      "path": "rules/index.json",
      "sha256": "sha256:0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e",
      "size": 21873,
      "kind": "index",
      "compression": "gzip",
      "decompressed_size": 94120
    },
    {
      "path": "rules/security/pack.json",
      "sha256": "sha256:2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a",
      "size": 4127,
      "kind": "rule",
      "compression": "gzip",
      "decompressed_size": 18930
    },
    {
      "path": "rules/security/rules/SEC-CTX-001.yaml",
      "sha256": "sha256:4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c",
      "size": 3841,
      "kind": "rule",
      "compression": "none",
      "decompressed_size": 3841
    },
    {
      "path": "rules/security/fixtures/SEC-CTX-001/pos-01.yaml",
      "sha256": "sha256:6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e",
      "size": 512,
      "kind": "rule",
      "compression": "none",
      "decompressed_size": 512
    },
    {
      "path": "relationships/edges.json",
      "sha256": "sha256:8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a",
      "size": 12483,
      "kind": "relationship",
      "compression": "gzip",
      "decompressed_size": 51230
    },
    {
      "path": "catalog/index.json",
      "sha256": "sha256:0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c",
      "size": 6214,
      "kind": "index",
      "compression": "gzip",
      "decompressed_size": 24190
    },
    {
      "path": "catalog/rules.json",
      "sha256": "sha256:3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f",
      "size": 18432,
      "kind": "catalog",
      "compression": "gzip",
      "decompressed_size": 72103
    },
    {
      "path": "LICENSES/Apache-2.0.txt",
      "sha256": "sha256:5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b",
      "size": 11357,
      "kind": "catalog",
      "compression": "none",
      "decompressed_size": 11357
    },
    {
      "path": "LICENSES/CC-BY-4.0.txt",
      "sha256": "sha256:7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d",
      "size": 18421,
      "kind": "catalog",
      "compression": "none",
      "decompressed_size": 18421
    }
  ],
  "partitions": [
    {
      "name": "schemas",
      "description": "Kubernetes OpenAPI schemas for all supported versions",
      "version": "1.0.0",
      "files": ["schemas/index.json", "schemas/1.34/_core/v1/Pod.json", "schemas/1.34/apps/v1/Deployment.json"]
    },
    {
      "name": "security",
      "description": "Security diagnostic rules",
      "version": "1.0.0",
      "files": ["rules/security/pack.json", "rules/security/rules/SEC-CTX-001.yaml", "rules/security/fixtures/SEC-CTX-001/pos-01.yaml"]
    }
  ],
  "limits": {
    "max_total_size": 524288000,
    "max_files": 100000,
    "max_schema_size": 16777216,
    "max_rule_pack_size": 33554432,
    "max_index_entries": 500000,
    "max_decompressed_size": 2147483648
  },
  "licenses": ["Apache-2.0", "CC-BY-4.0"],
  "signature": {
    "algorithm": "ed25519",
    "key_id": "kubecheck-release-2026",
    "value": "base64-encoded-signature-value",
    "signed_manifest_hash": "sha256:9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
  }
}
kubecheck-standard-2026.10.1/
├── manifest.json
├── signature.ed25519
├── schemas/
│   ├── index.json
│   ├── 1.33/
│   │   ├── _core/v1/Pod.json
│   │   └── apps/v1/Deployment.json
│   ├── 1.34/
│   │   ├── _core/v1/Pod.json
│   │   └── apps/v1/Deployment.json
│   └── 1.35/
│       ├── _core/v1/Pod.json
│       └── apps/v1/Deployment.json
├── rules/
│   ├── index.json
│   ├── security/
│   │   ├── pack.json
│   │   ├── rules/
│   │   │   └── SEC-CTX-001.yaml
│   │   └── fixtures/
│   │       └── SEC-CTX-001/
│   │           ├── pos-01.yaml
│   │           ├── pos-02.yaml
│   │           ├── neg-01.yaml
│   │           ├── neg-02.yaml
│   │           └── bnd-01.yaml
│   ├── scheduling/
│   └── networking/
├── relationships/
│   └── edges.json
├── catalog/
│   ├── index.json
│   └── rules.json
├── LICENSES/
│   ├── Apache-2.0.txt
│   └── CC-BY-4.0.txt
└── cache/                 (optional, generated by the loader)
    └── <bundle_id>/
        └── <manifest_hash>/
            ├── validated_files.json
            ├── schema_lookup.json
            └── rule_lookup.json



{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://kubecheck.example.com/schemas/bundle-manifest-1.0.0.json",
  "title": "KubeCheck Bundle Manifest",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "format_version", "bundle_version", "bundle_id",
    "engine_compatibility", "kubernetes_versions", "generation",
    "provenance", "files", "limits", "licenses", "signature"
  ],
  "properties": {
    "format_version": { "type": "string", "pattern": "^\\d+\\.\\d+\\.\\d+$" },
    "bundle_version": { "type": "integer", "minimum": 1 },
    "bundle_id": { "type": "string", "pattern": "^[a-z0-9][a-z0-9.-]*$" },
    "engine_compatibility": {
      "type": "object",
      "additionalProperties": false,
      "required": ["min_version", "max_version"],
      "properties": {
        "min_version": { "type": "string", "pattern": "^\\d+\\.\\d+\\.\\d+$" },
        "max_version": { "type": "string", "pattern": "^\\d+\\.\\d+\\.\\d+$" }
      }
    },
    "kubernetes_versions": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["minor", "patch_pin", "schema_version", "status"],
        "properties": {
          "minor": { "type": "string", "pattern": "^1\\.\\d+$" },
          "patch_pin": { "type": "string", "pattern": "^1\\.\\d+\\.\\d+$" },
          "schema_version": { "type": "string", "pattern": "^\\d+\\.\\d+\\.\\d+$" },
          "status": { "enum": ["supported", "deprecated"] },
          "supported_until": { "type": "string", "format": "date" }
        }
      }
    },
    "generation": {
      "type": "object",
      "additionalProperties": false,
      "required": ["timestamp", "generator_version", "source_commit", "reproducible"],
      "properties": {
        "timestamp": { "type": "string", "format": "date-time" },
        "generator_version": { "type": "string", "pattern": "^\\d+\\.\\d+\\.\\d+" },
        "source_commit": { "type": "string", "pattern": "^[a-f0-9]{40}$" },
        "reproducible": { "type": "boolean" }
      }
    },
    "provenance": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["source_id", "source_type", "source_version", "retrieved_at"],
        "properties": {
          "source_id": { "type": "string" },
          "source_type": { "enum": ["kubernetes-docs", "kubernetes-source", "yaml-spec", "cve", "internal"] },
          "source_url": { "type": "string", "format": "uri" },
          "source_path": { "type": "string" },
          "source_version": { "type": "string" },
          "retrieved_at": { "type": "string", "format": "date-time" },
          "content_hash": { "type": "string", "pattern": "^sha256:[a-f0-9]{64}$" }
        },
        "oneOf": [
          { "required": ["source_url"] },
          { "required": ["source_path"] }
        ]
      }
    },
    "files": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["path", "sha256", "size", "kind", "compression", "decompressed_size"],
        "properties": {
          "path": { "type": "string", "pattern": "^(?!/)(?!.*\\.\\.).+$" },
          "sha256": { "type": "string", "pattern": "^sha256:[a-f0-9]{64}$" },
          "size": { "type": "integer", "minimum": 0 },
          "kind": { "enum": ["schema", "rule", "lifecycle", "feature-gate", "relationship", "catalog", "index"] },
          "compression": { "enum": ["none", "gzip"] },
          "decompressed_size": { "type": "integer", "minimum": 0 }
        }
      }
    },
    "partitions": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["name", "description", "version", "files"],
        "properties": {
          "name": { "type": "string" },
          "description": { "type": "string" },
          "version": { "type": "string", "pattern": "^\\d+\\.\\d+\\.\\d+$" },
          "files": { "type": "array", "items": { "type": "string" } }
        }
      }
    },
    "limits": {
      "type": "object",
      "additionalProperties": false,
      "required": ["max_total_size", "max_files", "max_schema_size", "max_rule_pack_size", "max_index_entries", "max_decompressed_size"],
      "properties": {
        "max_total_size": { "type": "integer", "minimum": 1 },
        "max_files": { "type": "integer", "minimum": 1 },
        "max_schema_size": { "type": "integer", "minimum": 1 },
        "max_rule_pack_size": { "type": "integer", "minimum": 1 },
        "max_index_entries": { "type": "integer", "minimum": 1 },
        "max_decompressed_size": { "type": "integer", "minimum": 1 }
      }
    },
    "licenses": { "type": "array", "items": { "type": "string" } },
    "signature": {
      "type": "object",
      "additionalProperties": false,
      "required": ["algorithm", "key_id", "value", "signed_manifest_hash"],
      "properties": {
        "algorithm": { "enum": ["ed25519"] },
        "key_id": { "type": "string" },
        "value": { "type": "string" },
        "signed_manifest_hash": { "type": "string", "pattern": "^sha256:[a-f0-9]{64}$" }
      }
    }
  }
}