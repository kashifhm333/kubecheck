#!/usr/bin/env python3
"""
Unit tests for KubeCheck CLI binary lookup and platform isolation logic.
Verifies:
1. find_binary does not resolve itself or Python launcher entry points.
2. find_binary enforces OS-specific binary format validation (rejecting non-native binaries).
3. Recursion guard (_KUBECHECK_NATIVE_EXEC) suppresses recursive native invocation.
4. Pure-Python analyze fallback executes correctly on valid and invalid manifests.
"""

import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from kubecheck.cli import (
    EXIT_FINDINGS_DETECTED,
    EXIT_SUCCESS,
    find_binary,
    is_native_binary,
    python_analyze,
    run,
)


class TestCLIBinaryLookup(unittest.TestCase):
    def test_native_binary_detection_linux(self):
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"\x7fELF\x02\x01\x01\x00")
            f.flush()
            elf_path = Path(f.name)

        try:
            with mock.patch("platform.system", return_value="Linux"):
                self.assertTrue(is_native_binary(elf_path))
            with mock.patch("platform.system", return_value="Windows"):
                self.assertFalse(is_native_binary(elf_path))
            with mock.patch("platform.system", return_value="Darwin"):
                self.assertFalse(is_native_binary(elf_path))
        finally:
            elf_path.unlink()

    def test_native_binary_detection_windows(self):
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"MZ\x90\x00\x03\x00\x00\x00")
            f.flush()
            pe_path = Path(f.name)

        try:
            with mock.patch("platform.system", return_value="Windows"):
                self.assertTrue(is_native_binary(pe_path))
            with mock.patch("platform.system", return_value="Linux"):
                self.assertFalse(is_native_binary(pe_path))
            with mock.patch("platform.system", return_value="Darwin"):
                self.assertFalse(is_native_binary(pe_path))
        finally:
            pe_path.unlink()

    def test_native_binary_detection_darwin(self):
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"\xcf\xfa\xed\xfe")
            f.flush()
            macho_path = Path(f.name)

        try:
            with mock.patch("platform.system", return_value="Darwin"):
                self.assertTrue(is_native_binary(macho_path))
            with mock.patch("platform.system", return_value="Linux"):
                self.assertFalse(is_native_binary(macho_path))
            with mock.patch("platform.system", return_value="Windows"):
                self.assertFalse(is_native_binary(macho_path))
        finally:
            macho_path.unlink()

    def test_find_binary_recursion_guard(self):
        with mock.patch.dict(os.environ, {"_KUBECHECK_NATIVE_EXEC": "1"}):
            self.assertIsNone(find_binary("kubecheck"))

    def test_find_binary_skips_python_scripts_dir(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            fake_scripts = Path(tmp_dir) / "Scripts"
            fake_scripts.mkdir()
            fake_exe = fake_scripts / "kubecheck.exe"
            fake_exe.write_bytes(b"MZ\x90\x00\x03\x00\x00\x00")
            os.chmod(fake_exe, 0o755)

            with mock.patch("platform.system", return_value="Windows"):
                with mock.patch("sysconfig.get_path", return_value=str(fake_scripts)):
                    with mock.patch.dict(os.environ, {"PATH": str(fake_scripts), "KUBECHECK_BIN": ""}, clear=False):
                        with mock.patch.object(sys, "argv", [str(fake_scripts / "kubecheck"), "analyze"]):
                            result = find_binary("kubecheck")
                            self.assertIsNone(result)

    def test_python_analyze_valid_manifest(self):
        valid_pod = Path("tests/fixtures/yaml/valid/pod.yaml")
        if not valid_pod.exists():
            self.skipTest("Fixture not found")
        code = python_analyze([str(valid_pod)])
        self.assertEqual(code, EXIT_SUCCESS)

    def test_python_analyze_invalid_manifest(self):
        invalid_yaml = Path("tests/fixtures/yaml/invalid/duplicate_key.yaml")
        if not invalid_yaml.exists():
            self.skipTest("Fixture not found")
        code = python_analyze([str(invalid_yaml)])
        self.assertEqual(code, EXIT_FINDINGS_DETECTED)


if __name__ == "__main__":
    unittest.main()
