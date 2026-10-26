"""Strict bootstrap contract for the create-and-crash pre-ready recovery mode.

Validation is local and offline: it reads files and stats paths only. The
Nomad token is read into memory by `read_token` and never echoed.
"""

from __future__ import annotations

import ipaddress
import os
import posixpath
import re
import stat
import subprocess
from pathlib import Path
from typing import Any, Callable
from urllib.parse import urlsplit

from nomad_native_e2e import PreflightError, _keys, _uuid, _verify_runtime_bundle

LIVE_KANDEV_PORT = 8817
CHECKOUT_ROOT = Path(__file__).resolve().parent.parent

_TOP_REQUIRED = {
    "schema_version", "kandev_binary", "nomad", "ssh", "workspace_root", "allocation_root",
    "cpu_mhz", "memory_mb", "repository", "expected_file", "temp_root_parent",
}
_TOP_OPTIONAL = {"agent_profile_name", "partition", "pool", "ssh_client"}
_POOL_KEYS = {"node_pool", "placement", "ssh_user", "memory_max_mb", "host_identity_token_file"}
_NOMAD_KEYS = {"address", "token_file", "namespace", "region", "datacenter", "node_id", "platform"}
_SSH_KEYS = {"target", "port", "host_fingerprint", "identity_file"}
_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,63}")
_REMOTE_PATH = re.compile(r"/[A-Za-z0-9._@:+/-]+")
_BRANCH = re.compile(r"[A-Za-z0-9][A-Za-z0-9._/-]{0,127}")
_LOOPBACK_NAMES = {"localhost"}


def _inside(path: Path, parent: Path) -> bool:
    return path == parent or parent in path.parents


def _private_regular_file(path_text: Any, label: str, max_size: int = 1 << 16) -> Path:
    if not isinstance(path_text, str) or not os.path.isabs(path_text):
        raise PreflightError(f"{label} must be an absolute path")
    path = Path(path_text)
    try:
        info = path.lstat()
    except OSError as exc:
        raise PreflightError(f"{label} must exist") from exc
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        raise PreflightError(f"{label} must be a regular non-symlink file")
    if info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise PreflightError(f"{label} must be owned by this user with no group/world permissions")
    if not 0 < info.st_size <= max_size:
        raise PreflightError(f"{label} has an unsupported size")
    return path


def read_token(path: Path) -> str:
    token = Path(path).read_text(encoding="utf-8").strip()
    if not token or any(ch.isspace() for ch in token):
        raise PreflightError("nomad.token_file must contain exactly one non-empty token")
    return token


def nomad_address(value: Any) -> str:
    """http is accepted only for loopback; any other host must use https. Port is mandatory."""
    if not isinstance(value, str):
        raise PreflightError("nomad.address must be an http(s) URL")
    parsed = urlsplit(value)
    if parsed.scheme not in ("http", "https") or parsed.username or parsed.password or parsed.query or parsed.fragment or parsed.path not in ("", "/"):
        raise PreflightError("nomad.address must be scheme://host:port without credentials, path, or query")
    try:
        host, port = parsed.hostname, parsed.port
    except ValueError as exc:
        raise PreflightError("nomad.address has an invalid port") from exc
    if not host or port is None or not 1 <= port <= 65535:
        raise PreflightError("nomad.address must include an explicit host and port")
    try:
        loopback = ipaddress.ip_address(host).is_loopback
    except ValueError:
        loopback = host.lower() in _LOOPBACK_NAMES
        if not loopback and not re.fullmatch(r"[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?", host):
            raise PreflightError("nomad.address host is invalid")
    if parsed.scheme == "http" and not loopback:
        raise PreflightError("nomad.address must use https unless it is a loopback address")
    return f"{parsed.scheme}://{parsed.netloc}"


def _remote_path(value: Any, label: str) -> str:
    if not isinstance(value, str) or not _REMOTE_PATH.fullmatch(value) or posixpath.normpath(value) != value or value == "/":
        raise PreflightError(f"{label} must be a normalized absolute path using only safe characters")
    return value


def _positive_int(value: Any, label: str) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value < 1:
        raise PreflightError(f"{label} must be a positive integer")
    return value


