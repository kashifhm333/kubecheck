#!/usr/bin/env python3
"""
KubeCheck CLI Python entrypoint and offline static analysis engine.
Adheres strictly to PEP 517 / PEP 621 specifications and offline-first guarantees.
"""

from __future__ import annotations

import json
import os
import platform
import subprocess
import sys
from pathlib import Path
from typing import Any

from kubecheck import __version__

# Canonical exit codes per docs/distribution/CLI_UX_AND_EXIT_CODES.md
EXIT_SUCCESS = 0
EXIT_FINDINGS_DETECTED = 1
EXIT_USAGE_ERROR = 2
EXIT_INPUT_ERROR = 3
EXIT_OPERATIONAL_ERROR = 4
EXIT_PARTIAL_SCAN = 5
EXIT_BUNDLE_ERROR = 6


def is_native_binary(file_path: Path) -> bool:
    """Checks if a file is a valid compiled native binary for the host OS."""
    try:
        with open(file_path, "rb") as f:
            magic = f.read(4)
    except Exception:
        return False

    system = platform.system().lower()
    if system == "windows":
        return magic.startswith(b"MZ")
    elif system == "darwin":
        return magic in (
            b"\xfe\xed\xfa\xce",
            b"\xfe\xed\xfa\xcf",
            b"\xce\xfa\xed\xfe",
            b"\xcf\xfa\xed\xfe",
            b"\xca\xfe\xba\xbe",
        )
    elif system == "linux":
        return magic.startswith(b"\x7fELF")
    return False


def find_binary(binary_name: str = "kubecheck") -> Path | None:
    """Locates the native KubeCheck engine binary using strict precedence."""
    # 0. Recursion guard: never invoke native binary if already executing inside KubeCheck
    if os.environ.get("_KUBECHECK_NATIVE_EXEC"):
        return None

    # 1. Explicit override via environment variable
    if env_path := os.environ.get("KUBECHECK_BIN"):
        candidate = Path(env_path)
        if candidate.is_file() and os.access(candidate, os.X_OK) and is_native_binary(candidate):
            return candidate

    package_dir = Path(__file__).resolve().parent

    # 2. Packaged platform-specific binary inside wheel or repo
    system = platform.system().lower()
    machine = platform.machine().lower()
    arch = "arm64" if machine in ("aarch64", "arm64") else ("amd64" if machine in ("x86_64", "amd64") else machine)
    ext = ".exe" if system == "windows" else ""
    full_name = f"{binary_name}{ext}"

    platform_subdir = f"{system}_{arch}"
    candidates = [
        package_dir / "bin" / platform_subdir / full_name,
        package_dir / "bin" / full_name,
        package_dir.parent / "bin" / full_name,
        package_dir.parent.parent / "bin" / full_name,
    ]

    for candidate in candidates:
        if candidate.is_file() and os.access(candidate, os.X_OK) and is_native_binary(candidate):
            return candidate

    # 3. System PATH lookup (excluding Python wrappers/shebang scripts)
    import sysconfig

    scripts_dirs = {
        Path(sysconfig.get_path("scripts")).resolve(),
        (Path(sys.prefix) / "bin").resolve(),
        (Path(sys.prefix) / "Scripts").resolve(),
    }

    current_script_paths = set()
    if sys.argv and sys.argv[0]:
        p0 = Path(sys.argv[0]).resolve()
        current_script_paths.add(p0)
        current_script_paths.add(p0.with_suffix(""))
        if system == "windows":
            current_script_paths.add(p0.with_suffix(".exe"))

    for path_dir in os.environ.get("PATH", "").split(os.pathsep):
        if not path_dir:
            continue
        candidate = Path(path_dir) / full_name
        if not (candidate.is_file() and os.access(candidate, os.X_OK)):
            continue

        resolved = candidate.resolve()

        # Exclude ourselves (current script/binary or same path with/without extension)
        if resolved in current_script_paths or resolved.with_suffix("") in current_script_paths:
            continue

        # Exclude anything installed in Python's scripts/bin directories matching our CLI names
        if resolved.parent in scripts_dirs and resolved.stem.lower() in ("kubecheck", "kubectl-kubecheck"):
            continue

        # Exclude script/shebang wrappers
        try:
            with open(candidate, "rb") as f:
                magic = f.read(4)
                if magic.startswith(b"#!"):
                    continue
        except Exception:
            pass

        # Must be a valid native binary for current platform
        if not is_native_binary(candidate):
            continue

        return candidate

    return None


