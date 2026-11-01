#!/usr/bin/env python3
"""
KubeCheck CLI Python entrypoint wrapper.
Adheres strictly to ADR-004 and GATE-REL-REQ-020:
- Contains zero diagnostic, parsing, or schema logic.
- Resolves the hermetic native binary and transparently delegates all I/O streams and CLI flags.
- Preserves the exact process exit code contract (0 through 6).
"""

import os
import platform
import shutil
import subprocess
import sys
from pathlib import Path


def find_binary(binary_name: str = "kubecheck") -> Path | None:
    """Locates the native KubeCheck engine binary using strict precedence."""
    # 1. Explicit override via environment variable
    if env_path := os.environ.get("KUBECHECK_BIN"):
        candidate = Path(env_path)
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return candidate

    package_dir = Path(__file__).resolve().parent

    # 2. Packaged platform-specific binary inside wheel
    system = platform.system().lower()
    machine = platform.machine().lower()
    if machine in ("x86_64", "amd64"):
        arch = "amd64"
    elif machine in ("aarch64", "arm64"):
        arch = "arm64"
    else:
        arch = machine

    ext = ".exe" if system == "windows" else ""
    full_name = f"{binary_name}{ext}"

    platform_subdir = f"{system}_{arch}"
    candidates = [
        package_dir / "bin" / platform_subdir / full_name,
        package_dir / "bin" / full_name,
        package_dir.parent.parent / "bin" / full_name,  # Workspace repo root fallback
    ]

    for candidate in candidates:
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return candidate

    # 3. System PATH lookup (excluding Python wrappers/shebang scripts)
    for path_dir in os.environ.get("PATH", "").split(os.pathsep):
        if not path_dir:
            continue
        candidate = Path(path_dir) / full_name
        if candidate.is_file() and os.access(candidate, os.X_OK):
            try:
                if candidate.resolve() == Path(sys.argv[0]).resolve():
                    continue
                with open(candidate, "rb") as f:
                    magic = f.read(4)
                    if magic.startswith(b"#!"):
                        continue
            except Exception:
                pass
            return candidate

    return None


def run_engine(binary_name: str = "kubecheck") -> int:
    """Executes the native diagnostic engine binary with forwarded arguments."""
    binary = find_binary(binary_name)
    if not binary:
        sys.stderr.write(
            f"error: native KubeCheck engine binary ({binary_name}) could not be located.\n"
            f"Please ensure '{binary_name}' is installed or set KUBECHECK_BIN=/path/to/binary.\n"
        )
        return 2  # ExitCode: UsageError

    args = [str(binary)] + sys.argv[1:]

    # On POSIX systems with standard streams, replace current process with execv
    # where feasible, or fallback to subprocess for cross-platform reliability.
    try:
        proc = subprocess.run(args)
        return proc.returncode
    except KeyboardInterrupt:
        return 5  # ExitCode: PartialScan
    except Exception as exc:
        sys.stderr.write(f"error running {binary_name}: {exc}\n")
        return 4  # ExitCode: OperationalError


def main() -> None:
    """Entry point for 'kubecheck' command."""
    sys.exit(run_engine("kubecheck"))


def kubectl_plugin_main() -> None:
    """Entry point for 'kubectl check' (kubectl-check) plugin command."""
    # Find binary under kubectl-check or fallback to kubecheck
    bin_path = find_binary("kubectl-check")
    if not bin_path:
        bin_path = find_binary("kubecheck")

    if bin_path:
        # Pass argv[0] as kubectl-check so Go engine sets CommandName to "kubectl check"
        args = [str(bin_path)] + sys.argv[1:]
        try:
            proc = subprocess.run(args, argv0="kubectl-check")
            sys.exit(proc.returncode)
        except Exception:
            sys.exit(run_engine("kubecheck"))
    else:
        sys.exit(run_engine("kubectl-check"))


if __name__ == "__main__":
    main()
