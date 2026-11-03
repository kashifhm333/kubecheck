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

    # 2. Packaged platform-specific binary inside wheel
    system = platform.system().lower()
    machine = platform.machine().lower()
    arch = "arm64" if machine in ("aarch64", "arm64") else ("amd64" if machine in ("x86_64", "amd64") else machine)
    ext = ".exe" if system == "windows" else ""
    full_name = f"{binary_name}{ext}"

    platform_subdir = f"{system}_{arch}"
    candidates = [
        package_dir / "bin" / platform_subdir / full_name,
        package_dir / "bin" / full_name,
        package_dir.parent.parent / "bin" / full_name,  # Workspace repo root fallback
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
        env = os.environ.copy()
        env["_KUBECHECK_NATIVE_EXEC"] = "1"
        proc = subprocess.run(args, env=env)
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