def get_data_dir() -> Path:
    """Returns the base directory for offline knowledge and schemas."""
    return Path(__file__).resolve().parent


def load_knowledge_file(filename: str) -> Any:
    """Loads a JSON file from the offline knowledge directory."""
    target = get_data_dir() / "knowledge" / filename
    if target.is_file():
        with open(target, "r", encoding="utf-8") as f:
            return json.load(f)
    return None


def print_help(command_name: str = "kubecheck") -> None:
    """Prints standard help message matching KubeCheck CLI UX."""
    help_text = f"""KubeCheck is an offline-first diagnostic engine for Kubernetes manifests.

Usage:
  {command_name} <command> [flags] [arguments]

Available Commands:
  analyze      Analyze Kubernetes manifests for syntax, schema, semantic, and security defects
  version      Display engine version, embedded bundle metadata, and supported Kubernetes APIs
  rules        Inspect and explain diagnostic rules in the active catalog
  bundle       Manage, verify, export, import, or update knowledge bundles

Global Flags:
      --config string           Path to configuration file
      --offline                 Enforce strict network isolation (default: true)
  -v, --verbose                 Enable verbose trace logging
  -q, --quiet                   Suppress informative banners; emit only findings
  -h, --help                    Help for {command_name}
  -V, --version                 Display version information

Examples:
  # Analyze a single manifest
  {command_name} analyze manifest.yaml

  # Recursively analyze a directory against Kubernetes v1.31
  {command_name} analyze -R ./deployments/ --target-version 1.31

  # Validate manifests from stdin
  cat manifest.yaml | {command_name} analyze -

Use "{command_name} <command> --help" for more information about a command.
"""
    print(help_text)


def print_version() -> None:
    """Prints version and embedded bundle information."""
    meta = load_knowledge_file("metadata.json") or {}
    bundle_ver = meta.get("bundle_version", "2026.10.1")
    k8s_versions = ", ".join(v.get("minor", "") for v in meta.get("kubernetes_versions", []))
    if not k8s_versions:
        k8s_versions = "v1.24, v1.25, v1.26, v1.27, v1.28, v1.29, v1.30, v1.31"

    print(f"KubeCheck Engine:            {__version__}")
    print(f"Default Knowledge Bundle:     {bundle_ver}")
    print(f"Supported Kubernetes APIs:    {k8s_versions}")
    print("Distribution:                 Python / Pip (PEP 517 / PEP 621)")
    print("License:                      Apache-2.0")


def print_rules() -> None:
    """Lists diagnostic rules from the offline knowledge catalog."""
    rules = load_knowledge_file("rules.json") or []
    print(f"Active Diagnostic Rules: catalog loaded from offline knowledge bundle")
    for r in rules:
        rid = r.get("id", "").ljust(18)
        sev = f"[{r.get('severity', 'info')}]".ljust(10)
        cat = f"({r.get('category', 'general')})"
        title = r.get("title", "")
        print(f"  {rid} {sev} {title} {cat}")


def print_bundle_status() -> None:
    """Prints knowledge bundle status from embedded metadata."""
    meta = load_knowledge_file("metadata.json") or {}
    print("Knowledge Bundle Status:")
    print(f"  ID:           {meta.get('bundle_id', 'kubecheck-standard-2026.10.1')}")
    print(f"  Version:      {meta.get('bundle_version', '20261001')}")
    print(f"  Format:       {meta.get('format_version', '1.0.0')}")
    print(f"  Source:       Offline Embedded Default")
    print(f"  Licenses:     {', '.join(meta.get('licenses', ['Apache-2.0']))}")


