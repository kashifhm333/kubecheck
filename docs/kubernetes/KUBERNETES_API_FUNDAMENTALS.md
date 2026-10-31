# KubeCheck — Kubernetes API Fundamentals

**Document ID:** K8S-API-001  
**File:** `docs/kubernetes/KUBERNETES_API_FUNDAMENTALS.md`  
**Status:** Normative — Approved  
**Version:** 1.0.0  
**Product:** KubeCheck  
**Owner:** Kubernetes API Architecture  
**Last reviewed:** 2026-10-31  

**Related documents:**
- `docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md`
- `docs/yaml/YAML_LANGUAGE_SPEC.md`
- `docs/kubernetes/KUBERNETES_SCHEMA_VALIDATION.md`
- `docs/kubernetes/KUBERNETES_API_VERSION_LIFECYCLE.md`
- `docs/kubernetes/CRD_AND_CUSTOM_RESOURCE_HANDLING.md`
- `docs/architecture/SYSTEM_ARCHITECTURE.md`
- `docs/architecture/REQUIREMENTS_AND_NFRS.md`
- `docs/architecture/OFFLINE_KNOWLEDGE_BUNDLES.md`
- `docs/security/THREAT_MODEL.md`
- `docs/testing/OFFLINE_VALIDATION.md`

---

## 1. Purpose and scope

This document is the foundational reference for understanding and validating **Kubernetes API objects offline**. It defines the concepts KubeCheck must model to analyze manifests without a running cluster: API groups and versions, resources and kinds, object structure, metadata semantics, resource taxonomy, validation and pruning behavior, schema constraints, resource identity, and the boundary between facts derivable from local schemas and facts that require cluster state.

**In scope**

- API groups, versions, resources, kinds, and discovery concepts.
- `apiVersion`, `kind`, `metadata`, `spec`, and `status`.
- Namespaces, names, labels, selectors, annotations, owner references, and finalizers.
- Built-in resources, CustomResourceDefinitions (CRDs), aggregated APIs, and custom resources.
- Namespaced versus cluster-scoped resources.
- API-server validation, schema validation, admission, defaulting, and pruning.
- Field types, required fields, enumerations, unions, lists, maps, and schema constraints.
- Unknown fields and version-specific behavior.
- Resource identity and duplicate object detection.
- Object references, lifecycle relationships, and API compatibility.
- Offline limitations when cluster discovery is unavailable.
- Which facts are derivable from local schemas versus cluster state.
- The metadata needed to select the correct schema.

**Out of scope**

- Concrete schema bundle format (see `docs/architecture/OFFLINE_KNOWLEDGE_BUNDLES.md`).
- Rule-specific diagnostic semantics (see `docs/diagnostics/DIAGNOSTIC_TAXONOMY.md`).
- API-version lifecycle rules in detail (see `docs/kubernetes/KUBERNETES_API_VERSION_LIFECYCLE.md`).
- CRD-specific handling (see `docs/kubernetes/CRD_AND_CUSTOM_RESOURCE_HANDLING.md`).

**Governing principle:**

> **K8S-API-REQ-001 — KubeCheck MUST NOT assume that every API is built into Kubernetes.** It MUST treat built-in resources, CRDs, and aggregated APIs as distinct categories with different schema sources, validation guarantees, and offline-derivable facts.

---

## 2. Terminology and conventions

| Term | Definition |
|---|---|
| **API group** | A set of resources exposed together, identified by a group name (e.g., `apps`, `batch`, `rbac.authorization.k8s.io`). The core group has an empty group name and is accessed under `/api`. |
| **API version** | A version of a group's API (e.g., `v1`, `v1beta1`). The `apiVersion` field encodes `GROUP/VERSION` for named groups and `VERSION` for the core group. |
| **Resource** | A representation of a system entity, exposed via a collection and elements, addressable via a URL. Resources are plural (e.g., `pods`, `deployments`). |
| **Kind** | The name of a particular object schema (e.g., `Pod`, `Deployment`). A kind may be accepted by multiple resources. |
| **GVK** | Group, Version, Kind — the tuple that identifies an object's schema. |
| **GVR** | Group, Version, Resource — the tuple that identifies a REST endpoint. |
| **Object** | An instance of a resource, serialized as JSON or YAML, with `apiVersion`, `kind`, `metadata`, and typically `spec` and `status`. |
| **Built-in resource** | A resource served by the core Kubernetes API server with a schema shipped as part of Kubernetes. |
| **CustomResourceDefinition (CRD)** | A Kubernetes object that declaratively registers a new custom resource type with an API group, kind, and schema, served by the core API server. |
| **Aggregated API** | A custom API served by an extension API server registered through an `APIService` object; the core API server proxies requests to it. |
| **Custom resource** | An instance of a CRD-registered type. |
| **Structural schema** | A fully typed OpenAPI v3 schema with no untyped sections, required for pruning, defaulting, and conversion. |
| **Pruning** | The API server's removal of fields not defined in the structural schema before persistence. |
| **Defaulting** | The API server's population of omitted fields with schema-declared defaults. |
| **Admission** | The phase after validation and before persistence where mutating and validating admission controllers (including webhooks) may modify or reject the object. |

**Requirement IDs** use `K8S-API-REQ-NNN`. Facts labeled **DERIVABLE (local)** can be established from a local schema bundle and the manifest alone; facts labeled **CLUSTER-DEPENDENT** require a running cluster and are outside KubeCheck's offline scope.

---

## 3. API groups, versions, resources, kinds, and discovery

### 3.1 The group/version/resource model

