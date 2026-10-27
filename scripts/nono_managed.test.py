#!/usr/bin/env python3
"""Protect the managed runner's instance-selection boundary."""
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import socket
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('nono_managed', Path(__file__).with_name('nono_managed.py'))
managed = importlib.util.module_from_spec(spec)
spec.loader.exec_module(managed)


class FixtureBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix='kw-')
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name).resolve()
        self.listener = socket.socket()
        self.addCleanup(self.listener.close)
        self.listener.bind(('127.0.0.1', 0))
        self.listener.listen()
        port = self.listener.getsockname()[1]
        self.seed = {'schema_version': 1, 'purpose': 'nono_acceptance', 'status': 'ready',
            'root': str(self.root), 'url': f'http://127.0.0.1:{port}', 'port': port,
            'pid': os.getpid(), 'birth': managed.birth(os.getpid()),
            'tools_dir': str(self.root / 'tools'), 'repository_path': str(self.root / 'repository')}

    def test_real_owned_listener_is_accepted(self):
        self.assertEqual(managed.validate_seed(self.seed), self.root)

    def test_main_port_is_refused_even_with_valid_owned_root(self):
        self.seed.update(url='http://127.0.0.1:9998', port=9998)
        with self.assertRaisesRegex(managed.Blocked, 'unsafe_fixture_address'):
            managed.validate_seed(self.seed)

    def test_existing_unmarked_instance_and_recycled_pid_are_refused(self):
        for changes, reason in [({'purpose': 'user_test'}, 'fixture_identity_required'),
                                ({'birth': 'stale'}, 'fixture_process_identity_changed')]:
            with self.subTest(changes=changes), self.assertRaisesRegex(managed.Blocked, reason):
                managed.validate_seed({**self.seed, **changes})

    def test_listener_must_belong_to_the_captured_process(self):
        # The listener is live but PID 1 does not own this test's socket.
        self.seed.update(pid=1, birth=managed.birth(1))
        with self.assertRaisesRegex(managed.Blocked, 'fixture_process_identity_changed'):
            managed.validate_seed(self.seed)

    def test_foreign_or_symlinked_paths_are_refused(self):
        with tempfile.TemporaryDirectory() as outside:
            (self.root / 'escape').symlink_to(outside, target_is_directory=True)
            for path in [outside, str(self.root / 'escape/tools')]:
                with self.subTest(path=path), self.assertRaisesRegex(managed.Blocked, 'foreign_fixture_path'):
                    managed.validate_seed({**self.seed, 'tools_dir': path})

    def test_failure_output_does_not_include_supplied_secrets(self):
        fixture = self.root / 'manifest.json'
        fixture.write_text(json.dumps({**self.seed, 'purpose': 'secret-fixture-marker'}))
        auth = self.root / 'auth.json'
        auth.write_text('secret-login-marker')
        from unittest.mock import patch
        with patch.dict(os.environ, {'NONO_ACCEPTANCE_SEED_MANIFEST': str(fixture),
                                     'NONO_ACCEPTANCE_PI_AUTH': str(auth)}, clear=True):
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                result = managed.main(Path('/unused-binary'), None)
            self.assertEqual(result, 2)
            self.assertEqual(json.loads(output.getvalue()), {'status': 'blocked', 'reason': 'fixture_identity_required'})


if __name__ == '__main__':
    unittest.main()