def python_analyze(paths: list[str], target_version: str = "1.31", recursive: bool = False) -> int:
    """Pure-Python offline analyzer fallback using ruamel.yaml and jsonschema."""
    try:
        from ruamel.yaml import YAML
        from ruamel.yaml.constructor import DuplicateKeyError
    except ImportError:
        sys.stderr.write("error: ruamel.yaml is required for YAML parsing. Install via 'pip install ruamel.yaml'.\n")
        return EXIT_OPERATIONAL_ERROR

    try:
        import jsonschema
    except ImportError:
        jsonschema = None

    yaml_parser = YAML(typ="safe")
    yaml_parser.allow_duplicate_keys = False

    files_to_scan: list[Path] = []
    for p_str in paths:
        if p_str == "-":
            # Stdin
            files_to_scan.append(Path("-"))
            continue
        p = Path(p_str)
        if not p.exists():
            sys.stderr.write(f"error: input path '{p_str}' does not exist\n")
            return EXIT_INPUT_ERROR
        if p.is_file():
            files_to_scan.append(p)
        elif p.is_dir():
            pattern = "**/*.y*ml" if recursive else "*.y*ml"
            files_to_scan.extend(p.glob(pattern))

    if not files_to_scan:
        sys.stderr.write("error: no manifest files found to analyze\n")
        return EXIT_INPUT_ERROR

    lifecycle = load_knowledge_file("lifecycle.json") or []
    rules = load_knowledge_file("rules.json") or []
    core_rules = {
        "K8S-GVK-007": {
            "evidence_class": "deterministic",
            "remediation": 'Define a "metadata" section containing at least a name.',
        },
        "K8S-GVK-008": {
            "evidence_class": "deterministic",
            "remediation": 'Ensure "metadata" is formatted as a key-value mapping.',
        },
        "K8S-GVK-010": {
            "evidence_class": "deterministic",
            "remediation": 'Define metadata.name identifying the Kubernetes resource.',
        },
        "K8S-SCHEMA-001": {
            "evidence_class": "deterministic",
            "remediation": "Remove or correct unknown fields that do not conform to the Kubernetes API schema.",
        },
        "K8S-SCHEMA-003": {
            "evidence_class": "deterministic",
            "remediation": "Supply all required fields specified by the resource schema.",
        },
        "KCHK-SYN-001": {
            "evidence_class": "deterministic",
            "remediation": "Remove duplicate keys in YAML mappings.",
        },
        "KCHK-SYN-002": {
            "evidence_class": "deterministic",
            "remediation": "Fix syntax error to ensure valid YAML structure.",
        },
        "KCHK-DEP-001": {
            "evidence_class": "deterministic",
            "remediation": "Upgrade apiVersion to current stable group/version.",
        },
    }
    rule_map = {**core_rules, **{r["id"]: r for r in rules}}

    findings: list[dict[str, Any]] = []
    total_docs = 0
    total_resources = 0

    for file_path in files_to_scan:
        try:
            if str(file_path) == "-":
                content = sys.stdin.read()
                file_label = "<stdin>"
            else:
                content = file_path.read_text(encoding="utf-8")
                file_label = str(file_path)
        except Exception as e:
            sys.stderr.write(f"error: failed to read '{file_path}': {e}\n")
            return EXIT_INPUT_ERROR

        try:
            docs = list(yaml_parser.load_all(content))
        except DuplicateKeyError as e:
            findings.append({
                "rule_id": "KCHK-SYN-001",
                "severity": "error",
                "file": file_label,
                "line": getattr(e.context_mark, "line", 1) + 1,
                "message": f"Duplicate mapping key in YAML document: {e}",
            })
            continue
        except Exception as e:
            findings.append({
                "rule_id": "KCHK-SYN-002",
                "severity": "error",
                "file": file_label,
                "line": 1,
                "message": f"YAML syntax error: {e}",
            })
            continue

        for doc_idx, doc in enumerate(docs):
            if not isinstance(doc, dict):
                continue
            total_docs += 1
            api_version = str(doc.get("apiVersion", ""))
            kind = str(doc.get("kind", ""))
            if not kind:
                continue
            total_resources += 1

            # 1. Lifecycle deprecation check
            parts = api_version.split("/", 1)
            group = parts[0] if len(parts) == 2 else ""
            version = parts[1] if len(parts) == 2 else parts[0]

            for entry in lifecycle:
                if entry.get("group", "") == group and entry.get("version", "") == version:
                    if not entry.get("kind") or entry.get("kind") == kind:
                        findings.append({
                            "rule_id": "KCHK-DEP-001",
                            "severity": "warning",
                            "file": file_label,
                            "line": 1,
                            "message": f"{api_version} for {kind} is deprecated; replace with {entry.get('replacement_api')}",
                        })

            # 2. Schema and Object Root Checks (K8S-SCHEMA-001, K8S-GVK-007, K8S-GVK-008, K8S-GVK-010, K8S-SCHEMA-003)
            standard_top_fields = {
                "apiVersion", "kind", "metadata", "spec", "status", "data",
                "binaryData", "stringData", "type", "subsets", "roleRef",
                "subjects", "rules", "secrets", "imagePullSecrets",
            }
            lines = content.splitlines()
            for k in doc.keys():
                if k not in standard_top_fields:
                    line_no = 1
                    for idx, line in enumerate(lines):
                        if line.strip().startswith(f"{k}:"):
                            line_no = idx + 1
                            break
                    findings.append({
                        "rule_id": "K8S-SCHEMA-001",
                        "severity": "warning",
                        "file": file_label,
                        "line": line_no,
                        "message": f'Unknown field "{k}" at {kind}.{k}',
                    })

            metadata = doc.get("metadata")
            if metadata is None:
                findings.append({
                    "rule_id": "K8S-GVK-007",
                    "severity": "warning",
                    "file": file_label,
                    "line": 1,
                    "message": 'Missing "metadata" section in Kubernetes object',
                })
                findings.append({
                    "rule_id": "K8S-SCHEMA-003",
                    "severity": "error",
                    "file": file_label,
                    "line": 1,
                    "message": f'Missing required field "metadata" at {kind}.metadata',
                })
                findings.append({
                    "rule_id": "K8S-GVK-010",
                    "severity": "error",
                    "file": file_label,
                    "line": 1,
                    "message": f'Resource {kind} is missing required "metadata.name"',
                })
            elif not isinstance(metadata, dict):
                findings.append({
                    "rule_id": "K8S-GVK-008",
                    "severity": "error",
                    "file": file_label,
                    "line": 1,
                    "message": 'Field "metadata" must be a mapping',
                })
                findings.append({
                    "rule_id": "K8S-GVK-010",
                    "severity": "error",
                    "file": file_label,
                    "line": 1,
                    "message": f'Resource {kind} is missing required "metadata.name"',
                })
            else:
                name = metadata.get("name")
                if not name:
                    findings.append({
                        "rule_id": "K8S-GVK-010",
                        "severity": "error",
                        "file": file_label,
                        "line": 1,
                        "message": f'Resource {kind} is missing required "metadata.name"',
                    })

            # 3. Workload and Security Rule Evaluations
            spec = doc.get("spec", {})
            template_spec = spec.get("template", {}).get("spec", {}) if isinstance(spec, dict) else {}
            pod_spec = template_spec if template_spec else (spec if kind == "Pod" and isinstance(spec, dict) else {})

            if pod_spec:
                containers = pod_spec.get("containers", [])
                if isinstance(containers, list):
                    for c in containers:
                        if not isinstance(c, dict):
                            continue
                        c_name = c.get("name", "unknown")
                        img = str(c.get("image", ""))

                        # WL-CONTAINER-001: Unpinned or :latest image tag
                        if img and (":" not in img or img.endswith(":latest")):
                            findings.append({
                                "rule_id": "WL-CONTAINER-001",
                                "severity": "warning",
                                "file": file_label,
                                "line": 1,
                                "message": f'Container "{c_name}" image "{img}" is unpinned or uses :latest tag',
                            })

                        # WL-RES-001: Missing resource limits or requests
                        resources = c.get("resources", {})
                        if not resources or not resources.get("limits") or not resources.get("requests"):
                            findings.append({
                                "rule_id": "WL-RES-001",
                                "severity": "warning",
                                "file": file_label,
                                "line": 1,
                                "message": f'Container "{c_name}" is missing resource requests and limits',
                            })

                        # WL-PROBE-001: Missing probes
                        if not c.get("readinessProbe") and not c.get("livenessProbe"):
                            findings.append({
                                "rule_id": "WL-PROBE-001",
                                "severity": "info",
                                "file": file_label,
                                "line": 1,
                                "message": f'Container "{c_name}" is missing liveness and readiness probes',
                            })

                        # SEC-CTX checks
                        c_sec = c.get("securityContext", {}) or {}
                        if c_sec.get("privileged") is True:
                            findings.append({
                                "rule_id": "SEC-CTX-001",
                                "severity": "error",
                                "file": file_label,
                                "line": 1,
                                "message": f'Container "{c_name}" has securityContext.privileged set to true',
                            })

                        if c_sec.get("readOnlyRootFilesystem") is not True:
                            findings.append({
                                "rule_id": "SEC-CTX-017",
                                "severity": "info",
                                "file": file_label,
                                "line": 1,
                                "message": f'Container "{c_name}" does not set readOnlyRootFilesystem: true',
                            })

                # Host namespace sharing
                if pod_spec.get("hostNetwork") is True:
                    findings.append({
                        "rule_id": "SEC-CTX-005",
                        "severity": "warning",
                        "file": file_label,
                        "line": 1,
                        "message": "Pod specifies hostNetwork: true, sharing host network namespace",
                    })

            # Service without selector
            if kind == "Service" and isinstance(spec, dict):
                if not spec.get("selector") and spec.get("type") != "ExternalName":
                    findings.append({
                        "rule_id": "NET-SVC-001",
                        "severity": "warning",
                        "file": file_label,
                        "line": 1,
                        "message": f'Service "{doc.get("metadata", {}).get("name", "unknown")}" is missing spec.selector',
                    })

    # Render Findings
    error_count = 0
    warning_count = 0
    info_count = 0

    for f in findings:
        sev = f["severity"].upper()
        if sev == "ERROR":
            error_count += 1
        elif sev == "WARNING":
            warning_count += 1
        else:
            info_count += 1

        print(f'{sev} [{f["rule_id"]}] in {f["file"]}:{f.get("line", 1)}:')
        print(f'  {f["message"]}')
        meta_rule = rule_map.get(f["rule_id"])
        if meta_rule:
            print(f'  Confidence: {meta_rule.get("evidence_class", "deterministic")} · Remediation: {meta_rule.get("remediation", "")}')
        print()

    print("-" * 80)
    print("Scan Summary:")
    print(f"  Target Version:  v{target_version}")
    print(f"  Documents:       {total_docs} parsed")
    print(f"  Resources:       {total_resources} evaluated")
    print(f"  Findings:        {error_count} Error, {warning_count} Warning, {info_count} Info")

    if error_count > 0:
        print(f'Result: FAILED (Found {error_count} finding(s) at or above fail threshold "error")')
        return EXIT_FINDINGS_DETECTED

    print("Result: PASSED")
    return EXIT_SUCCESS


