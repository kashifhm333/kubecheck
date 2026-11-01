# KubeCheck

KubeCheck is an offline-first diagnostic engine for Kubernetes manifests.
It performs deterministic static analysis across multi-document YAML streams to identify syntax errors, OpenAPI schema violations, cross-resource selector and port mismatches, security vulnerabilities, and version deprecations without requiring a live cluster or internet connection.

## Installation

```bash
pip install kubecheck
```

## Quick Start

Analyze Kubernetes manifests:

```bash
kubecheck analyze deployment.yaml
```

Analyze recursively against Kubernetes v1.31 with Pod Security Standards enforcement:

```bash
kubecheck analyze -R ./deployments/ --target-version 1.31 --pss restricted
```

Export results in SARIF or JSON:

```bash
kubecheck analyze manifests/ --output sarif > kubecheck-results.sarif
kubecheck analyze manifests/ --output json > kubecheck-results.json
```

Use as a kubectl plugin:

```bash
kubectl check analyze manifests/
```
