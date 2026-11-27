#!/usr/bin/env python3
"""Read-only CI cost report. Saved evidence: --run-json RUN --jobs-json JOBS.

Or collect one attempt: --run-id ID --attempt N [--repo kdlbs/kandev].
Optional --graph JSON declares identity {workflow_id,path,head_sha} and needs
keyed by exact API job names (including expanded matrix names). Supply only a
graph verified against that workflow revision. Without it, scheduling delay
and execution critical path are unknown. Ready-to-start delay includes GitHub
scheduling and runner provisioning; it does not identify the queue cause.
"""
from __future__ import annotations

import argparse
from datetime import datetime
import json
from pathlib import Path
import re
import subprocess


def timestamp(value):
    try:
        result = datetime.fromisoformat(value.replace("Z", "+00:00"))
        return result if result.tzinfo else None
    except (AttributeError, TypeError, ValueError):
        return None


def duration(item):
    if item.get("conclusion") == "skipped":
        return 0.0
    start, end = timestamp(item.get("started_at")), timestamp(item.get("completed_at"))
    if item.get("status") != "completed" or start is None or end is None or end < start:
        return None
    return (end - start).total_seconds()


def graph_metrics(run, jobs, graph):
    identity = {key: run.get(key) for key in ("workflow_id", "path", "head_sha")}
    if any(value is None for value in identity.values()) or graph.get("identity") != identity:
        raise ValueError("graph workflow identity does not match the attempt")
    by_name = {job["name"]: job for job in jobs}
    needs = graph.get("needs", {})
    if len(by_name) != len(jobs) or set(needs) != set(by_name):
        raise ValueError("graph must map every job exactly once")
    paths, delays, visiting = {}, {}, set()
    origin = timestamp(run.get("run_started_at"))

    def visit(name):
        if name in visiting or name not in by_name:
            raise ValueError("graph contains a cycle or unknown dependency")
        if name in paths:
            return paths[name]
        visiting.add(name)
        predecessors = [visit(parent) for parent in needs[name]]
        item = by_name[name]
        seconds = duration(item)
        paths[name] = (max(predecessors, default=0) + seconds
                       if seconds is not None and all(p is not None for p in predecessors) else None)
        ends = [timestamp(by_name[parent].get("completed_at")) for parent in needs[name]] or [origin]
        start = timestamp(item.get("started_at"))
        delays[name] = None
        if start and all(ends) and item.get("conclusion") != "skipped":
            ready = max(ends)
            if start < ready:
                raise ValueError("job starts before its declared dependencies complete")
            delays[name] = (start - ready).total_seconds()
        visiting.remove(name)
        return paths[name]

    for name in by_name:
        visit(name)
    critical = max(paths.values(), default=0) if all(p is not None for p in paths.values()) else None
    return critical, delays


def summarize(run, evidence, graph=None):
    jobs = evidence["jobs"]
    seen = set()
    reasons = []
    if evidence.get("total_count") != len(jobs) or not jobs:
        reasons.append("job list is incomplete")
    for job in jobs:
        if any(job.get(k) != run.get(v) or run.get(v) is None
               for k, v in (("run_id", "id"), ("run_attempt", "run_attempt"), ("head_sha", "head_sha"))):
            raise ValueError("jobs must belong to the exact run attempt and source revision")
        if job["id"] in seen:
            raise ValueError("duplicate job ID")
        seen.add(job["id"])
    critical, delays = graph_metrics(run, jobs, graph) if graph else (None, {})
    rows = []
    for job in jobs:
        seconds = duration(job)
        if seconds is None:
            reasons.append(f"job {job['id']} has unknown execution duration")
        rows.append({**{k: job.get(k) for k in ("id", "name", "html_url", "status", "conclusion",
                                              "labels", "runner_name", "created_at", "started_at", "completed_at")},
                     "execution_seconds": seconds,
                     "dependency_ready_delay_seconds": delays.get(job["name"]),
                     "steps": [{**step, "execution_seconds": duration(step)} for step in job.get("steps", [])]})
    if run.get("status") != "completed":
        reasons.append("workflow attempt is not complete")
    start = timestamp(run.get("run_started_at"))
    ends = [timestamp(job.get("completed_at")) for job in jobs]
    ends = [end for end in ends if end is not None]
    elapsed = (max(ends) - start).total_seconds() if start and ends and max(ends) >= start else None
    known = sum(row["execution_seconds"] or 0 for row in rows) / 60
    return {"schema_version": 1, "workflow": {k: run.get(k) for k in
            ("id", "run_attempt", "workflow_id", "path", "head_sha", "event", "html_url", "status", "conclusion", "run_started_at")},
            "complete": not reasons, "incomplete_reasons": reasons,
            "known_runner_minutes": known, "runner_minutes": known if not reasons else None,
            "observed_elapsed_seconds": elapsed, "execution_critical_path_seconds": critical,
            "scheduling_note": "Dependency-ready delay includes scheduling and provisioning; cause unknown.",
            "jobs": rows}


def collect(repo, run_id, attempt):
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repo) or min(run_id, attempt) < 1:
        raise ValueError("invalid repository, run ID, or attempt")

    def get(endpoint):
        return json.loads(subprocess.check_output(["gh", "api", "--method", "GET", endpoint], timeout=60))

    endpoint = f"repos/{repo}/actions/runs/{run_id}/attempts/{attempt}"
    run = get(endpoint)
    jobs = []
    for page in range(1, 101):
        response = get(f"{endpoint}/jobs?per_page=100&page={page}")
        jobs.extend(response["jobs"])
        if len(jobs) >= response["total_count"] or not response["jobs"]:
            return run, {"total_count": response["total_count"], "jobs": jobs}
    raise ValueError("job collection exceeded the 100-page bound")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-json", type=Path)
    parser.add_argument("--jobs-json", type=Path)
    parser.add_argument("--run-id", type=int)
    parser.add_argument("--attempt", type=int)
    parser.add_argument("--repo", default="kdlbs/kandev")
    parser.add_argument("--graph", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.run_id and args.attempt and not (args.run_json or args.jobs_json):
        run, jobs = collect(args.repo, args.run_id, args.attempt)
    elif args.run_json and args.jobs_json and not (args.run_id or args.attempt):
        run, jobs = json.loads(args.run_json.read_text()), json.loads(args.jobs_json.read_text())
    else:
        parser.error("supply --run-id and --attempt OR --run-json and --jobs-json")
    report = summarize(run, jobs, json.loads(args.graph.read_text()) if args.graph else None)
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(f"Run {run['id']} attempt {run['run_attempt']}: {len(jobs['jobs'])} jobs; "
          f"{report['known_runner_minutes']:.1f} known runner-minutes; complete={report['complete']}")
    print(f"Observed elapsed: {report['observed_elapsed_seconds']} seconds; "
          f"execution critical path: {report['execution_critical_path_seconds']} seconds")
    for reason in report["incomplete_reasons"]:
        print(f"Incomplete: {reason}")


if __name__ == "__main__":
    main()
