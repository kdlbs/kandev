"""The coverage scenarios are registered end to end: CLI list, pool driver table and driver modules."""
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import nomad_native_e2e as cli  # noqa: E402
import nomad_native_e2e_pool as pool  # noqa: E402


class CoverageScenarioRegistrationTests(unittest.TestCase):
    def test_every_coverage_scenario_is_known_to_the_cli_and_the_pool_runner(self):
        self.assertEqual(len(cli.POOL_COVERAGE_SCENARIOS), 8)
        for name in cli.POOL_COVERAGE_SCENARIOS:
            self.assertIn(name, cli.REQUIRED_SCENARIOS)
            self.assertIn(name, pool.SCENARIOS)
            self.assertIn(name, pool.DRIVERS)
        self.assertEqual(set(cli.POOL_COVERAGE_SCENARIOS), set(pool.COVERAGE_DRIVERS))

    def test_every_coverage_driver_resolves_to_a_callable(self):
        for name, (module, func) in pool.COVERAGE_DRIVERS.items():
            self.assertTrue(callable(getattr(__import__(module), func)), name)


if __name__ == "__main__":
    unittest.main()
