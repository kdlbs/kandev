"""Compare complete Vitest JSON reports, preserving identity multiplicity.

Usage: python3 compare-setup-reports.py BASELINE_JSON CANDIDATE_JSON [...]
Prints compact evidence, including failed attempts. Raw reports stay outside Git.
Project labels are provenance, not identity, when comparing setup migrations.
"""
from collections import Counter
import hashlib
import json
from pathlib import Path
import sys


def normalize(path):
    raw = path.read_bytes()
    report = json.loads(raw)
    files = Counter()
    identities = Counter()
    outcomes = Counter()
    for suite in report["testResults"]:
        filename = suite["name"].replace("\\", "/").split("/apps/web/")[-1]
        files[filename] += 1
        for test in suite["assertionResults"]:
            identities[(filename, test["fullName"], test["status"])] += 1
            outcomes[test["status"]] += 1
    seconds = (max(s["endTime"] for s in report["testResults"]) - report["startTime"]) / 1000
    evidence = {
        "report": path.name,
        "sha256": hashlib.sha256(raw).hexdigest(),
        "success": report["success"],
        "files": sum(files.values()),
        "assertions": sum(identities.values()),
        "outcomes": dict(outcomes),
        "report_elapsed_seconds": round(seconds, 3),
    }
    return evidence, files, identities


if len(sys.argv) < 3:
    raise SystemExit("Supply a baseline and at least one candidate report")
baseline, baseline_files, baseline_identities = normalize(Path(sys.argv[1]))
results = [baseline]
equivalent = baseline["success"] and baseline["outcomes"].get("passed", 0) > 0
for argument in sys.argv[2:]:
    evidence, files, identities = normalize(Path(argument))
    evidence["files_match_baseline"] = files == baseline_files
    evidence["identity_outcomes_match_baseline"] = identities == baseline_identities
    evidence["missing_identity_outcomes"] = list((baseline_identities - identities).items())
    evidence["extra_identity_outcomes"] = list((identities - baseline_identities).items())
    equivalent = (equivalent and evidence["success"] and evidence["outcomes"].get("passed", 0) > 0
                  and files == baseline_files and identities == baseline_identities)
    results.append(evidence)
print(json.dumps({"all_passed_and_equivalent": equivalent, "reports": results}, indent=2))
raise SystemExit(0 if equivalent else 1)
