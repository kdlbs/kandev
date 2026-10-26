#!/usr/bin/env python3
"""Model-free ACP peer and process-tree fixture for the native executor E2E.

The normal mode implements a small ACP initialize/session/prompt exchange and
keeps a child and grandchild alive in the same process group. The runner can
independently record their PIDs while the real Kandev/agentctl session is up.
No network access, credentials, or model endpoints are used.
"""

import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path


def identity():
    return {"pid": os.getpid(), "ppid": os.getppid(), "pgid": os.getpgrp()}


def write_identity(path: str) -> None:
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    temporary = target.with_name(target.name + f".{os.getpid()}.tmp")
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w", encoding="ascii") as handle:
        json.dump(identity(), handle, sort_keys=True)
        handle.write("\n")
    os.replace(temporary, target)


def send(message):
    sys.stdout.write(json.dumps(message, separators=(",", ":")) + "\n")
    sys.stdout.flush()


def child_mode(role: str, identity_file: str) -> int:
    write_identity(identity_file)
    if role == "child":
        subprocess.Popen(
            [sys.executable, os.path.abspath(__file__), "--grandchild", "--identity-file", identity_file + ".grandchild"],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            close_fds=True,
        )
    while True:
        time.sleep(60)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--identity-dir")
    parser.add_argument("--child", action="store_true")
    parser.add_argument("--grandchild", action="store_true")
    parser.add_argument("--identity-file")
    args = parser.parse_args()
    if args.child or args.grandchild:
        if not args.identity_file:
            return 2
        return child_mode("grandchild" if args.grandchild else "child", args.identity_file)

    if not args.identity_dir:
        return 2

    identity_dir = Path(args.identity_dir)
    identity_dir.mkdir(parents=True, exist_ok=True)
    write_identity(str(identity_dir / "acp.json"))
    subprocess.Popen(
        [sys.executable, os.path.abspath(__file__), "--child", "--identity-file", str(identity_dir / "child.json")],
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        close_fds=True,
    )

    for line in sys.stdin:
        try:
            message = json.loads(line)
        except (json.JSONDecodeError, UnicodeDecodeError):
            continue
        if not isinstance(message, dict) or "id" not in message or not isinstance(message.get("method"), str):
            continue
        method = message["method"]
        if method == "initialize":
            result = {
                "protocolVersion": 1,
                "agentInfo": {"name": "kandev-nomad-inert", "version": "1"},
                "agentCapabilities": {},
            }
        elif method == "session/new":
            result = {
                "sessionId": "nomad-inert-session",
                "modes": {"currentModeId": "default", "availableModes": [{"id": "default", "name": "Default"}]},
            }
        elif method == "session/prompt":
            params = message.get("params") if isinstance(message.get("params"), dict) else {}
            send({
                "jsonrpc": "2.0",
                "method": "session/update",
                "params": {
                    "sessionId": params.get("sessionId", "nomad-inert-session"),
                    "update": {
                        "sessionUpdate": "agent_message_chunk",
                        "content": {"type": "text", "text": "nomad-inert-result"},
                    },
                },
            })
            result = {"stopReason": "end_turn"}
        elif method in {"session/cancel", "session/close"}:
            result = {}
        else:
            send({"jsonrpc": "2.0", "id": message["id"], "error": {"code": -32601, "message": "method not supported by inert fixture"}})
            continue
        send({"jsonrpc": "2.0", "id": message["id"], "result": result})
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
