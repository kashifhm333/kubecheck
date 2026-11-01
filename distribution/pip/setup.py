#!/usr/bin/env python3
"""
KubeCheck setuptools packaging script.
Builds the Python wrapper wheel that packages and invokes the native KubeCheck engine.
"""

from setuptools import setup, find_packages

setup(
    packages=find_packages(),
    package_data={
        "kubecheck": ["bin/*", "bin/*/*"],
    },
    include_package_data=True,
)