def _repository(value: Any, checkout: Path) -> dict[str, str]:
    if not isinstance(value, dict) or "base_branch" not in value or len(value) != 2 or not (value.keys() & {"path", "url"}):
        raise PreflightError("repository must contain base_branch and exactly one of path or url")
    if not isinstance(value["base_branch"], str) or not _BRANCH.fullmatch(value["base_branch"]):
        raise PreflightError("repository.base_branch is invalid")
    if "url" in value:
        url = value["url"]
        parsed = urlsplit(url) if isinstance(url, str) else None
        if parsed is None or parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise PreflightError("repository.url must be an https URL without credentials")
        return {"url": url, "base_branch": value["base_branch"]}
    path_text = value["path"]
    if not isinstance(path_text, str) or not os.path.isabs(path_text):
        raise PreflightError("repository.path must be an absolute path")
    path = Path(path_text)
    if path.is_symlink() or not path.is_dir():
        raise PreflightError("repository.path must be a real directory")
    resolved = path.resolve(strict=True)
    if _inside(resolved, checkout):
        raise PreflightError("repository.path must be outside the Kandev checkout")
    if not (resolved / ".git").exists():
        raise PreflightError("repository.path must be a git repository")
    return {"path": str(resolved), "base_branch": value["base_branch"]}


def _temp_root_parent(value: Any, checkout: Path) -> Path:
    if not isinstance(value, str) or not os.path.isabs(value):
        raise PreflightError("temp_root_parent must be an absolute path")
    path = Path(value)
    try:
        info = path.lstat()
    except OSError as exc:
        raise PreflightError("temp_root_parent must exist") from exc
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o022:
        raise PreflightError("temp_root_parent must be a real directory owned by this user and not group/world writable")
    resolved = path.resolve(strict=True)
    if _inside(resolved, checkout):
        raise PreflightError("temp_root_parent must be outside the Kandev checkout")
    for candidate in (resolved, *resolved.parents):
        if (candidate / ".git").exists():
            raise PreflightError("temp_root_parent must not be inside any git repository")
    return resolved


def _loopback_endpoint(value: Any, label: str) -> tuple[str, int]:
    match = re.fullmatch(r"(127\.0\.0\.1|localhost):(\d{1,5})", value) if isinstance(value, str) else None
    if match is None or not 1 <= int(match.group(2)) <= 65535:
        raise PreflightError(f"partition.{label} must be a loopback host:port")
    return match.group(1), int(match.group(2))


def validate_partition(value: Any, nomad_http_address: str) -> dict[str, str]:
    """Loopback relay endpoints for the disconnect scenario: the throwaway client dials `listen`, which relays to `upstream`."""
    if not isinstance(value, dict) or value.keys() != {"listen", "upstream"}:
        raise PreflightError("partition must contain exactly listen and upstream")
    listen, upstream = _loopback_endpoint(value["listen"], "listen"), _loopback_endpoint(value["upstream"], "upstream")
    nomad_port = urlsplit(nomad_http_address).port
    if listen[1] == upstream[1]:
        raise PreflightError("partition.listen and partition.upstream must use different ports")
    for label, port in (("listen", listen[1]), ("upstream", upstream[1])):
        if port == LIVE_KANDEV_PORT or port == nomad_port:
            raise PreflightError(f"partition.{label} must not use the live Kandev port or the Nomad HTTP port")
    return {"listen": value["listen"], "upstream": value["upstream"]}


def validate_pool(value: Any) -> dict[str, Any]:
    """Pool profile settings for the pool scenarios. Placement onto the fixture node is by node pool, never by node id."""
    if not isinstance(value, dict) or "node_pool" not in value or value.keys() - _POOL_KEYS:
        raise PreflightError("pool must contain node_pool and only node_pool, placement, ssh_user, memory_max_mb and host_identity_token_file")
    if not isinstance(value["node_pool"], str) or not _NAME.fullmatch(value["node_pool"]):
        raise PreflightError("pool.node_pool is invalid")
    if value.get("placement", "spread") not in ("spread", "binpack"):
        raise PreflightError("pool.placement must be spread or binpack")
    if "ssh_user" in value and (not isinstance(value["ssh_user"], str) or not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_-]{0,31}", value["ssh_user"])):
        raise PreflightError("pool.ssh_user is invalid")
    if "memory_max_mb" in value:
        _positive_int(value["memory_max_mb"], "pool.memory_max_mb")
    result = dict(value)
    if "host_identity_token_file" in value:
        result["host_identity_token_file"] = str(_private_regular_file(value["host_identity_token_file"], "pool.host_identity_token_file", max_size=4096))
    return result


def validate_ssh_client(value: Any) -> dict[str, Any]:
    """The fixture's ssh-node Nomad client agent: pool-partition stops and continues exactly this process."""
    if not isinstance(value, dict) or value.keys() != {"pid", "config"}:
        raise PreflightError("ssh_client must contain exactly pid and config")
    pid = _positive_int(value["pid"], "ssh_client.pid")
    if pid <= 1:
        raise PreflightError("ssh_client.pid is invalid")
    config = _private_regular_file(value["config"], "ssh_client.config", max_size=1 << 20)
    return {"pid": pid, "config": str(config)}


