#!/usr/bin/env python3
"""Capture boot payload size and route membership for the journey fixture."""

import argparse
import json
import time
import urllib.request


BOOT_MARKER = "__KANDEV_BOOT_PAYLOAD__="
ROUTES = (
    "/?workflowId=journey-workflow-0",
    "/tasks/journey-task-0000",
    "/tasks/journey-task-0001",
    "/tasks/journey-task-0001?sessionId=journey-task-0001-session-3",
)


def extract_payload(raw):
    html = raw.decode()
    start = html.index(BOOT_MARKER) + len(BOOT_MARKER)
    return json.JSONDecoder().raw_decode(html[start:])[0]


def task_ids(node):
    if not isinstance(node, dict):
        return []
    direct = node.get("taskIds")
    if isinstance(direct, list):
        return [value for value in direct if isinstance(value, str)]
    tasks = node.get("tasks")
    if isinstance(tasks, list):
        return [task.get("id") for task in tasks if isinstance(task, dict) and isinstance(task.get("id"), str)]
    return []


def session_ids(task_sessions):
    if not isinstance(task_sessions, dict):
        return []
    direct = task_sessions.get("sessionIds")
    if isinstance(direct, list):
        return [value for value in direct if isinstance(value, str)]
    items = task_sessions.get("items")
    if isinstance(items, dict):
        return [value for value in items if isinstance(value, str)]
    return []


def count_embedded_entities(node, entity_type, found=None):
    if found is None:
        found = set()
    if isinstance(node, list):
        for value in node:
            count_embedded_entities(value, entity_type, found)
    elif isinstance(node, dict):
        entity_id = node.get("id")
        if isinstance(entity_id, str):
            if entity_type == "task" and ("workflow_id" in node or "workspace_id" in node):
                found.add(entity_id)
            elif entity_type == "session" and "task_id" in node and "state" in node:
                found.add(entity_id)
        for value in node.values():
            count_embedded_entities(value, entity_type, found)
    return len(found)


def entity_count(payload, entity_type):
    entities = payload.get("entities")
    plural = f"{entity_type}s"
    table = entities.get(plural) if isinstance(entities, dict) else None
    if isinstance(table, dict):
        return len(table)
    return count_embedded_entities(payload, entity_type)


def summarize_payload(payload, route, run, elapsed_ms, html_bytes):
    state = payload.get("initialState", {})
    route_data = payload.get("routeData", {})
    detail = route_data.get("taskDetail", {}) if isinstance(route_data, dict) else {}
    detail_state = detail.get("initialState", {}) if isinstance(detail, dict) else {}
    target = detail_state or state
    kanban = target.get("kanban", {}) if isinstance(target, dict) else {}
    multi = target.get("kanbanMulti", {}) if isinstance(target, dict) else {}
    snapshots = multi.get("snapshots", {}) if isinstance(multi, dict) else {}
    session_state = target.get("taskSessions", {}) if isinstance(target, dict) else {}
    session_rows = session_ids(session_state)
    return {
        "route": route,
        "run": run,
        "boot_version": payload.get("version", 1),
        "ms": elapsed_ms,
        "html_bytes": html_bytes,
        "boot_bytes": len(json.dumps(payload, separators=(",", ":")).encode()),
        "boot_keys": list(payload),
        "state_keys": list(state) if isinstance(state, dict) else [],
        "task_entities": entity_count(payload, "task"),
        "session_entities": entity_count(payload, "session"),
        "detail_session": detail.get("sessionId") if isinstance(detail, dict) else None,
        "board_tasks": len(task_ids(kanban)),
        "snapshot_tasks": sum(len(task_ids(snapshot)) for snapshot in snapshots.values()) if isinstance(snapshots, dict) else 0,
        "detail_session_rows": len(session_rows),
        "message_rows": sum(len(value) for value in target.get("messages", {}).get("bySession", {}).values()),
        "turn_rows": sum(len(value) for value in target.get("turns", {}).get("bySession", {}).values()),
    }


def measure(base_url):
    assert base_url.startswith(("http://localhost:", "http://127.0.0.1:"))
    assert not base_url.endswith(":38429"), "Use an isolated instance"
    out = []
    for route in ROUTES:
        for run in range(3):
            start = time.perf_counter()
            raw = urllib.request.urlopen(base_url.rstrip("/") + route).read()
            elapsed_ms = (time.perf_counter() - start) * 1000
            payload = extract_payload(raw)
            out.append(summarize_payload(payload, route, run, elapsed_ms, len(raw)))
    return out


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", required=True)
    args = parser.parse_args()
    print(json.dumps(measure(args.base_url), indent=2))


if __name__ == "__main__":
    main()
