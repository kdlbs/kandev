#!/usr/bin/env python3
"""Post idempotent release notices to eligible pull request contributors."""

from __future__ import annotations

import html
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import tomllib
from dataclasses import dataclass
from typing import Callable
from urllib.parse import quote, urlsplit


STABLE_TAG_RE = re.compile(r"^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:\.(?:0|[1-9][0-9]*))?$")
COMMIT_RE = re.compile(r"^(?:[0-9a-f]{40}|[0-9a-f]{64})$", re.IGNORECASE)
MAINTAINERS_RE = re.compile(r"{%\s*set\s+maintainers\s*=\s*(.*?)\s*%}", re.DOTALL)
URL_RE = re.compile(r"https?://[^\s<>\]\)\"']+", re.IGNORECASE)
RATE_LIMIT_RE = re.compile(r"rate limit|secondary rate limit|abuse detection", re.IGNORECASE)


class ReleaseNotificationError(Exception):
    """A safe, user-facing error that prevents the run from succeeding."""


class ApiError(Exception):
    def __init__(
        self,
        message: str,
        *,
        status: int | None = None,
        uncertain: bool | None = None,
        rate_limited: bool = False,
    ) -> None:
        super().__init__(message)
        self.status = status
        self.uncertain = (status is None or status >= 500) if uncertain is None else uncertain
        self.rate_limited = rate_limited or status == 429

    @property
    def stops_writes(self) -> bool:
        return self.status in {401, 403, 429} or self.rate_limited


class GitCommandError(Exception):
    pass


@dataclass(frozen=True)
class Release:
    release_id: int
    tag: str
    body: str
    url: str
    published_at: str


@dataclass(frozen=True)
class Candidate:
    number: int
    login: str
    merge_sha: str


@dataclass
class ResultRow:
    number: int | None
    login: str
    status: str
    detail: str
    comment_id: int | None = None


@dataclass
class RunSummary:
    release: Release
    comment: str
    rows: list[ResultRow]
    dry_run: bool
    duplicate_references: int = 0

    @property
    def counts(self) -> dict[str, int]:
        counts = {
            "planned": 0,
            "posted": 0,
            "already_notified": 0,
            "excluded": 0,
            "failed": 0,
            "unprocessed": 0,
        }
        for row in self.rows:
            key = "posted" if row.status == "posted (reconciled)" else row.status
            if key in counts:
                counts[key] += 1
        counts["skipped"] = counts["already_notified"] + counts["excluded"]
        return counts

    @property
    def failed(self) -> bool:
        return self.counts["failed"] > 0 or self.counts["unprocessed"] > 0


def _repository_parts(repository: str) -> tuple[str, str]:
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ReleaseNotificationError("GITHUB_REPOSITORY must contain an owner and repository name.")
    owner, name = repository.split("/", 1)
    return owner, name


def _release_url(release: Release, repository: str) -> str:
    return f"https://github.com/{repository}/releases/tag/{quote(release.tag, safe='')}"


def _visible_comment(release: Release, repository: str) -> str:
    return (
        f"This PR was included in [Kandev {release.tag}]({_release_url(release, repository)}). "
        "Thanks for contributing!"
    )


def render_comment(release: dict, repository: str) -> str:
    tag = release.get("tag_name")
    release_id = release.get("id")
    if not isinstance(tag, str) or not STABLE_TAG_RE.fullmatch(tag):
        raise ReleaseNotificationError("Cannot render a notice for a non-Stable release tag.")
    if not isinstance(release_id, int) or isinstance(release_id, bool) or release_id <= 0:
        raise ReleaseNotificationError("Cannot render a notice without a valid release identity.")
    value = Release(release_id, tag, "", "", "")
    return f"{_visible_comment(value, repository)}\n\n<!-- kandev-release-notice:{release_id} -->"


