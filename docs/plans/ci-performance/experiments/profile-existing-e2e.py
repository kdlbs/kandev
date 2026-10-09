"""Read existing Playwright blob artifacts; no extraction or test execution.

Usage: python3 profile-existing-e2e.py ARTIFACT_DIRECTORY OUTPUT_JSON
Fixture durations include child operations. Never add a child operation to its
parent's total or interpret summed fixture time as workflow elapsed time.
"""
import collections
import io
import json
import math
import pathlib
import re
import statistics
import sys
import zipfile

artifacts = pathlib.Path(sys.argv[1])
rows = []
tests = {}
provenance = set()
artifact_paths = sorted(artifacts.glob("blob-report-*.zip"))
if not artifact_paths:
    raise SystemExit("No blob artifacts found")
for artifact in artifact_paths:
    artifact_provenance = set()
    with zipfile.ZipFile(artifact) as outer:
        for name in outer.namelist():
            if not name.endswith(".zip"):
                continue
            with zipfile.ZipFile(io.BytesIO(outer.read(name))) as blob:
                steps = {}
                for line in blob.read("report.jsonl").splitlines():
                    event = json.loads(line)
                    params = event.get("params", {})
                    if event["method"] == "onProject":
                        project = params["project"]
                        workflow_url = project.get("metadata", {}).get("ci", {}).get("buildHref", "")
                        if not re.fullmatch(r"https://github\.com/[^/]+/[^/]+/actions/runs/[1-9][0-9]*", workflow_url):
                            raise SystemExit(f"Unverified workflow identity in {artifact.name}")
                        artifact_provenance.add(workflow_url)
                        def visit(entries):
                            for entry in entries:
                                if "testId" in entry:
                                    tests[entry["testId"]] = {"project": project["name"], "file": entry["location"]["file"], "title": entry["title"]}
                                visit(entry.get("entries", []))
                        visit(project["suites"])
                    elif event["method"] == "onStepBegin":
                        steps[(params["testId"], params["resultId"], params["step"]["id"])] = params["step"]
                    elif event["method"] == "onStepEnd":
                        key = (params["testId"], params["resultId"], params["step"]["id"])
                        step = steps.get(key)
                        if not step or step["category"] != "fixture":
                            continue
                        ancestor = step
                        phase = "unknown"
                        while ancestor:
                            if ancestor["title"] in ("Before Hooks", "After Hooks", "Worker Cleanup"):
                                phase = ancestor["title"]
                                break
                            ancestor = steps.get((*key[:2], ancestor.get("parentStepId")))
                        rows.append({"artifact": artifact.name, "testId": params["testId"],
                                     "fixture": step["title"], "phase": phase,
                                     "milliseconds": params["step"]["duration"]})
    if len(artifact_provenance) != 1:
        raise SystemExit(f"Expected one workflow identity in {artifact.name}")
    provenance.update(artifact_provenance)
    if len(provenance) != 1:
        raise SystemExit("Artifacts span multiple workflow runs")
for row in rows:
    row.update(tests.get(row["testId"], {}))

def aggregate(items):
    grouped = collections.defaultdict(list)
    for row in items:
        grouped[(row["fixture"], row["phase"])].append(row["milliseconds"])
    return [{"fixture": fixture, "phase": phase, "calls": len(values),
             "total_seconds": round(sum(values)/1000, 3),
             "median_ms": statistics.median(values), "p95_ms": sorted(values)[math.ceil(len(values)*0.95)-1],
             "p99_ms": sorted(values)[math.ceil(len(values)*0.99)-1], "max_ms": max(values)}
            for (fixture, phase), values in sorted(grouped.items())]

focused = ("chat/message-queue.spec.ts", "session/session-idle-parking.spec.ts",
           "session/mobile-transient-turn-runtime-continuity.spec.ts",
           "docker/docker-launch.spec.ts", "kubernetes/kubernetes-docker-workloads.spec.ts")
result = {"workflow_urls": sorted(provenance), "artifacts": len(artifact_paths),
          "run_attempt": None, "attempt_attribution": "unknown",
          "aggregate": aggregate(rows),
          "slow_test_page_setup": sorted([r for r in rows if r["fixture"] == 'Fixture "testPage"' and r["phase"] == "Before Hooks" and r["milliseconds"] >= 10000], key=lambda r: r["milliseconds"], reverse=True),
          "focused": {file: aggregate([row for row in rows if row.get("file") == file]) for file in focused}}
pathlib.Path(sys.argv[2]).write_text(json.dumps(result, indent=2)+"\n")
print(json.dumps({"workflow_urls": result["workflow_urls"], "artifacts": result["artifacts"],
                  "largest_fixture_totals": sorted(result["aggregate"], key=lambda x: x["total_seconds"], reverse=True)[:12]}, indent=2))
