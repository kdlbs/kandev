#!/usr/bin/env python3
"""Native inventory and complete-result contracts for repository CI cohorts."""
import importlib.util
import json
import sys
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("repository-test-shard.py")
spec = importlib.util.spec_from_file_location("repository_shard", SCRIPT)
shard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(shard)


def event(action, **fields):
    return {"Action": action, "Package": shard.PACKAGE, **fields}


def listing(names):
    return [event("start"), *[event("output", Output=name + "\n") for name in names],
            event("output", Output="ok  \t" + shard.PACKAGE + "\t0.003s\n"), event("pass")]


def results(names):
    events = [event("start")]
    for name in names:
        events.extend([event("run", Test=name), event("pass", Test=name)])
    return [*events, event("pass")]


class RepositoryShardTests(unittest.TestCase):
    def test_native_inventory_and_disjoint_selectors_cover_future_names(self):
        names = ["TestZulu", "FuzzInput", "Example_save", "TestNewFeature", "TestAlpha"]
        native = shard.inventory(listing(names))
        self.assertEqual(native, sorted(names))
        groups = [shard.partition(native, index) for index in (1, 2, 3)]
        self.assertEqual(sorted(name for group in groups for name in group), sorted(names))
        self.assertTrue(all(groups))
        for index in (0, 4):
            with self.assertRaises(ValueError):
                shard.partition(native, index)

    def test_inventory_rejects_missing_duplicate_or_invalid_evidence(self):
        good = listing(["TestAlpha", "TestBeta", "TestGamma"])
        bad = [good[:-1], good + [event("pass")], listing([]), listing(["TestAlpha"] * 3),
               listing(["TestAlpha", "TestBeta/sub", "TestGamma"]),
               listing(["TestAlpha", "TestMain", "TestGamma"]),
               [*good[:-1], event("fail")],
               [*good[:-1], event("skip")],
               [*good[:-1], event("pass", Package="other")],
               [event("run", Test="TestAlpha"), *good],
               [event("output", Output="unexpected output\n"), *good]]
        for events in bad:
            with self.subTest(events=events), self.assertRaises(ValueError):
                shard.inventory(events)

    def test_result_accepts_nested_tests_fuzz_seeds_and_explicit_skips(self):
        names = ["TestAlpha", "FuzzInput", "Example_save"]
        events = results(names)[:-1]
        events += [event("pass", Test="TestAlpha/nested"),
                   event("pass", Test="FuzzInput/seed#0"), event("pass")]
        shard.validate_result(events, names)
        shard.validate_result([event("start"), event("skip", Test="TestAlpha"), event("pass")], ["TestAlpha"])

    def test_result_rejects_omission_extra_duplicate_failure_or_missing_package_end(self):
        names = ["TestAlpha", "TestBeta"]
        good = results(names)
        bad = [results(names[:1]), results(names + ["TestExtra"]),
               good[:-1], good + [event("pass")],
               [*good[:-1], event("pass", Test="TestAlpha"), event("pass")],
               [*good[:-1], event("fail")],
               [*good[:-1], event("fail", Test="TestAlpha/nested"), event("pass")],
               [*good[:-1], event("run", Test="TestExtra/nested"), event("pass")],
               [*good[:-1], event("pass", Package="other")],
               [event("skip")]]
        for events in bad:
            with self.subTest(events=events), self.assertRaises(ValueError):
                shard.validate_result(events, names)

    def test_command_failure_cannot_be_hidden_by_existing_output(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(RuntimeError):
                shard.execute([sys.executable, "-c", "print('partial'); raise SystemExit(7)"],
                              Path(directory), "failed")
            self.assertIn("partial", Path(directory, "failed.jsonl").read_text())

    def test_commands_preserve_race_coverage_timeout_and_only_existing_postgres_skip(self):
        output = Path("/tmp/repository-test-output")
        for mode in ("normal", "postgres"):
            for selector in (None, "^(TestAlpha|FuzzInput)$"):
                args = shard.command(mode, output, selector)
                self.assertIn("-race", args)
                self.assertIn("-trimpath", args)
                self.assertIn("-json", args)
                self.assertEqual(args[-1], "./internal/task/repository/sqlite")
                self.assertIn("-timeout=90m" if mode == "postgres" else "-timeout=120m", args)
                self.assertEqual("-run" in args, selector is not None)
                self.assertEqual("-list" in args, selector is None)
                if mode == "normal":
                    self.assertIn("-covermode=atomic", args)
                    self.assertNotIn("-skip", args)
                elif selector is not None:
                    self.assertEqual(args[args.index("-skip") + 1],
                                     "^TestDeliveryRetentionStorage(ProtectedGap)?$/SQLite$")

    def test_real_go_enumeration_and_selectors_retain_nested_fuzz_and_examples(self):
        with tempfile.TemporaryDirectory(prefix="repository-shard-") as directory:
            root = Path(directory)
            (root / "go.mod").write_text("module github.com/kandev/kandev\n\ngo 1.20\n")
            package_dir = root / "internal/task/repository/sqlite"
            package_dir.mkdir(parents=True)
            (package_dir / "sample.go").write_text("package sqlite\nfunc value() int { return 1 }\n")
            (package_dir / "sample_test.go").write_text('''package sqlite
import ("testing"; "fmt")
func TestAlpha(t *testing.T) { t.Run("nested",func(t *testing.T){}) }
func TestBeta(t *testing.T) { t.Skip("explicit fixture skip") }
func TestGamma(t *testing.T) {}
func TestFutureFeature(t *testing.T) {}
func FuzzInput(f *testing.F) { f.Add("seed"); f.Fuzz(func(t *testing.T,s string){}) }
func Example() { fmt.Println("example")
// Output: example
}
''')
            def run(*args):
                result = subprocess.run(["go", "test", "-json", "-count=1", *args, "."],
                                        cwd=package_dir, capture_output=True, text=True, timeout=60)
                self.assertEqual(result.returncode, 0, result.stderr)
                return [json.loads(line) for line in result.stdout.splitlines()]
            native = shard.inventory(run("-list", "^(Test|Fuzz|Example)"))
            self.assertEqual(len(native), 6)
            all_names = []
            for index in (1, 2, 3):
                selected = shard.partition(native, index)
                events = run("-run", "^(" + "|".join(selected) + ")$")
                shard.validate_result(events, selected)
                all_names.extend(e["Test"] for e in events if e["Action"] in ("pass", "skip")
                                 and e.get("Test") and "/" not in e["Test"])
            self.assertEqual(sorted(all_names), native)
            output = root / "cohort"
            result = subprocess.run([sys.executable, str(SCRIPT), "--mode", "normal",
                                     "--shard", "1", "--output", str(output)],
                                    cwd=root, capture_output=True, text=True, timeout=120)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            report = json.loads((output / "selection.json").read_text())
            self.assertTrue(report["complete"])
            self.assertEqual(report["inventory"], native)
            self.assertEqual(report["selected"], shard.partition(native, 1))
            self.assertTrue((output / "coverage.out").is_file())


if __name__ == "__main__":
    unittest.main()