def parse_maintainers(path: str | Path) -> list[str]:
    try:
        config = tomllib.loads(Path(path).read_text(encoding="utf-8"))
        body = config["changelog"]["body"]
    except (OSError, KeyError, TypeError, tomllib.TOMLDecodeError) as error:
        raise ReleaseNotificationError("Could not read the maintainer declaration from cliff.toml.") from error

    if not isinstance(body, str):
        raise ReleaseNotificationError("The changelog body in cliff.toml must be a string.")
    declarations = MAINTAINERS_RE.findall(body)
    if len(declarations) != 1:
        raise ReleaseNotificationError("cliff.toml must contain exactly one maintainer declaration.")
    try:
        maintainers = json.loads(declarations[0])
    except json.JSONDecodeError as error:
        raise ReleaseNotificationError("The cliff.toml maintainer declaration must be a JSON-compatible list.") from error
    if (
        not isinstance(maintainers, list)
        or not maintainers
        or any(not isinstance(login, str) or not login.strip() for login in maintainers)
    ):
        raise ReleaseNotificationError("The cliff.toml maintainer declaration must contain maintainer logins.")
    return sorted({login.strip().casefold() for login in maintainers})


class GitHubApi:
    def __init__(
        self,
        repository: str,
        *,
        runner: Callable[..., subprocess.CompletedProcess[str]] | None = None,
    ) -> None:
        _repository_parts(repository)
        self.repository = repository
        self.runner = runner or subprocess.run

    @staticmethod
    def _status_from_stderr(stderr: str) -> int | None:
        match = re.search(r"\bHTTP\s+(\d{3})\b", stderr, re.IGNORECASE)
        if match is None:
            match = re.search(r"\b(401|403|404|422|429|5\d\d)\b", stderr)
        return int(match.group(1)) if match else None

    def _request(
        self,
        method: str,
        route: str,
        *,
        body: str | None = None,
        paginate: bool = False,
    ) -> object:
        command = ["gh", "api"]
        if paginate:
            command.extend(["--paginate", "--slurp"])
        command.extend(["--method", method, "--header", "Accept: application/vnd.github+json", route])
        if body is not None:
            command.extend(["--field", f"body={body}"])
        try:
            completed = self.runner(command, capture_output=True, text=True, check=False)
        except OSError as error:
            raise ApiError("GitHub CLI could not run.", uncertain=method == "POST") from error
        if completed.returncode != 0:
            stderr = completed.stderr or ""
            status = self._status_from_stderr(stderr)
            raise ApiError(
                f"GitHub API {method} request failed" + (f" (HTTP {status})." if status else "."),
                status=status,
                uncertain=(method == "POST" and (status is None or status >= 500)),
                rate_limited=bool(RATE_LIMIT_RE.search(stderr)),
            )
        try:
            return json.loads(completed.stdout)
        except (json.JSONDecodeError, TypeError) as error:
            raise ApiError(
                f"GitHub API {method} response was not valid JSON.",
                uncertain=method == "POST",
            ) from error

    def get_latest_release(self) -> dict:
        value = self._request("GET", f"repos/{self.repository}/releases/latest")
        if not isinstance(value, dict):
            raise ApiError("GitHub returned invalid release metadata.")
        return value

    def get_release_by_tag(self, tag: str) -> dict:
        value = self._request("GET", f"repos/{self.repository}/releases/tags/{quote(tag, safe='')}")
        if not isinstance(value, dict):
            raise ApiError("GitHub returned invalid release metadata.")
        return value

    def get_pull_request(self, number: int) -> dict:
        value = self._request("GET", f"repos/{self.repository}/pulls/{number}")
        if not isinstance(value, dict):
            raise ApiError("GitHub returned invalid pull request metadata.")
        return value

    def list_comments(self, number: int) -> list[dict]:
        value = self._request(
            "GET",
            f"repos/{self.repository}/issues/{number}/comments?per_page=100",
            paginate=True,
        )
        if not isinstance(value, list):
            raise ApiError("GitHub returned invalid pull request comments.")
        pages = value if all(isinstance(page, list) for page in value) else [value]
        comments = [comment for page in pages for comment in page]
        if any(not isinstance(comment, dict) for comment in comments):
            raise ApiError("GitHub returned an invalid pull request comment.")
        return comments

    def create_comment(self, number: int, body: str) -> dict:
        value = self._request(
            "POST",
            f"repos/{self.repository}/issues/{number}/comments",
            body=body,
        )
        if not isinstance(value, dict):
            raise ApiError("GitHub did not confirm the created comment.", uncertain=True)
        return value


