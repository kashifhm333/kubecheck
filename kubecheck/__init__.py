"""
KubeCheck - Offline-first CLI tool to diagnose Kubernetes YAML manifest problems locally.

Copyright 2026 Kashif Hussain.
Licensed under the Apache License, Version 2.0.
"""

__version__ = "0.1.0"
__author__ = "Kashif Hussain"
__email__ = "memonkashif0001@gmail.com"

from kubecheck.cli import run

__all__ = ["__version__", "run"]
