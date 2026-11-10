#!/usr/bin/env python3
"""Guard publication, credential, and recovery boundaries in Actions definitions."""
import re
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
PATH = ROOT / '.github/workflows/announce-discord-release.yml'
WORKFLOW = PATH.read_text() if PATH.exists() else ''
RELEASE = (ROOT / '.github/workflows/release.yml').read_text()


def job(source, name):
    match = re.search(rf'(?ms)^  {name}:\n.*?(?=^  [\w-]+:\n|\Z)', source[source.index('jobs:'):] if 'jobs:' in source else '')
    return match.group() if match else ''


class DiscordWorkflowTest(unittest.TestCase):
    # @covers AC-RELEASE-DISCORD-001.1, AC-RELEASE-DISCORD-001.2, AC-RELEASE-DISCORD-001.4
    def test_automatic_delivery_requires_complete_normal_publication(self):
        block = job(RELEASE, 'announce-discord')
        dependencies = ['prepare', 'publish-release', 'publish-npm', 'update-homebrew-tap', 'update-scoop-bucket', 'publish-plugin-sdk', 'stable-complete']
        self.assertIn('needs: [' + ', '.join(dependencies) + ']', block)
        condition = re.search(r'if: >-\s*\$\{\{(.*?)\}\}', block, re.S)
        self.assertIsNotNone(condition)
        expression = condition.group(1)
        good = {'cancelled()': False, 'github.event_name': 'workflow_dispatch', 'inputs.channel': 'stable',
                'inputs.dry_run': False, 'inputs.desktop_validation_only': False, 'inputs.backfill_tag': '',
                'github.run_attempt': 1, **{f'needs.{name}.result': 'success' for name in dependencies}}
        def eligible(values):
            resolved = expression
            for key, value in values.items(): resolved = resolved.replace(key, repr(value))
            resolved = resolved.replace('&&', ' and ').replace('||', ' or ').replace('!', ' not ')
            return eval(' '.join(resolved.split()), {'__builtins__': {}}, {})
        self.assertTrue(eligible(good))
        excluded = [('cancelled()', True), ('github.event_name', 'schedule'), ('inputs.channel', 'nightly'),
                    ('inputs.dry_run', True), ('inputs.desktop_validation_only', True), ('inputs.backfill_tag', 'v1.2.3'),
                    ('github.run_attempt', 2)]
        excluded += [(f'needs.{name}.result', status) for name in dependencies for status in ('failure', 'skipped', 'cancelled')]
        for key, value in excluded:
            with self.subTest(key=key, value=value): self.assertFalse(eligible(dict(good, **{key: value})))
        self.assertIn('uses: ./.github/workflows/announce-discord-release.yml', block)
        self.assertIn('release_tag: ${{ needs.prepare.outputs.tag }}', block)
        self.assertIn('mode: create', block)
        self.assertIn('acknowledge_create: true', block)
        self.assertNotIn('inputs.notify_contributors', block)

    # @covers AC-RELEASE-DISCORD-003.1, AC-RELEASE-DISCORD-003.4
    def test_manual_recovery_defaults_to_preview(self):
        self.assertIn('workflow_dispatch:', WORKFLOW)
        self.assertIn('workflow_call:', WORKFLOW)
        self.assertEqual(WORKFLOW.count('default: preview'), 2)
        for name in ('release_tag', 'mode', 'message_id', 'acknowledge_create'):
            self.assertEqual(len(re.findall(rf'^      {name}:$', WORKFLOW, re.M)), 2)
        self.assertIn('options: [preview, create, update]', WORKFLOW)
        self.assertIn('Publication succeeded and no announcement exists for this release', WORKFLOW)
        preview = job(WORKFLOW, 'preview')
        self.assertIn("inputs.mode == 'preview'", preview)
        self.assertNotIn('secrets.', preview)
        self.assertNotIn('environment:', preview)
        delivery = job(WORKFLOW, 'deliver')
        self.assertIn("inputs.mode != 'preview'", delivery)
        self.assertIn('environment: release', delivery)
        self.assertIn('DISCORD_GENERAL_WEBHOOK_URL: ${{ secrets.DISCORD_GENERAL_WEBHOOK_URL }}', delivery)
        self.assertIn('ACKNOWLEDGE_CREATE: ${{ inputs.acknowledge_create }}', delivery)
        self.assertIn('DISCORD_MESSAGE_ID: ${{ inputs.message_id }}', delivery)
        self.assertIn('group: discord-release-announcements-${{ github.repository }}', delivery)
        self.assertIn('cancel-in-progress: false', delivery)
        self.assertIn('queue: max', delivery)

    # @covers AC-RELEASE-DISCORD-001.3, AC-RELEASE-DISCORD-003.2
    def test_trusted_source_least_privilege_and_receipts(self):
        for name in ('preview', 'deliver'):
            block = job(WORKFLOW, name)
            with self.subTest(job=name):
                self.assertIn('timeout-minutes: 10', block)
                self.assertIn('ref: ${{ github.workflow_sha }}', block)
                self.assertIn('persist-credentials: false', block)
                self.assertIn('fetch-depth: 0', block)
                self.assertIn('GITHUB_WORKFLOW_REF: ${{ github.workflow_ref }}', block)
                self.assertIn('GITHUB_WORKFLOW_SHA: ${{ github.workflow_sha }}', block)
                self.assertIn('HELPER_CHECKOUT_SHA', block)
                self.assertIn('RELEASE_TAG: ${{ inputs.release_tag }}', block)
                self.assertIn('python3 .github/scripts/announce-discord-release.py', block)
                self.assertIn('if: ${{ always() }}', block)
                self.assertIn('path: ${{ runner.temp }}/discord-release', block)
        self.assertNotIn(': write', WORKFLOW)
        self.assertIn('contents: read', WORKFLOW)
        recovery = job(RELEASE, 'discord-recovery-notice')
        self.assertIn('Discord recovery required', recovery)
        for name in ('publish-plugin-sdk', 'stable-complete'):
            self.assertIn(f"needs.{name}.result == 'success'", recovery)

    def test_all_script_tests_registered_in_ci_and_local_target(self):
        for filename in ('.github/workflows/lint-action-pinning.yml', 'Makefile'):
            source = (ROOT / filename).read_text()
            for suite in ('discord-release-content', 'announce-discord-release', 'announce-discord-release-workflow-contract'):
                self.assertIn(f'python3 .github/scripts/{suite}_test.py', source)


if __name__ == '__main__': unittest.main()