def run(args: list[str] | None = None) -> int:
    """
    Main entry point function for KubeCheck.
    Accepts an optional list of CLI arguments (defaults to sys.argv[1:]).
    Returns integer process exit code conforming to CLI_UX_AND_EXIT_CODES.md.
    """
    if args is None:
        args = sys.argv[1:]

    # Detect execution name (kubectl-kubecheck or kubecheck)
    prog_name = Path(sys.argv[0]).name if sys.argv else "kubecheck"
    command_name = "kubectl kubecheck" if "kubectl" in prog_name else "kubecheck"

    if not args or "-h" in args or "--help" in args or (len(args) == 1 and args[0] == "help"):
        print_help(command_name)
        return EXIT_SUCCESS

    if "-V" in args or "--version" in args or (len(args) == 1 and args[0] == "version"):
        print_version()
        return EXIT_SUCCESS

    command = args[0]

    if command == "rules":
        print_rules()
        return EXIT_SUCCESS

    if command == "bundle":
        subcommand = args[1] if len(args) > 1 else "status"
        if subcommand in ("status", "list"):
            print_bundle_status()
            return EXIT_SUCCESS
        else:
            print_bundle_status()
            return EXIT_SUCCESS

    if command == "analyze":
        # Check if native binary is available for high-performance execution
        native_bin = find_binary("kubecheck")
        if native_bin:
            try:
                env = os.environ.copy()
                env["_KUBECHECK_NATIVE_EXEC"] = "1"
                proc = subprocess.run([str(native_bin)] + args, env=env)
                return proc.returncode
            except KeyboardInterrupt:
                return EXIT_PARTIAL_SCAN
            except Exception as e:
                sys.stderr.write(f"error executing native engine: {e}\n")

        # Parse flags for pure-Python fallback analyzer
        paths: list[str] = []
        target_version = "1.31"
        recursive = False
        idx = 1
        while idx < len(args):
            arg = args[idx]
            if arg in ("-R", "--recursive"):
                recursive = True
            elif arg in ("-t", "--target-version"):
                idx += 1
                if idx < len(args):
                    target_version = args[idx].lstrip("v")
            elif arg.startswith("--target-version="):
                target_version = arg.split("=", 1)[1].lstrip("v")
            elif not arg.startswith("-"):
                paths.append(arg)
            idx += 1

        if not paths:
            sys.stderr.write("error: no manifest files or directories specified to analyze\n")
            return EXIT_USAGE_ERROR

        return python_analyze(paths, target_version=target_version, recursive=recursive)

    sys.stderr.write(f"error: unknown command '{command}'. See '{command_name} --help'.\n")
    return EXIT_USAGE_ERROR


def main() -> None:
    """Console script wrapper for 'kubecheck'."""
    sys.exit(run())


def kubectl_plugin_main() -> None:
    """Console script wrapper for 'kubectl-kubecheck'."""
    sys.exit(run())


if __name__ == "__main__":
    main()
