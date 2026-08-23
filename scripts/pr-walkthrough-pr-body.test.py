#!/usr/bin/env python3
"""Tests for the trusted PR walkthrough body updater."""

import json
from pathlib import Path
import subprocess
import tempfile
import unittest


REPO_ROOT = Path(__file__).resolve().parent.parent
SCRIPT = REPO_ROOT / "scripts" / "pr-walkthrough-pr-body"
PR_NUMBER = 2906
HEAD_SHA = "0123456789abcdef0123456789abcdef01234567"
SHORT_HEAD_SHA = HEAD_SHA[:12]
URL = f"https://walkthrough.kandev.ai/pr/{PR_NUMBER}/{SHORT_HEAD_SHA}.html"
START = "<!-- kandev-pr-walkthrough-start -->"
END = "<!-- kandev-pr-walkthrough-end -->"


class PRWalkthroughPRBodyTest(unittest.TestCase):
    def run_updater(
        self,
        body: str | None,
        *,
        url: str = URL,
        response_number: int = PR_NUMBER,
        response_sha: str = HEAD_SHA,
    ) -> tuple[subprocess.CompletedProcess[str], Path, tempfile.TemporaryDirectory[str]]:
        temp_dir = tempfile.TemporaryDirectory()
        root = Path(temp_dir.name)
        response = root / "pr.json"
        output = root / "payload.json"
        response.write_text(
            json.dumps(
                {
                    "number": response_number,
                    "body": body,
                    "head": {"sha": response_sha},
                }
            ),
            encoding="utf-8",
        )
        result = subprocess.run(
            [
                "python3",
                str(SCRIPT),
                "--github-response",
                str(response),
                "--output",
                str(output),
                "--url",
                url,
                "--pr-number",
                str(PR_NUMBER),
                "--head-sha",
                HEAD_SHA,
            ],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
        return result, output, temp_dir

    def payload_body(self, output: Path) -> str:
        return json.loads(output.read_text(encoding="utf-8"))["body"]

    def test_prepends_prominent_callout_without_changing_existing_body(self) -> None:
        original = "Summary paragraph.\n\n## Important Changes\n\n- Existing content\n"
        result, output, temp_dir = self.run_updater(original)
        self.addCleanup(temp_dir.cleanup)

        self.assertEqual(result.returncode, 0, result.stderr)
        updated = self.payload_body(output)
        self.assertTrue(updated.startswith(f"{START}\n> [!TIP]\n"))
        self.assertIn(
            f"> **PR walkthrough:** [Open the visual walkthrough]({URL})",
            updated,
        )
        self.assertEqual(updated.split(f"{END}\n\n", 1)[1], original)

    def test_replaces_the_owned_url_without_duplicating_the_block(self) -> None:
        old_sha = "b" * 40
        old_url = f"https://walkthrough.kandev.ai/pr/{PR_NUMBER}/{old_sha}.html"
        original = (
            f"{START}\n> [!TIP]\n"
            f"> **PR walkthrough:** [Open the visual walkthrough]({old_url})\n"
            f"{END}\n\nContributor content"
        )
        result, output, temp_dir = self.run_updater(original)
        self.addCleanup(temp_dir.cleanup)

        self.assertEqual(result.returncode, 0, result.stderr)
        updated = self.payload_body(output)
        self.assertEqual(updated.count(START), 1)
        self.assertEqual(updated.count(END), 1)
        self.assertIn(URL, updated)
        self.assertNotIn(old_url, updated)
        self.assertTrue(updated.endswith("\n\nContributor content"))

    def test_reports_an_unchanged_body_without_writing_a_payload(self) -> None:
        original = (
            f"{START}\n> [!TIP]\n"
            f"> **PR walkthrough:** [Open the visual walkthrough]({URL})\n"
            f"{END}\n\nContributor content"
        )
        result, output, temp_dir = self.run_updater(original)
        self.addCleanup(temp_dir.cleanup)

        self.assertEqual(result.returncode, 3, result.stderr)
        self.assertFalse(output.exists())

    def test_accepts_an_empty_github_body(self) -> None:
        result, output, temp_dir = self.run_updater(None)
        self.addCleanup(temp_dir.cleanup)

        self.assertEqual(result.returncode, 0, result.stderr)
        updated = self.payload_body(output)
        self.assertTrue(updated.startswith(START))
        self.assertTrue(updated.endswith(END))

    def test_rejects_a_url_that_does_not_match_the_event_identity(self) -> None:
        result, output, temp_dir = self.run_updater(
            "Contributor content",
            url=f"https://walkthrough.kandev.ai/pr/999/{HEAD_SHA}.html",
        )
        self.addCleanup(temp_dir.cleanup)

        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(output.exists())
        self.assertIn("expected walkthrough URL", result.stderr)

    def test_rejects_a_github_response_for_another_pull_request(self) -> None:
        result, output, temp_dir = self.run_updater(
            "Contributor content",
            response_number=PR_NUMBER + 1,
        )
        self.addCleanup(temp_dir.cleanup)

        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(output.exists())
        self.assertIn("number does not match", result.stderr)

    def test_allows_the_pr_head_to_advance_while_publication_finishes(self) -> None:
        result, output, temp_dir = self.run_updater(
            "Contributor content",
            response_sha="c" * 40,
        )
        self.addCleanup(temp_dir.cleanup)

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(URL, self.payload_body(output))

    def test_rejects_unbalanced_or_non_leading_owned_markers(self) -> None:
        for body in (
            f"{START}\nmissing end marker",
            f"Contributor content\n\n{START}\nowned\n{END}",
            f"{START}\none\n{END}\n{START}\ntwo\n{END}",
        ):
            with self.subTest(body=body):
                result, output, temp_dir = self.run_updater(body)
                self.addCleanup(temp_dir.cleanup)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(output.exists())
                self.assertIn("marker", result.stderr)


if __name__ == "__main__":
    unittest.main()
