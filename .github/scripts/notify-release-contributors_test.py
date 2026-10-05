#!/usr/bin/env python3
"""Behavior tests for release contributor notification selection and recovery."""

import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT = REPO_ROOT / ".github" / "scripts" / "notify-release-contributors.py"
SPEC = importlib.util.spec_from_file_location("notify_release_contributors", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
notify = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = notify
SPEC.loader.exec_module(notify)

REPOSITORY = "kdlbs/kandev"
TAG = "v1.2.3"
RELEASE = {
    "id": 73,
    "tag_name": TAG,
    "name": TAG,
    "body": "Included: https://github.com/kdlbs/kandev/pull/1",
    "html_url": f"https://github.com/{REPOSITORY}/releases/tag/{TAG}",
    "published_at": "2026-01-02T03:04:05Z",
    "draft": False,
    "prerelease": False,
}


def pull_request(
    number: int,
    *,
    login: str = "external-contributor",
    user_type: str = "User",
    state: str = "closed",
    merged: bool = True,
    base_repository: str = REPOSITORY,
    merge_sha: str | None = None,
) -> dict:
    return {
        "number": number,
        "state": state,
        "merged_at": "2026-01-01T00:00:00Z" if merged else None,
        "merge_commit_sha": merge_sha or f"{number:040x}",
        "base": {"repo": {"full_name": base_repository}},
        "user": {"login": login, "type": user_type},
    }


class FakeApi:
    def __init__(self, *, release: dict | None = None, prs: dict | None = None) -> None:
        self.release = release or dict(RELEASE)
        self.prs = prs or {1: pull_request(1)}
        self.latest_calls = 0
        self.tag_calls: list[str] = []
        self.pull_calls: list[int] = []
        self.comment_reads: list[int] = []
        self.posts: list[tuple[int, str]] = []
        self.comments: dict[int, list[dict]] = {}
        self.read_failures: dict[int, Exception] = {}
        self.post_failures: dict[int, Exception] = {}
        self.post_side_effect_before_failure: set[int] = set()

    def get_latest_release(self) -> dict:
        self.latest_calls += 1
        if self.release is None:
            raise notify.ApiError("not found", status=404)
        return self.release

    def get_release_by_tag(self, tag: str) -> dict:
        self.tag_calls.append(tag)
        if tag != self.release.get("tag_name"):
            raise notify.ApiError("not found", status=404)
        return self.release

    def get_pull_request(self, number: int) -> dict:
        self.pull_calls.append(number)
        if number not in self.prs:
            raise notify.ApiError("not found", status=404)
        return self.prs[number]

    def list_comments(self, number: int) -> list[dict]:
        self.comment_reads.append(number)
        if number in self.read_failures:
            raise self.read_failures[number]
        return list(self.comments.get(number, []))

    def create_comment(self, number: int, body: str) -> dict:
        self.posts.append((number, body))
        comment = {
            "id": len(self.posts),
            "body": body,
            "html_url": f"https://github.com/{REPOSITORY}/pull/{number}#issuecomment-{len(self.posts)}",
        }
        if number in self.post_side_effect_before_failure:
            self.comments.setdefault(number, []).append(comment)
        if number in self.post_failures:
            raise self.post_failures[number]
        self.comments.setdefault(number, []).append(comment)
        return comment


class FakeGit:
    def __init__(self, *, included: set[str] | None = None) -> None:
        self.included = included
        self.resolved_tags: list[str] = []
        self.ancestry_checks: list[tuple[str, str]] = []

    def resolve_tag(self, tag: str) -> str:
        self.resolved_tags.append(tag)
        return "f" * 40

    def is_ancestor(self, merge_sha: str, tag: str) -> bool:
        self.ancestry_checks.append((merge_sha, tag))
        return self.included is None or merge_sha in self.included


def run_notifier(
    api: FakeApi,
    *,
    git: FakeGit | None = None,
    notes: str | None = None,
    maintainers: list[str] | None = None,
    sleep_calls: list[float] | None = None,
    dry_run: bool = True,
) -> object:
    if notes is not None:
        api.release = dict(api.release, body=notes)
    return notify.ReleaseNotifier(
        api=api,
        git=git or FakeGit(),
        repository=REPOSITORY,
        maintainers=maintainers or [],
        sleep_func=(sleep_calls.append if sleep_calls is not None else lambda _: None),
    ).run(dry_run=dry_run)


class ReleaseContributorNotificationsTest(unittest.TestCase):
    def test_latest_release_preview_uses_one_resolution_and_never_posts(self) -> None:
        api = FakeApi()
        result = run_notifier(api)

        self.assertEqual(api.latest_calls, 1)
        self.assertEqual(api.tag_calls, [])
        self.assertEqual(api.posts, [])
        self.assertEqual(result.release.tag, TAG)
        self.assertEqual([row.status for row in result.rows], ["planned"])

    def test_normal_run_posts_a_notice(self) -> None:
        api = FakeApi()

        result = run_notifier(api, dry_run=False)

        self.assertEqual([row.status for row in result.rows], ["posted"])
        self.assertEqual(len(api.posts), 1)

    def test_explicit_missing_tag_fails_without_falling_back_to_latest(self) -> None:
        api = FakeApi()
        api.release = dict(RELEASE, tag_name="v9.9.9")

        with self.assertRaises(notify.ReleaseNotificationError):
            notify.ReleaseNotifier(
                api=api,
                git=FakeGit(),
                repository=REPOSITORY,
                maintainers=[],
            ).run(release_tag="v1.2.3")

        self.assertEqual(api.tag_calls, ["v1.2.3"])
        self.assertEqual(api.latest_calls, 0)
        self.assertEqual(api.posts, [])

    def test_latest_release_must_be_published_stable_semver(self) -> None:
        for changes in (
            {"draft": True},
            {"prerelease": True},
            {"published_at": None},
            {"tag_name": "v1.2.3-rc.1"},
        ):
            with self.subTest(changes=changes):
                api = FakeApi(release=dict(RELEASE, **changes))
                with self.assertRaises(notify.ReleaseNotificationError):
                    run_notifier(api)
                self.assertEqual(api.posts, [])

    def test_duplicate_pull_links_are_resolved_once_and_issue_links_are_ignored(self) -> None:
        api = FakeApi()
        notes = (
            "https://github.com/kdlbs/kandev/pull/1 "
            "[repeat](https://github.com/kdlbs/kandev/pull/1) "
            "https://github.com/kdlbs/kandev/issues/2 "
            "https://github.com/other/project/pull/3"
        )

        result = run_notifier(api, notes=notes)

        self.assertEqual(api.pull_calls, [1])
        self.assertEqual(result.counts["planned"], 1)
        self.assertEqual(api.posts, [])

    def test_no_eligible_prs_produces_no_comment_writes(self) -> None:
        api = FakeApi(prs={1: pull_request(1, user_type="Bot")})

        result = run_notifier(api, dry_run=False)

        self.assertEqual(result.counts["excluded"], 1)
        self.assertEqual(api.posts, [])

    def test_bots_maintainers_open_prs_and_unrelated_merges_are_excluded(self) -> None:
        prs = {
            1: pull_request(1),
            2: pull_request(2, user_type="Bot"),
            3: pull_request(3, login="CARLOSFLORENCIO"),
            4: pull_request(4, state="open", merged=False),
            5: pull_request(5),
            6: pull_request(6, base_repository="another/repository"),
        }
        api = FakeApi(prs=prs)
        git = FakeGit(included={prs[1]["merge_commit_sha"]})
        notes = " ".join(
            f"https://github.com/{REPOSITORY}/pull/{number}" for number in prs
        )

        result = run_notifier(
            api,
            git=git,
            notes=notes,
            maintainers=["carlosflorencio"],
        )

        self.assertEqual(api.pull_calls, [1, 2, 3, 4, 5, 6])
        self.assertEqual([row.number for row in result.rows if row.status == "planned"], [1])
        self.assertEqual(result.counts["excluded"], 5)
        self.assertEqual(len(git.ancestry_checks), 2)
        self.assertEqual(api.posts, [])

    def test_paginated_comment_adapter_reads_every_page(self) -> None:
        pages = [
            [{"id": 1, "body": "first page"}],
            [{"id": 2, "body": "<!-- kandev-release-notice:73 -->"}],
        ]
        command_calls: list[list[str]] = []

        def runner(command: list[str], **_kwargs: object) -> object:
            command_calls.append(command)
            return type(
                "ProcessResult",
                (),
                {"returncode": 0, "stdout": json.dumps(pages), "stderr": ""},
            )()

        api = notify.GitHubApi(REPOSITORY, runner=runner)
        comments = api.list_comments(12)

        self.assertEqual([comment["id"] for comment in comments], [1, 2])
        release = notify._release_from_payload(RELEASE, REPOSITORY)
        self.assertIsNotNone(notify._notice_exists(comments, release, REPOSITORY))
        self.assertIn("--paginate", command_calls[0])
        self.assertIn("--slurp", command_calls[0])

    def test_marker_and_exact_legacy_notice_both_prevent_duplicates(self) -> None:
        for existing in (
            f"{notify.render_comment(RELEASE, REPOSITORY)}\n\n<!-- kandev-release-notice:73 -->",
            f"This PR was included in [Kandev {TAG}](https://github.com/{REPOSITORY}/releases/tag/{TAG}). Thanks for contributing!",
        ):
            with self.subTest(existing=existing):
                api = FakeApi()
                api.comments[1] = [{"id": 19, "body": existing}]
                result = run_notifier(api)
                self.assertEqual(result.counts["already_notified"], 1)
                self.assertEqual(api.posts, [])

    def test_manually_posted_v097_notice_is_recognized_without_a_marker(self) -> None:
        old_release = dict(RELEASE, tag_name="v0.97.0")
        old_visible = (
            "This PR was included in [Kandev v0.97.0]("
            f"https://github.com/{REPOSITORY}/releases/tag/v0.97.0). Thanks for contributing!"
        )
        api = FakeApi(release=old_release)
        api.comments[1] = [{"id": 50, "body": old_visible}]

        result = run_notifier(api)

        self.assertEqual(result.counts["already_notified"], 1)
        self.assertEqual(api.posts, [])

    def test_summary_escapes_metadata_and_reports_each_result_count(self) -> None:
        summary = run_notifier(FakeApi())
        summary.rows[0] = notify.ResultRow(
            1,
            "author|<script>\ncontinued",
            "excluded",
            "bad|value\nnext",
        )

        rendered = notify.render_summary(summary, REPOSITORY)

        self.assertIn("author\\|&lt;script&gt; continued", rendered)
        self.assertIn("bad\\|value next", rendered)
        self.assertIn("skipped: 1", rendered)
        self.assertIn("This PR was included in", rendered)

    def test_api_failure_message_does_not_expose_command_output(self) -> None:
        def runner(_command: list[str], **_kwargs: object) -> object:
            return type(
                "ProcessResult",
                (),
                {"returncode": 1, "stdout": "", "stderr": "token=very-secret HTTP 403 forbidden"},
            )()

        api = notify.GitHubApi(REPOSITORY, runner=runner)
        with self.assertRaises(notify.ApiError) as raised:
            api.get_latest_release()

        self.assertEqual(raised.exception.status, 403)
        self.assertNotIn("very-secret", str(raised.exception))

    def test_real_changelog_maintainer_declaration_is_the_authority(self) -> None:
        self.assertEqual(
            notify.parse_maintainers(REPO_ROOT / "cliff.toml"),
            ["carlosflorencio", "jcfs", "nnobre", "zeval"],
        )

    def test_read_failure_is_a_preflight_error_with_no_posts(self) -> None:
        api = FakeApi()
        api.read_failures[1] = notify.ApiError("forbidden", status=403)

        with self.assertRaises(notify.ReleaseNotificationError):
            run_notifier(api)

        self.assertEqual(api.posts, [])

    def test_partial_post_failure_preserves_success_and_waits_between_posts(self) -> None:
        api = FakeApi(prs={1: pull_request(1), 2: pull_request(2)})
        api.post_failures[1] = notify.ApiError("server error", status=500)
        notes = " ".join(f"https://github.com/{REPOSITORY}/pull/{number}" for number in (1, 2))
        sleep_calls: list[float] = []

        result = run_notifier(api, notes=notes, sleep_calls=sleep_calls, dry_run=False)

        self.assertEqual([row.status for row in result.rows], ["failed", "posted"])
        self.assertEqual(result.counts["failed"], 1)
        self.assertEqual(result.counts["posted"], 1)
        self.assertEqual([number for number, _ in api.posts], [1, 2])
        self.assertEqual(sleep_calls, [1])

        retry_preview = run_notifier(api, notes=notes)
        self.assertEqual(retry_preview.counts["already_notified"], 1)
        self.assertEqual(retry_preview.counts["planned"], 1)

    def test_uncertain_post_is_reconciled_from_conversation_comments(self) -> None:
        api = FakeApi()
        api.post_side_effect_before_failure.add(1)
        api.post_failures[1] = notify.ApiError("connection closed", uncertain=True)

        result = run_notifier(api, dry_run=False)

        self.assertEqual(result.rows[0].status, "posted (reconciled)")
        self.assertEqual(result.counts["posted"], 1)
        self.assertEqual(len(api.posts), 1)
        self.assertGreaterEqual(api.comment_reads.count(1), 2)

    def test_rate_limit_stops_writes_and_reports_remaining_recipients(self) -> None:
        notes = " ".join(f"https://github.com/{REPOSITORY}/pull/{number}" for number in (1, 2))
        for status, rate_limited in ((429, False), (403, True)):
            with self.subTest(status=status, rate_limited=rate_limited):
                api = FakeApi(prs={1: pull_request(1), 2: pull_request(2)})
                api.post_failures[1] = notify.ApiError(
                    "rate limit exceeded",
                    status=status,
                    rate_limited=rate_limited,
                )

                result = run_notifier(api, notes=notes, dry_run=False)

                self.assertEqual([row.status for row in result.rows], ["failed", "unprocessed"])
                self.assertEqual(len(api.posts), 1)

    def test_comment_text_is_exact_and_includes_release_identity(self) -> None:
        self.assertEqual(
            notify.render_comment(RELEASE, REPOSITORY),
            "This PR was included in [Kandev v1.2.3](https://github.com/kdlbs/kandev/releases/tag/v1.2.3). Thanks for contributing!\n\n<!-- kandev-release-notice:73 -->",
        )

    def test_malformed_or_duplicated_maintainer_declaration_fails(self) -> None:
        for body in (
            "{% set maintainers = ['single-quoted'] %}",
            "{% set maintainers = [\"one\"] %} {% set maintainers = [\"two\"] %}",
        ):
            with self.subTest(body=body), tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "cliff.toml"
                path.write_text(f'[changelog]\nbody = {json.dumps(body)}\n', encoding="utf-8")
                with self.assertRaises(notify.ReleaseNotificationError):
                    notify.parse_maintainers(path)


if __name__ == "__main__":
    unittest.main()