class GitRepository:
    def __init__(self, *, runner: Callable[..., subprocess.CompletedProcess[str]] | None = None) -> None:
        self.runner = runner or subprocess.run

    def _run(self, command: list[str]) -> subprocess.CompletedProcess[str]:
        try:
            return self.runner(command, capture_output=True, text=True, check=False)
        except OSError as error:
            raise GitCommandError("Git could not run.") from error

    def resolve_tag(self, tag: str) -> str:
        if not STABLE_TAG_RE.fullmatch(tag):
            raise GitCommandError("The release tag is not a Stable version tag.")
        ref = f"refs/tags/{tag}^{{commit}}"
        result = self._run(["git", "rev-parse", "--verify", ref])
        if result.returncode != 0:
            fetch = self._run(["git", "fetch", "--no-tags", "origin", f"refs/tags/{tag}:refs/tags/{tag}"])
            if fetch.returncode != 0:
                raise GitCommandError("The selected release tag is not available in the checkout.")
            result = self._run(["git", "rev-parse", "--verify", ref])
        commit = (result.stdout or "").strip()
        if result.returncode != 0 or not COMMIT_RE.fullmatch(commit):
            raise GitCommandError("The selected release tag does not resolve to a commit.")
        return commit

    def is_ancestor(self, merge_sha: str, tag: str) -> bool:
        if not COMMIT_RE.fullmatch(merge_sha):
            raise GitCommandError("A pull request has an invalid merge commit identity.")
        result = self._run(["git", "merge-base", "--is-ancestor", merge_sha, f"refs/tags/{tag}"])
        if result.returncode == 0:
            return True
        if result.returncode == 1:
            return False
        raise GitCommandError("Git could not check whether the pull request belongs to the release.")


def _release_from_payload(payload: dict, repository: str) -> Release:
    if not isinstance(payload, dict):
        raise ReleaseNotificationError("GitHub returned invalid release metadata.")
    release_id = payload.get("id")
    tag = payload.get("tag_name")
    if not isinstance(release_id, int) or isinstance(release_id, bool) or release_id <= 0:
        raise ReleaseNotificationError("The selected release has an invalid identity.")
    if not isinstance(tag, str) or not STABLE_TAG_RE.fullmatch(tag):
        raise ReleaseNotificationError("The selected release tag is not a Stable semantic version.")
    if payload.get("draft") is not False or payload.get("prerelease") is not False:
        raise ReleaseNotificationError("The selected release must be published and must not be a prerelease.")
    published_at = payload.get("published_at")
    if not isinstance(published_at, str) or not published_at.strip():
        raise ReleaseNotificationError("The selected release has not been published.")
    body = payload.get("body", "")
    if body is None:
        body = ""
    if not isinstance(body, str):
        raise ReleaseNotificationError("The selected release notes are invalid.")
    release = Release(release_id, tag, body, "", published_at)
    return Release(release_id, tag, body, _release_url(release, repository), published_at)


def _extract_references(notes: str, repository: str) -> tuple[list[tuple[int, str]], int]:
    owner, name = _repository_parts(repository)
    references: list[tuple[int, str]] = []
    seen: set[tuple[int, str]] = set()
    duplicates = 0
    for raw_url in URL_RE.findall(notes):
        candidate = raw_url.rstrip(".,;:")
        parsed = urlsplit(candidate)
        if parsed.scheme.casefold() != "https" or parsed.netloc.casefold() != "github.com":
            continue
        segments = parsed.path.strip("/").split("/")
        if len(segments) != 4:
            continue
        linked_owner, linked_name, kind, number_text = segments
        if (linked_owner.casefold(), linked_name.casefold()) != (owner.casefold(), name.casefold()):
            continue
        if kind not in {"pull", "issues"} or not re.fullmatch(r"[0-9]+", number_text):
            continue
        number = int(number_text)
        if number <= 0:
            continue
        reference = (number, kind)
        if reference in seen:
            duplicates += 1
            continue
        seen.add(reference)
        references.append(reference)
    return references, duplicates


def _notice_exists(comments: list[dict], release: Release, repository: str) -> dict | None:
    marker = f"<!-- kandev-release-notice:{release.release_id} -->"
    visible = _visible_comment(release, repository)
    for comment in comments:
        body = comment.get("body")
        if not isinstance(body, str):
            continue
        if any(line.strip() == marker for line in body.splitlines()) or body.strip() == visible:
            return comment
    return None


def _comment_id(comment: dict | None) -> int | None:
    if comment is None:
        return None
    value = comment.get("id")
    return value if isinstance(value, int) and not isinstance(value, bool) and value > 0 else None


