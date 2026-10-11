"""Exercise opt-in renderer and startup proof without a real daemon."""
from pathlib import Path
import json
import os
import subprocess
import tempfile
import unittest

RECIPE = Path(__file__).resolve().parent
IMAGE = 'registry.invalid/worker@sha256:' + 'a' * 64


class ValidationRecipeTest(unittest.TestCase):
    def test_receipt_records_removed_probe_and_rejects_an_escaped_child(self):
        for group in ('/docker/proof', '/kubepods/companion/docker/proof', '/foreign/proof'):
            with self.subTest(group=group), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                cgroups = root/'cgroups'
                parent = cgroups/'kubepods/companion'
                child = cgroups/group.lstrip('/')
                parent.mkdir(parents=True)
                child.mkdir(parents=True)
                for name, value in {'memory.max': '3221225472', 'memory.current': '100',
                                    'cgroup.subtree_control': ''}.items():
                    (parent/name).write_text(value)
                (child/'memory.max').write_text('67108864')
                (child/'memory.current').write_text('16777216')
                (root/'self-cgroup').write_text('0::/kubepods/companion\n')
                (root/'child-cgroup').write_text('0::' + group + '\n')
                stage = (RECIPE/'daemon-validation-preflight.sh').read_text()
                stage = stage.replace('\nwait "$daemon_pid"\n', '\ncat "$receipt"\nwait "$daemon_pid"\n')
                stage = stage.replace('/proc/self/cgroup', str(root/'self-cgroup'))
                stage = stage.replace('/proc/$pid/cgroup', str(root/'child-cgroup'))
                stage = stage.replace('/sys/fs/cgroup', str(cgroups))
                stage = stage.replace('/run/docker/validation-accounting.json', str(root/'receipt'))
                docker = root/'docker'
                docker.write_text('''#!/bin/sh
case "$1" in
  info) case "$*" in *CgroupVersion*) echo '2 cgroupfs';; *ID*) echo daemon;; esac;;
  image) exit 0;;
  create) echo proof;;
  start) echo 16777316 > "$PARENT_CURRENT";;
  inspect) echo 1;;
  rm) echo "$3" >> "$REMOVED";;
  *) exit 2;;
esac
''')
                docker.chmod(0o755)
                launch = 'start_validation_daemon() { /bin/sleep 0.3 & daemon_pid=$!; }\n'
                result = subprocess.run(['sh', '-ceu',
                    'validation_budget=3221225472\nvalidation_image=sha256:fixture\n' + launch + stage],
                    capture_output=True, text=True, timeout=5,
                    env={**os.environ, 'PATH': f"{root}:{os.environ['PATH']}",
                         'PARENT_CURRENT': str(parent/'memory.current'), 'REMOVED': str(root/'removed')})
                if group == '/kubepods/companion/docker/proof':
                    self.assertEqual(result.returncode, 0, result.stderr)
                    receipt = json.loads(result.stdout)
                    self.assertEqual(receipt['probe_cgroup'], '/docker/proof')
                    self.assertEqual(receipt['companion_cgroup'], '/kubepods/companion')
                else:
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn('escaped companion', result.stderr)
                    self.assertFalse((root/'receipt').exists())
                self.assertEqual((root/'removed').read_text().strip(), 'proof')

    def run_cgroup_stage(self, group, limit='3221225472', inherited=''):
        stage = (RECIPE/'daemon-validation-preflight.sh').read_text().split('# Image availability', 1)[0]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            cgroups = root/'cgroups'
            own = cgroups/group.lstrip('/')
            own.mkdir(parents=True)
            # In a host namespace the mount root is not the bounded companion.
            (cgroups/'memory.max').write_text('max')
            (own/'memory.max').write_text(limit)
            (own/'cgroup.controllers').write_text('cpu memory pids')
            (own/'cgroup.subtree_control').write_text('')
            (root/'self-cgroup').write_text('0::' + group + '\n')
            stage = stage.replace('/proc/self/cgroup', str(root/'self-cgroup'))
            stage = stage.replace('/sys/fs/cgroup', str(cgroups))
            stage = stage.replace('/run/docker/validation-accounting.json', str(root/'receipt'))
            docker = root/'docker'
            docker.write_text('#!/bin/sh\n'
                'test "${DOCKER_HOST:-}" = unix:///run/docker/docker.sock || exit 1\n'
                'case "$*" in "info") exit 0;; "info --format "*) echo "2 cgroupfs";; *) exit 2;; esac\n')
            docker.chmod(0o755)
            launch = '''
start_validation_daemon() {
  printf 'parent=%s\n' "$validation_cgroup_parent"
  /bin/sleep 30 & daemon_pid=$!
}
'''
            result = subprocess.run(['sh', '-ceu',
                'validation_budget=3221225472\n' + launch + stage],
                capture_output=True, text=True, timeout=5,
                env={**os.environ, 'PATH': f"{root}:{os.environ['PATH']}", 'DOCKER_HOST': inherited})
            return result, (own/'cgroup.subtree_control').read_text()

    def test_preflight_uses_bounded_companion_in_either_cgroup_namespace(self):
        for group in ('/', '/kubepods.slice/pod-123/companion.scope'):
            with self.subTest(group=group):
                result, controllers = self.run_cgroup_stage(group)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('parent=' + group.rstrip('/') + '/docker', result.stdout)
                self.assertEqual(controllers.strip(), '+cpu +memory +pids')

    def test_preflight_refuses_unbounded_or_wrong_companion_budget_before_launch(self):
        for limit in ('max', '2147483648'):
            with self.subTest(limit=limit):
                result, controllers = self.run_cgroup_stage('/kubepods/companion', limit)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('parent=', result.stdout)
                self.assertEqual(controllers, '')

    def test_companion_readiness_uses_its_actual_socket(self):
        for inherited in ('', 'unix:///wrong.sock'):
            with self.subTest(inherited=inherited):
                result, _ = self.run_cgroup_stage('/', inherited=inherited)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_default_renderer_does_not_enable_validation(self):
        result = subprocess.run(['bash', str(RECIPE/'render-template.sh'), IMAGE],
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('FULL_WORKER_CHECK_MODE', result.stdout)

    def test_isolated_renderer_embeds_accounting_before_readiness(self):
        result = subprocess.run(['bash', str(RECIPE/'render-template.sh'), '--isolated', IMAGE],
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        for token in ('FULL_WORKER_CHECK_MODE', 'validation-accounting.json', 'parent_delta',
                      '/proc/', 'memory.max', '--cgroup-parent="$validation_cgroup_parent"', 'kill', IMAGE):
            self.assertIn(token, result.stdout)
        self.assertNotIn('VALIDATION_HELPER_REQUIRED', result.stdout)

    def test_preparation_checks_accounting_before_repository_setup(self):
        source = (RECIPE/'prepare.sh').read_text()
        self.assertIn('check.py --preflight', source)
        self.assertLess(source.index('check.py --preflight'), source.index('workspace={{workspace.path}}'))

    def test_baked_runner_has_no_runtime_dependency(self):
        source = (RECIPE/'Dockerfile').read_text()
        self.assertIn('COPY check.py /opt/full-worker/check.py', source)


if __name__ == '__main__':
    unittest.main()
