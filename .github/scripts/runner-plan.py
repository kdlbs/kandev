#!/usr/bin/env python3
"""Build deterministic runner assignments for eligible Linux CI jobs."""

import argparse
import hashlib
import json
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


@dataclass(frozen=True)
class RunnerPlan:
    outputs: dict[str, str]
    warnings: list[str]


GITHUB_ASSIGNMENT = "github"
EXTERNAL_ASSIGNMENT = "external"


def _parse_percentage(raw: str | None) -> tuple[int, list[str]]:
    if raw is None or raw.strip() == "":
        return 0, []
    value = raw.strip()
    if not value.isascii() or not value.isdigit():
        return 0, [f"KANDEV_CI_EXTERNAL_PERCENT must be an integer from 0 to 100, got {raw!r}"]
    percentage = int(value)
    if percentage > 100:
        return 0, [f"KANDEV_CI_EXTERNAL_PERCENT must be an integer from 0 to 100, got {raw!r}"]
    return percentage, []


def _external_for_singleton(
    *, workflow: str, run_id: str, family: str, percentage: int
) -> bool:
    if percentage <= 0:
        return False
    if percentage >= 100:
        return True
    key = f"{workflow}\0{run_id}\0{family}".encode("utf-8")
    bucket = int.from_bytes(hashlib.sha256(key).digest()[:8], "big") % 100
    return bucket < percentage


def _matrix_assignments(
    *, workflow: str, run_id: str, family: str, instances: list[dict[str, int]], percentage: int
) -> set[int]:
    external_count = (len(instances) * percentage) // 100
    ranked: list[tuple[bytes, int]] = []
    for index, instance in enumerate(instances):
        matrix_key = json.dumps(instance, sort_keys=True, separators=(",", ":"))
        digest = hashlib.sha256(
            f"{workflow}\0{run_id}\0{family}\0{matrix_key}".encode("utf-8")
        ).digest()
        ranked.append((digest, index))
    return {index for _, index in sorted(ranked)[:external_count]}


def _plan_family(
    *,
    workflow: str,
    run_id: str,
    family: str,
    tier: str,
    instances: list[dict[str, int]],
    burst: str,
    percentage: int,
    labels: dict[str, str],
) -> list[dict[str, int | str]]:
    label_configured = bool(labels[tier])
    if burst != "true" or not label_configured or percentage == 0:
        external_indices: set[int] = set()
    elif len(instances) == 1:
        external_indices = (
            {0}
            if _external_for_singleton(
                workflow=workflow,
                run_id=run_id,
                family=family,
                percentage=percentage,
            )
            else set()
        )
    else:
        external_indices = _matrix_assignments(
            workflow=workflow,
            run_id=run_id,
            family=family,
            instances=instances,
            percentage=percentage,
        )

    return [
        {
            **instance,
            "runner": EXTERNAL_ASSIGNMENT
            if index in external_indices
            else GITHUB_ASSIGNMENT,
        }
        for index, instance in enumerate(instances)
    ]


def _families(workflow: str) -> Iterable[tuple[str, str, list[dict[str, int]]]]:
    def singleton() -> list[dict[str, int]]:
        return [{}]
    if workflow == "e2e":
        return (
            ("changes", "light", singleton()),
            ("build", "standard", singleton()),
            ("e2e", "standard", [{"shard": shard} for shard in range(1, 15)]),
            ("e2e_report", "standard", singleton()),
            ("e2e_gate", "light", singleton()),
        )
    if workflow == "backend":
        return (
            ("changes", "light", singleton()),
            ("static_checks", "standard", singleton()),
            (
                "backend_test",
                "standard",
                [{"shard": shard, "index": shard - 1} for shard in range(1, 3)],
            ),
            ("test_ambient_env", "standard", singleton()),
            ("test", "light", singleton()),
        )
    if workflow == "frontend":
        return (
            ("changes", "light", singleton()),
            ("frontend", "standard", singleton()),
            ("frontend_gate", "light", singleton()),
        )
    if workflow in {"architecture", "action-pinning", "harness-lint"}:
        return (("lint", "light", singleton()),)
    raise ValueError(f"unsupported workflow: {workflow}")


def _output_key(family: str, instances: list[dict[str, int]]) -> str:
    return f"{family}_matrix" if len(instances) > 1 else f"{family}_runner"


def build_plan(
    *,
    workflow: str,
    run_id: str,
    burst: str,
    percent: str | None,
    light_label: str,
    standard_label: str,
) -> RunnerPlan:
    percentage, warnings = _parse_percentage(percent)
    labels = {"light": light_label, "standard": standard_label}
    outputs: dict[str, str] = {}
    for family, tier, instances in _families(workflow):
        planned = _plan_family(
            workflow=workflow,
            run_id=run_id,
            family=family,
            tier=tier,
            instances=instances,
            burst=burst,
            percentage=percentage,
            labels=labels,
        )
        key = _output_key(family, instances)
        if len(instances) == 1:
            outputs[key] = planned[0]["runner"]
        else:
            outputs[key] = json.dumps(
                {"include": planned}, separators=(",", ":"), sort_keys=True
            )
    return RunnerPlan(outputs=outputs, warnings=warnings)


def _write_outputs(plan: RunnerPlan, output_path: Path) -> None:
    with output_path.open("a", encoding="utf-8") as output:
        for key, value in plan.outputs.items():
            output.write(f"{key}={value}\n")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workflow", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--burst", default="")
    parser.add_argument("--percent")
    parser.add_argument("--light-label", default="")
    parser.add_argument("--standard-label", default="")
    parser.add_argument("--output", type=Path, default=Path("/dev/stdout"))
    args = parser.parse_args()
    plan = build_plan(
        workflow=args.workflow,
        run_id=args.run_id,
        burst=args.burst,
        percent=args.percent,
        light_label=args.light_label,
        standard_label=args.standard_label,
    )
    for warning in plan.warnings:
        print(f"::warning::{warning}")
    _write_outputs(plan, args.output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