class ReleaseNotifier:
    def __init__(
        self,
        *,
        api: object,
        git: object,
        repository: str,
        maintainers: list[str],
        sleep_func: Callable[[float], None] = time.sleep,
    ) -> None:
        _repository_parts(repository)
        self.api = api
        self.git = git
        self.repository = repository
        self.maintainers = {login.casefold() for login in maintainers}
        self.sleep_func = sleep_func

    def _resolve_release(self, release_tag: str) -> Release:
        tag = release_tag.strip()
        if tag and not STABLE_TAG_RE.fullmatch(tag):
            raise ReleaseNotificationError("Release tag must be a Stable tag such as v1.2.3.")
        try:
            payload = self.api.get_release_by_tag(tag) if tag else self.api.get_latest_release()
        except ApiError as error:
            if error.status == 404:
                if tag:
                    raise ReleaseNotificationError(f"Published release tag {tag} was not found.") from error
                raise ReleaseNotificationError("No latest published Stable release was found.") from error
            raise ReleaseNotificationError("Could not read release metadata from GitHub.") from error
        except Exception as error:
            raise ReleaseNotificationError("Could not read release metadata from GitHub.") from error
        release = _release_from_payload(payload, self.repository)
        if tag and release.tag != tag:
            raise ReleaseNotificationError("GitHub returned a different release tag than the one requested.")
        return release

    @staticmethod
    def _excluded(number: int, reason: str, login: str = "") -> ResultRow:
        return ResultRow(number, login, "excluded", reason)

    def _candidate(self, number: int, tag: str) -> tuple[Candidate | None, ResultRow | None]:
        try:
            pull = self.api.get_pull_request(number)
        except ApiError as error:
            if error.status == 404:
                return None, self._excluded(number, "Release note reference does not identify a pull request.")
            raise ReleaseNotificationError(f"Could not read pull request #{number} metadata from GitHub.") from error
        except Exception as error:
            raise ReleaseNotificationError(f"Could not read pull request #{number} metadata from GitHub.") from error

        pull_number = pull.get("number")
        if not isinstance(pull_number, int) or isinstance(pull_number, bool) or pull_number != number:
            raise ReleaseNotificationError(f"GitHub returned unexpected metadata for pull request #{number}.")
        base = pull.get("base")
        base_repository = base.get("repo") if isinstance(base, dict) else None
        base_repo = base_repository.get("full_name") if isinstance(base_repository, dict) else None
        if not isinstance(base_repo, str):
            raise ReleaseNotificationError(f"GitHub returned incomplete base repository data for pull request #{number}.")
        if base_repo.casefold() != self.repository.casefold():
            return None, self._excluded(number, "Pull request targets a different repository.")
        user = pull.get("user")
        if not isinstance(user, dict) or not isinstance(user.get("login"), str):
            return None, self._excluded(number, "Pull request author is unavailable.")
        login = user["login"].strip()
        if not login:
            return None, self._excluded(number, "Pull request author is unavailable.")
        user_type = user.get("type")
        if user_type != "User" or login.casefold().endswith("[bot]"):
            return None, self._excluded(number, "Pull request author is not an external human.", login)
        if login.casefold() in self.maintainers:
            return None, self._excluded(number, "Pull request author is a maintainer.", login)
        if pull.get("state") != "closed" or not pull.get("merged_at"):
            return None, self._excluded(number, "Pull request is not merged.", login)
        merge_sha = pull.get("merge_commit_sha")
        if not isinstance(merge_sha, str) or not COMMIT_RE.fullmatch(merge_sha):
            return None, self._excluded(number, "Pull request merge commit is unavailable.", login)
        try:
            belongs_to_release = self.git.is_ancestor(merge_sha, tag)
        except Exception as error:
            raise ReleaseNotificationError(f"Could not verify pull request #{number} against the release tag.") from error
        if not belongs_to_release:
            return None, self._excluded(number, "Pull request merge commit is outside the selected release.", login)
        return Candidate(number, login, merge_sha), None

    def run(self, *, release_tag: str = "", dry_run: bool = False) -> RunSummary:
        release = self._resolve_release(release_tag)
        try:
            self.git.resolve_tag(release.tag)
        except Exception as error:
            raise ReleaseNotificationError("Could not resolve the selected release tag in Git.") from error
        comment = render_comment(
            {"id": release.release_id, "tag_name": release.tag},
            self.repository,
        )
        visible_comment = _visible_comment(release, self.repository)
        references, duplicate_count = _extract_references(release.body, self.repository)
        rows: list[ResultRow] = []
        eligible: list[Candidate] = []

        for number, kind in references:
            if kind == "issues":
                rows.append(self._excluded(number, "Release notes link to an issue, not a pull request."))
                continue
            candidate, excluded = self._candidate(number, release.tag)
            if excluded is not None:
                rows.append(excluded)
            elif candidate is not None:
                eligible.append(candidate)

        # Read every eligible conversation before the first write. A failed
        # preflight read must never leave a partially notified release.
        planned: list[Candidate] = []
        for candidate in eligible:
            try:
                comments = self.api.list_comments(candidate.number)
            except Exception as error:
                raise ReleaseNotificationError(
                    f"Could not read comments for pull request #{candidate.number}; no comments were posted."
                ) from error
            existing = _notice_exists(comments, release, self.repository)
            if existing is None:
                planned.append(candidate)
                rows.append(ResultRow(candidate.number, candidate.login, "planned", "Ready to notify."))
            else:
                rows.append(
                    ResultRow(
                        candidate.number,
                        candidate.login,
                        "already_notified",
                        "A notice for this release already exists.",
                        _comment_id(existing),
                    )
                )

        summary = RunSummary(release, comment, rows, dry_run, duplicate_count)
        if dry_run:
            return summary

        post_attempts = 0
        stop_reason = ""
        for candidate in planned:
            row = next(
                row
                for row in summary.rows
                if row.number == candidate.number and row.status == "planned"
            )
            try:
                comments = self.api.list_comments(candidate.number)
            except Exception as error:
                row.status = "failed"
                row.detail = f"Could not recheck pull request #{candidate.number} before posting."
                if isinstance(error, ApiError) and error.stops_writes:
                    stop_reason = "GitHub authorization or rate limiting stopped further writes."
                if stop_reason:
                    break
                continue
            existing = _notice_exists(comments, release, self.repository)
            if existing is not None:
                row.status = "already_notified"
                row.detail = "A notice appeared before this run could post another one."
                row.comment_id = _comment_id(existing)
                continue

            if post_attempts:
                self.sleep_func(1)
            post_attempts += 1
            try:
                response = self.api.create_comment(candidate.number, comment)
                response_id = _comment_id(response)
                if response.get("body") != comment or response_id is None:
                    raise ApiError("GitHub did not confirm the created comment.", uncertain=True)
                row.status = "posted"
                row.detail = "Notice posted."
                row.comment_id = response_id
                continue
            except Exception as error:
                api_error = error if isinstance(error, ApiError) else ApiError("Comment write result is uncertain.")
                if api_error.uncertain:
                    try:
                        comments = self.api.list_comments(candidate.number)
                    except Exception as read_error:
                        comments = []
                        if isinstance(read_error, ApiError) and read_error.stops_writes:
                            stop_reason = "GitHub authorization or rate limiting stopped further writes."
                    reconciled = _notice_exists(comments, release, self.repository)
                    if reconciled is not None:
                        row.status = "posted (reconciled)"
                        row.detail = "GitHub stored the notice before the write response was lost."
                        row.comment_id = _comment_id(reconciled)
                        continue
                row.status = "failed"
                row.detail = "GitHub could not confirm this notice. Rerun to reconcile before retrying."
                if api_error.stops_writes:
                    stop_reason = "GitHub authorization or rate limiting stopped further writes."
                if stop_reason:
                    break

        if stop_reason:
            completed_numbers = {
                row.number
                for row in summary.rows
                if row.status not in {"planned"} and row.number is not None
            }
            for candidate in planned:
                if candidate.number in completed_numbers:
                    continue
                row = next(
                    row
                    for row in summary.rows
                    if row.number == candidate.number and row.status == "planned"
                )
                row.status = "unprocessed"
                row.detail = stop_reason
        return summary


