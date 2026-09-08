#!/usr/bin/env python3
"""Tests for deterministic external CI runner allocation."""

import importlib.util
from pathlib import Path
import unittest


SCRIPT = Path(__file__).with_name("runner-plan.py")
SPEC = importlib.util.spec_from_file_location("runner_plan", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
runner_plan = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner_plan)

E2E_FAMILIES = [
    {"name": "changes", "tier": "light"},
    {
        "name": "e2e",
        "tier": "standard",
        "instances": [{"shard": shard} for shard in range(1, 15)],
    },
    {"name": "e2e_gate", "tier": "light"},
]
FRONTEND_FAMILIES = [
    {"name": "frontend", "tier": "standard"},
    {"name": "frontend_gate", "tier": "light"},
]
BACKEND_MATRIX_FAMILY = [
    {
        "name": "backend_test",
        "tier": "standard",
        "instances": [{"shard": 1, "index": 0}, {"shard": 2, "index": 1}],
    }
]


class RunnerPlanTest(unittest.TestCase):
    def test_missing_percentage_defaults_to_github(self) -> None:
        plan = runner_plan.build_plan(
            workflow="e2e",
            run_id="100",
            families=E2E_FAMILIES,
            burst="true",
            percent=None,
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertEqual(plan.plan["changes_runner"], "ubuntu-latest")
        self.assertFalse(plan.warnings)

    def test_burst_off_ignores_percentage(self) -> None:
        plan = runner_plan.build_plan(
            workflow="frontend",
            run_id="101",
            families=FRONTEND_FAMILIES,
            burst="false",
            percent="100",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertEqual(plan.plan["frontend_runner"], "ubuntu-latest")
        self.assertEqual(plan.plan["frontend_gate_runner"], "ubuntu-latest")

    def test_matrix_percentage_uses_floor_and_is_stable(self) -> None:
        first = runner_plan.build_plan(
            workflow="e2e",
            run_id="102",
            families=E2E_FAMILIES,
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )
        second = runner_plan.build_plan(
            workflow="e2e",
            run_id="102",
            families=E2E_FAMILIES,
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        first_matrix = first.plan["e2e_matrix"]["include"]
        second_matrix = second.plan["e2e_matrix"]["include"]
        self.assertEqual(first_matrix, second_matrix)
        self.assertEqual(
            sum(item["runner"] == "standard-runner" for item in first_matrix), 7
        )

    def test_backend_matrix_percentage_uses_floor(self) -> None:
        plan = runner_plan.build_plan(
            workflow="backend",
            run_id="103",
            families=BACKEND_MATRIX_FAMILY,
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        matrix = plan.plan["backend_test_matrix"]["include"]
        self.assertEqual(
            sum(item["runner"] == "standard-runner" for item in matrix), 1
        )
        self.assertEqual(
            [(item["shard"], item["index"]) for item in matrix],
            [(1, 0), (2, 1)],
        )

    def test_workflow_owned_family_names_need_no_python_change(self) -> None:
        plan = runner_plan.build_plan(
            workflow="new-workflow",
            run_id="109",
            families=[{"name": "new_job", "tier": "standard"}],
            burst="true",
            percent="100",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertEqual(plan.plan, {"new_job_runner": "standard-runner"})

    def test_invalid_family_definition_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "duplicate family name"):
            runner_plan.build_plan(
                workflow="frontend",
                run_id="110",
                families=[
                    {"name": "same", "tier": "light"},
                    {"name": "same", "tier": "standard"},
                ],
                burst="true",
                percent="50",
                light_label="light-runner",
                standard_label="standard-runner",
            )

    def test_singleton_allocation_is_stable_for_reruns(self) -> None:
        first = runner_plan.build_plan(
            workflow="frontend",
            run_id="104",
            families=FRONTEND_FAMILIES,
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )
        second = runner_plan.build_plan(
            workflow="frontend",
            run_id="104",
            families=FRONTEND_FAMILIES,
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )
        self.assertEqual(first.plan, second.plan)

    def test_empty_tier_label_falls_back_to_github(self) -> None:
        plan = runner_plan.build_plan(
            workflow="frontend",
            run_id="105",
            families=FRONTEND_FAMILIES,
            burst="true",
            percent="100",
            light_label="",
            standard_label="",
        )

        self.assertEqual(plan.plan["frontend_runner"], "ubuntu-latest")

    def test_invalid_percentage_fails_closed_with_warning(self) -> None:
        plan = runner_plan.build_plan(
            workflow="e2e",
            run_id="106",
            families=E2E_FAMILIES,
            burst="true",
            percent="101",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertTrue(plan.warnings)
        self.assertEqual(plan.plan["changes_runner"], "ubuntu-latest")
        matrix = plan.plan["e2e_matrix"]["include"]
        self.assertTrue(all(item["runner"] == "ubuntu-latest" for item in matrix))

    def test_unicode_digits_fail_closed_with_warning(self) -> None:
        plan = runner_plan.build_plan(
            workflow="e2e",
            run_id="108",
            families=E2E_FAMILIES,
            burst="true",
            percent="²",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertTrue(plan.warnings)
        self.assertEqual(plan.plan["changes_runner"], "ubuntu-latest")
        matrix = plan.plan["e2e_matrix"]["include"]
        self.assertTrue(all(item["runner"] == "ubuntu-latest" for item in matrix))

    def test_full_percentage_uses_external_for_non_empty_tier(self) -> None:
        plan = runner_plan.build_plan(
            workflow="frontend",
            run_id="107",
            families=FRONTEND_FAMILIES,
            burst="true",
            percent="100",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertEqual(plan.plan["frontend_runner"], "standard-runner")
        self.assertEqual(plan.plan["frontend_gate_runner"], "light-runner")


if __name__ == "__main__":
    unittest.main()
