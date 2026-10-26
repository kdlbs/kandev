"""Strict ownership registry for disposable local E2E fixture processes.

Only direct children launched by this registry can be signaled. A PID is never
signaled unless its recorded process start identity and process group still
match. If the leader is gone but members remain in its group, the record is
retained for manual reconciliation rather than signaling a possibly reused
group ID.
"""

from __future__ import annotations

import json
import os
import re
import signal
import stat
import subprocess
import time
import uuid
from pathlib import Path


class ProcessOwnershipError(RuntimeError):
    pass


class OwnedProcessRegistry:
    _filename = "runner-processes.json"

    def __init__(self, root: Path):
        self.root = Path(root)
        self._validate_root()
        self.path = self.root / self._filename
        self._children = {}

    def start(self, label, argv, *, cwd, env):
        if not isinstance(label, str) or not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}", label):
            raise ProcessOwnershipError("process label is invalid")
        if not isinstance(argv, (list, tuple)) or not argv or not isinstance(argv[0], str) or not os.path.isabs(argv[0]):
            raise ProcessOwnershipError("process executable must be an absolute path")
        if not isinstance(env, dict) or any(not isinstance(k, str) or not isinstance(v, str) for k, v in env.items()):
            raise ProcessOwnershipError("process environment must be an explicit string mapping")
        cwd_path = Path(cwd).resolve(strict=True)
        root_path = self.root.resolve(strict=True)
        if root_path not in cwd_path.parents and cwd_path != root_path:
            raise ProcessOwnershipError("process working directory must be inside fixture_root")

        records = self.read_records()
        if any(item["label"] == label for item in records):
            raise ProcessOwnershipError("process label is already registered")

        # Never inherit the caller's credentials/configuration implicitly.
        child = subprocess.Popen(
            list(argv),
            cwd=str(cwd_path),
            env=dict(env),
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
            close_fds=True,
        )
        self._children[child.pid] = child
        try:
            identity = self._wait_for_identity(child.pid, timeout=2)
            if identity is None:
                raise ProcessOwnershipError("started process exited before ownership could be recorded")
            started_at, pgid = identity
            if pgid != child.pid:
                raise ProcessOwnershipError("started process is not the leader of its isolated process group")
            record = {"label": label, "pid": child.pid, "started_at": started_at, "pgid": pgid}
            self._write_records(records + [record])
            return record
        except Exception:
            # Popen itself proves this exact child was created by this call.
            if child.poll() is None:
                child.terminate()
                try:
                    child.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    pass
            self._children.pop(child.pid, None)
            raise

    def stop(self, label, timeout=5):
        records = self.read_records()
        record = next((item for item in records if item["label"] == label), None)
        if record is None:
            raise ProcessOwnershipError("process label is not registered")

        identity = self._process_identity(record["pid"])
        if identity is None:
            if self._group_members(record["pgid"]):
                raise ProcessOwnershipError("recorded leader is absent but its process group still has members")
            self._write_records([item for item in records if item["label"] != label])
            self._children.pop(record["pid"], None)
            return "already_absent"
        if identity != (record["started_at"], record["pgid"]):
            raise ProcessOwnershipError("PID start identity or process group changed; refusing to signal")

        try:
            os.kill(record["pid"], signal.SIGTERM)
        except ProcessLookupError:
            pass
        deadline = time.monotonic() + max(0, timeout)
        child = self._children.get(record["pid"])
        while time.monotonic() < deadline:
            if child is not None:
                try:
                    child.wait(timeout=min(0.05, max(0.001, deadline - time.monotonic())))
                except subprocess.TimeoutExpired:
                    pass
            current = self._process_identity(record["pid"])
            if current is None:
                break
            if current != (record["started_at"], record["pgid"]):
                raise ProcessOwnershipError("PID identity changed while waiting; retaining ownership record")
            time.sleep(0.05)
        else:
            raise ProcessOwnershipError("owned process did not stop before timeout; retaining ownership record")

        if self._group_members(record["pgid"]):
            raise ProcessOwnershipError("leader stopped but process group still has members; retaining ownership record")
        self._write_records([item for item in records if item["label"] != label])
        self._children.pop(record["pid"], None)
        return "stopped"

    def crash(self, label, timeout=5):
        """SIGKILL one positively owned fixture leader after an exact identity check."""
        records = self.read_records()
        record = next((item for item in records if item["label"] == label), None)
        if record is None:
            raise ProcessOwnershipError("process label is not registered")

        identity = self._process_identity(record["pid"])
        if identity is None:
            if self._group_members(record["pgid"]):
                raise ProcessOwnershipError("recorded leader is absent but its process group still has members")
            self._write_records([item for item in records if item["label"] != label])
            self._children.pop(record["pid"], None)
            return "already_absent"
        if identity != (record["started_at"], record["pgid"]):
            raise ProcessOwnershipError("PID start identity or process group changed; refusing to signal")

        try:
            os.kill(record["pid"], signal.SIGKILL)
        except ProcessLookupError:
            pass
        deadline = time.monotonic() + max(0, timeout)
        child = self._children.get(record["pid"])
        while time.monotonic() < deadline:
            if child is not None:
                try:
                    child.wait(timeout=min(0.05, max(0.001, deadline - time.monotonic())))
                except subprocess.TimeoutExpired:
                    pass
            current = self._process_identity(record["pid"])
            if current is None:
                break
            if current != (record["started_at"], record["pgid"]):
                raise ProcessOwnershipError("PID identity changed while waiting; retaining ownership record")
            time.sleep(0.05)
        else:
            raise ProcessOwnershipError("owned process did not stop before timeout; retaining ownership record")

        if self._group_members(record["pgid"]):
            raise ProcessOwnershipError("leader stopped but process group still has members; retaining ownership record")
        self._write_records([item for item in records if item["label"] != label])
        self._children.pop(record["pid"], None)
        return "crashed"

    def stop_all(self, timeout=5):
        results = []
        errors = []
        for record in reversed(self.read_records()):
            try:
                results.append((record["label"], self.stop(record["label"], timeout=timeout)))
            except ProcessOwnershipError as exc:
                errors.append(f"{record['label']}: {exc}")
        if errors:
            raise ProcessOwnershipError("; ".join(errors))
        return results

    def read_records(self):
        self._validate_root()
        try:
            info = self.path.lstat()
        except FileNotFoundError:
            return []
        except OSError as exc:
            raise ProcessOwnershipError("process registry cannot be inspected") from exc
        if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
            raise ProcessOwnershipError("process registry must be a regular non-symlink file")
        if info.st_uid != os.getuid() or info.st_mode & 0o077:
            raise ProcessOwnershipError("process registry must be user-owned and private")
        try:
            payload = json.loads(self.path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise ProcessOwnershipError("process registry is unreadable or invalid") from exc
        if not isinstance(payload, dict) or payload.get("schema_version") != 1 or not isinstance(payload.get("processes"), list):
            raise ProcessOwnershipError("process registry schema is invalid")
        records = payload["processes"]
        labels = set()
        for item in records:
            if (
                not isinstance(item, dict)
                or set(item) != {"label", "pid", "started_at", "pgid"}
                or not isinstance(item["label"], str)
                or not isinstance(item["pid"], int)
                or not isinstance(item["pgid"], int)
                or isinstance(item["pid"], bool)
                or isinstance(item["pgid"], bool)
                or item["pid"] <= 1
                or item["pgid"] <= 1
                or not isinstance(item["started_at"], str)
                or not item["started_at"]
                or item["label"] in labels
            ):
                raise ProcessOwnershipError("process registry contains an invalid ownership record")
            labels.add(item["label"])
        return records

    def _write_records(self, records):
        self._validate_root()
        if self.path.exists() or self.path.is_symlink():
            info = self.path.lstat()
            if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
                raise ProcessOwnershipError("refusing to replace an unowned process registry")
        temp_path = self.root / f".{self._filename}.{uuid.uuid4().hex}.tmp"
        fd = os.open(temp_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as stream:
                json.dump({"schema_version": 1, "processes": records}, stream, separators=(",", ":"))
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(temp_path, self.path)
            os.chmod(self.path, 0o600)
        except Exception:
            try:
                temp_path.unlink()
            except OSError:
                pass
            raise

    def _validate_root(self):
        try:
            info = self.root.lstat()
        except OSError as exc:
            raise ProcessOwnershipError("fixture_root is unavailable") from exc
        if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
            raise ProcessOwnershipError("fixture_root must be a real directory")
        if info.st_uid != os.getuid() or info.st_mode & 0o077:
            raise ProcessOwnershipError("fixture_root must be user-owned and private")

    @staticmethod
    def _process_identity(pid):
        result = subprocess.run(
            ["/bin/ps", "-p", str(pid), "-o", "lstart=", "-o", "pgid="],
            capture_output=True,
            text=True,
            check=False,
            env={"PATH": "/usr/bin:/bin"},
            timeout=5,
        )
        line = result.stdout.strip()
        if result.returncode == 0 and line:
            return OwnedProcessRegistry._parse_process_identity_line(line)
        if result.returncode not in (0, 1):
            raise ProcessOwnershipError("could not inspect process identity")

        # A failed/empty per-PID probe is not absence by itself. Confirm absence
        # with a successful complete process table; permission/tool errors must
        # retain the ownership record and prevent any signal or release.
        table = subprocess.run(
            ["/bin/ps", "-axo", "pid=,lstart=,pgid="],
            capture_output=True,
            text=True,
            check=False,
            env={"PATH": "/usr/bin:/bin"},
            timeout=5,
        )
        if table.returncode != 0:
            raise ProcessOwnershipError("could not confirm process absence")
        for table_line in table.stdout.splitlines():
            fields = table_line.split()
            if len(fields) < 7 or not fields[0].isdigit():
                continue
            if int(fields[0]) == pid:
                return OwnedProcessRegistry._parse_process_identity_line(" ".join(fields[1:]))
        return None

    @staticmethod
    def _parse_process_identity_line(line):
        fields = line.rsplit(None, 1)
        if len(fields) != 2:
            raise ProcessOwnershipError("could not parse process start identity")
        try:
            return fields[0], int(fields[1])
        except ValueError as exc:
            raise ProcessOwnershipError("could not parse process group identity") from exc

    @staticmethod
    def _group_members(pgid):
        result = subprocess.run(
            ["/bin/ps", "-axo", "pid=,pgid="],
            capture_output=True,
            text=True,
            check=False,
            env={"PATH": "/usr/bin:/bin"},
            timeout=5,
        )
        if result.returncode != 0:
            raise ProcessOwnershipError("could not inspect process group membership")
        members = []
        for line in result.stdout.splitlines():
            fields = line.split()
            if len(fields) == 2 and fields[1].isdigit() and int(fields[1]) == pgid:
                try:
                    members.append(int(fields[0]))
                except ValueError:
                    continue
        return members

    def _wait_for_identity(self, pid, timeout):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            identity = self._process_identity(pid)
            if identity is not None:
                return identity
            time.sleep(0.05)
        return None
