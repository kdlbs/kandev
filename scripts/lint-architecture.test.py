#!/usr/bin/env python3
"""Discover and run the modular architecture-lint regression suite."""

import sys
import unittest
from pathlib import Path


TEST_DIR = Path(__file__).resolve().parent / "architecture_lint_tests"


if __name__ == "__main__":
    sys.path.insert(0, str(TEST_DIR))
    suite = unittest.defaultTestLoader.discover(str(TEST_DIR), pattern="test_*.py")
    result = unittest.TextTestRunner(verbosity=1).run(suite)
    raise SystemExit(0 if result.wasSuccessful() else 1)