Kubernetes organizes its API into **groups**, each with one or more **versions**, each exposing one or more **resources**. The API conventions define an API group as "a set of resources that are exposed together" and state that the group and version are exposed in the `apiVersion` field as `GROUP/VERSION`, for example `policy.k8s.io/v1`. A resource is "a representation of a system entity, sent or retrieved as JSON via HTTP to the server"; resources are exposed via collections and elements. A kind is "the name of a particular object schema".

The core group is special: it has an **empty group name**, is served under `/api`, and its `apiVersion` is written as just the version (e.g., `v1`). Named groups are served under `/apis/<group>/<version>`. Examples of built-in groups include:

| Group | `apiVersion` example | Representative resources |
|---|---|---|
| Core (`""`) | `v1` | Pod, Service, ConfigMap, Secret, Namespace, ServiceAccount |
| `apps` | `apps/v1` | Deployment, StatefulSet, DaemonSet, ReplicaSet |
| `batch` | `batch/v1` | Job, CronJob |
| `rbac.authorization.k8s.io` | `rbac.authorization.k8s.io/v1` | Role, ClusterRole, RoleBinding, ClusterRoleBinding |
| `storage.k8s.io` | `storage.k8s.io/v1` | StorageClass, VolumeAttachment |
| `networking.k8s.io` | `networking.k8s.io/v1` | Ingress, NetworkPolicy |

### 3.2 Discovery

In a live cluster, the API server publishes its available groups and versions through **Discovery API** endpoints. The root paths `/api` and `/apis` advertise the supported group-versions, and each group-version advertises its resource list at `/apis/<group>/<version>` (or `/api/<version>` for the core group). The `APIResourceList` includes, for each resource, whether it is **namespaced**, its kind, short names, and supported verbs.

**KubeCheck consequence:** Offline, KubeCheck **cannot query discovery**. It must rely on a **local schema bundle** that enumerates the supported GVKs and their resource metadata (namespaced vs cluster-scoped, plural names, short names, supported verbs where relevant). The bundle is the offline equivalent of discovery for the GVKs it covers, and the **absence of a GVK from the bundle is not evidence that the GVK does not exist** — it is evidence that KubeCheck cannot validate it offline (`K8S-API-REQ-010`).

### 3.3 Built-in resources versus extension mechanisms

Kubernetes provides **two** mechanisms for adding custom APIs: **CustomResourceDefinitions (CRDs)** and the **aggregation layer**.

| Mechanism | Who serves it | Schema source | Offline derivability |
|---|---|---|---|
| **Built-in resources** | Core `kube-apiserver` | Kubernetes OpenAPI schemas shipped with the release | **DERIVABLE (local):** if the schema bundle contains the GVK. |
| **CRDs** | Core `kube-apiserver` (via `apiextensions-apiserver`) | The CRD's `spec.versions[*].schema.openAPIV3Schema` | **DERIVABLE (local):** if the CRD definition is supplied locally. |
| **Aggregated APIs** | Extension API server registered via `APIService` | Defined by the extension API server; **not** known to the core API server's schema store | **CLUSTER-DEPENDENT:** cannot be validated offline without a supplied extension schema. |

A CRD "allows you to declaratively define a new custom API with an API group, kind, and schema that you specify," and the control plane "serves and handles the storage of your custom resource". The aggregation layer "sits behind the primary API server, which acts as a proxy," and the main API server "delegates requests to your API server for the custom APIs that you specify". To register an aggregated API, you add an `APIService` object that "claims" the URL path, and the aggregation layer proxies requests to the registered service.

**K8S-API-REQ-002:** KubeCheck MUST classify every analyzed GVK as one of `built-in`, `crd`, or `aggregated-unknown`, and MUST adjust its validation claims accordingly. It MUST NOT claim schema validation for an aggregated API whose schema is not present locally.

---

## 4. Object structure: apiVersion, kind, metadata, spec, status

### 4.1 The four top-level fields

Every Kubernetes object accepted or returned by the API server has a schema "identified by the `kind` and `apiVersion` fields". The API conventions define the standard top-level structure:

| Field | Type | Purpose | Required |
|---|---|---|---|
| `apiVersion` | `string` | Identifies the group and version of the object's schema. | Yes |
| `kind` | `string` | Identifies the object's schema (kind). | Yes |
| `metadata` | `ObjectMeta` | Object identity and metadata. | Yes for most resources |
| `spec` | resource-specific | Desired state (end-user intent). | Varies by resource |
| `status` | resource-specific | Observed state (system-reported). | Populated by the system |

**K8S-API-REQ-003:** KubeCheck MUST require `apiVersion` and `kind` as string scalars at the object root. A manifest missing either, or with a non-string value, MUST produce a GVK error (`NORM-REQ-030`).

### 4.2 `apiVersion` semantics

The `apiVersion` field encodes `GROUP/VERSION` for named groups and `VERSION` for the core group. KubeCheck MUST parse it into `(group, version)` and MUST use the resulting GVK to select the schema. It MUST NOT infer the group from the `kind` alone, because the same kind can exist in multiple groups (e.g., `Deployment` in `apps` and historically in `extensions`).

**K8S-API-REQ-004:** KubeCheck MUST reject an `apiVersion` that does not match the pattern `(<group>/)?<version>` where `group` is a DNS-subdomain-like string and `version` matches `v[0-9]+((alpha|beta)[0-9]+)?`. It MUST NOT attempt to normalize or guess the group.

### 4.3 `kind` semantics

The `kind` is "a string value representing the REST resource this object represents" and "cannot be updated". It is in **CamelCase** by convention. KubeCheck MUST compare kinds **case-sensitively** against the schema bundle's kind index.

### 4.4 `metadata`

`metadata` carries object identity and metadata. The relevant subfields for offline analysis are:

