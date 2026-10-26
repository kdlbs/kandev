"""Build safe Nomad profile payloads from the original fixture reference manifest.

The manifest is the source of truth for secret references. Never feed this
builder an executor/profile GET response: Kandev masks secret references in
those projections and a masked value is not a credential reference.
"""

from __future__ import annotations

import json
import argparse
import os
import re
import stat
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Mapping


class ProfilePayloadError(ValueError):
    pass


_REF = re.compile(r"^\$kandev-secret:([0-9a-fA-F-]{36})$")
_CONFIG_KEYS = {
    "address", "namespace", "region", "datacenter", "node_id", "platform",
    "ssh_target", "ssh_port", "ssh_host_fingerprint", "workspace_root",
    "allocation_root", "cpu_mhz", "memory_mb",
}
_POOL_REQUIRED = {"address", "namespace", "region", "datacenter", "platform", "node_pool"}
_POOL_OPTIONAL = {
    "placement", "ssh_user", "cpu_mhz", "memory_mb", "memory_max_mb", "pin_node_id", "exclude_local", "prefer_local",
}


@dataclass(frozen=True)
class OriginalProfileReferences:
    executor_id: str
    profile_id: str
    profile_name: str
    nomad_token_ref: str
    config: Mapping[str, str]


def load_original_reference_manifest(path: str | Path, fixture_root: str | Path) -> OriginalProfileReferences:
    root = Path(fixture_root).resolve(strict=True)
    source = Path(path)
    try:
        info = source.lstat()
        resolved = source.resolve(strict=True)
    except OSError as exc:
        raise ProfilePayloadError("original profile reference manifest is unavailable") from exc
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        raise ProfilePayloadError("original profile reference manifest must be a regular file")
    if root not in resolved.parents or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ProfilePayloadError("original profile reference manifest must be private and fixture-owned")
    try:
        value = json.loads(resolved.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ProfilePayloadError("original profile reference manifest is invalid") from exc
    return validate_profile_source(value)


def validate_profile_source(source: Any) -> OriginalProfileReferences:
    required = {"schema_version", "executor_id", "profile_id", "profile_name", "nomad_token_ref", "config"}
    if not isinstance(source, dict) or set(source) != required:
        raise ProfilePayloadError("profile source must contain only original reference-manifest fields")
    if source["schema_version"] != 1:
        raise ProfilePayloadError("profile reference manifest version is unsupported")
    if not isinstance(source["executor_id"], str) or not re.fullmatch(r"[0-9a-fA-F-]{36}", source["executor_id"]):
        raise ProfilePayloadError("executor_id must be a UUID")
    if not isinstance(source["profile_id"], str) or not re.fullmatch(r"[0-9a-fA-F-]{36}", source["profile_id"]):
        raise ProfilePayloadError("profile_id must be a UUID")
    if not isinstance(source["profile_name"], str) or not source["profile_name"].strip():
        raise ProfilePayloadError("profile_name is required")
    if not isinstance(source["nomad_token_ref"], str) or not _REF.fullmatch(source["nomad_token_ref"]):
        raise ProfilePayloadError("nomad_token_ref must be the original $kandev-secret:<UUID> reference")
    config = source["config"]
    pool = isinstance(config, dict) and "node_pool" in config
    if not isinstance(config, dict) or (pool and not _POOL_REQUIRED <= set(config) <= _POOL_REQUIRED | _POOL_OPTIONAL) \
            or (not pool and set(config) != _CONFIG_KEYS):
        raise ProfilePayloadError("profile config fields do not match the typed Nomad profile")
    if any(not isinstance(key, str) or not isinstance(value, str) or not value for key, value in config.items()):
        raise ProfilePayloadError("profile config values must be non-empty strings")
    if config.get("nomad_token") in {"***", ""}:
        raise ProfilePayloadError("masked or empty Nomad token values cannot be submitted")
    return OriginalProfileReferences(
        executor_id=source["executor_id"],
        profile_id=source["profile_id"],
        profile_name=source["profile_name"],
        nomad_token_ref=source["nomad_token_ref"],
        config=dict(config),
    )


def build_profile_payload(source: Any) -> dict[str, Any]:
    if not isinstance(source, OriginalProfileReferences):
        raise ProfilePayloadError("profile payloads must come from the private original reference manifest")
    config = dict(source.config)
    config["nomad_token"] = source.nomad_token_ref
    # Validate the exact API payload shape at this boundary, before any API
    # mutation or task submission can happen.
    if config["nomad_token"] == "***" or not _REF.fullmatch(config["nomad_token"]):
        raise ProfilePayloadError("nomad_token must remain an original secret reference")
    return {"name": source.profile_name, "config": config}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixture-root", required=True)
    parser.add_argument("--source-manifest", required=True)
    args = parser.parse_args(argv)
    source = load_original_reference_manifest(args.source_manifest, args.fixture_root)
    print(json.dumps(build_profile_payload(source), sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