def _markdown_cell(value: str) -> str:
    normalized = html.escape(value, quote=False).replace("|", "\\|")
    normalized = normalized.replace("`", "\\`").replace("\r", " ").replace("\n", " ")
    return normalized


def render_summary(summary: RunSummary, repository: str) -> str:
    counts = summary.counts
    visible = summary.comment.split("\n\n<!--", 1)[0]
    mode = "Preview only. No comments were posted." if summary.dry_run else "Posting run."
    lines = [
        "## Release contributor notifications",
        "",
        f"Release: [{_markdown_cell(summary.release.tag)}]({summary.release.url}) (release ID {summary.release.release_id})",
        "",
        f"Mode: {mode}",
        "",
        "Comment text:",
        "",
        f"> {_markdown_cell(visible)}",
        "",
        "| Target PR | Contributor | Result | Details |",
        "| --- | --- | --- | --- |",
    ]
    for row in summary.rows:
        target = (
            f"[#{row.number}](https://github.com/{repository}/pull/{row.number})"
            if row.number is not None
            else "Release notes"
        )
        detail = _markdown_cell(row.detail)
        if row.comment_id is not None and row.number is not None:
            detail += f" ([comment](https://github.com/{repository}/pull/{row.number}#issuecomment-{row.comment_id}))"
        lines.append(
            f"| {target} | {_markdown_cell(row.login)} | {_markdown_cell(row.status)} | {detail} |"
        )
    lines.extend(
        [
            "",
            f"Posted: {counts['posted']}; already notified: {counts['already_notified']}; "
            f"excluded: {counts['excluded']}; skipped: {counts['skipped']}; "
            f"planned: {counts['planned']}; failed: {counts['failed']}; unprocessed: {counts['unprocessed']}.",
        ]
    )
    if summary.duplicate_references:
        lines.append(f"Duplicate release-note references ignored: {summary.duplicate_references}.")
    return "\n".join(lines) + "\n"