| Field | Type | Purpose |
|---|---|---|
| `name` | `string` | Resource name within its scope. |
| `generateName` | `string` | Prefix for a server-generated name; mutually exclusive with `name` on create. |
| `namespace` | `string` | Namespace for namespaced resources; must be absent or empty for cluster-scoped resources. |
| `labels` | `map[string]string` | Identifying metadata used by selectors. |
| `annotations` | `map[string]string` | Non-identifying metadata. |
| `uid` | `string` | Server-assigned unique identifier; **not** settable by clients. |
| `ownerReferences` | `[]OwnerReference` | References to owner objects for garbage collection. |
| `finalizers` | `[]string` | Cleanup obligations that delay deletion. |
| `resourceVersion` | `string` | Server-assigned concurrency token; **not** settable by clients. |
| `creationTimestamp` | `string` | Server-assigned creation time; **not** settable by clients. |

**K8S-API-REQ-005:** KubeCheck MUST treat `uid`, `resourceVersion`, `creationTimestamp`, and `generation` as **server-assigned** fields. Their presence in a manifest is a `warning`-severity finding because they are ignored or rejected on create, and their values cannot be validated offline.

### 4.5 `spec` and `status`

The API conventions state that `/pods` "only allows updates to `metadata` and `spec`, since those reflect end-user intent". `status` is the system-reported observed state and is not settable by ordinary clients on create. KubeCheck MUST:

- Validate `spec` against the resource schema.
- Validate `status` structurally if it is present in the manifest, but MUST emit an informational finding stating that `status` is system-managed and its presence in a manifest is unusual.
- NOT require `status` to be present.

**K8S-API-REQ-006:** KubeCheck MUST NOT treat a missing `status` as an error. It MUST treat a present `status` as an informational finding explaining that the API server manages this field.

---

## 5. Metadata semantics

### 5.1 Names and namespaces

A resource's **name** is unique within its scope (namespace for namespaced resources, cluster for cluster-scoped resources). Names are DNS-subdomain-conforming strings for most resources. `generateName` is a prefix used by the server to generate a unique name; it is mutually exclusive with `name` on create.

**Namespaces** are themselves cluster-scoped resources. A namespaced resource's `metadata.namespace` identifies its namespace. If `metadata.namespace` is omitted on create, the API server uses the namespace of the request context (typically `default` for `kubectl`), but this defaulting is **cluster-dependent** and MUST NOT be applied offline.

**K8S-API-REQ-007:** KubeCheck MUST NOT invent a namespace for a namespaced resource when `metadata.namespace` is absent. It MUST mark the resource identity as `namespace-unresolved` and MUST NOT compare it against resources in a specific namespace. This is a `warning`-severity finding.

### 5.2 Labels and selectors

**Labels** are key/value pairs attached to objects for identification and selection. Label keys must be unique per object, and the name segment must be 63 characters or less. **Selectors** express a set of objects via label matching, using either **equality-based** (`environment = production`) or **set-based** (`environment in (production, qa)`) forms. Selectors are used by Services, ReplicaSets, Deployments, NetworkPolicies, PodDisruptionBudgets, and other controllers.

**K8S-API-REQ-008:** KubeCheck MUST validate label keys and values against the documented syntax (DNS prefix, 63-character name segment, allowed characters) as a **schema** finding when they violate the constraint. It MUST NOT validate selector **matching** against cluster state, because the set of candidate objects is not known offline beyond the supplied input set.

### 5.3 Annotations

