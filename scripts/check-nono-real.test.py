#!/usr/bin/env python3
"""Safety contracts for the opt-in Nono conformance runner."""
import contextlib
import importlib.util
import io
import json
import tempfile
import os
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('nono_real', Path(__file__).with_name('check-nono-real.py'))
runner = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = runner
spec.loader.exec_module(runner)


class NonoRunnerTests(unittest.TestCase):
    def test_product_run_requires_an_explicit_owned_fixture(self):
        with patch.dict(os.environ, {}, clear=True), patch.object(runner, 'prerequisites', return_value=(Path('/verified/nono'), None)):
            with patch.object(runner, 'landlock_abi', return_value=8), patch.object(subprocess, 'Popen') as spawn:
                output = io.StringIO()
                with contextlib.redirect_stdout(output):
                    self.assertEqual(runner.main(['--run']), 2)
                self.assertEqual(json.loads(output.getvalue())['reason'], 'missing_product_fixture')
                spawn.assert_not_called()

    # @covers AC-AGENTS-SANDBOX-003.2, AC-AGENTS-SANDBOX-006.4
    def test_missing_binary_refuses_before_starting_a_process(self):
        with patch.dict(os.environ, {}, clear=True), patch.object(subprocess, 'Popen') as spawn:
            with contextlib.redirect_stdout(io.StringIO()):
                result = runner.main(['--conformance'])
            self.assertEqual(result, 2)
            spawn.assert_not_called()

    def test_wrong_digest_and_unsupported_host_refuse_before_process_start(self):
        with tempfile.TemporaryDirectory() as root:
            binary = Path(root) / 'nono'
            binary.write_text('#!/bin/sh\necho forged-runtime\n')
            binary.chmod(0o700)
            for system, reason in [('Linux', 'binary_digest_mismatch'), ('Darwin', 'unsupported_host')]:
                with self.subTest(system=system), patch.dict(os.environ, {'NONO_ACCEPTANCE_BINARY': str(binary)}, clear=True):
                    with patch.object(runner.platform, 'system', return_value=system), patch.object(subprocess, 'Popen') as spawn:
                        output = io.StringIO()
                        with contextlib.redirect_stdout(output):
                            self.assertEqual(runner.main(['--check-prerequisites']), 2)
                        self.assertEqual(json.loads(output.getvalue())['reason'], reason)
                        spawn.assert_not_called()

    def test_missing_landlock_refuses_before_process_start(self):
        with patch.object(runner, 'prerequisites', return_value=(Path('/verified/nono'), None)):
            with patch.object(runner, 'landlock_abi', return_value=0, create=True), patch.object(subprocess, 'Popen') as spawn:
                with contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(runner.main(['--check-prerequisites']), 2)
                spawn.assert_not_called()

    @unittest.skipUnless(os.environ.get('NONO_ACCEPTANCE_BINARY'), 'explicit pinned runtime required')
    def test_real_conformance_proves_positive_and_negative_contracts(self):
        with tempfile.TemporaryDirectory() as root:
            record = Path(root) / 'evidence.md'
            completed = subprocess.run([sys.executable, str(Path(runner.__file__)),
                '--conformance', '--record', str(record)], capture_output=True, text=True, timeout=120)
            self.assertEqual(completed.returncode, 0, completed.stdout)
            report = json.loads(completed.stdout.splitlines()[-1])
            self.assertEqual(report['scope'], 'runtime_conformance')
            self.assertEqual(set(report['checks']), {'pipe', 'filesystem', 'network', 'approval_denied',
                'approval_granted', 'approval_invalid', 'approval_timeout', 'pty_input_resize',
                'cancellation_cleanup', 'fresh_restart'})
            self.assertTrue(all(value == 'passed' for value in report['checks'].values()))
            self.assertEqual(report['active_status_evidence'], 'unsupported')
            self.assertIn('runtime_conformance', record.read_text())
            self.assertNotIn(str(Path.home()), record.read_text())


if __name__ == '__main__':
    unittest.main()
