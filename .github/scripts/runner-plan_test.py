#!/usr/bin/env python3
"""Tests for deterministic external CI runner allocation."""

import importlib.util
import json
from pathlib import Path
import unittest


SCRIPT = Path(__file__).with_name("runner-plan.py")
SPEC = importlib.util.spec_from_file_location("runner_plan", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
runner_plan = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner_plan)


class RunnerPlanTest(unittest.TestCase):
    def test_missing_percentage_defaults_to_github(self) -> None:
        plan = runner_plan.build_plan(
            workflow="e2e",
            run_id="100",
            burst="true",
            percent=None,
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertEqual(plan.outputs["changes_runner"], "github")
        self.assertFalse(plan.warnings)

    def test_burst_off_ignores_percentage(self) -> None:
        plan = runner_plan.build_plan(
            workflow="frontend",
            run_id="101",
            burst="false",
            percent="100",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertEqual(plan.outputs["frontend_runner"], "github")
        self.assertEqual(plan.outputs["frontend_gate_runner"], "github")

    def test_matrix_percentage_uses_floor_and_is_stable(self) -> None:
        first = runner_plan.build_plan(
            workflow="e2e",
            run_id="102",
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )
        second = runner_plan.build_plan(
            workflow="e2e",
            run_id="102",
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        first_matrix = json.loads(first.outputs["e2e_matrix"])["include"]
        second_matrix = json.loads(second.outputs["e2e_matrix"])["include"]
        self.assertEqual(first_matrix, second_matrix)
        self.assertEqual(
            sum(item["runner"] == "external" for item in first_matrix), 7
        )

    def test_backend_matrix_percentage_uses_floor(self) -> None:
        plan = runner_plan.build_plan(
            workflow="backend",
            run_id="103",
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        matrix = json.loads(plan.outputs["backend_test_matrix"])["include"]
        self.assertEqual(
            sum(item["runner"] == "external" for item in matrix), 1
        )
        self.assertEqual([(item["shard"], item["index"]) for item in matrix], [(1, 0), (2, 1)])

    def test_singleton_allocation_is_stable_for_reruns(self) -> None:
        first = runner_plan.build_plan(
            workflow="frontend",
            run_id="104",
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )
        second = runner_plan.build_plan(
            workflow="frontend",
            run_id="104",
            burst="true",
            percent="50",
            light_label="light-runner",
            standard_label="standard-runner",
        )
        self.assertEqual(first.outputs, second.outputs)

    def test_empty_tier_label_falls_back_to_github(self) -> None:
        plan = runner_plan.build_plan(
            workflow="frontend",
            run_id="105",
            burst="true",
            percent="100",
            light_label="",
            standard_label="",
        )

        self.assertEqual(plan.outputs["frontend_runner"], "github")

    def test_invalid_percentage_fails_closed_with_warning(self) -> None:
        plan = runner_plan.build_plan(
            workflow="e2e",
            run_id="106",
            burst="true",
            percent="101",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertTrue(plan.warnings)
        self.assertEqual(plan.outputs["changes_runner"], "github")
        matrix = json.loads(plan.outputs["e2e_matrix"])["include"]
        self.assertTrue(all(item["runner"] == "github" for item in matrix))

    def test_full_percentage_uses_external_for_non_empty_tier(self) -> None:
        plan = runner_plan.build_plan(
            workflow="frontend",
            run_id="107",
            burst="true",
            percent="100",
            light_label="light-runner",
            standard_label="standard-runner",
        )

        self.assertEqual(plan.outputs["frontend_runner"], "external")
        self.assertEqual(plan.outputs["frontend_gate_runner"], "external")


if __name__ == "__main__":
    unittest.main()
