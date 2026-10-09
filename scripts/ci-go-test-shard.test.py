#!/usr/bin/env python3
import contextlib
import importlib.util
import io
import re
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location("go_shard", Path(__file__).with_name("ci-go-test-shard.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class GoTestShardTest(unittest.TestCase):
    def test_complete_disjoint_membership_includes_seeds_examples_and_duplicate_package_names(self):
        names = MODULE.discover_names(
            "TestZulu\nFuzzSeed\nExample\nTestAlpha\nTestUnicodeΩ\nTestAlpha\n"
            "BenchmarkTiming\nok github.com/example/package 0.1s\n"
        )
        first = MODULE.select_names(names, 1, 2)
        second = MODULE.select_names(names, 2, 2)
        self.assertEqual(set(first) | set(second), set(names))
        self.assertFalse(set(first) & set(second))
        self.assertEqual(len(names), 5)
        for selected in (first, second):
            pattern = re.compile("^(" + "|".join(re.escape(name) for name in selected) + ")$")
            self.assertEqual([name for name in names if pattern.fullmatch(name)], selected)

    def test_empty_and_invalid_shards_fail(self):
        for names, shard, shards in [([], 1, 2), (["TestOne"], 2, 2), (["TestOne"], 0, 2)]:
            with self.assertRaises(ValueError):
                MODULE.select_names(names, shard, shards)

    def test_discovery_failure_stops_execution(self):
        result = subprocess.CompletedProcess([], 3, stdout="", stderr="compile failed\n")
        with patch.object(MODULE.subprocess, "run", return_value=result) as run:
            with contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(MODULE.main(["--shard", "1", "--shards", "2", "./package/..."]), 3)
            self.assertEqual(run.call_count, 1)

    def test_test_failure_exit_and_race_timeout_are_preserved(self):
        discovered = subprocess.CompletedProcess([], 0, stdout="TestOne\nTestTwo\n", stderr="")
        failed = subprocess.CompletedProcess([], 7)
        with patch.object(MODULE.subprocess, "run", side_effect=[discovered, failed]) as run:
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(MODULE.main(["--shard", "1", "--shards", "2", "./package/..."]), 7)
            command = run.call_args_list[1].args[0]
            self.assertIn("-race", command)
            self.assertEqual(command[command.index("-timeout") + 1], "25m")
            self.assertEqual(command[command.index("-run") + 1], "^(TestOne)$")


if __name__ == "__main__":
    unittest.main()
