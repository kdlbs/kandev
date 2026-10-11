"""Validation policy and admission contracts; live cgroup proof is separate."""
import importlib.util
import sys
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch, Mock

IMAGE = 'registry.invalid/worker@sha256:' + 'a' * 64


class CheckTest(unittest.TestCase):
    def setUp(self):
        source = Path(__file__).with_name('check.py')
        self.assertTrue(source.exists(), 'isolated validation runner is missing')
        spec = importlib.util.spec_from_file_location('worker_check', source)
        self.module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = self.module
        spec.loader.exec_module(self.module)
        self.env = {'FULL_WORKER_CHECK_IMAGE': IMAGE}
        self.policy = self.module.Policy.from_env(self.env)

    def test_policy_rejects_mutable_images_and_invalid_limits(self):
        for override in ({'FULL_WORKER_CHECK_IMAGE': 'worker:latest'},
                         {'FULL_WORKER_CHECK_MEMORY_BYTES': '0'},
                         {'FULL_WORKER_CHECK_MEMORY_BYTES': '3221225472'},
                         {'FULL_WORKER_CHECK_JOB_SECONDS': '0'},
                         {'FULL_WORKER_CHECK_QUEUE_SECONDS': '-1'}):
            with self.subTest(override=override), self.assertRaises(ValueError):
                self.module.Policy.from_env({**self.env, **override})

    def test_workspace_canonicalization_rejects_outside_symlinks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            workspace = root / 'workspace'
            workspace.mkdir()
            (workspace / 'escape').symlink_to(root)
            self.assertEqual(self.module.workspace_cwd(workspace, workspace), workspace)
            with self.assertRaises(ValueError):
                self.module.workspace_cwd(workspace / 'escape', workspace)

    def test_child_limits_argv_and_secret_boundary(self):
        args = self.module.create_args(self.policy, Path('/workspace'), 'lint',
                                       ['echo', 'a b', '$(false)', '--flag'], 'owner',
                                       {'GITHUB_TOKEN': 'secret', 'AGENTCTL_TOKEN': 'secret',
                                        'KANDEV_E2E_MOCK': 'true', 'LANG': 'C.UTF-8'})
        for flag, value in (('--memory', '2147483648'), ('--memory-swap', '2147483648'),
                            ('--cpus', '2'), ('--pids-limit', '512'), ('--user', '1000:1000')):
            self.assertEqual(args[args.index(flag) + 1], value)
        self.assertEqual(args[-4:], ['echo', 'a b', '$(false)', '--flag'])
        self.assertNotIn('secret', ' '.join(args))
        self.assertNotIn('/run/kandev', ' '.join(args))
        self.assertNotIn('docker.sock', ' '.join(args))
        self.assertNotIn('--privileged', args)
        self.assertIn('LANG=C.UTF-8', args)
        self.assertIn('FULL_WORKER_CHECK_INSIDE=1', args)

    def test_shared_memory_budget_counts_existing_workloads(self):
        container = {'HostConfig': {'Memory': 1073741824}, 'State': {'Running': True}}
        self.assertFalse(self.module.memory_available(self.policy, [container]))
        self.assertTrue(self.module.memory_available(self.policy, []))
        container['HostConfig']['Memory'] = 0
        with self.assertRaises(ValueError):
            self.module.memory_available(self.policy, [container])

    def test_receipt_must_prove_live_daemon_and_budget(self):
        receipt = {'version': 1, 'daemon_id': 'daemon', 'generation': 'boot',
                   'memory_max': self.policy.budget, 'probe_memory_max': 67108864,
                   'probe_memory_current': 16777216, 'probe_cgroup': '/docker/probe',
                   'parent_delta': 16777216}
        info = {'ID': 'daemon', 'CgroupVersion': '2', 'CgroupDriver': 'cgroupfs'}
        self.module.validate_receipt(self.policy, receipt, info)
        for override in ({'daemon_id': 'other'}, {'memory_max': 0},
                         {'probe_cgroup': '/../foreign'}, {'parent_delta': 0}):
            with self.subTest(override=override), self.assertRaises(ValueError):
                self.module.validate_receipt(self.policy, {**receipt, **override}, info)

    def test_browser_unsafe_arguments_fail_before_docker(self):
        for args in (['pnpm', 'e2e:run', '--workers=4'],
                     ['pnpm', 'e2e:run', '--shards', '2'],
                     ['pnpm', 'e2e:run', '--project', 'containers'],
                     ['pnpm', 'e2e:run', '-j2']):
            with self.subTest(args=args), self.assertRaises(ValueError):
                self.module.create_args(self.policy, Path('/workspace'), 'browser', args, 'owner', {})

    def test_foreign_slot_is_never_reclaimed(self):
        docker = self.module.Docker()
        slot = {'Id': 'foreign', 'Config': {'Labels': {}}, 'State': {'Status': 'exited'}}
        with patch.object(docker, 'inspect', return_value=slot), patch.object(docker, 'call') as call:
            with self.assertRaises(ValueError):
                self.module.reconcile_slot(docker, Path('/workspace'))
            call.assert_not_called()

    def test_running_owned_slot_waits_and_exited_slot_uses_exact_id(self):
        docker = self.module.Docker()
        slot = {'Id': 'exact', 'Config': {'Labels': {
            'kandev.check.version': '1', 'kandev.check.workspace': '/workspace',
            'kandev.check.owner': 'owner', 'kandev.check.image': IMAGE,
            'kandev.check.deadline': '9999999999'}}, 'State': {'Status': 'running'}}
        with patch.object(docker, 'inspect', return_value=slot), patch.object(docker, 'call') as call:
            self.assertFalse(self.module.reconcile_slot(docker, Path('/workspace')))
            call.assert_not_called()
            slot['State']['Status'] = 'exited'
            self.assertTrue(self.module.reconcile_slot(docker, Path('/workspace')))
            call.assert_called_once_with('rm', 'exact')

    def test_timeout_returns_timeout_status_and_stops_exact_workload(self):
        slot = {'Id': 'exact', 'Config': {'Labels': {
            'kandev.check.version': '1', 'kandev.check.workspace': '/workspace',
            'kandev.check.owner': 'owner', 'kandev.check.image': IMAGE,
            'kandev.check.deadline': '9999999999'}}, 'State': {'Status': 'running'}}
        receipt = {'version': 1, 'daemon_id': 'daemon', 'generation': 'boot',
                   'memory_max': self.policy.budget, 'probe_memory_max': 67108864,
                   'probe_memory_current': 16777216, 'probe_cgroup': '/docker/probe',
                   'parent_delta': 16777216}
        info = {'ID': 'daemon', 'CgroupVersion': '2', 'CgroupDriver': 'cgroupfs'}
        docker = Mock()
        docker.call.side_effect = [json.dumps(info), '', '', '']
        docker.inspect.return_value = slot
        process = Mock()
        process.wait.side_effect = [subprocess.TimeoutExpired('docker', 1800), 0]
        with patch.object(self.module, 'Docker', return_value=docker), \
             patch.object(self.module, 'RECEIPT') as receipt_file, \
             patch.object(self.module, 'acquire_slot', return_value='exact'), \
             patch.object(self.module.uuid, 'uuid4', return_value='owner'), \
             patch.object(self.module.subprocess, 'Popen', return_value=process):
            receipt_file.read_text.return_value = json.dumps(receipt)
            code = self.module.run_job(self.policy, Path('/workspace'), 'test', ['true'], {})
        self.assertEqual(code, 124)
        self.assertIn(unittest.mock.call('stop', '--time', '5', 'exact'), docker.call.call_args_list)
        self.assertIn(unittest.mock.call('rm', 'exact'), docker.call.call_args_list)

    def test_created_slot_deadline_and_image_pull_are_bounded(self):
        args = self.module.create_args(self.policy, Path('/workspace'), 'test', ['true'], 'owner', {})
        self.assertIn('--pull=never', args)
        docker = self.module.Docker()
        slot = {'Id': 'exact', 'Config': {'Labels': {
            'kandev.check.version': '1', 'kandev.check.workspace': '/workspace',
            'kandev.check.owner': 'owner', 'kandev.check.image': IMAGE,
            'kandev.check.deadline': '1'}}, 'State': {'Status': 'created'}}
        with patch.object(docker, 'inspect', return_value=slot), patch.object(docker, 'call') as call:
            self.assertTrue(self.module.reconcile_slot(docker, Path('/workspace')))
            call.assert_called_once_with('rm', 'exact')

    def test_make_options_preserve_argument_boundaries_without_jobserver(self):
        args = self.module.create_args(self.policy, Path('/workspace'), 'build',
                                       ['make', '--no-print-directory', 'build'], 'owner',
                                       {'MAKEFLAGS': r"r -j2 --jobserver-auth=3,4 -- GOOS=linux TITLE=a\ b"})
        self.assertIn('GOOS=linux', args)
        self.assertIn('TITLE=a b', args)
        self.assertNotIn('--jobserver-auth=3,4', args)
        self.assertNotIn('MAKEFLAGS=', ' '.join(args))

    def test_browser_grep_value_is_not_a_project(self):
        try:
            self.module.create_args(self.policy, Path('/workspace'), 'browser',
                                    ['pnpm', 'e2e:run', '--grep', 'containers'], 'owner', {})
        except ValueError as error:
            self.fail(str(error))


if __name__ == '__main__':
    unittest.main()