def validate_bootstrap(config: Any, run: Callable[..., Any] = subprocess.run, checkout_root: Path = CHECKOUT_ROOT) -> dict[str, Any]:
    """Returns a normalized copy; raises PreflightError before anything is created."""
    if not isinstance(config, dict) or config.keys() - (_TOP_REQUIRED | _TOP_OPTIONAL) or _TOP_REQUIRED - config.keys():
        raise PreflightError("bootstrap config has missing or unsupported fields")
    if config["schema_version"] != 1:
        raise PreflightError("bootstrap config must identify schema 1")
    checkout = Path(checkout_root).resolve(strict=True)
    binary = Path(config["kandev_binary"]) if isinstance(config["kandev_binary"], str) and os.path.isabs(config["kandev_binary"]) else None
    if binary is None or not binary.is_file():
        raise PreflightError("kandev_binary must be an absolute path to an existing file")
    bundle = _verify_runtime_bundle(binary, run)

    nomad = _keys(config["nomad"], _NOMAD_KEYS, "nomad")
    address = nomad_address(nomad["address"])
    token_file = _private_regular_file(nomad["token_file"], "nomad.token_file", max_size=4096)
    for key in ("namespace", "region", "datacenter"):
        if not isinstance(nomad[key], str) or not _NAME.fullmatch(nomad[key]):
            raise PreflightError(f"nomad.{key} is invalid")
    _uuid(nomad["node_id"], "nomad.node_id")
    if not isinstance(nomad["platform"], str) or not re.fullmatch(r"(linux|darwin)/(amd64|arm64)", nomad["platform"]):
        raise PreflightError("nomad.platform must be linux|darwin/amd64|arm64")

    ssh = _keys(config["ssh"], _SSH_KEYS, "ssh")
    if not isinstance(ssh["target"], str) or not re.fullmatch(r"([A-Za-z_][A-Za-z0-9_-]{0,31}@)?[A-Za-z0-9][A-Za-z0-9.-]*", ssh["target"]):
        raise PreflightError("ssh.target must be [user@]host")
    if not isinstance(ssh["port"], int) or isinstance(ssh["port"], bool) or not 1 <= ssh["port"] <= 65535:
        raise PreflightError("ssh.port must be a valid TCP port")
    if not isinstance(ssh["host_fingerprint"], str) or not re.fullmatch(r"SHA256:[A-Za-z0-9+/]{43}", ssh["host_fingerprint"]):
        raise PreflightError("ssh.host_fingerprint must be a SHA256:<base64> fingerprint")
    identity = _private_regular_file(ssh["identity_file"], "ssh.identity_file")

    workspace_root = _remote_path(config["workspace_root"], "workspace_root")
    allocation_root = _remote_path(config["allocation_root"], "allocation_root")
    if allocation_root.count("/") < 3:
        raise PreflightError("allocation_root must be at least two directories below the filesystem root")
    expected_file = config["expected_file"]
    if not isinstance(expected_file, str) or not expected_file or os.path.isabs(expected_file) or ".." in Path(expected_file).parts:
        raise PreflightError("expected_file must be a relative path inside the repository")
    repository = _repository(config["repository"], checkout)
    if "path" in repository and not (Path(repository["path"]) / expected_file).is_file():
        raise PreflightError("expected_file is not present in repository.path")
    profile_name = config.get("agent_profile_name")
    if profile_name is not None and (not isinstance(profile_name, str) or not profile_name.strip()):
        raise PreflightError("agent_profile_name must be a non-empty string")
    return {
        "kandev_binary": str(binary), "bundle": bundle["bundle"],
        "nomad": {**nomad, "address": address, "token_file": str(token_file)},
        "ssh": {**ssh, "identity_file": str(identity)},
        "workspace_root": workspace_root, "allocation_root": allocation_root,
        "cpu_mhz": _positive_int(config["cpu_mhz"], "cpu_mhz"), "memory_mb": _positive_int(config["memory_mb"], "memory_mb"),
        "repository": repository, "expected_file": expected_file,
        "temp_root_parent": str(_temp_root_parent(config["temp_root_parent"], checkout)),
        "agent_profile_name": profile_name,
        "partition": validate_partition(config["partition"], address) if "partition" in config else None,
        "pool": validate_pool(config["pool"]) if "pool" in config else None,
        "ssh_client": validate_ssh_client(config["ssh_client"]) if "ssh_client" in config else None,
    }