**Annotations** are key/value pairs for **non-identifying** metadata. They are not used for selection. Annotation values may be arbitrarily large (subject to the API server's request size limit). KubeCheck MUST validate annotation keys against the same syntax as label keys, but MUST NOT impose a value-length limit beyond the parser's scalar limit.

### 5.4 Owner references

`metadata.ownerReferences` references an owner object by **name and UID within the same namespace** as the dependent object. Kubernetes sets this field automatically for dependents of ReplicaSets, DaemonSets, Deployments, Jobs, CronJobs, and ReplicationControllers. The `blockOwnerDeletion` boolean controls whether a dependent blocks garbage collection of its owner.

**K8S-API-REQ-009:** KubeCheck MUST validate that each `ownerReference` has `apiVersion`, `kind`, `name`, and `uid` fields of the correct types. It MUST NOT verify that the referenced owner **exists**, because existence is cluster state. It MAY emit an informational finding when an `ownerReference` is present in a manifest created outside a controller, noting that the controller normally sets this field.

### 5.5 Finalizers

`metadata.finalizers` is a list of strings representing cleanup obligations that delay deletion after `metadata.deletionTimestamp` is set. KubeCheck MUST validate that each finalizer is a string matching the qualified-name pattern. It MUST NOT assume that a particular finalizer is present or absent based on cluster behavior.

---

## 6. Resource taxonomy

### 6.1 Built-in resources

Built-in resources are served by the core `kube-apiserver` and have schemas shipped as part of the Kubernetes release. Examples include Pod, Service, ConfigMap, Secret, Deployment, StatefulSet, DaemonSet, Job, CronJob, Namespace, ServiceAccount, Role, ClusterRole, StorageClass, and PersistentVolume. The exact set of built-in resources is **version-dependent**; KubeCheck's schema bundle for a target Kubernetes version defines the set it can validate.

### 6.2 CustomResourceDefinitions

A CRD "declaratively define[s] a new custom API with an API group, kind, and schema that you specify". The CRD's `spec` includes:

| Field | Purpose |
|---|---|
| `group` | The API group for the custom resource. |
| `names` | Kind, plural, singular, shortNames, categories. |
| `scope` | `Namespaced` or `Cluster`. |
| `versions` | One or more versions, each with `name`, `served`, `storage`, `schema`, `subresources`, `additionalPrinterColumns`. |
| `conversion` | Conversion strategy between versions. |

The CRD itself is cluster-scoped and its name must be in the format `<plural>.<group>`.

**K8S-API-REQ-010:** KubeCheck MUST treat a GVK as a CRD only when a CRD definition supplying that GVK is present in the local CRD input or the bundle's CRD catalog. It MUST NOT infer CRD status from the group name alone, because a group name can be used by an aggregated API or by a built-in API in a different cluster.

### 6.3 Aggregated APIs

Aggregated APIs are registered via `APIService` objects. The `APIService` "represents a server for a particular GroupVersion" and its name must be `version.group`. The aggregation layer proxies requests to the extension API server. The extension API server's schema is **not** part of the core API server's OpenAPI document; it is served by the extension server itself.

**K8S-API-REQ-011:** KubeCheck MUST NOT claim schema validation for an aggregated API unless a schema for its GVK is supplied locally (for example, an OpenAPI document exported from the extension API server and included in the bundle or as a user input). Without such a schema, KubeCheck MUST emit a `schema-unchecked` finding stating that the GVK is an aggregated API whose schema is not available offline.

### 6.4 Custom resources

A custom resource is an instance of a CRD-registered type. It has the same top-level structure (`apiVersion`, `kind`, `metadata`, `spec`, `status`) but its `spec` and `status` schemas are defined by the CRD's OpenAPI v3 schema. CRD schema validation and pruning behavior are covered in `docs/kubernetes/CRD_AND_CUSTOM_RESOURCE_HANDLING.md`.

---

## 7. Namespaced versus cluster-scoped resources

Every resource type is either **namespaced** (its objects live in a namespace) or **cluster-scoped** (its objects are global). Most resources — Pods, ReplicaSets, Jobs, Deployments, Services, Secrets, ConfigMaps — are namespaced; Nodes, Namespaces, PersistentVolumes, StorageClasses, ClusterRoles, and ClusterRoleBindings are cluster-scoped. CRDs declare their scope via `spec.scope` with values `Namespaced` or `Cluster`.

**K8S-API-REQ-012:** KubeCheck MUST determine scope from the schema bundle's resource metadata (for built-ins) or the CRD's `spec.scope` (for CRDs). It MUST validate that:

- A cluster-scoped resource does **not** set `metadata.namespace`.
- A namespaced resource's `metadata.namespace`, if present, is a valid DNS label.

A namespace on a cluster-scoped resource is an `error`-severity schema finding. A missing namespace on a namespaced resource is a `warning`-severity finding (the API server may default it, but KubeCheck cannot assume the default offline).

**K8S-API-REQ-013:** KubeCheck MUST NOT treat a missing namespace as making the resource cluster-scoped. Scope is a property of the **resource type**, not of the individual object.

---

## 8. API server processing: validation, admission, defaulting, pruning

### 8.1 The processing order

For a create or update request, the API server processes the object in this order:

1. **Decoding and pruning.** The request body is decoded; unknown fields are pruned (for structural-schema CRDs) or handled per `fieldValidation` (for built-ins).
2. **Schema validation.** The object is validated against its OpenAPI schema (required fields, types, enums, formats, ranges).
3. **Mutating admission.** Mutating admission webhooks run and may modify the object; defaulting webhooks populate unset fields.
4. **Validating admission.** Validating admission webhooks run and may reject the object.
5. **Persistence.** The object is persisted to etcd.

### 8.2 Defaulting

**Defaulting** is the population of omitted fields with schema-declared defaults. For CRDs, defaults are declared in the structural schema and applied by the API server. For built-ins, defaults are applied by the API server's defaulting logic and may also be applied by mutating webhooks.

**K8S-API-REQ-014:** KubeCheck MUST NOT apply schema defaults to the NIR (`NORM-REQ-041`). It MAY report that a field is absent and that the schema declares a default, as an `info`-severity finding, but it MUST NOT mutate the object or assume the defaulted value for validation of other fields. This is because:
- The effective default may depend on the API server version and feature gates.
- Mutating webhooks may override or add defaults.
- Assuming a default can mask a genuine defect (e.g., an omitted field that the user believed was required).

### 8.3 Pruning

**Pruning** is the removal of fields not defined in the structural schema before persistence. For CRDs with structural schemas, pruning is the default behavior: `spec.preserveUnknownFields` defaults to `false`, and unknown fields are pruned before persistence. The `x-kubernetes-preserve-unknown-fields: true` extension opts specific subtrees out of pruning.

For built-in resources, the API server's `fieldValidation` parameter controls unknown-field handling:

| `fieldValidation` value | Behavior |
|---|---|
| `Ignore` | Silently drop unknown fields; keep only the last duplicate field. |
| `Warn` | Emit a warning for each dropped unknown field; request succeeds if no other errors. |
| `Strict` | Fail the request if any unknown fields or duplicate fields are present. |

The `fieldValidation` parameter is supported when the `ServerSideFieldValidation` feature gate is enabled.

**K8S-API-REQ-015:** KubeCheck MUST treat unknown fields as **schema findings** rather than silently ignoring them. For built-ins, it SHOULD emit a `warning`-severity finding for each unknown field, aligned with `fieldValidation: Warn`. For CRDs, it MUST report unknown fields and note that the API server would prune them unless `x-kubernetes-preserve-unknown-fields` is set on the enclosing subtree.

### 8.4 Admission

Admission controllers and webhooks are **cluster-dependent**: their behavior depends on what is installed in the target cluster. KubeCheck MUST NOT simulate admission. It MAY emit an informational finding when an object relies on a mutating webhook for required defaulting, but it cannot verify that the webhook is installed or will behave as expected.

**K8S-API-REQ-016:** KubeCheck MUST NOT claim that an object will be admitted. It MUST state in every report header that local validation establishes schema conformance and implemented rule checks, not admission-controller acceptance or controller behavior (`PRD-012`).

---

## 9. Field types, required fields, enums, unions, lists, maps, schema constraints

### 9.1 OpenAPI v3 schema

Kubernetes schemas are expressed as **OpenAPI v3** structural schemas for CRDs, and as OpenAPI-derived schemas for built-ins. A schema describes a tree of `type`, `properties`, and `required` fields. Each field supports constraints depending on its type:

| Type | Common constraints |
|---|---|
| `string` | `minLength`, `maxLength`, `pattern`, `enum`, `format` |
| `integer` / `number` | `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf` |
| `array` | `minItems`, `maxItems`, `uniqueItems`, `items` |
| `object` | `properties`, `required`, `additionalProperties`, `minProperties`, `maxProperties` |
| `boolean` | (no numeric constraints) |

The `required` list "enumerates which child properties must be present. If a required field is missing, writes will be rejected". Enums define closed sets of allowed values.

**K8S-API-REQ-017:** KubeCheck MUST validate every schema constraint it can evaluate deterministically offline: type, required, enum, format (where the format has a well-defined regular expression or semantic), `minLength`/`maxLength`/`pattern`, `minimum`/`maximum`/`exclusiveMinimum`/`exclusiveMaximum`, `minItems`/`maxItems`/`uniqueItems`, and `additionalProperties` (where the schema declares it).

### 9.2 Unions

OpenAPI v3 supports `oneOf`, `anyOf`, `allOf`, and `not`. However, Kubernetes' **structural schema** subset for CRDs **removes** `oneOf`, `anyOf`, `allOf`, and `not`, because these constructs are not supported by the OpenAPI v2 conversion that Kubernetes uses for some tooling. CRD authors are advised to replace unions with `x-kubernetes-preserve-unknown-fields` or explicit fields.

**K8S-API-REQ-018:** KubeCheck MUST support `oneOf`, `anyOf`, `allOf`, and `not` when they appear in a built-in resource schema that uses them. For CRD schemas, it MUST recognize that these keywords are not part of the structural subset and MUST emit a `warning`-severity finding if a CRD schema uses them, because the API server may strip or reject them.

### 9.3 Lists versus maps

The API conventions recommend "lists of named subobjects preferred over maps". A list of named subobjects (e.g., `containers: [{name: app, image: nginx}]`) has stable key-based merge semantics and is easier to validate than a map keyed by an arbitrary string. KubeCheck MUST validate both forms against the schema; it MUST NOT prefer one form over the other when the schema allows both.

### 9.4 Field optionality

The API conventions distinguish `optional` and `required` fields and define defaulting for optional fields. In OpenAPI terms, a field is required if it appears in the `required` array of its parent object schema; otherwise it is optional. KubeCheck MUST treat a missing required field as an `error`-severity schema finding and a present-but-`null` field as an `error`-severity finding distinct from absent (`NORM-REQ-042`).

---

## 10. Unknown fields and version-specific behavior

### 10.1 Unknown fields

An **unknown field** is a field present in the object but not defined in the schema for its GVK and version. Unknown fields are handled differently across resource categories:

| Category | Unknown-field behavior | KubeCheck finding |
|---|---|---|
| Built-in, `fieldValidation: Strict` | Request rejected | `error` |
| Built-in, `fieldValidation: Warn` | Warning emitted; field dropped | `warning` |
| Built-in, `fieldValidation: Ignore` | Silently dropped | `warning` (KubeCheck always reports) |
| CRD with structural schema, `preserveUnknownFields: false` | Pruned before persistence | `warning` |
| CRD with `x-kubernetes-preserve-unknown-fields: true` on the subtree | Preserved | `info`: preserved by explicit opt-out |

**K8S-API-REQ-019:** KubeCheck MUST report every unknown field it detects. The severity MUST depend on the resource category and the schema's pruning configuration, but the finding MUST NOT be suppressed. A user who relies on an unknown field being preserved is at risk of silent data loss.

### 10.2 Version-specific behavior

A GVK's schema can differ across versions. For example, a field present in `v1beta1` may be removed in `v1`, or a field may change type. KubeCheck MUST select the schema for the **specific version** declared in `apiVersion`. It MUST NOT fall back to another version's schema, because doing so would produce incorrect validation results.

**K8S-API-REQ-020:** When the declared version is not present in the bundle for its group and kind, KubeCheck MUST emit a `schema-unchecked` finding with severity `warning`, stating that the specific version is not covered. It MUST NOT substitute a different version.

---

## 11. Resource identity and duplicate object detection

### 11.1 Resource identity

The **resource identity** used for duplicate detection and cross-document references is the tuple:

```
(group, version, kind, namespace, name)
```

This is derived from the **resolved** NIR values (`NORM-REQ-048`, `NORM-REQ-049`). Two objects with the same identity in the same input set are duplicates.

**K8S-API-REQ-021:** KubeCheck MUST detect duplicate resource identities within a single invocation and MUST emit a `maintainability` finding citing both anchors. It MUST NOT drop either object from analysis; both remain in the inventory and may be independently validated (`SARCH-REQ-031`).

### 11.2 Identity edge cases

| Case | Identity handling |
|---|---|
| `metadata.name` absent, `metadata.generateName` present | Identity uses `generateName` with a `generated=true` marker; two objects with the same `generateName` are **not** duplicates because the server generates distinct names. |
| `metadata.name` absent, `generateName` absent | `MISSING_NAME` GVK error; object excluded from inventory (`NORM-REQ-050`). |
| Cluster-scoped resource with `metadata.namespace` set | Identity includes the namespace for reporting, but the schema finding flags the invalid namespace. |
| Same GVK, different namespaces | Not duplicates; distinct identities. |
| Same name, different GVKs | Not duplicates; distinct identities. |

**K8S-API-REQ-022:** Duplicate detection MUST be **scope-aware**: a namespaced object in namespace `a` and a namespaced object in namespace `b` are not duplicates, even if their names match. A cluster-scoped object and a namespaced object with the same name are not duplicates.

---

## 12. Object references, lifecycle relationships, and API compatibility

### 12.1 References

Objects reference each other through several mechanisms:

| Mechanism | Example | Offline derivability |
|---|---|---|
| **Label selector** | Service `spec.selector` → Pods | **PARTIAL:** can verify the selector syntax and, if the candidate objects are in the input set, whether any match; cannot verify against cluster state. |
| **`ownerReferences`** | ReplicaSet owns Pods | **PARTIAL:** can validate the reference structure; cannot verify the owner exists. |
| **`configMapRef` / `secretRef`** | Pod env `valueFrom` | **PARTIAL:** can validate the reference structure and, if the referenced object is in the input set, whether it exists; cannot verify cluster state. |
| **`serviceAccountName`** | Pod → ServiceAccount | **PARTIAL:** can validate the field type; cannot verify the ServiceAccount exists. |
| **`roleRef`** | RoleBinding → Role/ClusterRole | **PARTIAL:** can validate `roleRef` structure and `apiGroup`; cannot verify the role exists. |
| **`persistentVolumeClaim`** | Pod → PVC | **PARTIAL:** can validate the claim name; cannot verify the PVC exists or is bound. |
| **`ingressClassName`** | Ingress → IngressClass | **PARTIAL:** can validate the string; cannot verify the class exists. |

**K8S-API-REQ-023:** KubeCheck MUST distinguish between **structural reference validation** (which it can do offline) and **referential existence** (which it cannot). A reference that is structurally valid but whose target is not in the input set MUST produce an `info`-severity finding stating that existence cannot be verified offline, not an `error`-severity finding claiming the target is missing.

### 12.2 Lifecycle relationships

Kubernetes controllers establish lifecycle relationships: a Deployment owns ReplicaSets, a ReplicaSet owns Pods, a StatefulSet owns Pods, a Job owns Pods, and a CronJob owns Jobs. These relationships are expressed through `ownerReferences` and label selectors. KubeCheck can validate the **declarative intent** of these relationships in a manifest but cannot verify that the controllers are running or that the relationships have been established.

**K8S-API-REQ-024:** KubeCheck MUST NOT report a lifecycle relationship as "healthy" or "established." It may report that the manifest declares the relationship and that the declaration is structurally valid.

### 12.3 API compatibility

A GVK's schema evolves across Kubernetes releases. Fields may be added, deprecated, or removed; types may change; defaults may change. KubeCheck's schema bundle is **versioned per Kubernetes release**, and each GVK's schema is selected for the declared version (`K8S-API-REQ-020`). API-version lifecycle (deprecated, removed) is a separate concern covered in `docs/kubernetes/KUBERNETES_API_VERSION_LIFECYCLE.md`.

**K8S-API-REQ-025:** KubeCheck MUST NOT assume that a schema valid in one Kubernetes version is valid in another. Cross-version validation claims MUST be scoped to the version whose schema was used.

---

## 13. Offline limitations

### 13.1 What KubeCheck cannot do offline

| Limitation | Reason | Mitigation |
|---|---|---|
| **Discover API groups, versions, and resources** | Discovery requires a live API server (`/api`, `/apis`). | Use a local schema bundle that enumerates the supported GVKs. |
| **Read CRDs installed in the target cluster** | CRD definitions are cluster state. | Accept CRD definitions as local input; validate against them. |
| **Read aggregated API schemas** | Extension API server schemas are served by the extension server. | Accept an OpenAPI document exported from the extension API server. |
| **Simulate admission** | Admission controllers and webhooks are cluster state. | Do not claim admission acceptance; report structural conformance only. |
| **Apply server defaults** | Defaults depend on the API server version and feature gates. | Do not apply defaults; report absent fields with schema-declared defaults as `info`. |
| **Verify reference existence** | The target object may exist in the cluster but not in the input set. | Validate reference structure; report existence as `info` when unverifiable. |
| **Verify RBAC authorization** | Authorization depends on cluster roles, bindings, and the requesting identity. | Validate `roleRef` structure; do not claim authorization outcome. |
| **Verify storage class, ingress class, or device plugin availability** | These are cluster-scoped resources whose existence depends on the cluster. | Validate the reference field type; report existence as `info` when unverifiable. |

**K8S-API-REQ-026:** KubeCheck MUST state its offline limitations in every report header and MUST NOT claim any capability in §13.1 as an analysis outcome.

### 13.2 What KubeCheck can do offline

| Capability | Scope |
|---|---|
| **YAML parsing and normalization** | Fully offline (`docs/yaml/YAML_VALIDATION_AND_NORMALIZATION.md`). |
| **GVK extraction and root validation** | Fully offline. |
| **Schema validation for built-in GVKs present in the bundle** | Fully offline for the declared version. |
| **Schema validation for CRDs supplied locally** | Fully offline against the supplied CRD schema. |
| **Structural reference validation** | Fully offline. |
| **Duplicate identity detection** | Fully offline. |
| **API-version lifecycle checks** | Fully offline if lifecycle metadata is in the bundle. |
| **Deterministic security checks** | Fully offline (`docs/diagnostics/`). |
| **Cross-document consistency within the input set** | Fully offline for the objects in the set. |

---

## 14. Resource examples and derivability

### 14.1 Built-in resource: Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    app: web
spec:
  replicas: 3
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: app
        image: nginx:1.27
        ports:
        - containerPort: 80
```

| Fact | Derivability |
|---|---|
| `apiVersion` is well-formed and resolves to group `apps`, version `v1` | **DERIVABLE (local)** |
| `kind` is a string and matches the schema's kind | **DERIVABLE (local)** |
| `spec.replicas` is an integer | **DERIVABLE (local)** |
| `spec.selector.matchLabels` is a valid label selector | **DERIVABLE (local)** |
| `spec.template.spec.containers[0].image` is a string | **DERIVABLE (local)** |
| The selector matches the template labels | **DERIVABLE (local)** |
| The Deployment will be admitted | **CLUSTER-DEPENDENT** |
| The Deployment controller will create ReplicaSets | **CLUSTER-DEPENDENT** |
| The image `nginx:1.27` exists in a registry | **CLUSTER-DEPENDENT** (network/registry state) |
| The Pods will schedule | **CLUSTER-DEPENDENT** |

### 14.2 CRD instance

```yaml
apiVersion: example.com/v1
kind: Widget
metadata:
  name: sample
spec:
  size: 3
  color: blue
```

| Fact | Derivability |
|---|---|
| The GVK is `example.com/v1, Kind=Widget` | **DERIVABLE (local)** |
| The schema requires `size` (integer) and `color` (string, enum `[red, green, blue]`) | **DERIVABLE (local)** if the CRD definition is supplied |
| `size: 3` conforms to the schema | **DERIVABLE (local)** if the CRD definition is supplied |
| `color: blue` conforms to the enum | **DERIVABLE (local)** if the CRD definition is supplied |
| The CRD's controller will reconcile the object | **CLUSTER-DEPENDENT** |
| The object will be admitted | **CLUSTER-DEPENDENT** |
| The CRD is installed in the target cluster | **CLUSTER-DEPENDENT** |

### 14.3 Aggregated API instance

```yaml
apiVersion: metrics.k8s.io/v1beta1
kind: PodMetrics
metadata:
  name: web-abc
  namespace: default
```

| Fact | Derivability |
|---|---|
| The GVK is `metrics.k8s.io/v1beta1, Kind=PodMetrics` | **DERIVABLE (local)** |
| The object has the required top-level fields | **DERIVABLE (local)** |
| The `spec` and `status` schema conform to `PodMetrics` | **CLUSTER-DEPENDENT** unless an exported schema is supplied locally |
| The extension API server is running | **CLUSTER-DEPENDENT** |
| The metrics are current | **CLUSTER-DEPENDENT** |

**K8S-API-REQ-027:** For every finding, KubeCheck MUST label its derivability as `local` or `cluster-dependent`. A `cluster-dependent` finding MUST be `info`-severity and MUST state that the fact cannot be verified offline.

---

## 15. Schema selection metadata

To select the correct schema offline, KubeCheck needs the following metadata, which the local schema bundle MUST provide for every covered GVK:

| Metadata field | Purpose | Source (built-in) | Source (CRD) |
|---|---|---|---|
| `group` | API group | Kubernetes OpenAPI schema | `spec.group` |
| `version` | API version | Kubernetes OpenAPI schema | `spec.versions[*].name` |
| `kind` | Object kind | Kubernetes OpenAPI schema | `spec.names.kind` |
| `plural` | Resource plural name | Kubernetes OpenAPI schema | `spec.names.plural` |
| `singular` | Resource singular name | Kubernetes OpenAPI schema | `spec.names.singular` |
| `shortNames` | CLI short names | Kubernetes OpenAPI schema | `spec.names.shortNames` |
| `scope` | `Namespaced` or `Cluster` | Kubernetes OpenAPI schema | `spec.scope` |
| `served` | Whether the version is served | Kubernetes OpenAPI schema | `spec.versions[*].served` |
| `storage` | Whether the version is the storage version | Kubernetes OpenAPI schema | `spec.versions[*].storage` |
| `schema` | OpenAPI v3 schema | Kubernetes OpenAPI schema | `spec.versions[*].schema.openAPIV3Schema` |
| `subresources` | `status` and `scale` subresources | Kubernetes OpenAPI schema | `spec.versions[*].subresources` |
| `additionalPrinterColumns` | Printer columns | Kubernetes OpenAPI schema | `spec.versions[*].additionalPrinterColumns` |
| `conversion` | Conversion strategy | (not applicable to built-ins) | `spec.conversion` |

**K8S-API-REQ-028:** The schema bundle MUST include, for every covered GVK, the metadata in §15. When a required metadata field is missing for a GVK, KubeCheck MUST emit a `schema-unchecked` finding for that GVK and MUST NOT attempt to guess the missing value.

**K8S-API-REQ-029:** The schema bundle MUST distinguish built-in schemas from CRD schemas. A CRD schema MUST carry the CRD's group, kind, scope, and version list, so that KubeCheck can attribute findings to the CRD definition rather than to Kubernetes itself.

---

## 16. Authoritative references

| Reference | Scope | Type | Retrieved |
|---|---|---|---|
| Kubernetes API Conventions (SIG Architecture) | Kinds, resources, API groups, metadata, spec and status, references, lists vs maps, defaulting, optional vs required | Official community document | 2026-10-31 |
| Kubernetes Documentation — Extending the Kubernetes API | CRDs vs aggregation layer, custom resources, custom controllers | Official documentation | 2026-10-31 |
| Kubernetes Documentation — API Aggregation Layer | `APIService`, extension API servers, discovery latency | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Custom Resources | CRD scope, pruning, structural schemas | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Owners and Dependents | `ownerReferences`, `blockOwnerDeletion`, finalizers | Official documentation | 2026-10-31 |
| Kubernetes Documentation — Labels and Selectors | Label syntax, selector forms | Official documentation | 2026-10-31 |
| Kubernetes Documentation — API Discovery | `/api`, `/apis`, `APIResourceList`, namespaced flag | Official documentation | 2026-10-31 |
| OpenAPI v3 structural schema (KEP-2335) | Structural schema requirements, pruning, defaulting | Kubernetes Enhancement Proposal | 2026-10-31 |
| `fieldValidation` parameter | `Ignore` / `Warn` / `Strict` semantics | Kubernetes API reference | 2026-10-31 |
| Admission webhooks | Mutating and validating webhook ordering, defaulting | Kubernetes documentation | 2026-10-31 |
| CRD scope | `spec.scope` values and behavior | Kubernetes documentation | 2026-10-31 |
| `kubectl api-resources` | Namespaced vs cluster-scoped listing | Kubernetes documentation | 2026-10-31 |

**Licensing note:** Kubernetes documentation is licensed under CC BY 4.0. Kubernetes Enhancement Proposals are licensed under the Apache 2.0 license. No line numbers, benchmark results, or implementation status are asserted beyond what the cited sources state.

---

## 17. Open questions and unresolved decisions

| ID | Question | Status | Owner |
|---|---|---|---|
| OPEN-K8S-API-01 | Exact initial set of built-in GVKs covered by the first schema bundle (scope depends on bundle size; see `OPEN-001` in `PRODUCT_VISION.md`). | **OPEN** | Product Architecture / Bundle maintainers |
| OPEN-K8S-API-02 | Whether the first release will accept CRD definitions as a separate `--crd-dir` input or only via the bundle's CRD catalog. | **OPEN** | CLI Architecture |
| OPEN-K8S-API-03 | Whether aggregated API schemas will ever be accepted as local inputs, and under what format (OpenAPI document, protobuf descriptors). | **OPEN** | Software Architecture |
| OPEN-K8S-API-04 | Whether unknown-field severity for built-ins should default to `warning` (aligned with `fieldValidation: Warn`) or `error` (aligned with `Strict`). | **OPEN** | Diagnostics Architecture |
| OPEN-K8S-API-05 | Whether `status` presence in a manifest should be `info` or `warning` severity. | **OPEN** | Diagnostics Architecture |
| OPEN-K8S-API-06 | How to represent CRD conversion metadata in the bundle without implying that KubeCheck can perform conversion. | **OPEN** | Bundle maintainers |

---

## 18. Definition of done

This document is complete when the following conditions hold:

1. API groups, versions, resources, kinds, and discovery are defined, with the offline consequence that discovery cannot be queried.
2. The four top-level fields (`apiVersion`, `kind`, `metadata`, `spec`, `status`) are specified with their required/optional status and offline handling.
3. Metadata semantics (names, namespaces, labels, selectors, annotations, owner references, finalizers) are defined with offline-derivable and cluster-dependent aspects distinguished.
4. Built-in resources, CRDs, and aggregated APIs are distinguished, and KubeCheck's validation claims are scoped accordingly.
5. Namespaced versus cluster-scoped determination is specified from schema metadata and CRD `spec.scope`.
6. API-server processing (validation, admission, defaulting, pruning) is described, and KubeCheck's offline stance on each is stated.
7. Field types, required fields, enums, unions, lists, maps, and schema constraints are covered with KubeCheck's validation policy.
8. Unknown fields and version-specific behavior are specified with severity and reporting rules.
9. Resource identity and duplicate detection are defined with scope-awareness and edge cases.
10. Object references, lifecycle relationships, and API compatibility are covered with offline limits.
11. Offline limitations are enumerated and mapped to mitigation strategies.
12. For every example, facts derivable from local schemas are distinguished from facts requiring cluster state.
13. Schema-selection metadata is enumerated, and the bundle's obligation to provide it is stated.
14. No assumption is made that every API is built into Kubernetes (`K8S-API-REQ-001`).

---

## 19. Consistency and completeness review

A review was performed against the requested scope before finalization.

- **API groups, versions, resources, kinds, discovery:** §3, with offline discovery consequences.
- **apiVersion, kind, metadata, spec, status:** §4.
- **Namespaces, names, labels, selectors, annotations, owner references:** §5.
- **Built-in resources, CRDs, aggregated APIs, custom resources:** §6, with distinct validation claims.
- **Namespaced versus cluster-scoped:** §7, with scope determination and validation rules.
- **API-server validation, schema validation, admission, defaulting, pruning:** §8, with KubeCheck's offline stance.
- **Field types, required fields, enums, unions, lists, maps, schema constraints:** §9, including the CRD structural-schema restriction on unions.
- **Unknown fields and version-specific behavior:** §10, with severity rules and version-specific schema selection.
- **Resource identity and duplicate detection:** §11, with scope-aware identity.
- **Object references, lifecycle relationships, API compatibility:** §12, with structural vs existence distinction.
- **Offline limitations:** §13, with explicit capability boundary.
- **Derivability:** §14, with per-example tables distinguishing local from cluster-dependent facts.
- **Schema selection metadata:** §15, with the bundle's obligation.
- **No assumption that every API is built-in:** enforced by `K8S-API-REQ-001` and the three-way taxonomy in §6.

No empty boilerplate section was added solely to satisfy a checklist. The document is complete for its assigned responsibility.
