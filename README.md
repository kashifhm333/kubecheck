# KubeCheck

[![PyPI version](https://img.shields.io/pypi/v/kubecheck.svg)](https://pypi.org/project/kubecheck/)
[![Python Versions](https://img.shields.io/pypi/pyversions/kubecheck.svg)](https://pypi.org/project/kubecheck/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Offline-First](https://img.shields.io/badge/Operation-100%25_Offline-success.svg)](#offline-first-architecture)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.24--v1.31-326CE5.svg)](https://kubernetes.io)

KubeCheck is a lightweight, offline-first static analysis CLI tool designed to diagnose Kubernetes manifest problems locally.
It traverses syntax validation, OpenAPI schema conformance, cross-resource semantic references, and security best practices across multi-document YAML files.
The default analysis workflow runs entirely on your local machine with zero network calls, no cloud dependencies, and no live cluster required.

---

## Key Features

- **100% Offline-First:** Bundles versioned schemas and diagnostic rules locally. Analysis executes with strict network isolation.
- **Deep Static Analysis:** Validates YAML syntax, duplicate keys, OpenAPI v2/v3 schema constraints, deprecated API versions, and workload security contexts.
- **Security & Best Practices:** Enforces checks for unpinned image tags, missing CPU/memory limits, absent liveness/readiness probes, privileged containers, and wildcard RBAC permissions.
- **Fast & Lightweight:** Near-instant evaluation (< 50 ms) with minimal memory footprint (~8 MB RSS peak).
- **Dual Invocation:** Runs as a standalone CLI (`kubecheck`) or directly as a `kubectl` plugin (`kubectl kubecheck`).

---

## Installation

Install globally via `pip`:

```bash
pip install kubecheck
```

*Requirements: Python 3.8 or higher.*

---

## Quick Start

### 1. Analyze a Manifest File

```bash
kubecheck analyze deployment.yaml
```

### 2. Recursively Scan a Directory

```bash
kubecheck analyze -R ./deployments/
```

### 3. Target Specific Kubernetes API Version

Validate against specific Kubernetes versions (v1.24 through v1.31) to detect deprecated or removed APIs:

```bash
kubecheck analyze -R ./deployments/ --target-version 1.31
```

### 4. Use via Standard Input (Piping)

```bash
cat manifest.yaml | kubecheck analyze -
```

### 5. Inspect Built-in Diagnostic Rules

```bash
kubecheck rules
```

---

## Using as a `kubectl` Plugin

Once installed via `pip`, KubeCheck automatically provides the `kubectl-kubecheck` executable shim:

```bash
kubectl kubecheck analyze manifest.yaml
```

---

## Exit Codes Contract

KubeCheck provides deterministic process exit codes for automated CI/CD gating:

| Exit Code | Label | Meaning |
|---|---|---|
| `0` | `SUCCESS` | Clean scan; zero findings at or above failure threshold. |
| `1` | `FINDINGS_DETECTED` | One or more findings detected at or above failure threshold (`error`). |
| `2` | `USAGE_ERROR` | Invalid CLI arguments, flags, or configuration syntax error. |
| `3` | `INPUT_ERROR` | File not found, permission denied, or unreadable input stream. |
| `4` | `OPERATIONAL_ERROR` | Internal limit exceeded (alias bomb, memory governor, recursion cap). |
| `5` | `PARTIAL_SCAN` | Scan cancelled by SIGINT/SIGTERM user interrupt. |
| `6` | `BUNDLE_ERROR` | Corrupted, invalid, or missing offline knowledge bundle. |

---

## Offline-First Architecture

KubeCheck is strictly hermetic by default. It contains pre-compiled schema catalogs and diagnostic rule definitions.
No data is ever transmitted off your system during manifest analysis, making it safe for air-gapped systems, secure defense networks, and restricted corporate build runners.

---

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE).
