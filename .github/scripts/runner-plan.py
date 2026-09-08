#!/usr/bin/env python3
"""Build deterministic runner assignments from workflow-owned family data."""

import argparse
import hashlib
import json
import re
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class RunnerPlan:
    plan: dict[str, object]
    warnings: list[str]


@dataclass(frozen=True)
class Family:
    name: str
    tier: str
    instances: tuple[dict[str, object], ...]
    is_matrix: bool


GITHUB_RUNNER = "ubuntu-latest"
VALID_TIERS = frozenset({"light", "standard"})
FAMILY_NAME = re.compile(r"[a-z][a-z0-9_]*\Z")


def _parse_percentage(raw: str | None) -> tuple[int, list[str]]:
    if raw is None or raw.strip() == "":
        return 0, []
    value = raw.strip()
    warning = (
        "KANDEV_CI_EXTERNAL_PERCENT must be an integer from 0 to 100, "
        f"got {raw!r}"
    )
    if not value.isascii() or not value.isdigit():
        return 0, [warning]
    percentage = int(value)
    if percentage > 100:
        return 0, [warning]
    return percentage, []


def _validate_families(raw: object) -> tuple[Family, ...]:
    if not isinstance(raw, list) or not raw:
        raise ValueError("families must be a non-empty JSON array")

    families: list[Family] = []
    names: set[str] = set()
    for entry in raw:
        if not isinstance(entry, dict):
            raise ValueError("each family must be a JSON object")
        name = entry.get("name")
        tier = entry.get("tier")
        if not isinstance(name, str) or FAMILY_NAME.fullmatch(name) is None:
            raise ValueError(f"invalid family name: {name!r}")
        if name in names:
            raise ValueError(f"duplicate family name: {name}")
        if tier not in VALID_TIERS:
            raise ValueError(f"family {name!r} has invalid tier: {tier!r}")

        is_matrix = "instances" in entry
        instances_value = entry.get("instances", [{}])
        if not isinstance(instances_value, list) or not instances_value:
            raise ValueError(f"family {name!r} instances must be a non-empty array")
        instances: list[dict[str, object]] = []
        for instance in instances_value:
            if not isinstance(instance, dict):
                raise ValueError(f"family {name!r} instances must be JSON objects")
            if "runner" in instance:
                raise ValueError(f"family {name!r} instance cannot define runner")
            instances.append(instance)

        names.add(name)
        families.append(
            Family(
                name=name,
                tier=tier,
                instances=tuple(instances),
                is_matrix=is_matrix,
            )
        )
    return tuple(families)


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
    *,
    workflow: str,
    run_id: str,
    family: str,
    instances: tuple[dict[str, object], ...],
    percentage: int,
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
    family: Family,
    burst: str,
    percentage: int,
    external_label: str,
) -> list[dict[str, object]]:
    if burst != "true" or not external_label or percentage == 0:
        external_indices: set[int] = set()
    elif not family.is_matrix:
        external_indices = (
            {0}
            if _external_for_singleton(
                workflow=workflow,
                run_id=run_id,
                family=family.name,
                percentage=percentage,
            )
            else set()
        )
    else:
        external_indices = _matrix_assignments(
            workflow=workflow,
            run_id=run_id,
            family=family.name,
            instances=family.instances,
            percentage=percentage,
        )

    return [
        {
            **instance,
            "runner": external_label if index in external_indices else GITHUB_RUNNER,
        }
        for index, instance in enumerate(family.instances)
    ]


def build_plan(
    *,
    workflow: str,
    run_id: str,
    families: object,
    burst: str,
    percent: str | None,
    light_label: str,
    standard_label: str,
) -> RunnerPlan:
    percentage, warnings = _parse_percentage(percent)
    labels = {"light": light_label, "standard": standard_label}
    plan: dict[str, object] = {}
    for family in _validate_families(families):
        planned = _plan_family(
            workflow=workflow,
            run_id=run_id,
            family=family,
            burst=burst,
            percentage=percentage,
            external_label=labels[family.tier],
        )
        if family.is_matrix:
            plan[f"{family.name}_matrix"] = {"include": planned}
        else:
            plan[f"{family.name}_runner"] = planned[0]["runner"]
    return RunnerPlan(plan=plan, warnings=warnings)


def _write_output(plan: RunnerPlan, output_path: Path) -> None:
    value = json.dumps(plan.plan, separators=(",", ":"), sort_keys=True)
    with output_path.open("a", encoding="utf-8") as output:
        output.write(f"plan={value}\n")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workflow", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--families", required=True)
    parser.add_argument("--burst", default="")
    parser.add_argument("--percent")
    parser.add_argument("--light-label", default="")
    parser.add_argument("--standard-label", default="")
    parser.add_argument("--output", type=Path, default=Path("/dev/stdout"))
    args = parser.parse_args()
    try:
        families = json.loads(args.families)
        plan = build_plan(
            workflow=args.workflow,
            run_id=args.run_id,
            families=families,
            burst=args.burst,
            percent=args.percent,
            light_label=args.light_label,
            standard_label=args.standard_label,
        )
    except (json.JSONDecodeError, ValueError) as error:
        parser.error(str(error))
    for warning in plan.warnings:
        print(f"::warning::{warning}")
    _write_output(plan, args.output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
