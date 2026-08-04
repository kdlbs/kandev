"""Tracked-file and git-ref access for deterministic scans."""

from __future__ import annotations

import subprocess
from pathlib import Path


class GitDiscoveryError(RuntimeError):
    """Raised when tracked repository state cannot be discovered."""


class ConfigurationError(RuntimeError):
    """Raised when linter configuration or tracked source cannot be read."""


def run_git(root: Path, *args: str) -> bytes:
    try:
        result = subprocess.run(
            ["git", "-C", str(root), *args],
            capture_output=True,
            check=False,
        )
    except OSError as exc:
        raise GitDiscoveryError(f"could not execute git: {exc}") from exc
    if result.returncode != 0:
        detail = result.stderr.decode("utf-8", errors="replace").strip()
        raise GitDiscoveryError(detail or f"git {' '.join(args)} failed")
    return result.stdout


def discover_root(cwd: Path) -> Path:
    try:
        output = run_git(cwd, "rev-parse", "--show-toplevel")
    except GitDiscoveryError as exc:
        raise GitDiscoveryError(f"unable to discover git repository: {exc}") from exc
    return Path(output.decode("utf-8").strip()).resolve()


def tracked_files(root: Path) -> list[str]:
    output = run_git(root, "ls-files", "-z", "--")
    return sorted(path for path in output.decode("utf-8").split("\0") if path)


def read_text(root: Path, relative: str) -> str:
    try:
        return (root / relative).read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        raise ConfigurationError(f"cannot read tracked file {relative}: {exc}") from exc
