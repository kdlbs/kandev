#!/usr/bin/env python3
"""Read one UTF-8 file from the head of the PR bound by the workflow."""

import base64
import binascii
import json
import os
from pathlib import PurePosixPath
import re
import subprocess
import sys
from urllib.parse import quote


MAX_FILE_BYTES = 256 * 1024
REPOSITORY_PATTERN = re.compile(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+")
SHA_PATTERN = re.compile(r"[0-9a-f]{40}")


class PrFileError(Exception):
    """A safe, user-facing validation or retrieval error."""


def require_environment() -> tuple[str, str]:
    repository = os.environ.get("GITHUB_REPOSITORY", "")
    pr_number = os.environ.get("CLAUDE_REVIEW_PR_NUMBER", "")
    if REPOSITORY_PATTERN.fullmatch(repository) is None:
        raise PrFileError("GITHUB_REPOSITORY is not a valid owner/repository")
    if re.fullmatch(r"[1-9][0-9]*", pr_number) is None:
        raise PrFileError("CLAUDE_REVIEW_PR_NUMBER is not a valid pull request number")
    return repository, pr_number


def validate_path(raw_path: str) -> str:
    path = PurePosixPath(raw_path)
    normalized_path = path.as_posix()
    if (
        not raw_path
        or raw_path.startswith("/")
        or "\\" in raw_path
        or any(part in ("", ".", "..") for part in path.parts)
        or any(ord(character) < 32 for character in raw_path)
        or normalized_path != raw_path
    ):
        raise PrFileError(
            "path must be a relative repository path without traversal"
        )
    return normalized_path


def gh_api(endpoint: str) -> object:
    try:
        result = subprocess.run(
            ["gh", "api", "--method", "GET", endpoint],
            check=True,
            capture_output=True,
            text=True,
        )
        return json.loads(result.stdout)
    except FileNotFoundError as error:
        raise PrFileError("GitHub CLI is unavailable") from error
    except subprocess.CalledProcessError as error:
        raise PrFileError("GitHub API request failed") from error
    except json.JSONDecodeError as error:
        raise PrFileError("GitHub API returned invalid JSON") from error


def get_head_sha(repository: str, pr_number: str) -> str:
    response = gh_api(f"repos/{repository}/pulls/{pr_number}")
    if not isinstance(response, dict):
        raise PrFileError("pull request response is invalid")
    head = response.get("head")
    sha = head.get("sha") if isinstance(head, dict) else None
    if not isinstance(sha, str) or SHA_PATTERN.fullmatch(sha) is None:
        raise PrFileError("pull request head SHA is invalid")
    return sha


def decode_file(response: object, requested_path: str) -> str:
    if not isinstance(response, dict):
        raise PrFileError("file response is invalid")
    if response.get("type") != "file" or "target" in response:
        raise PrFileError("requested path is not a regular file")
    if response.get("path") != requested_path:
        raise PrFileError("GitHub returned a different repository path")

    size = response.get("size")
    if not isinstance(size, int) or isinstance(size, bool) or size < 0:
        raise PrFileError("file size is invalid")
    if size > MAX_FILE_BYTES:
        raise PrFileError("requested file exceeds the 256 KiB review limit")
    if response.get("encoding") != "base64" or not isinstance(
        response.get("content"), str
    ):
        raise PrFileError("requested file has unsupported content encoding")

    try:
        encoded_content = "".join(response["content"].split())
        raw_content = base64.b64decode(encoded_content, validate=True)
    except (binascii.Error, ValueError) as error:
        raise PrFileError("requested file contains invalid base64 data") from error
    if len(raw_content) != size:
        raise PrFileError("requested file size does not match its content")
    try:
        return raw_content.decode("utf-8")
    except UnicodeDecodeError as error:
        raise PrFileError("requested file is not valid UTF-8 text") from error


def read_pr_file(raw_path: str) -> str:
    repository, pr_number = require_environment()
    requested_path = validate_path(raw_path)
    head_sha = get_head_sha(repository, pr_number)
    encoded_path = quote(requested_path, safe="/")
    endpoint = f"repos/{repository}/contents/{encoded_path}?ref={head_sha}"
    return decode_file(gh_api(endpoint), requested_path)


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: claude-read-pr-file.py <repository-relative-path>",
            file=sys.stderr,
        )
        return 2
    try:
        sys.stdout.write(read_pr_file(sys.argv[1]))
    except PrFileError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
