#!/usr/bin/env python3
"""Attempt-specific occupancy and incomplete-evidence regression tests."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("efficiency", Path(__file__).with_name("ci-efficiency.py"))
efficiency = importlib.util.module_from_spec(spec)
spec.loader.exec_module(efficiency)


def job(name, start, end, **extra):
    return dict(id=name, name=name, run_id=42, run_attempt=2, head_sha="abc",
                status="completed", conclusion="success", started_at=start,
                completed_at=end, steps=[], **extra)


class EfficiencyTests(unittest.TestCase):
    def setUp(self):
        self.run = dict(id=42, run_attempt=2, head_sha="abc", workflow_id=7,
                        path=".github/workflows/test.yml", status="completed",
                        run_started_at="2026-10-08T12:00:00Z")
        self.jobs = [job("a", "2026-10-08T12:01:00Z", "2026-10-08T12:03:00Z"),
                     job("b", "2026-10-08T12:02:00Z", "2026-10-08T12:05:00Z")]

    def report(self, graph=None):
        return efficiency.summarize(self.run, {"total_count": len(self.jobs), "jobs": self.jobs}, graph)

    # @covers AC-PLATFORM-CI-PERFORMANCE-005.1
    def test_overlap_is_summed_occupancy_not_wall_time_or_inferred_queue(self):
        self.jobs[0]["started_at"] = "2026-10-08T12:00:00Z"
        result = self.report()
        self.assertEqual(result["known_runner_minutes"], 6)
        self.assertEqual(result["observed_elapsed_seconds"], 300)
        self.assertIsNone(result["execution_critical_path_seconds"])
        self.assertIsNone(result["jobs"][0]["dependency_ready_delay_seconds"])

    def test_attempt_and_source_cannot_mix(self):
        for field, value in [("run_attempt", 1), ("run_id", 99), ("head_sha", "other")]:
            with self.subTest(field=field):
                original = self.jobs[0][field]
                self.jobs[0][field] = value
                with self.assertRaises(ValueError):
                    self.report()
                self.jobs[0][field] = original

    def test_cancelled_execution_counts_but_unfinished_is_unknown(self):
        self.jobs[0]["conclusion"] = "cancelled"
        self.jobs[1].update(status="in_progress", completed_at=None)
        result = self.report()
        self.assertEqual(result["known_runner_minutes"], 2)
        self.assertFalse(result["complete"])
        self.assertIsNone(result["runner_minutes"])
        self.assertIsNone(result["jobs"][1]["execution_seconds"])

    def test_skipped_has_zero_occupancy_even_with_synthetic_timestamps(self):
        self.jobs[0]["conclusion"] = "skipped"
        self.assertEqual(self.report()["runner_minutes"], 3)

    def test_invalid_timestamps_and_missing_jobs_are_not_zero_cost(self):
        for value in [None, "bad", "2026-10-08T11:00:00Z"]:
            self.jobs[0]["completed_at"] = value
            self.assertFalse(self.report()["complete"])
        result = efficiency.summarize(self.run, {"total_count": 3, "jobs": self.jobs})
        self.assertIn("job list is incomplete", result["incomplete_reasons"])

    def test_graph_requires_exact_identity_and_complete_acyclic_job_mapping(self):
        self.jobs[1]["started_at"] = "2026-10-08T12:04:00Z"
        graph = {"identity": {k: self.run[k] for k in ("workflow_id", "path", "head_sha")},
                 "needs": {"a": [], "b": ["a"]}}
        result = self.report(graph)
        self.assertEqual(result["execution_critical_path_seconds"], 180)
        self.assertEqual(result["jobs"][1]["dependency_ready_delay_seconds"], 60)
        graph["identity"]["head_sha"] = "wrong"
        with self.assertRaises(ValueError):
            self.report(graph)
        graph["identity"]["head_sha"] = "abc"
        graph["needs"]["a"] = ["b"]
        with self.assertRaises(ValueError):
            self.report(graph)
        graph["needs"].pop("a")
        with self.assertRaises(ValueError):
            self.report(graph)

    def test_duplicate_jobs_are_rejected(self):
        self.jobs.append(self.jobs[0])
        with self.assertRaises(ValueError):
            self.report()


if __name__ == "__main__":
    unittest.main()
