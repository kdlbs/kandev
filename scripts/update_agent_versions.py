#!/usr/bin/env python3
"""Update Kandev's allowlisted core-agent npm package pins."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import NamedTuple, Sequence


STABLE_VERSION_RE = re.compile(r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$")


class Target(NamedTuple):
    path: str
    occurrences: int
    template: str = "{version}"


class Pin(NamedTuple):
    agent: str
    package: str
    source_path: str
    version_constant: str
    targets: tuple[Target, ...]


class Update(NamedTuple):
    agent: str
    package: str
    current: str
    latest: str

    def as_dict(self) -> dict[str, str]:
        return {
            "agent": self.agent,
            "package": self.package,
            "current": self.current,
            "latest": self.latest,
        }


AGENT_DIR = "apps/backend/internal/agent/agents"
LIFECYCLE_TEST = "apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go"
VERSION_DOC = f"{AGENT_DIR}/ACP_BRIDGE_VERSIONS.md"
NPM_METADATA_TIMEOUT_SECONDS = 30

PINS = (
    Pin(
        "Claude",
        "@agentclientprotocol/claude-agent-acp",
        f"{AGENT_DIR}/claude_acp.go",
        "claudeACPVersion",
        (
            Target(f"{AGENT_DIR}/claude_acp.go", 1),
            Target(f"{AGENT_DIR}/claude_acp_test.go", 2),
            Target(
                LIFECYCLE_TEST, 1, "@agentclientprotocol/claude-agent-acp@{version}"
            ),
            Target(
                VERSION_DOC,
                1,
                "| Claude | `@agentclientprotocol/claude-agent-acp` | `{version}` |",
            ),
        ),
    ),
    Pin(
        "Codex",
        "@agentclientprotocol/codex-acp",
        f"{AGENT_DIR}/codex_acp.go",
        "codexACPVersion",
        (
            Target(f"{AGENT_DIR}/codex_acp.go", 1),
            Target(f"{AGENT_DIR}/codex_acp_test.go", 2),
            Target(LIFECYCLE_TEST, 1, "@agentclientprotocol/codex-acp@{version}"),
            Target(
                VERSION_DOC,
                1,
                "| Codex | `@agentclientprotocol/codex-acp` | `{version}` |",
            ),
        ),
    ),
    Pin(
        "OpenCode",
        "opencode-ai",
        f"{AGENT_DIR}/opencode_acp.go",
        "opencodeACPVersion",
        (
            Target(f"{AGENT_DIR}/opencode_acp.go", 1),
            Target(f"{AGENT_DIR}/opencode_acp_test.go", 9),
            Target(
                VERSION_DOC, 1, "| OpenCode | `opencode-ai` | `{version}` |"
            ),
            Target(VERSION_DOC, 1, "opencode-ai@{version}"),
        ),
    ),
    Pin(
        "Copilot",
        "@github/copilot",
        f"{AGENT_DIR}/copilot_acp.go",
        "copilotACPVersion",
        (
            Target(f"{AGENT_DIR}/copilot_acp.go", 1),
            Target(f"{AGENT_DIR}/copilot_acp_test.go", 5),
            Target(
                VERSION_DOC,
                1,
                "| Copilot | `@github/copilot` | `{version}` |",
            ),
        ),
    ),
    Pin(
        "Gemini",
        "@google/gemini-cli",
        f"{AGENT_DIR}/gemini.go",
        "geminiVersion",
        (
            Target(f"{AGENT_DIR}/gemini.go", 1),
            Target(f"{AGENT_DIR}/gemini_test.go", 3),
            Target(
                VERSION_DOC,
                1,
                "| Gemini | `@google/gemini-cli` | `{version}` |",
            ),
        ),
    ),
)


def validate_version(version: str, package: str) -> None:
    if not STABLE_VERSION_RE.fullmatch(version):
        raise ValueError(
            f"{package} latest must be a stable semantic version, got {version!r}"
        )


def fetch_latest_version(package: str) -> str:
    completed = subprocess.run(
        ["npm", "view", package, "dist-tags.latest", "--json"],
        check=True,
        capture_output=True,
        text=True,
        timeout=NPM_METADATA_TIMEOUT_SECONDS,
    )
    version = json.loads(completed.stdout)
    if not isinstance(version, str):
        raise ValueError(f"{package} latest metadata must be a string, got {version!r}")
    validate_version(version, package)
    return version


def fetch_latest_versions(pins: Sequence[Pin] = PINS) -> dict[str, str]:
    return {pin.package: fetch_latest_version(pin.package) for pin in pins}


def read_current_version(root: Path, pin: Pin) -> str:
    source = (root / pin.source_path).read_text()
    pattern = re.compile(
        rf'(?m)^\s*(?:const\s+)?{re.escape(pin.version_constant)}\s*=\s*"([^"]+)"\s*$'
    )
    matches = pattern.findall(source)
    if len(matches) != 1:
        raise ValueError(
            f"{pin.source_path}: expected one {pin.version_constant} assignment, "
            f"found {len(matches)}"
        )
    current = matches[0]
    validate_version(current, pin.package)
    return current


def target_version_pattern(target: Target, version: str) -> re.Pattern[str]:
    prefix, marker, suffix = target.template.partition("{version}")
    if not marker or "{version}" in suffix:
        raise ValueError(f"{target.path}: target template must contain one {{version}} marker")
    return re.compile(
        rf"{re.escape(prefix)}(?<![0-9.]){re.escape(version)}(?![0-9.]){re.escape(suffix)}"
    )


def plan_updates(
    root: Path,
    latest_versions: dict[str, str],
    pins: Sequence[Pin] = PINS,
) -> tuple[list[Update], dict[Path, str]]:
    updates: list[Update] = []
    proposed: dict[Path, str] = {}

    for pin in pins:
        if pin.package not in latest_versions:
            raise ValueError(f"missing latest version for {pin.package}")
        latest = latest_versions[pin.package]
        validate_version(latest, pin.package)
        current = read_current_version(root, pin)

        for target in pin.targets:
            path = root / target.path
            content = proposed.get(path)
            if content is None:
                content = path.read_text()
            current_target = target.template.format(version=current)
            latest_target = target.template.format(version=latest)
            pattern = target_version_pattern(target, current)
            actual = len(pattern.findall(content))
            if actual != target.occurrences:
                raise ValueError(
                    f"{target.path}: expected {target.occurrences} occurrences of "
                    f"{current_target!r} for {pin.agent}, found {actual}"
                )
            if current != latest:
                proposed[path] = pattern.sub(lambda _: latest_target, content)

        if current != latest:
            updates.append(Update(pin.agent, pin.package, current, latest))

    return updates, proposed


def update_repository(
    root: Path,
    latest_versions: dict[str, str],
    pins: Sequence[Pin] = PINS,
) -> list[Update]:
    updates, proposed = plan_updates(root, latest_versions, pins)
    for path, content in proposed.items():
        path.write_text(content)
    return updates


def json_report(updates: Sequence[Update]) -> str:
    return json.dumps(
        {"updates": [update.as_dict() for update in updates]},
        indent=2,
        sort_keys=True,
    ) + "\n"


def markdown_report(updates: Sequence[Update]) -> str:
    if not updates:
        return "## Agent version updates\n\nNo core agent updates are available.\n"
    lines = [
        "## Agent version updates",
        "",
        "| Agent | Package | Current | Latest |",
        "| --- | --- | --- | --- |",
    ]
    lines.extend(
        f"| {update.agent} | `{update.package}` | `{update.current}` | `{update.latest}` |"
        for update in updates
    )
    lines.extend(
        [
            "",
            "Each agent update requires independent compatibility review. This pull request is not auto-merged.",
            "",
        ]
    )
    return "\n".join(lines)


def write_report(path: str | None, content: str) -> None:
    if path:
        Path(path).write_text(content)


def parse_args(argv: Sequence[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--root",
        type=Path,
        default=Path(__file__).resolve().parent.parent,
        help="repository root",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help=(
            "report available updates without changing files "
            "(exits 1 if updates are available, 0 if pins are current)"
        ),
    )
    parser.add_argument("--json-report", help="write the update report as JSON")
    parser.add_argument("--markdown-report", help="write the update report as Markdown")
    return parser.parse_args(argv)


def run(argv: Sequence[str]) -> int:
    args = parse_args(argv)
    latest_versions = fetch_latest_versions()
    updates, proposed = plan_updates(args.root, latest_versions)

    json_content = json_report(updates)
    markdown_content = markdown_report(updates)
    write_report(args.json_report, json_content)
    write_report(args.markdown_report, markdown_content)
    sys.stdout.write(markdown_content)

    if args.check:
        return 1 if updates else 0
    for path, content in proposed.items():
        path.write_text(content)
    return 0


def main() -> int:
    try:
        return run(sys.argv[1:])
    except (
        json.JSONDecodeError,
        OSError,
        subprocess.CalledProcessError,
        subprocess.TimeoutExpired,
        ValueError,
    ) as error:
        print(f"error: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
