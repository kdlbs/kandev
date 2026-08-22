#!/usr/bin/env python3
"""Seed a Kandev instance with a small team so workspace visibility and
membership can be clicked through.

Idempotent: re-running against an already-seeded instance reuses the existing
accounts and workspaces instead of failing on "email is already in use".
"""

from __future__ import annotations

import json
import sys
import urllib.error
import urllib.request
from http.cookiejar import CookieJar

PASSWORD = "kandev-demo-pw-1"


class Client:
    def __init__(self, base: str) -> None:
        self.base = base.rstrip("/")
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(CookieJar())
        )

    def call(self, method: str, path: str, body: dict | None = None) -> dict:
        data = json.dumps(body).encode() if body is not None else None
        request = urllib.request.Request(
            f"{self.base}{path}", data=data, method=method,
            headers={"Content-Type": "application/json"},
        )
        try:
            with self.opener.open(request, timeout=30) as response:
                raw = response.read().decode()
        except urllib.error.HTTPError as error:
            raw = error.read().decode()
            try:
                return {"_error": json.loads(raw).get("error", raw), "_status": error.code}
            except json.JSONDecodeError:
                return {"_error": raw, "_status": error.code}
        return json.loads(raw) if raw else {}


def unwrap_id(payload: dict) -> str | None:
    """Accept either {"id": ...} or {"user": {"id": ...}}."""
    if "id" in payload:
        return payload["id"]
    for key in ("user", "workspace", "task"):
        nested = payload.get(key)
        if isinstance(nested, dict) and "id" in nested:
            return nested["id"]
    return None


def ensure_user(client: Client, email: str, name: str, role: str) -> str:
    created = client.call(
        "POST", "/api/v1/users",
        {"email": email, "password": PASSWORD, "display_name": name, "role": role},
    )
    user_id = unwrap_id(created)
    if user_id:
        return user_id
    # Already present: find it in the directory.
    for user in client.call("GET", "/api/v1/users").get("users", []):
        if user.get("email") == email:
            return user["id"]
    raise SystemExit(f"could not create or find {email}: {created}")


def ensure_workspace(client: Client, name: str) -> str:
    for workspace in client.call("GET", "/api/v1/workspaces").get("workspaces", []):
        if workspace.get("name") == name:
            return workspace["id"]
    created = client.call("POST", "/api/v1/workspaces", {"name": name})
    workspace_id = unwrap_id(created)
    if not workspace_id:
        raise SystemExit(f"could not create workspace {name}: {created}")
    return workspace_id


def first_workflow(client: Client, workspace_id: str) -> str | None:
    payload = client.call("GET", f"/api/v1/workspaces/{workspace_id}/workflows")
    flows = payload.get("workflows") if isinstance(payload, dict) else payload
    if isinstance(flows, list) and flows:
        return flows[0].get("id")
    return None


def main() -> None:
    base = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8231"
    client = Client(base)

    setup = client.call(
        "POST", "/api/v1/auth/setup",
        {"email": "ana@example.com", "password": PASSWORD, "display_name": "Ana Ferreira"},
    )
    if "_error" in setup:
        login = client.call(
            "POST", "/api/v1/auth/login",
            {"email": "ana@example.com", "password": PASSWORD},
        )
        if "_error" in login:
            raise SystemExit(f"cannot authenticate as the admin: {login}")
    print("signed in as Ana (admin)")

    bruno = ensure_user(client, "bruno@example.com", "Bruno Costa", "member")
    carla = ensure_user(client, "carla@example.com", "Carla Nunes", "member")
    dana = ensure_user(client, "dana@example.com", "Dana Vieira (contractor)", "guest")
    print("colleagues ready: Bruno (member), Carla (member), Dana (guest)")

    team = ensure_workspace(client, "Platform Team")
    private = ensure_workspace(client, "Ana - security spike")

    client.call("PUT", f"/api/v1/workspaces/{team}/visibility", {"visibility": "org"})
    print("Platform Team is now visible to the whole organization")

    client.call("PUT", f"/api/v1/workspaces/{team}/members/{carla}", {"role": "viewer"})
    print("Carla narrowed to viewer on Platform Team")

    client.call("PUT", f"/api/v1/workspaces/{private}/members/{dana}", {"role": "collaborator"})
    print("Dana (guest) admitted to the private workspace only")

    workflow = first_workflow(client, team)
    if workflow:
        for title in (
            "Rate-limit the public search endpoint",
            "Flaky checkout test on CI",
            "Upgrade the Postgres driver",
        ):
            client.call("POST", "/api/v1/tasks", {
                "title": title, "workspace_id": team,
                "workflow_id": workflow, "start_agent": False,
            })
        print("seeded 3 tasks on the shared board")

    summary = f"""
Kandev team-access demo
  URL: {base}

  Platform Team        {team}   visibility = org
  Ana - security spike {private}   visibility = private

Sign in (password for all: {PASSWORD}):
  ana@example.com    admin   owns both workspaces
  bruno@example.com  member  sees Platform Team with NO invitation (collaborator)
  carla@example.com  member  narrowed to VIEWER on Platform Team
  dana@example.com   guest   sees ONLY the private workspace she was added to
"""
    print(summary)
    with open("/tmp/kandev-team-access-demo/demo-info.txt", "w", encoding="utf-8") as handle:
        handle.write(summary)


if __name__ == "__main__":
    main()