def _failure_summary(message: str, release_tag: str) -> str:
    safe_message = _markdown_cell(message)
    requested = _markdown_cell(release_tag.strip() or "latest published Stable release")
    return (
        "## Release contributor notifications\n\n"
        f"Requested release: {requested}\n\n"
        "Preflight failed before a comment was posted.\n\n"
        f"Error: {safe_message}\n"
    )


def _write_summary(summary: str, path: str | None) -> None:
    if path:
        with Path(path).open("a", encoding="utf-8") as output:
            output.write(summary)
    else:
        print(summary, end="")


def _github_log_value(value: str) -> str:
    return value.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")


def _parse_boolean(value: str) -> bool:
    normalized = value.strip().casefold()
    if normalized in {"", "false"}:
        return False
    if normalized == "true":
        return True
    raise ReleaseNotificationError("DRY_RUN must be either true or false.")


def main() -> int:
    repository = os.environ.get("GITHUB_REPOSITORY", "")
    release_tag = os.environ.get("RELEASE_TAG", "")
    summary_path = os.environ.get("GITHUB_STEP_SUMMARY")
    try:
        _repository_parts(repository)
        dry_run = _parse_boolean(os.environ.get("DRY_RUN", "false"))
        for label, key in (
            ("Caller workflow ref", "GITHUB_WORKFLOW_REF"),
            ("Caller workflow SHA", "GITHUB_WORKFLOW_SHA"),
            ("Reusable workflow ref", "JOB_WORKFLOW_REF"),
            ("Reusable workflow SHA", "JOB_WORKFLOW_SHA"),
            ("Helper checkout SHA", "HELPER_CHECKOUT_SHA"),
        ):
            value = os.environ.get(key, "")
            if value:
                print(f"{label}: {_github_log_value(value)}")
        root = Path(__file__).resolve().parents[2]
        maintainers = parse_maintainers(root / "cliff.toml")
        notifier = ReleaseNotifier(
            api=GitHubApi(repository),
            git=GitRepository(),
            repository=repository,
            maintainers=maintainers,
        )
        result = notifier.run(release_tag=release_tag, dry_run=dry_run)
    except ReleaseNotificationError as error:
        message = str(error)
        _write_summary(_failure_summary(message, release_tag), summary_path)
        print(f"::error::{_github_log_value(message)}", file=sys.stderr)
        return 1
    except OSError:
        message = "Could not write the workflow summary."
        print(f"::error::{message}", file=sys.stderr)
        return 1

    rendered = render_summary(result, repository)
    _write_summary(rendered, summary_path)
    counts = result.counts
    print(
        "Contributor notification counts: "
        f"posted={counts['posted']}, already_notified={counts['already_notified']}, "
        f"excluded={counts['excluded']}, failed={counts['failed']}, "
        f"unprocessed={counts['unprocessed']}"
    )
    if result.failed:
        print("::error::One or more contributor notices failed or remain unprocessed.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
