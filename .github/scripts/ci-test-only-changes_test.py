#!/usr/bin/env python3
"""Conservative E2E exemption tests using Git's raw NUL diff format."""
import importlib.util
import contextlib
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("selection", Path(__file__).with_name("ci-test-only-changes.py"))
selection = importlib.util.module_from_spec(spec)
spec.loader.exec_module(selection)


def change(path, status="M", old="100644", new="100644", target=None):
    old_oid = ("0" if old == "000000" else "a") * 40
    new_oid = ("0" if new == "000000" else "b") * 40
    return (f":{old} {new} {old_oid} {new_oid} {status}\0{path}\0" + (f"{target}\0" if target else "")).encode()


class TestOnlyChangesTests(unittest.TestCase):
    def check(self, data, event="pull_request", embeds=()):
        return selection.classify(data, event, embeds)[0]

    # @covers AC-PLATFORM-CI-PERFORMANCE-006.1 through 006.4
    def test_pr_4349_go_tests_are_independent(self):
        files = [f"apps/backend/internal/orchestrator/executor/{name}_test.go" for name in
                 ("executor_credentials", "executor_host_gh_bridge", "executor_host_gh_fake")]
        self.assertTrue(self.check(b"".join(change(f) for f in files)))
        self.assertTrue(self.check(change(files[0]) + change("docs/example.md")))

    def test_regular_add_delete_and_test_rename_are_eligible(self):
        path = "apps/backend/internal/a/café_test.go"
        for data in (change(path, "A", "000000"), change(path, "D", new="000000"),
                     change(path, "R100", target="apps/backend/internal/b/other_test.go")):
            self.assertTrue(self.check(data))

    def test_production_rename_mixed_paths_and_nonregular_entries_keep_full_suite(self):
        test = "apps/backend/internal/a/value_test.go"
        for data in (change("apps/backend/internal/a/value.go", "R100", target=test),
                     change(test, "R100", target="apps/backend/internal/a/value.go"),
                     change(test, old="120000"), change(test, new="160000"),
                     change(test, "T"), change(test, "C100", target=test),
                     change(test)+change("apps/web/x.test.ts"),
                     change(test)+change("apps/cli/bin/cli.js"),
                     change("apps/backend/internal/a/testdata/value_test.go"),
                     change(test)+change(".github/workflows/e2e-tests.yml")):
            self.assertFalse(self.check(data), data)

    def test_malformed_unknown_or_empty_diff_never_exempts(self):
        for data in (b"", b"M\0test.go\0", change("../value_test.go"),
                     change("apps/backend/internal/a/value_test.go")[:-1],
                     change("apps/backend//value_test.go"), change("apps/backend/./value_test.go"),
                     change("apps/backend/value_test.go").replace(b"a"*40, b"a"*41),
                     change("apps/backend/value_test.go", "R999", target="apps/backend/other_test.go"),
                     change("apps/backend/value_test.go", "A", "000000").replace(b"0"*40, b"a"*40)):
            self.assertFalse(self.check(data))

    def test_embedded_tests_or_directory_content_remains_application_input(self):
        path = "apps/backend/internal/a/value_test.go"
        for pattern in ("apps/backend/internal/a/*", "apps/backend/internal/a", path):
            self.assertFalse(self.check(change(path), embeds=[pattern]))
        self.assertTrue(self.check(change(path), embeds=["apps/backend/internal/a/*.json"]))

    def test_non_pr_events_and_docs_alone_do_not_use_this_exemption(self):
        data = change("apps/backend/internal/a/value_test.go")
        for event in ("push", "merge_group", "workflow_dispatch", ""):
            self.assertFalse(self.check(data, event))
        self.assertFalse(self.check(change("docs/example.md")))

    def test_only_potential_exemptions_need_embedding_evidence(self):
        for path, reason in [("apps/web/component.tsx", "application-or-unknown-change"),
                             ("apps/backend/internal/a/value_test.go", "unverified-inputs")]:
            with self.subTest(path=path):
                output = io.StringIO()
                with mock.patch("sys.argv", ["selector", "--base", "base", "--event", "pull_request"]), \
                     mock.patch.dict(selection.os.environ, {"GITHUB_OUTPUT": ""}), \
                     mock.patch.object(selection, "git", side_effect=[b"a"*40, b"b"*40, change(path)]), \
                     mock.patch.object(selection, "embed_patterns", side_effect=ValueError("unavailable")), \
                     contextlib.redirect_stdout(output):
                    selection.main()
                self.assertEqual(output.getvalue(), f"run=true\nreason={reason}\n")

    def test_real_git_diff_and_embed_scan(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            def git(*args):
                return subprocess.check_output(["git", "-C", directory, *args], stderr=subprocess.DEVNULL)
            git("init"); git("config", "user.name", "Fixture"); git("config", "user.email", "fixture@example.com")
            source = root / "apps/backend/internal/a"
            source.mkdir(parents=True)
            (source / "value.go").write_text('package a\n//go:embed "*.json"\nvar data string\n')
            (source / "value_test.go").write_text("package a\n")
            git("add", "."); git("commit", "-m", "fixture")
            (source / "value_test.go").write_text("package a\n// changed\n")
            git("add", "."); git("commit", "-m", "change")
            raw = git("diff", "--raw", "-z", "--no-abbrev", "--no-renames", "HEAD~", "HEAD")
            embeds = selection.embed_patterns(directory, "HEAD")
            self.assertEqual(embeds, ["apps/backend/internal/a/*.json"])
            self.assertTrue(self.check(raw, embeds=embeds))
            for pattern in ('[!a]*', '`*.json`', '"\\x61_test.go"'):
                (source / "value.go").write_text(f'package a\n//go:embed {pattern}\nvar data string\n')
                git("add", "."); git("commit", "-m", "unsupported pattern")
                with self.assertRaises(ValueError, msg=pattern):
                    selection.embed_patterns(directory, "HEAD")


if __name__ == "__main__":
    unittest.main()
