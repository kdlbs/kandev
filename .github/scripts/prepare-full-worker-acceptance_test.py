#!/usr/bin/env python3
"""Test the real CI preparation process without building a Docker image."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).with_name('prepare-full-worker-acceptance.sh')
SPEC = 'tests/kubernetes/kubernetes-session-resilience.spec.ts'
IMAGE = 'sha256:' + 'a' * 64


class FullWorkerAcceptanceTest(unittest.TestCase):
    def run_prepare(self, files, *, status=0, image=IMAGE):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            scripts = root/'.github/scripts'
            scripts.mkdir(parents=True)
            self.assertTrue(SOURCE.is_file(), 'CI worker preparation is missing')
            shutil.copy2(SOURCE, scripts/SOURCE.name)
            recipe = root/'k8s/worker-images/full'
            recipe.mkdir(parents=True)
            (recipe/'build.sh').write_text(
                '#!/bin/bash\nprintf "%s\\n" "$@" > "$BUILD_ARGS"\n'
                'printf "IMAGE_ID=%s\\n" "$FAKE_IMAGE"\nexit "$FAKE_STATUS"\n')
            manifest = root/'manifest.json'
            manifest.write_text(json.dumps({'shards': [{'index': 2, 'files': files}]}))
            job_env, args = root/'job.env', root/'args'
            env = {**os.environ, 'GITHUB_ENV': str(job_env), 'RUNNER_TEMP': str(root),
                   'BUILD_ARGS': str(args), 'FAKE_IMAGE': image, 'FAKE_STATUS': str(status)}
            result = subprocess.run(['bash', str(scripts/SOURCE.name), str(manifest), '2'],
                                    env=env, capture_output=True, text=True, timeout=15)
            return result, job_env.read_text() if job_env.exists() else '', args.read_text() if args.exists() else ''

    def test_unrelated_shard_does_not_build_or_export_an_image(self):
        result, exported, args = self.run_prepare(['tests/kubernetes/kubernetes-task-pod.spec.ts'])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((exported, args), ('', ''))

    def test_selected_shard_builds_verifies_and_exports_only_exact_image_id(self):
        result, exported, args = self.run_prepare([SPEC])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(args, '--build\n--verify\n')
        self.assertEqual(exported, 'KANDEV_E2E_FULL_WORKER_IMAGE=' + IMAGE + '\n')

    def test_failed_verification_does_not_enable_acceptance(self):
        result, exported, _ = self.run_prepare([SPEC], status=12)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(exported, '')

    def test_invalid_or_ambiguous_image_output_is_rejected(self):
        for image in ('worker:latest', IMAGE + '\nIMAGE_ID=' + IMAGE):
            with self.subTest(image=image):
                result, exported, _ = self.run_prepare([SPEC], image=image)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(exported, '')


if __name__ == '__main__':
    unittest.main()
