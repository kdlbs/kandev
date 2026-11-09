#!/usr/bin/env python3
"""Protect image publication credentials, input data, and promotion ordering."""

import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
CI = (ROOT / ".github/workflows/ci-base-image.yml").read_text()
WEEKLY = (ROOT / ".github/workflows/universal-rebuild.yml").read_text()


class ImagePublicationContractTest(unittest.TestCase):
    # @covers AC-CI-ACTIONS-SECURITY-002.2
    def test_image_source_checkouts_never_persist_credentials(self):
        for workflow in (CI, WEEKLY):
            self.assertEqual(workflow.count("uses: actions/checkout@"), 1)
            self.assertIn("persist-credentials: false", workflow)

    # @covers AC-CI-ACTIONS-SECURITY-002.1
    def test_weekly_tag_is_literal_data_when_promoting(self):
        block = WEEKLY.split("      - name: Promote staging to final tags\n", 1)[1]
        script = "\n".join(line[10:] for line in block.split("        run: |\n", 1)[1].splitlines())
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            docker = root / "docker"
            docker.write_text("#!/usr/bin/env python3\nimport json,sys\nfrom pathlib import Path\nPath('args.json').write_text(json.dumps(sys.argv[1:]))\n")
            docker.chmod(0o755)
            payload = '20261026$(touch marker)`touch marker`"\nwith space'
            script = script.replace("${{ steps.date.outputs.tag }}", payload)
            result = subprocess.run([shutil.which("bash"), "-c", script], cwd=root,
                                    env={**os.environ, "PATH": f"{root}:{os.environ['PATH']}",
                                         "DATE_TAG": payload, "STAGING_IMAGE": "fixture:staging"},
                                    text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse((root / "marker").exists())
            self.assertEqual(json.loads((root / "args.json").read_text()),
                             ["buildx", "imagetools", "create", "--tag", "ghcr.io/kdlbs/kandev:universal",
                              "--tag", f"ghcr.io/kdlbs/kandev:universal-weekly-{payload}", "fixture:staging"])
            self.assertIn("DATE_TAG: ${{ steps.date.outputs.tag }}", block)

    # @covers AC-CI-ACTIONS-SECURITY-002.4
    def test_publication_keeps_existing_event_and_size_gates(self):
        self.assertIn("github.event_name == 'workflow_dispatch'", CI)
        self.assertIn("github.event_name == 'push' && github.ref == 'refs/heads/main'", CI)
        self.assertIn("push: ${{ github.event_name == 'workflow_dispatch' ||", CI)
        self.assertIn('schedule:', WEEKLY)
        self.assertIn('workflow_dispatch: {}', WEEKLY)
        self.assertLess(WEEKLY.index("- name: Enforce size budget"), WEEKLY.index("- name: Promote staging to final tags"))
        gate = WEEKLY.split("- name: Enforce size budget", 1)[1].split("- name: Promote staging", 1)[0]
        self.assertIn('"$TOTAL" -gt "$MAX_COMPRESSED_BYTES"', gate)
        self.assertIn("exit 1", gate)
        lint = (ROOT / ".github/workflows/lint-action-pinning.yml").read_text()
        self.assertIn("python3 .github/scripts/image-publication-workflow-contract_test.py", lint)


if __name__ == "__main__":
    unittest.main()
