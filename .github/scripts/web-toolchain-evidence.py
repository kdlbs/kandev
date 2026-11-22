#!/usr/bin/env python3
"""Bounded evidence harness for the native web verification toolchain.

This is not a task runner. It supports three explicit operations:

* ``compare-types`` provisions an exact reference compiler in an isolated
  temporary directory, runs the pinned reference and the candidate compilers on
  the same project or corpus, and rejects any diagnostic mismatch.
* ``compare-lint`` runs the existing ESLint host and the candidate Oxlint
  configuration, then compares complete findings and file selections.
* ``benchmark`` alternates complete package-command samples and applies the
  adoption thresholds. Every attempt is preserved, including failures.

Design constraints (see ``docs/specs/platform/system-design/web-native-verification.md``):

* every process is spawned with an argv array and an owned working directory;
* every started process is joined on success, failure, timeout and interruption;
* output, file inventories and process budgets are bounded;
* a missing binary, parser failure, empty selection, crash or incomplete report
  cannot produce a successful comparison;
* unavailable CPU or memory evidence is reported as unknown, not zero.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import resource
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path

SCHEMA_VERSION = 1
MAX_FILES = 200_000
MAX_OUTPUT_BYTES = 8 * 1024 * 1024
DEFAULT_TIMEOUT_SECONDS = 1800.0
DEFAULT_REFERENCE_TYPESCRIPT = "7.1.0-beta"
DEFAULT_TSRS = "0.20.1-ts7.1.0-beta"
DEFAULT_CHECKERS = 4

TYPE_EXTENSIONS = (".ts", ".tsx", ".mts", ".cts")
LINT_EXTENSIONS = (".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts")

_DIAGNOSTIC_RE = re.compile(
    r"^(?P<file>.*?)\((?P<line>\d+),(?P<column>\d+)\):\s+"
    r"(?P<severity>error|warning|message)\s+TS(?P<code>\d+):\s*(?P<message>.*)$"
)
_GLOBAL_DIAGNOSTIC_RE = re.compile(
    r"^(?P<severity>error|warning|message)\s+TS(?P<code>\d+):\s*(?P<message>.*)$"
)
_ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")


class EvidenceError(RuntimeError):
    """Evidence could not be produced or is not trustworthy."""

    def __init__(self, message: str, attempt: "Attempt | None" = None) -> None:
        super().__init__(message)
        self.attempt = attempt


@dataclass
class Attempt:
    """One bounded child-process observation."""

    label: str
    argv: list[str]
    cwd: str
    started_at: str
    exit_code: int | None
    timed_out: bool
    interrupted: bool
    elapsed_seconds: float
    cpu_seconds: float | None
    max_rss_kb: int | None
    stdout: str
    stderr: str
    stdout_truncated: bool
    stderr_truncated: bool
    unsupported: dict[str, str] = field(default_factory=dict)

    def to_dict(self) -> dict:
        return asdict(self)

    @property
    def completed(self) -> bool:
        return not self.timed_out and not self.interrupted and self.exit_code is not None


@dataclass
class NormalizedDiagnostic:
    file: str | None
    line: int
    column: int
    code: str
    severity: str
    message: str

    def key(self) -> tuple:
        return (
            self.file or "",
            self.line,
            self.column,
            self.code,
            self.severity,
            self.message,
        )

    def to_dict(self) -> dict:
        return asdict(self)


@dataclass
class Inventory:
    files: list[str]
    truncated: bool = False

    def to_dict(self) -> dict:
        return {
            "count": len(self.files),
            "files": self.files,
            "truncated": self.truncated,
        }


@dataclass
class Comparison:
    label: str
    reference_attempt: str
    candidate_attempt: str
    missing: list[dict]
    extra: list[dict]
    order_only: bool
    ok: bool
    reason: str | None = None

    def to_dict(self) -> dict:
        return asdict(self)


def _now_iso() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def _require_executable(path: str | None, label: str) -> str:
    """Resolve a binary without ever substituting a different workspace tool."""
    if not path:
        raise EvidenceError(f"{label} executable is required")
    candidate = Path(path)
    if candidate.is_dir():
        candidate = candidate / "node_modules" / ".bin" / Path(label).name
    if candidate.is_file() and os.access(candidate, os.X_OK):
        return str(candidate.resolve())
    resolved = shutil.which(path)
    if resolved:
        return resolved
    raise EvidenceError(f"{label} executable not found or not executable: {path}")


def _read_vmhwm_kb(pid: int) -> int | None:
    """Peak RSS for one Linux process; unavailable on other hosts."""
    try:
        with open(f"/proc/{pid}/status", "r", encoding="utf-8") as handle:
            for line in handle:
                if line.startswith("VmHWM:"):
                    return int(line.split()[1])
    except (OSError, ValueError):
        return None
    return None


def _read_bounded(path: Path) -> tuple[str, bool]:
    if not path.exists():
        return "", False
    with open(path, "rb") as handle:
        data = handle.read(MAX_OUTPUT_BYTES + 1)
    truncated = len(data) > MAX_OUTPUT_BYTES
    text = data[:MAX_OUTPUT_BYTES].decode("utf-8", errors="replace")
    return text, truncated


def run_process(
    label: str,
    argv: list[str],
    cwd: Path,
    *,
    timeout: float = DEFAULT_TIMEOUT_SECONDS,
    env: dict[str, str] | None = None,
    unset_env: list[str] | None = None,
) -> Attempt:
    """Run one child and always join it.

    Raises :class:`EvidenceError` for a missing binary or an interruption after
    the child has been killed and reaped.
    """
    if not isinstance(argv, list) or not argv:
        raise EvidenceError(f"{label}: argv must be a non-empty list")
    executable = _require_executable(argv[0], label)
    argv = [executable, *argv[1:]]

    started_at = _now_iso()
    start = time.monotonic()
    before_usage = resource.getrusage(resource.RUSAGE_CHILDREN)

    child_env = os.environ.copy()
    for key in unset_env or []:
        child_env.pop(key, None)
    if env:
        child_env.update(env)

    max_rss_kb: int | None = None
    stop_monitor = threading.Event()
    truncated = {"stdout": False, "stderr": False}

    def monitor() -> None:
        nonlocal max_rss_kb
        while not stop_monitor.is_set():
            rss = _read_vmhwm_kb(process.pid)
            if rss is not None and (max_rss_kb is None or rss > max_rss_kb):
                max_rss_kb = rss
            stop_monitor.wait(0.02)

    def read_stream(stream, buffer: bytearray, key: str) -> None:
        total = 0
        while True:
            chunk = stream.read(65536)
            if not chunk:
                return
            remaining = MAX_OUTPUT_BYTES - total
            if remaining > 0:
                buffer.extend(chunk[:remaining])
                total += min(len(chunk), remaining)
            if total >= MAX_OUTPUT_BYTES:
                truncated[key] = True
                _kill_process_group(process)
                return

    timed_out = False
    interrupted = False
    exit_code: int | None = None
    stdout_buffer = bytearray()
    stderr_buffer = bytearray()
    process = subprocess.Popen(
        argv,
        cwd=str(cwd),
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=child_env,
        start_new_session=True,
    )
    stdout_thread = threading.Thread(
        target=read_stream, args=(process.stdout, stdout_buffer, "stdout"), daemon=True
    )
    stderr_thread = threading.Thread(
        target=read_stream, args=(process.stderr, stderr_buffer, "stderr"), daemon=True
    )
    stdout_thread.start()
    stderr_thread.start()
    monitor_thread = threading.Thread(target=monitor, daemon=True)
    monitor_thread.start()
    try:
        exit_code = process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        _kill_process_group(process)
        exit_code = process.wait()
    except KeyboardInterrupt:
        interrupted = True
        _kill_process_group(process)
        exit_code = process.wait()
    finally:
        stop_monitor.set()
        monitor_thread.join(timeout=1.0)
        stdout_thread.join(timeout=5.0)
        stderr_thread.join(timeout=5.0)
        for stream in (process.stdout, process.stderr):
            try:
                stream.close()
            except (AttributeError, OSError):
                pass
    elapsed = time.monotonic() - start

    after_usage = resource.getrusage(resource.RUSAGE_CHILDREN)
    cpu_seconds = max(
        0.0,
        (after_usage.ru_utime + after_usage.ru_stime)
        - (before_usage.ru_utime + before_usage.ru_stime),
    )
    attempt = Attempt(
        label=label,
        argv=argv,
        cwd=str(cwd),
        started_at=started_at,
        exit_code=exit_code,
        timed_out=timed_out,
        interrupted=interrupted,
        elapsed_seconds=round(elapsed, 3),
        cpu_seconds=round(cpu_seconds, 3),
        max_rss_kb=max_rss_kb,
        stdout=bytes(stdout_buffer).decode("utf-8", errors="replace"),
        stderr=bytes(stderr_buffer).decode("utf-8", errors="replace"),
        stdout_truncated=truncated["stdout"],
        stderr_truncated=truncated["stderr"],
    )
    if interrupted:
        raise EvidenceError(
            f"{label}: interrupted after joining the child", attempt=attempt
        )
    return attempt


def _kill_process_group(process: subprocess.Popen) -> None:
    try:
        os.killpg(os.getpgid(process.pid), signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        try:
            process.kill()
        except ProcessLookupError:
            pass


def _strip_ansi(text: str) -> str:
    return _ANSI_RE.sub("", text)


def _normalize_message(message: str) -> str:
    return re.sub(r"\s+", " ", message).strip()


def _normalize_path(path: str, root: Path) -> str:
    candidate = Path(path)
    if not candidate.is_absolute():
        candidate = root / candidate
    try:
        resolved = candidate.resolve()
        return resolved.relative_to(root.resolve()).as_posix()
    except (OSError, ValueError):
        return path.replace("\\", "/")


def parse_diagnostics(raw: str, root: Path) -> list[NormalizedDiagnostic]:
    """Parse complete ``tsc --pretty false`` output.

    Any non-empty line that is neither a diagnostic nor a continuation line of a
    diagnostic makes the output untrustworthy.
    """
    if not raw.strip():
        return []
    diagnostics: list[NormalizedDiagnostic] = []
    current: NormalizedDiagnostic | None = None
    for raw_line in _strip_ansi(raw).splitlines():
        line = raw_line.rstrip()
        if not line.strip():
            continue
        match = _DIAGNOSTIC_RE.match(line) or _GLOBAL_DIAGNOSTIC_RE.match(line)
        if match:
            groups = match.groupdict()
            current = NormalizedDiagnostic(
                file=_normalize_path(groups["file"], root) if groups.get("file") else None,
                line=int(groups.get("line") or 0),
                column=int(groups.get("column") or 0),
                code=f"TS{groups['code']}",
                severity=groups["severity"],
                message=_normalize_message(groups["message"]),
            )
            diagnostics.append(current)
            continue
        if current is not None and raw_line.startswith((" ", "\t")):
            current.message = _normalize_message(f"{current.message} {line}")
            continue
        raise EvidenceError(f"unparseable compiler output: {line[:200]}")
    return diagnostics


def _diagnostic_multiset(diagnostics: list[NormalizedDiagnostic]) -> dict[tuple, int]:
    counts: dict[tuple, int] = {}
    for diagnostic in diagnostics:
        counts[diagnostic.key()] = counts.get(diagnostic.key(), 0) + 1
    return counts


def compare_type_diagnostics(
    reference: list[NormalizedDiagnostic],
    candidate: list[NormalizedDiagnostic],
    *,
    reference_attempt: str,
    candidate_attempt: str,
    label: str,
) -> Comparison:
    reference_counts = _diagnostic_multiset(reference)
    candidate_counts = _diagnostic_multiset(candidate)
    missing: list[NormalizedDiagnostic] = []
    extra: list[NormalizedDiagnostic] = []
    for key, count in reference_counts.items():
        remaining = count - candidate_counts.get(key, 0)
        for _ in range(max(0, remaining)):
            missing.append(_diagnostic_for_key(key, reference))
    for key, count in candidate_counts.items():
        remaining = count - reference_counts.get(key, 0)
        for _ in range(max(0, remaining)):
            extra.append(_diagnostic_for_key(key, candidate))
    order_only = (
        not missing
        and not extra
        and [item.key() for item in reference] != [item.key() for item in candidate]
    )
    ok = not missing and not extra
    return Comparison(
        label=label,
        reference_attempt=reference_attempt,
        candidate_attempt=candidate_attempt,
        missing=[item.to_dict() for item in missing],
        extra=[item.to_dict() for item in extra],
        order_only=order_only,
        ok=ok,
        reason=None if ok else "diagnostic mismatch",
    )


def _diagnostic_for_key(
    key: tuple, diagnostics: list[NormalizedDiagnostic]
) -> NormalizedDiagnostic:
    for diagnostic in diagnostics:
        if diagnostic.key() == key:
            return diagnostic
    raise AssertionError("diagnostic key missing from its own list")


def collect_type_inventory(
    web_root: Path,
    tsc: str,
    *,
    project: str,
    extra_args: list[str] | None = None,
    env: dict[str, str] | None = None,
) -> Inventory:
    attempt = run_process(
        "type-inventory",
        [tsc, "--listFilesOnly", "--project", project, "--pretty", "false", *(extra_args or [])],
        web_root,
        env=env,
    )
    files = _validate_inventory_attempt(attempt, web_root, "type-inventory")
    if len(files) > MAX_FILES:
        return Inventory(files=files[:MAX_FILES], truncated=True)
    return Inventory(files=files)


def provision_reference_typescript(
    tmp_dir: Path, version: str, attempts: list[Attempt] | None = None
) -> tuple[Path, Attempt]:
    """Install the exact reference compiler outside workspace dependencies."""
    npm = _require_executable("npm", "npm")
    attempt = run_process(
        "reference-install",
        [
            npm,
            "install",
            "--ignore-scripts",
            "--no-audit",
            "--no-fund",
            "--no-save",
            f"typescript@{version}",
        ],
        tmp_dir,
        timeout=600.0,
    )
    if attempts is not None:
        attempts.append(attempt)
    if attempt.exit_code != 0:
        raise EvidenceError(
            f"reference TypeScript {version} installation failed: {attempt.stderr[:500]}",
            attempt=attempt,
        )
    binary = tmp_dir / "node_modules" / ".bin" / "tsc"
    if not binary.exists():
        raise EvidenceError("reference installation did not provide a tsc binary")
    return binary, attempt


def tool_version(
    binary: str,
    cwd: Path,
    attempts: list[Attempt] | None = None,
    label: str = "version",
) -> str:
    attempt = run_process(label, [binary, "--version"], cwd, timeout=120.0)
    if attempts is not None:
        attempts.append(attempt)
    if attempt.exit_code != 0:
        raise EvidenceError(f"could not read version from {binary}: {attempt.stderr[:300]}")
    output = (attempt.stdout or attempt.stderr).strip().splitlines()
    if not output:
        raise EvidenceError(f"{binary} reported no version")
    return output[0]


def source_identity(root: Path, extensions: tuple[str, ...]) -> dict:
    """Bind a report to the exact source revision it observed."""
    digest = hashlib.sha256()
    count = 0
    for path in sorted(root.rglob("*")):
        if not path.is_file() or path.suffix not in extensions:
            continue
        relative = path.relative_to(root).as_posix()
        if "node_modules/" in relative or relative.startswith("dist/"):
            continue
        try:
            data = path.read_bytes()
        except OSError as error:
            raise EvidenceError(f"could not read source file {relative}: {error}") from error
        digest.update(relative.encode())
        digest.update(b"\0")
        digest.update(hashlib.sha256(data).digest())
        count += 1
        if count > MAX_FILES:
            raise EvidenceError("source inventory exceeded the file bound")
    if count == 0:
        raise EvidenceError("source inventory is empty")
    git_revision = None
    try:
        revision = subprocess.run(
            ["git", "-C", str(root), "rev-parse", "HEAD"],
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )
        if revision.returncode == 0:
            git_revision = revision.stdout.strip()
    except (OSError, subprocess.SubprocessError):
        git_revision = None
    return {
        "root": str(root.resolve()),
        "file_count": count,
        "sha256": f"sha256:{digest.hexdigest()}",
        "git_revision": git_revision,
    }


def config_identity(web_root: Path) -> dict:
    """Bind a measurement to the configuration inputs it actually ran."""
    digest = hashlib.sha256()
    files: dict[str, str] = {}
    candidates = [
        web_root / "tsconfig.json",
        web_root / "package.json",
        web_root.parent / "tsconfig.json",
        web_root.parent / "pnpm-lock.yaml",
    ]
    for path in candidates:
        if not path.is_file():
            continue
        relative = path.relative_to(web_root).as_posix()
        data = path.read_bytes()
        file_hash = hashlib.sha256(data).hexdigest()
        files[relative] = file_hash
        digest.update(relative.encode())
        digest.update(b"\0")
        digest.update(file_hash.encode())
        digest.update(b"\0")
    if not files:
        raise EvidenceError("configuration identity is empty")
    return {"files": files, "sha256": f"sha256:{digest.hexdigest()}"}


def _cache_state(web_root: Path, project: str) -> dict:
    config = _read_tsconfig(web_root / project)
    candidates = {web_root / "tsconfig.tsbuildinfo"}
    configured = config.get("tsBuildInfoFile")
    if isinstance(configured, str):
        candidates.add((web_root / configured).resolve())
    entries = []
    for path in sorted(candidates):
        if path.is_file():
            entries.append(
                {
                    "path": _normalize_path(str(path), web_root),
                    "bytes": path.stat().st_size,
                    "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
                }
            )
    return {
        "incremental": bool(config.get("incremental", False)),
        "tsbuildinfo": entries,
    }


def _read_tsconfig(path: Path) -> dict:
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as error:
        raise EvidenceError(f"could not read tsconfig {path}: {error}") from error
    text = _strip_json_comments(text)
    try:
        data = json.loads(text)
    except json.JSONDecodeError as error:
        raise EvidenceError(f"invalid tsconfig {path}: {error}") from error
    if not isinstance(data, dict):
        raise EvidenceError(f"tsconfig {path} must contain an object")
    return data


def _strip_json_comments(text: str) -> str:
    """Remove JSONC comments without touching comment-like characters in strings.

    ``tsconfig.json`` contains path patterns such as ``"@/*"``; a naive regular
    expression would corrupt them.
    """
    output: list[str] = []
    index = 0
    in_string = False
    escaped = False
    while index < len(text):
        character = text[index]
        if in_string:
            output.append(character)
            if escaped:
                escaped = False
            elif character == "\\":
                escaped = True
            elif character == '"':
                in_string = False
            index += 1
            continue
        if character == '"':
            in_string = True
            output.append(character)
            index += 1
            continue
        if text.startswith("//", index):
            newline = text.find("\n", index)
            if newline == -1:
                break
            index = newline
            continue
        if text.startswith("/*", index):
            end = text.find("*/", index + 2)
            if end == -1:
                break
            index = end + 2
            continue
        output.append(character)
        index += 1
    return "".join(output)


def _write_report(path: Path | None, report: dict) -> None:
    serializable = dict(report)
    serializable["attempts"] = [
        attempt.to_dict() if isinstance(attempt, Attempt) else attempt
        for attempt in report.get("attempts", [])
    ]
    text = json.dumps(serializable, indent=2, sort_keys=True) + "\n"
    if path is None:
        sys.stdout.write(text)
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")
    sys.stderr.write(f"wrote {path}\n")


SUPPORTED_CHECKER_COUNTS = (1, 4, 8)
COMPILER_TOOLS = ("tsc", "reference", "tsrs-default", "tsrs-fast")
CACHE_SCENARIOS = ("cold", "warm")
CORPUS_EXPECTATIONS = {"clean", "errors"}


def _checker_arguments(checkers: int | None) -> list[str]:
    if checkers is None:
        return []
    if checkers not in SUPPORTED_CHECKER_COUNTS:
        raise EvidenceError(
            f"unsupported checker count {checkers}; supported values are "
            f"{SUPPORTED_CHECKER_COUNTS}"
        )
    return ["--checkers", str(checkers)]


def _validate_compiler_attempt(
    attempt: Attempt, root: Path, label: str
) -> list[NormalizedDiagnostic]:
    """Reject incomplete or internally inconsistent compiler output."""
    if attempt.timed_out:
        raise EvidenceError(f"{label}: timed out")
    if attempt.interrupted:
        raise EvidenceError(f"{label}: interrupted")
    if not attempt.completed:
        raise EvidenceError(f"{label}: exited without a completed process record")
    if attempt.stdout_truncated or attempt.stderr_truncated:
        raise EvidenceError(f"{label}: output exceeded the {MAX_OUTPUT_BYTES}-byte bound")
    if attempt.exit_code not in (0, 2):
        raise EvidenceError(
            f"{label}: unexpected exit {attempt.exit_code}; stderr starts with "
            f"{attempt.stderr[:300]!r}"
        )
    try:
        diagnostics = parse_diagnostics(attempt.stdout, root)
    except EvidenceError as error:
        raise EvidenceError(f"{label}: {error}") from error
    if attempt.exit_code == 0 and diagnostics:
        raise EvidenceError(f"{label}: reported diagnostics with a success exit")
    if attempt.exit_code == 2 and not diagnostics:
        raise EvidenceError(
            f"{label}: failure exit without complete diagnostics; stderr starts with "
            f"{attempt.stderr[:300]!r}"
        )
    return diagnostics


def _validate_inventory_attempt(attempt: Attempt, root: Path, label: str) -> list[str]:
    if attempt.timed_out or attempt.interrupted or not attempt.completed:
        raise EvidenceError(f"{label}: inventory process did not complete")
    if attempt.stdout_truncated or attempt.stderr_truncated:
        raise EvidenceError(f"{label}: inventory output exceeded the bound")
    if attempt.exit_code != 0:
        raise EvidenceError(
            f"{label}: inventory exited {attempt.exit_code}; stderr starts with "
            f"{attempt.stderr[:300]!r}"
        )
    files: list[str] = []
    for line in attempt.stdout.splitlines():
        value = line.strip()
        if not value:
            continue
        if not value.startswith("/"):
            raise EvidenceError(f"{label}: unexpected inventory line {value[:200]!r}")
        normalized = _normalize_path(value, root)
        if "node_modules/" in normalized:
            continue
        if not normalized.endswith(TYPE_EXTENSIONS):
            continue
        files.append(normalized)
    if not files:
        raise EvidenceError(f"{label}: inventory is empty")
    return sorted(set(files))


def _compiler_argv(
    tool: str,
    binary: str,
    common: list[str],
    cache_file: Path,
    checkers: int | None,
) -> list[str]:
    checker_args = _checker_arguments(checkers)
    args = [binary, *common]
    if tool == "reference":
        args.extend(checker_args)
    elif tool == "tsrs-default":
        # Explicit false overrides an inherited TSRS_FAST=1.
        args.extend(["--fast", "false", *checker_args])
    elif tool == "tsrs-fast":
        args.extend(["--fast", *checker_args])
    elif tool != "tsc":
        raise EvidenceError(f"unknown compiler tool {tool}")
    args.extend(["--tsBuildInfoFile", str(cache_file)])
    return args


def _inherited_tsrs_overrides() -> list[str]:
    """Environment switches that could change default-mode behavior."""
    return sorted(
        key
        for key in os.environ
        if key.startswith("TSRS_") and key != "TSRS_BINARY"
    )


def _base_type_report(args: argparse.Namespace) -> dict:
    return {
        "schema_version": SCHEMA_VERSION,
        "operation": "compare-types",
        "generated_at": _now_iso(),
        "ok": False,
        "complete": False,
        "failures": [],
        "web_root": str(Path(args.web_root).resolve()) if args.web_root else args.web_root,
        "project": args.project,
        "source": None,
        "pins": {},
        "settings": {
            "checkers": args.checkers,
            "mode": args.mode,
            "fast_env": args.fast_env,
            "heap_mb": args.heap_mb,
            "requested_cache_state": args.cache_state,
            "cache_scenarios": [],
            "project_cache": "isolated temporary tsBuildInfoFile per tool and scenario",
        },
        "inventory": None,
        "attempts": [],
        "reference": None,
        "comparisons": [],
        "cache_scenarios": {},
        "corpus": None,
        "adoption_eligible": False,
        "limits": {
            "max_files": MAX_FILES,
            "max_output_bytes": MAX_OUTPUT_BYTES,
            "timeout_seconds": DEFAULT_TIMEOUT_SECONDS,
            "resource_scope": "per-child process; peak RSS is not aggregate job memory",
        },
    }


def compare_types_command(args: argparse.Namespace) -> int:
    report = _base_type_report(args)
    try:
        _run_compare_types(args, report)
    except EvidenceError as error:
        if error.attempt is not None:
            report["attempts"].append(error.attempt)
        report["failures"].append(f"incomplete evidence: {error}")
    report["ok"] = bool(report["complete"]) and not report["failures"]
    _write_report(Path(args.output) if args.output else None, report)
    return 0 if report["ok"] else 2


def _run_compare_types(args: argparse.Namespace, report: dict) -> None:
    web_root = Path(args.web_root).resolve()
    if not web_root.is_dir():
        raise EvidenceError(f"web root does not exist: {web_root}")
    project = args.project
    _read_tsconfig(web_root / project)
    if args.mode not in {"default", "fast"}:
        raise EvidenceError(f"invalid mode: {args.mode}")
    if args.cache_state not in {"cold", "warm", "unknown"}:
        raise EvidenceError(f"invalid cache state: {args.cache_state}")
    if args.cache_state == "warm":
        raise EvidenceError(
            "warm-only comparison has no seeded cache; use --cache-state unknown "
            "to measure cold and fresh incremental runs"
        )
    scenarios = CACHE_SCENARIOS if args.cache_state == "unknown" else (args.cache_state,)
    _checker_arguments(args.checkers)
    report["settings"]["cache_scenarios"] = list(scenarios)

    reference_version = args.reference_version
    attempts: list[Attempt] = report["attempts"]
    with tempfile.TemporaryDirectory(prefix="kandev-ts-cache-") as cache_tmp, (
        tempfile.TemporaryDirectory(prefix="kandev-ts-reference-")
    ) as reference_tmp:
        cache_root = Path(cache_tmp)
        reference_bin = args.reference_bin
        if reference_bin is None:
            reference_bin_path, _provisioning_attempt = provision_reference_typescript(
                Path(reference_tmp), reference_version, attempts
            )
            reference_bin = str(reference_bin_path)
        reference_bin = _require_executable(reference_bin, "reference tsc")
        reference_version_output = tool_version(reference_bin, web_root, attempts, "version-reference")
        if reference_version != "unverified" and reference_version not in reference_version_output:
            raise EvidenceError(
                f"reference compiler version mismatch: expected {reference_version}, "
                f"got {reference_version_output}"
            )

        tsrs = _require_executable(
            args.tsrs or str(web_root / "node_modules" / ".bin" / "tsrs"), "tsrs"
        )
        tsrs_version = tool_version(tsrs, web_root, attempts, "version-tsrs")
        if DEFAULT_TSRS not in tsrs_version:
            raise EvidenceError(
                f"tsrs version mismatch: expected {DEFAULT_TSRS}, got {tsrs_version}"
            )
        tsc = _require_executable(
            args.tsc or str(web_root / "node_modules" / ".bin" / "tsc"), "tsc"
        )
        tsc_version = tool_version(tsc, web_root, attempts, "version-tsc")
        report["pins"] = {
            "typescript_workspace": tsc_version,
            "typescript_reference": reference_version_output,
            "tsrs": tsrs_version,
            "reference_expected": reference_version,
            "tsrs_expected": DEFAULT_TSRS,
        }

        heap_env = {"NODE_OPTIONS": f"--max-old-space-size={args.heap_mb}"}
        binaries = {
            "tsc": tsc,
            "reference": reference_bin,
            "tsrs-default": tsrs,
            "tsrs-fast": tsrs,
        }
        inventories = {}
        for tool, binary in (("tsc", tsc), ("reference", reference_bin), ("tsrs-fast", tsrs)):
            attempt = run_process(
                f"inventory-{tool}",
                [
                    binary,
                    "--listFilesOnly",
                    "--project",
                    project,
                    "--pretty",
                    "false",
                ],
                web_root,
                env=heap_env,
            )
            attempts.append(attempt)
            inventories[tool] = Inventory(
                files=_validate_inventory_attempt(attempt, web_root, f"inventory-{tool}")
            )
        if any(item.truncated for item in inventories.values()):
            raise EvidenceError("type inventory exceeded the file bound")
        baseline_files = set(inventories["tsc"].files)
        inventory_differences = {
            label: {
                "missing": sorted(baseline_files - set(item.files)),
                "extra": sorted(set(item.files) - baseline_files),
            }
            for label, item in inventories.items()
            if set(item.files) != baseline_files
        }
        report["inventory"] = {
            "baseline": inventories["tsc"].to_dict(),
            "by_tool": {label: item.to_dict() for label, item in inventories.items()},
            "equal": not inventory_differences,
            "differences": inventory_differences,
        }

        project_results = _run_project_comparison(
            web_root,
            project,
            binaries,
            args.checkers,
            heap_env,
            attempts,
            cache_root,
            scenarios,
        )
        report["cache_scenarios"]["project"] = {
            tool: {
                scenario: {
                    "exit_code": item["attempt"].exit_code,
                    "diagnostic_count": len(item["diagnostics"]),
                }
                for scenario, item in tool_results.items()
            }
            for tool, tool_results in project_results.items()
        }
        report["reference"] = {
            scenario: {
                "exit_code": item["attempt"].exit_code,
                "diagnostics": [diagnostic.to_dict() for diagnostic in item["diagnostics"]],
            }
            for scenario, item in project_results["reference"].items()
        }
        comparisons: list[Comparison] = []
        for tool in ("tsrs-default", "tsrs-fast"):
            for scenario in scenarios:
                comparisons.append(
                    compare_type_diagnostics(
                        project_results["reference"][scenario]["diagnostics"],
                        project_results[tool][scenario]["diagnostics"],
                        reference_attempt=f"reference:{scenario}",
                        candidate_attempt=f"{tool}:{scenario}",
                        label=f"project:{tool}:{scenario}",
                    )
                )
        report["comparisons"] = [comparison.to_dict() for comparison in comparisons]

        probe = _run_incremental_probe(
            binaries, args.checkers, heap_env, attempts, cache_root
        )
        report["cache_scenarios"]["incremental_probe"] = probe["report"]

        corpus_root, corpus_mode = _resolve_corpus(args)
        corpus = None
        if corpus_root is not None:
            corpus = _run_corpus(
                corpus_root,
                binaries,
                args.checkers,
                heap_env,
                attempts,
                cache_root,
                scenarios,
            )
            report["corpus"] = corpus
        else:
            report["corpus"] = {
                "mode": corpus_mode,
                "root": None,
                "projects": [],
                "ok": True,
                "note": "diagnostic-only comparison; adoption evidence requires the corpus",
            }
        report["settings"]["corpus_mode"] = corpus_mode
        report["adoption_eligible"] = bool(
            corpus_mode == "required" and corpus is not None and corpus["ok"]
        )

        failures: list[str] = report["failures"]
        if inventory_differences:
            failures.append("compiler input inventories differ")
        for comparison in comparisons:
            if not comparison.ok:
                failures.append(f"{comparison.label} does not match the reference diagnostics")
        for tool, result in probe["report"].items():
            if not result["ok"]:
                failures.append(f"incremental probe failed for {tool}")
        if corpus is not None:
            for result in corpus["projects"]:
                if not result["ok"]:
                    failures.append(f"corpus {result['project']} does not meet its expectations")
            if corpus_mode == "required" and not corpus["ok"]:
                failures.append("required corpus comparison failed")
        report["source"] = source_identity(web_root, TYPE_EXTENSIONS)
        report["complete"] = True


def _run_project_comparison(
    web_root: Path,
    project: str,
    binaries: dict[str, str],
    checkers: int | None,
    env: dict[str, str],
    attempts: list[Attempt],
    cache_root: Path,
    scenarios: tuple[str, ...],
) -> dict:
    common = ["--noEmit", "--project", project, "--pretty", "false"]
    results: dict[str, dict[str, dict]] = {}
    for tool in COMPILER_TOOLS:
        cache_file = cache_root / f"project-{tool}.tsbuildinfo"
        results[tool] = {}
        for scenario in scenarios:
            if scenario == "cold":
                cache_file.unlink(missing_ok=True)
            argv = _compiler_argv(tool, binaries[tool], common, cache_file, checkers)
            attempt = run_process(
                f"{tool}-{scenario}",
                argv,
                web_root,
                env=env,
                unset_env=_inherited_tsrs_overrides(),
            )
            attempts.append(attempt)
            diagnostics = _validate_compiler_attempt(
                attempt, web_root, f"{tool}-{scenario}"
            )
            results[tool][scenario] = {
                "attempt": attempt,
                "diagnostics": diagnostics,
            }
    return results


def _write_probe_project(project: Path) -> None:
    project.mkdir(parents=True, exist_ok=True)
    (project / "tsconfig.json").write_text(
        json.dumps(
            {
                "compilerOptions": {
                    "strict": True,
                    "noEmit": True,
                    "target": "ES2022",
                    "module": "ESNext",
                    "moduleResolution": "Bundler",
                    "skipLibCheck": True,
                    "incremental": True,
                    "lib": ["ES2022", "DOM"],
                },
                "include": ["*.ts"],
            }
        ),
        encoding="utf-8",
    )
    (project / "index.ts").write_text("export const value = 1;\n", encoding="utf-8")


def _run_incremental_probe(
    binaries: dict[str, str],
    checkers: int | None,
    env: dict[str, str],
    attempts: list[Attempt],
    cache_root: Path,
) -> dict:
    """Cold, warm, then changed-invalid warm runs in an owned project.

    A tool that reuses a stale success for the changed source is rejected.
    """
    results: dict[str, dict] = {}
    with tempfile.TemporaryDirectory(prefix="kandev-incremental-probe-") as temp:
        for tool in COMPILER_TOOLS:
            project = Path(temp) / tool
            _write_probe_project(project)
            cache_file = cache_root / f"probe-{tool}.tsbuildinfo"
            common = ["--noEmit", "--pretty", "false"]
            phases: dict[str, dict] = {}
            for scenario in CACHE_SCENARIOS:
                if scenario == "cold":
                    cache_file.unlink(missing_ok=True)
                argv = _compiler_argv(tool, binaries[tool], common, cache_file, checkers)
                attempt = run_process(
                    f"probe-{tool}-{scenario}",
                    argv,
                    project,
                    env=env,
                    unset_env=_inherited_tsrs_overrides(),
                )
                attempts.append(attempt)
                diagnostics = _validate_compiler_attempt(
                    attempt, project, f"probe-{tool}-{scenario}"
                )
                phases[scenario] = {
                    "exit_code": attempt.exit_code,
                    "diagnostic_count": len(diagnostics),
                }
            (project / "index.ts").write_text(
                'export const value: number = "not a number";\n', encoding="utf-8"
            )
            argv = _compiler_argv(tool, binaries[tool], common, cache_file, checkers)
            attempt = run_process(
                f"probe-{tool}-changed",
                argv,
                project,
                env=env,
                unset_env=_inherited_tsrs_overrides(),
            )
            attempts.append(attempt)
            diagnostics = _validate_compiler_attempt(
                attempt, project, f"probe-{tool}-changed"
            )
            phases["changed"] = {
                "exit_code": attempt.exit_code,
                "diagnostic_count": len(diagnostics),
                "codes": sorted({diagnostic.code for diagnostic in diagnostics}),
            }
            clean = (
                phases["cold"]["diagnostic_count"] == 0
                and phases["warm"]["diagnostic_count"] == 0
            )
            changed = bool(diagnostics) and "TS2322" in phases["changed"]["codes"]
            results[tool] = {
                "ok": clean and changed,
                "phases": phases,
            }
    return {"report": results}


def _default_corpus_dir() -> Path:
    return Path(__file__).resolve().parent / "fixtures" / "web-toolchain" / "types"


def _resolve_corpus(args: argparse.Namespace) -> tuple[Path | None, str]:
    if args.no_corpus:
        return None, "diagnostic-only"
    corpus_root = Path(args.corpus) if args.corpus else _default_corpus_dir()
    if not corpus_root.is_absolute():
        corpus_root = (Path(__file__).resolve().parent / corpus_root).resolve()
    if not corpus_root.is_dir():
        raise EvidenceError(
            f"required corpus directory does not exist: {corpus_root}; "
            "pass --no-corpus only for a diagnostic-only run"
        )
    if not (corpus_root / "corpus.json").is_file():
        raise EvidenceError(f"corpus manifest does not exist: {corpus_root / 'corpus.json'}")
    return corpus_root, "required"


def _load_corpus_manifest(corpus_root: Path) -> dict[str, dict]:
    path = corpus_root / "corpus.json"
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise EvidenceError(f"invalid corpus manifest {path}: {error}") from error
    projects = data.get("projects") if isinstance(data, dict) else None
    if not isinstance(projects, dict) or not projects:
        raise EvidenceError(f"corpus manifest {path} must declare projects")
    for name, entry in projects.items():
        if not isinstance(entry, dict) or entry.get("expected") not in CORPUS_EXPECTATIONS:
            raise EvidenceError(
                f"corpus manifest entry {name!r} must use expected=clean|errors"
            )
        if entry["expected"] == "errors":
            required = entry.get("required_codes")
            if not isinstance(required, list) or not required:
                raise EvidenceError(
                    f"corpus manifest entry {name!r} must declare required_codes"
                )
            minimum = entry.get("minimum_count")
            if not isinstance(minimum, int) or minimum < 1:
                raise EvidenceError(
                    f"corpus manifest entry {name!r} must declare minimum_count"
                )
            exact = entry.get("exact_count")
            if exact is not None and (
                not isinstance(exact, int) or exact < minimum
            ):
                raise EvidenceError(
                    f"corpus manifest entry {name!r} has an invalid exact_count"
                )
    return projects


def _corpus_expectation_ok(entry: dict, diagnostics: list[NormalizedDiagnostic]) -> bool:
    if entry["expected"] == "clean":
        return not diagnostics
    exact = entry.get("exact_count")
    if exact is not None and len(diagnostics) != int(exact):
        return False
    codes = {diagnostic.code for diagnostic in diagnostics}
    return (
        len(diagnostics) >= int(entry["minimum_count"])
        and set(entry["required_codes"]).issubset(codes)
    )


def _run_corpus(
    corpus_root: Path,
    binaries: dict[str, str],
    checkers: int | None,
    env: dict[str, str],
    attempts: list[Attempt],
    cache_root: Path,
    scenarios: tuple[str, ...],
) -> dict:
    manifest = _load_corpus_manifest(corpus_root)
    found = {
        path.parent.relative_to(corpus_root).as_posix()
        for path in corpus_root.rglob("tsconfig.json")
    }
    declared = set(manifest)
    if found != declared:
        raise EvidenceError(
            "corpus manifest does not match projects: "
            f"missing={sorted(declared - found)} extra={sorted(found - declared)}"
        )
    projects: list[dict] = []
    corpus_ok = True
    for name in sorted(declared):
        project_dir = corpus_root / name
        entry = manifest[name]
        results: dict[str, dict[str, dict]] = {}
        project_ok = True
        for tool in ("reference", "tsrs-default", "tsrs-fast"):
            results[tool] = {}
            cache_file = cache_root / f"corpus-{name.replace('/', '-')}-{tool}.tsbuildinfo"
            for scenario in scenarios:
                if scenario == "cold":
                    cache_file.unlink(missing_ok=True)
                argv = _compiler_argv(
                    tool,
                    binaries[tool],
                    ["--noEmit", "--pretty", "false"],
                    cache_file,
                    checkers,
                )
                attempt = run_process(
                    f"corpus-{name}-{tool}-{scenario}",
                    argv,
                    project_dir,
                    env=env,
                    unset_env=_inherited_tsrs_overrides(),
                )
                attempts.append(attempt)
                diagnostics = _validate_compiler_attempt(
                    attempt, project_dir, f"corpus-{name}-{tool}-{scenario}"
                )
                expected_ok = _corpus_expectation_ok(entry, diagnostics)
                if not expected_ok:
                    project_ok = False
                results[tool][scenario] = {
                    "exit_code": attempt.exit_code,
                    "diagnostics": [item.to_dict() for item in diagnostics],
                    "expectation_ok": expected_ok,
                }
        comparisons: list[Comparison] = []
        for tool in ("tsrs-default", "tsrs-fast"):
            for scenario in scenarios:
                reference_diagnostics = results["reference"][scenario]["diagnostics"]
                candidate_diagnostics = results[tool][scenario]["diagnostics"]
                comparison = compare_type_diagnostics(
                    [
                        NormalizedDiagnostic(**item)
                        for item in reference_diagnostics
                    ],
                    [NormalizedDiagnostic(**item) for item in candidate_diagnostics],
                    reference_attempt=f"reference:{scenario}",
                    candidate_attempt=f"{tool}:{scenario}",
                    label=f"corpus:{name}:{tool}:{scenario}",
                )
                comparisons.append(comparison)
                if not comparison.ok:
                    project_ok = False
        if not project_ok:
            corpus_ok = False
        projects.append(
            {
                "project": name,
                "expected": entry,
                "modes": results,
                "comparisons": [comparison.to_dict() for comparison in comparisons],
                "ok": project_ok,
            }
        )
    return {
        "mode": "required",
        "root": str(corpus_root),
        "manifest": str(corpus_root / "corpus.json"),
        "projects": projects,
        "ok": corpus_ok,
    }


_ESLINT_RULE_PREFIX_RE = re.compile(r"^@typescript-eslint/(.*)$")
_OXLINT_CODE_RE = re.compile(r"^(?P<plugin>[^()]+)\((?P<rule>[^()]+)\)$")


def _normalize_rule_id(rule: str) -> str:
    """Normalize engine-specific prefixes so mapped rules can be compared."""
    match = _ESLINT_RULE_PREFIX_RE.match(rule)
    if match:
        return match.group(1)
    return rule


def _normalize_oxlint_code(code: str | None) -> str:
    if not code:
        return "parser"
    match = _OXLINT_CODE_RE.match(code)
    if not match:
        return code
    plugin, rule = match.group("plugin"), match.group("rule")
    if plugin in {"eslint", "typescript"}:
        return rule
    return f"{plugin}/{rule}"


def parse_eslint_json(raw: str) -> tuple[list[dict], Inventory]:
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as error:
        raise EvidenceError(f"eslint did not produce valid JSON: {error}") from error
    if not isinstance(payload, list):
        raise EvidenceError("eslint JSON must be a list of file results")
    findings: list[dict] = []
    files: list[str] = []
    for result in payload:
        if not isinstance(result, dict) or "filePath" not in result:
            raise EvidenceError("malformed eslint file result")
        path = str(result["filePath"])
        files.append(path)
        for message in result.get("messages", []):
            findings.append(
                {
                    "file": path,
                    "line": int(message.get("line") or 0),
                    "column": int(message.get("column") or 0),
                    "rule": _normalize_rule_id(str(message.get("ruleId") or "parser")),
                    "severity": "error" if int(message.get("severity") or 2) == 2 else "warning",
                    "message": message.get("message") or "",
                }
            )
    return findings, Inventory(files=sorted(set(files)))


def parse_oxlint_json(raw: str) -> list[dict]:
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as error:
        raise EvidenceError(
            f"oxlint did not produce valid JSON: {error}; output starts with {raw[:200]!r}"
        ) from error
    if not isinstance(payload, dict) or "diagnostics" not in payload:
        raise EvidenceError("malformed oxlint JSON report")
    findings: list[dict] = []
    for diagnostic in payload["diagnostics"]:
        spans = diagnostic.get("labels") or []
        if not spans:
            raise EvidenceError("oxlint diagnostic has no source span")
        span = spans[0].get("span") or {}
        findings.append(
            {
                "file": diagnostic.get("filename") or "",
                "line": int(span.get("line") or 0),
                "column": int(span.get("column") or 0),
                "rule": _normalize_oxlint_code(diagnostic.get("code")),
                "severity": diagnostic.get("severity") or "error",
                "message": diagnostic.get("message") or "",
            }
        )
    return findings


def compare_lint_findings(
    baseline: list[dict],
    candidate: list[dict],
    *,
    label: str = "full",
) -> Comparison:
    def key_for(item: dict) -> tuple:
        return (
            item["file"],
            item["rule"],
            item["line"],
            item.get("column", 0),
            item["severity"],
            item["message"],
        )

    def counts(items: list[dict]) -> dict[tuple, int]:
        output: dict[tuple, int] = {}
        for item in items:
            key = key_for(item)
            output[key] = output.get(key, 0) + 1
        return output

    baseline_counts = counts(baseline)
    candidate_counts = counts(candidate)
    missing_keys = {
        key: count - candidate_counts.get(key, 0)
        for key, count in baseline_counts.items()
        if count > candidate_counts.get(key, 0)
    }
    extra_keys = {
        key: count - baseline_counts.get(key, 0)
        for key, count in candidate_counts.items()
        if count > baseline_counts.get(key, 0)
    }
    missing: list[dict] = []
    extra: list[dict] = []
    for key, count in missing_keys.items():
        for _ in range(count):
            missing.append(_finding_for_key(key, baseline))
    for key, count in extra_keys.items():
        for _ in range(count):
            extra.append(_finding_for_key(key, candidate))
    order_only = (
        not missing
        and not extra
        and [key_for(item) for item in baseline] != [key_for(item) for item in candidate]
    )
    ok = not missing and not extra
    return Comparison(
        label=label,
        reference_attempt="eslint",
        candidate_attempt="oxlint",
        missing=missing,
        extra=extra,
        order_only=order_only,
        ok=ok,
        reason=None if ok else "lint finding mismatch",
    )


def _finding_for_key(key: tuple, findings: list[dict]) -> dict:
    for finding in findings:
        candidate = (
            finding["file"],
            finding["rule"],
            finding["line"],
            finding.get("column", 0),
            finding["severity"],
            finding["message"],
        )
        if candidate == key:
            return finding
    raise AssertionError("lint finding key missing from its own list")


def _pnpm_tool_version(
    pnpm: str, tool: str, cwd: Path, attempts: list[Attempt] | None = None
) -> str:
    attempt = run_process("version", [pnpm, "exec", tool, "--version"], cwd, timeout=120.0)
    if attempts is not None:
        attempts.append(attempt)
    if attempt.exit_code != 0:
        raise EvidenceError(f"could not read {tool} version: {attempt.stderr[:300]}")
    output = (attempt.stdout or attempt.stderr).strip().splitlines()
    if not output:
        raise EvidenceError(f"{tool} reported no version")
    return output[0]


def _validate_file_list_attempt(attempt: Attempt, root: Path, label: str) -> list[str]:
    if attempt.timed_out or attempt.interrupted or not attempt.completed:
        raise EvidenceError(f"{label}: file inventory did not complete")
    if attempt.exit_code != 0:
        raise EvidenceError(
            f"{label}: file inventory exited {attempt.exit_code}; stderr starts with "
            f"{attempt.stderr[:300]!r}"
        )
    if attempt.stdout_truncated or attempt.stderr_truncated:
        raise EvidenceError(f"{label}: file inventory output exceeded the bound")
    files: list[str] = []
    for line in attempt.stdout.splitlines():
        value = line.strip()
        if not value:
            continue
        if value.lower().startswith(("warning", "error", "found ")):
            raise EvidenceError(f"{label}: unexpected file inventory line {value[:200]!r}")
        files.append(_normalize_path(value, root))
    if not files:
        raise EvidenceError(f"{label}: file inventory is empty")
    return sorted(set(files))


def _base_lint_report(args: argparse.Namespace) -> dict:
    return {
        "schema_version": SCHEMA_VERSION,
        "operation": "compare-lint",
        "generated_at": _now_iso(),
        "ok": False,
        "complete": False,
        "failures": [],
        "web_root": str(Path(args.web_root).resolve()) if args.web_root else args.web_root,
        "config": args.config,
        "baseline_config": args.baseline_config,
        "source": None,
        "pins": {},
        "inventory": None,
        "attempts": [],
        "findings": None,
    }


def compare_lint_command(args: argparse.Namespace) -> int:
    report = _base_lint_report(args)
    try:
        _run_compare_lint(args, report)
    except EvidenceError as error:
        if error.attempt is not None:
            report["attempts"].append(error.attempt)
        report["failures"].append(f"incomplete evidence: {error}")
    report["ok"] = bool(report["complete"]) and not report["failures"]
    _write_report(Path(args.output) if args.output else None, report)
    return 0 if report["ok"] else 2


def _run_compare_lint(args: argparse.Namespace, report: dict) -> None:
    web_root = Path(args.web_root).resolve()
    if not web_root.is_dir():
        raise EvidenceError(f"web root does not exist: {web_root}")

    def resolve_config(value: str, label: str) -> Path:
        candidate = Path(value)
        if not candidate.is_absolute() and not candidate.is_file():
            candidate = web_root / candidate
        if not candidate.is_file():
            raise EvidenceError(f"{label} configuration does not exist: {candidate}")
        return candidate.resolve()

    config = resolve_config(args.config, "candidate")
    baseline_config = resolve_config(args.baseline_config, "baseline")
    report["config"] = str(config)
    report["baseline_config"] = str(baseline_config)

    pnpm = _require_executable("pnpm", "pnpm")
    report["pins"] = {
        "eslint": _pnpm_tool_version(pnpm, "eslint", web_root, report["attempts"]),
        "oxlint": _pnpm_tool_version(pnpm, "oxlint", web_root, report["attempts"]),
    }
    paths = args.paths or ["."]
    attempts: list[Attempt] = report["attempts"]
    baseline = run_process(
        "eslint",
        [
            pnpm,
            "exec",
            "eslint",
            "--config",
            str(baseline_config),
            "--max-warnings",
            "0",
            "--format",
            "json",
            *paths,
        ],
        web_root,
        timeout=args.timeout,
    )
    attempts.append(baseline)
    if baseline.timed_out or baseline.interrupted or not baseline.completed:
        raise EvidenceError("eslint did not complete")
    if baseline.stdout_truncated or baseline.stderr_truncated:
        raise EvidenceError("eslint output exceeded the bound")
    if baseline.exit_code not in (0, 1):
        raise EvidenceError(
            f"eslint failed with exit {baseline.exit_code}: {baseline.stderr[:500]}"
        )
    baseline_findings, baseline_inventory = parse_eslint_json(baseline.stdout)

    candidate = run_process(
        "oxlint",
        [
            pnpm,
            "exec",
            "oxlint",
            "-c",
            str(config),
            "--deny-warnings",
            "--format",
            "json",
            *paths,
        ],
        web_root,
        timeout=args.timeout,
    )
    attempts.append(candidate)
    if candidate.timed_out or candidate.interrupted or not candidate.completed:
        raise EvidenceError("oxlint did not complete")
    if candidate.stdout_truncated or candidate.stderr_truncated:
        raise EvidenceError("oxlint output exceeded the bound")
    if candidate.exit_code not in (0, 1):
        raise EvidenceError(
            f"oxlint failed with exit {candidate.exit_code}: {candidate.stderr[:500]}"
        )
    candidate_findings = parse_oxlint_json(candidate.stdout)

    candidate_files_attempt = run_process(
        "oxlint-files",
        [pnpm, "exec", "oxlint", "-c", str(config), "--debug=files", *paths],
        web_root,
        timeout=args.timeout,
    )
    attempts.append(candidate_files_attempt)
    candidate_inventory = Inventory(
        files=_validate_file_list_attempt(
            candidate_files_attempt, web_root, "oxlint-files"
        )
    )
    if not baseline_inventory.files or not candidate_inventory.files:
        raise EvidenceError("lint selection is empty")

    baseline_paths = {_normalize_path(path, web_root) for path in baseline_inventory.files}
    candidate_paths = set(candidate_inventory.files)
    missing_files = sorted(baseline_paths - candidate_paths)
    extra_files = sorted(candidate_paths - baseline_paths)

    comparison = compare_lint_findings(baseline_findings, candidate_findings)
    failures: list[str] = report["failures"]
    if missing_files or extra_files:
        failures.append("lint file inventories differ")
    if not comparison.ok:
        failures.append("lint findings differ")
    if baseline_findings or candidate_findings:
        failures.append("lint findings are not zero")
    if baseline.exit_code != 0 or candidate.exit_code != 0:
        failures.append("a lint command rejected its findings under the zero-warning policy")

    report["source"] = source_identity(web_root, LINT_EXTENSIONS)
    report["inventory"] = {
        "baseline": baseline_inventory.to_dict(),
        "candidate": candidate_inventory.to_dict(),
        "equal": not missing_files and not extra_files,
        "missing": missing_files,
        "extra": extra_files,
    }
    report["findings"] = {
        "baseline": baseline_findings,
        "candidate": candidate_findings,
        "missing": comparison.missing,
        "extra": comparison.extra,
        "order_only": comparison.order_only,
    }
    report["complete"] = True


def _clear_incremental_caches(web_root: Path, output: Path) -> list[str]:
    """Remove per-compiler incremental artifacts so every sample is cold.

    The baseline and candidate use different compilers; letting one read the
    other's build info would silently change the declared cache class.
    """
    removed: list[str] = []
    for path in sorted(web_root.glob("*.tsbuildinfo")):
        archived = output / "cache" / f"{path.stem}-{time.time_ns()}{path.suffix}"
        archived.parent.mkdir(parents=True, exist_ok=True)
        try:
            path.replace(archived)
        except OSError:
            path.unlink(missing_ok=True)
        removed.append(_normalize_path(str(path), web_root))
    return removed


def _median(values: list[float]) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    middle = len(ordered) // 2
    if len(ordered) % 2:
        return ordered[middle]
    return (ordered[middle - 1] + ordered[middle]) / 2


def _base_benchmark_report(args: argparse.Namespace) -> dict:
    return {
        "schema_version": SCHEMA_VERSION,
        "operation": "benchmark",
        "generated_at": _now_iso(),
        "ok": False,
        "complete": False,
        "failures": [],
        "web_root": args.web_root,
        "source": None,
        "pins": {},
        "settings": {},
        "samples": [],
        "attempts": [],
        "medians": {},
        "reductions": {},
        "thresholds": {},
        "costs": {},
    }


def benchmark_command(args: argparse.Namespace) -> int:
    report = _base_benchmark_report(args)
    try:
        _run_benchmark(args, report)
    except EvidenceError as error:
        if error.attempt is not None:
            report["attempts"].append(error.attempt)
        report["failures"].append(f"incomplete evidence: {error}")
    report["ok"] = bool(report["complete"]) and not report["failures"]
    _write_report(Path(args.output) / "report.json", report)
    return 0 if report["ok"] else 2


def _run_benchmark(args: argparse.Namespace, report: dict) -> None:
    web_root = Path(args.web_root).resolve()
    if not web_root.is_dir():
        raise EvidenceError(f"web root does not exist: {web_root}")
    output = Path(args.output)
    output.mkdir(parents=True, exist_ok=True)
    if args.pairs < 1:
        raise EvidenceError("at least one pair is required")

    pnpm = _require_executable("pnpm", "pnpm")
    tsc = _require_executable(args.tsc or str(web_root / "node_modules" / ".bin" / "tsc"), "tsc")
    tsrs = _require_executable(args.tsrs or str(web_root / "node_modules" / ".bin" / "tsrs"), "tsrs")
    tsrs_version = tool_version(tsrs, web_root)
    if DEFAULT_TSRS not in tsrs_version:
        raise EvidenceError(
            f"tsrs version mismatch: expected {DEFAULT_TSRS}, got {tsrs_version}"
        )
    heap_env = {"NODE_OPTIONS": f"--max-old-space-size={args.heap_mb}"}

    prechecks = [
        ("pretypecheck", [pnpm, "run", "pretypecheck"]),
        ("rich-output-check", [pnpm, "run", "rich-output:check"]),
    ]
    baseline_compiler = ("baseline-tsc", [tsc, "--noEmit"])
    candidate_compiler = ("candidate-tsrs", [tsrs, "--noEmit", "--fast"])
    baseline_lint = ("baseline-eslint", [pnpm, "exec", "eslint", "--max-warnings", "0", "."])
    candidate_lint = (
        "candidate-oxlint",
        [pnpm, "exec", "oxlint", "-c", "oxlint.config.mjs", "--deny-warnings", "."],
    )

    cleared_caches: list[str] = []

    source_before = source_identity(web_root, TYPE_EXTENSIONS + LINT_EXTENSIONS)
    config_before = config_identity(web_root)
    samples: list[dict] = report["samples"]
    attempts: list[Attempt] = report["attempts"]
    failures: list[str] = report["failures"]
    for pair_index in range(args.pairs):
        order = ("baseline", "candidate") if pair_index % 2 == 0 else ("candidate", "baseline")
        pair: dict = {"pair": pair_index + 1, "order": list(order), "samples": {}}
        for side in order:
            lint_command = baseline_lint if side == "baseline" else candidate_lint
            compiler = baseline_compiler if side == "baseline" else candidate_compiler
            lint_attempt = run_process(
                f"{lint_command[0]}-pair{pair_index + 1}", lint_command[1], web_root, timeout=args.timeout
            )
            attempts.append(lint_attempt)
            cleared_caches.extend(_clear_incremental_caches(web_root, output))
            compiler_attempts = []
            for label, argv in prechecks:
                precheck_attempt = run_process(
                    f"{label}-{side}-pair{pair_index + 1}", argv, web_root, timeout=args.timeout
                )
                attempts.append(precheck_attempt)
                compiler_attempts.append(precheck_attempt)
            compiler_attempt = run_process(
                f"{compiler[0]}-pair{pair_index + 1}",
                compiler[1],
                web_root,
                timeout=args.timeout,
                env=heap_env,
            )
            attempts.append(compiler_attempt)
            compiler_attempts.append(compiler_attempt)
            side_ok = all(attempt.exit_code == 0 for attempt in [lint_attempt, *compiler_attempts])
            typecheck_seconds = round(sum(attempt.elapsed_seconds for attempt in compiler_attempts), 3)
            pair["samples"][side] = {
                "ok": side_ok,
                "lint_seconds": lint_attempt.elapsed_seconds,
                "typecheck_seconds": typecheck_seconds,
                "typecheck_attempts": [attempt.to_dict() for attempt in compiler_attempts],
                "lint_attempt": lint_attempt.to_dict(),
            }
            if not side_ok:
                failures.append(f"pair {pair_index + 1} {side} command failed")
        samples.append(pair)

    source_after = source_identity(web_root, TYPE_EXTENSIONS + LINT_EXTENSIONS)
    config_after = config_identity(web_root)
    source_stable = source_before["sha256"] == source_after["sha256"]
    config_stable = config_before["sha256"] == config_after["sha256"]

    baseline_lint_values = [
        pair["samples"]["baseline"]["lint_seconds"]
        for pair in samples
        if pair["samples"]["baseline"]["ok"]
    ]
    candidate_lint_values = [
        pair["samples"]["candidate"]["lint_seconds"]
        for pair in samples
        if pair["samples"]["candidate"]["ok"]
    ]
    baseline_typecheck_values = [
        pair["samples"]["baseline"]["typecheck_seconds"]
        for pair in samples
        if pair["samples"]["baseline"]["ok"]
    ]
    candidate_typecheck_values = [
        pair["samples"]["candidate"]["typecheck_seconds"]
        for pair in samples
        if pair["samples"]["candidate"]["ok"]
    ]

    baseline_lint_median = _median(baseline_lint_values)
    candidate_lint_median = _median(candidate_lint_values)
    baseline_typecheck_median = _median(baseline_typecheck_values)
    candidate_typecheck_median = _median(candidate_typecheck_values)

    def reduction(baseline: float | None, candidate: float | None) -> float | None:
        if baseline in (None, 0) or candidate is None:
            return None
        return round((baseline - candidate) / baseline, 4)

    lint_reduction = reduction(baseline_lint_median, candidate_lint_median)
    typecheck_reduction = reduction(baseline_typecheck_median, candidate_typecheck_median)
    combined_baseline = (
        baseline_lint_median + baseline_typecheck_median
        if baseline_lint_median is not None and baseline_typecheck_median is not None
        else None
    )
    combined_candidate = (
        candidate_lint_median + candidate_typecheck_median
        if candidate_lint_median is not None and candidate_typecheck_median is not None
        else None
    )
    combined_reduction = reduction(combined_baseline, combined_candidate)

    complete_pairs = [
        pair
        for pair in samples
        if pair["samples"]["baseline"]["ok"] and pair["samples"]["candidate"]["ok"]
    ]
    if not source_stable:
        failures.append("source changed during measurement")
    if not config_stable:
        failures.append("configuration changed during measurement")
    if len(complete_pairs) < args.pairs:
        failures.append("not every sample pair completed successfully")
    if lint_reduction is None or lint_reduction < args.min_lint_reduction:
        failures.append(
            f"lint reduction {lint_reduction} is below {args.min_lint_reduction}"
        )
    if typecheck_reduction is None or typecheck_reduction < args.min_typecheck_reduction:
        failures.append(
            f"typecheck reduction {typecheck_reduction} is below {args.min_typecheck_reduction}"
        )
    if combined_reduction is None or combined_reduction <= 0:
        failures.append("combined occupied frontend execution did not decrease")

    report.update({
        "web_root": str(web_root),
        "source": {"before": source_before, "after": source_after, "stable": source_stable},
        "config": {"before": config_before, "after": config_after, "stable": config_stable},
        "pins": {
            "tsrs": tsrs_version,
            "typescript_workspace": tool_version(tsc, web_root),
        },
        "settings": {
            "pairs": args.pairs,
            "heap_mb": args.heap_mb,
            "cold": args.cold,
            "cache_isolated_per_sample": True,
            "cleared_caches": sorted(set(cleared_caches)),
            "cache_class": "cold",
            "baseline_commands": {
                "lint": baseline_lint[1],
                "typecheck_prechecks": [argv for _label, argv in prechecks],
                "compiler": baseline_compiler[1],
            },
            "candidate_commands": {
                "lint": candidate_lint[1],
                "typecheck_prechecks": [argv for _label, argv in prechecks],
                "compiler": candidate_compiler[1],
            },
        },
        "samples": samples,
        "attempts": [attempt.to_dict() for attempt in attempts],
        "medians": {
            "baseline_lint_seconds": baseline_lint_median,
            "candidate_lint_seconds": candidate_lint_median,
            "baseline_typecheck_seconds": baseline_typecheck_median,
            "candidate_typecheck_seconds": candidate_typecheck_median,
            "baseline_combined_seconds": combined_baseline,
            "candidate_combined_seconds": combined_candidate,
        },
        "reductions": {
            "lint": lint_reduction,
            "typecheck": typecheck_reduction,
            "combined": combined_reduction,
        },
        "thresholds": {
            "min_lint_reduction": args.min_lint_reduction,
            "min_typecheck_reduction": args.min_typecheck_reduction,
        },
        "costs": {
            "dependency_install": None,
            "cache_restore": None,
            "reference_install": None,
            "resource_scope": (
                "per-child process; a pnpm wrapper's RSS is not its child tool's RSS"
            ),
            "note": "reported per attempt in `attempts`; unavailable classes remain null",
        },
    })
    report["complete"] = True


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="operation", required=True)

    types = subparsers.add_parser("compare-types", help="compare compiler diagnostics")
    types.add_argument("--web-root", required=True)
    types.add_argument("--project", default="tsconfig.json")
    types.add_argument("--tsc", default=None)
    types.add_argument("--tsrs", default=None)
    types.add_argument("--reference-bin", default=None)
    types.add_argument("--reference-version", default=DEFAULT_REFERENCE_TYPESCRIPT)
    types.add_argument("--mode", choices=["default", "fast"], default="fast")
    types.add_argument("--cache-state", choices=["cold", "warm", "unknown"], default="unknown")
    types.add_argument("--checkers", type=int, default=4)
    types.add_argument("--fast-env", action="store_true")
    types.add_argument("--heap-mb", type=int, default=8192)
    types.add_argument("--corpus", default=None)
    types.add_argument("--no-corpus", action="store_true")
    types.add_argument("--output", default=None)
    types.set_defaults(func=compare_types_command)

    lint = subparsers.add_parser("compare-lint", help="compare lint findings")
    lint.add_argument("--web-root", required=True)
    lint.add_argument("--config", default="oxlint.config.mjs")
    lint.add_argument("--baseline-config", default="eslint.config.mjs")
    lint.add_argument("--paths", nargs="*", default=None)
    lint.add_argument("--timeout", type=float, default=DEFAULT_TIMEOUT_SECONDS)
    lint.add_argument("--output", default=None)
    lint.set_defaults(func=compare_lint_command)

    benchmark = subparsers.add_parser("benchmark", help="measure complete command pairs")
    benchmark.add_argument("--web-root", required=True)
    benchmark.add_argument("--pairs", type=int, default=3)
    benchmark.add_argument("--output", required=True)
    benchmark.add_argument("--tsc", default=None)
    benchmark.add_argument("--tsrs", default=None)
    benchmark.add_argument("--heap-mb", type=int, default=8192)
    benchmark.add_argument("--cold", action="store_true")
    benchmark.add_argument("--timeout", type=float, default=DEFAULT_TIMEOUT_SECONDS)
    benchmark.add_argument("--min-lint-reduction", type=float, default=0.5)
    benchmark.add_argument("--min-typecheck-reduction", type=float, default=0.75)
    benchmark.set_defaults(func=benchmark_command)
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        return args.func(args)
    except EvidenceError as error:
        sys.stderr.write(f"web-toolchain-evidence: {error}\n")
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
