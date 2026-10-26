#!/usr/bin/env python3
"""Release selection and real HTTP-boundary tests for Discord delivery."""
import copy
import importlib
from http.client import IncompleteRead
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
from urllib.error import HTTPError, URLError

sys.path.insert(0, str(Path(__file__).parent))
SCRIPT = Path(__file__).with_name('announce-discord-release.py')
announce = importlib.import_module('announce-discord-release') if SCRIPT.exists() else None
GH = importlib.import_module('notify-release-contributors')
REPO = 'kdlbs/kandev'
TAG = 'v1.2.3'
URL = 'https://github.com/kdlbs/kandev/releases/tag/v1.2.3'
HOOK = 'https://discord.com/api/webhooks/111/test_token'
RELEASE = {'id': 73, 'tag_name': TAG, 'draft': False, 'prerelease': False,
           'published_at': '2026-10-25T12:00:00Z', 'html_url': URL,
           'body': '### Features\n- new mode ([#1](https://github.com/kdlbs/kandev/pull/1))\n'}


class FakeGit:
    def resolve_tag(self, tag): return 'a' * 40
    def is_ancestor(self, sha, tag): return sha == 'a' * 40
    def find_commit(self, text, tag): return None


class GitHubRunner:
    def __init__(self):
        self.release = copy.deepcopy(RELEASE)
        self.pull = {'number': 1, 'state': 'closed', 'merged_at': '2026-10-24T00:00:00Z',
                     'base': {'repo': {'full_name': REPO}}, 'merge_commit_sha': 'a' * 40,
                     'user': {'login': 'jcfs'}}
        self.pull_status = 0
        self.calls = []
    def __call__(self, command, **kwargs):
        route = command[-1]
        self.calls.append(command)
        if '/releases/tags/' in route: value, status = self.release, 0
        elif '/pulls/' in route: value, status = self.pull, self.pull_status
        elif '/commits/' in route: value, status = {'author': {'login': 'commit-author'}}, 0
        else: raise AssertionError('Unexpected GitHub request: ' + route)
        return subprocess.CompletedProcess(command, status, json.dumps(value), 'HTTP ' + str(status) if status else '')


class Response(io.BytesIO):
    def __init__(self, value): super().__init__(json.dumps(value).encode())


class DiscordHTTP:
    def __init__(self):
        self.calls = []
        self.failure = None
        self.existing = None
    def open(self, request, timeout):
        method = request.get_method()
        self.calls.append((method, request.full_url, json.loads(request.data) if request.data else None))
        if method in ('POST', 'PATCH') and self.failure:
            failure = self.failure
            if isinstance(failure, list):
                failure = failure.pop(0) if failure else None
            if failure: raise failure
        if method == 'GET' and '/messages/' not in request.full_url:
            return Response({'id': '111', 'type': 1, 'channel_id': '222', 'guild_id': '333'})
        if method == 'GET':
            if self.existing is None:
                raise HTTPError(request.full_url, 404, 'not found', {}, io.BytesIO(b'{}'))
            return Response(self.existing)
        body = json.loads(request.data)
        self.existing = dict(body, id='444', webhook_id='111', channel_id='222')
        return Response(self.existing)


class AnnouncementTest(unittest.TestCase):
    def setUp(self):
        self.assertIsNotNone(announce, 'The Discord announcement helper must exist.')
        self.runner = GitHubRunner()
        self.api = GH.GitHubApi(REPO, runner=self.runner)
        self.git = FakeGit()
        self.http = DiscordHTTP()
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.env = {'GITHUB_REPOSITORY': REPO, 'RELEASE_TAG': TAG, 'ANNOUNCEMENT_MODE': 'preview',
                    'ANNOUNCEMENT_OUTPUT_DIR': self.temp.name, 'GITHUB_STEP_SUMMARY': self.temp.name + '/summary.md'}
        self.sleeps = []
    def execute(self):
        with patch('sys.stdout', new=io.StringIO()):
            return announce.execute(self.env, self.api, self.git,
                lambda url: announce.DiscordClient(url, opener=self.http, sleep=self.sleeps.append))
    def enable_create(self):
        self.env.update(ANNOUNCEMENT_MODE='create', ACKNOWLEDGE_CREATE='true', DISCORD_GENERAL_WEBHOOK_URL=HOOK)

    # @covers AC-RELEASE-DISCORD-003.1, AC-RELEASE-DISCORD-002.2
    def test_preview_has_no_discord_writes_and_includes_maintainer(self):
        self.execute()
        payload = json.loads(Path(self.temp.name, 'payload.json').read_text())
        self.assertIn('https://github.com/jcfs', payload['embeds'][0]['description'])
        self.assertEqual(self.http.calls, [])
        self.assertEqual(len([c for c in self.runner.calls if '/releases/tags/' in c[-1]]), 1)

    # @covers AC-RELEASE-DISCORD-001.2
    def test_release_selection_rejects_invalid_or_unpublished_metadata(self):
        for changes in ({'draft': True}, {'prerelease': True}, {'tag_name': 'v1.2.4'}, {'published_at': None}, {'body': []}):
            with self.subTest(changes=changes):
                self.runner.release = dict(RELEASE, **changes)
                with self.assertRaises(announce.AnnouncementError): self.execute()
        self.assertEqual(self.http.calls, [])
        for tag in ('', 'latest', 'v1.2.3-nightly.sha123', 'v01.2.3', '--help'):
            with self.subTest(tag=tag):
                self.env['RELEASE_TAG'] = tag
                with self.assertRaises(announce.AnnouncementError): self.execute()

    def test_invalid_pr_metadata_does_not_invent_credit(self):
        for change in ({'merged_at': None}, {'merge_commit_sha': 'b' * 40}, {'base': {'repo': {'full_name': 'other/repo'}}}, {'base': {'repo': None}}):
            with self.subTest(change=change):
                old = copy.deepcopy(self.runner.pull)
                self.runner.pull.update(change)
                self.execute()
                self.assertIn('Contributor unavailable', Path(self.temp.name, 'payload.json').read_text())
                self.runner.pull = old
        self.runner.pull_status = 403
        with self.assertRaises(announce.AnnouncementError): self.execute()
        self.assertEqual(self.http.calls, [])

    def test_missing_pr_reference_falls_back_to_published_credit(self):
        self.runner.pull_status = 404
        self.runner.release['body'] = self.runner.release['body'].rstrip() + ' by @external-user\n'
        self.execute()
        self.assertIn('https://github.com/external-user', Path(self.temp.name, 'payload.json').read_text())

    def test_unlinked_entry_uses_verified_commit_account(self):
        self.runner.release['body'] = '### Features\n- direct change\n'
        self.git.find_commit = lambda text, tag: 'a' * 40
        self.execute()
        self.assertIn('https://github.com/commit-author', Path(self.temp.name, 'payload.json').read_text())

    # @covers AC-RELEASE-DISCORD-002.6, AC-RELEASE-DISCORD-003.2
    def test_single_webhook_single_message(self):
        self.enable_create()
        self.execute()
        writes = [c for c in self.http.calls if c[0] in ('POST', 'PATCH')]
        self.assertEqual(len(writes), 1)
        self.assertEqual(writes[0][1], HOOK + '?wait=true')
        self.assertEqual(writes[0][2]['allowed_mentions'], {'parse': []})
        receipt = json.loads(Path(self.temp.name, 'receipt.json').read_text())
        self.assertEqual(receipt['url'], 'https://discord.com/channels/333/222/444')
        self.assertNotIn('test_token', Path(self.temp.name, 'summary.md').read_text())

    # @covers AC-RELEASE-DISCORD-003.4
    def test_create_requires_acknowledgement_and_valid_inputs(self):
        self.enable_create()
        for values in ({'ACKNOWLEDGE_CREATE': 'false'}, {'DISCORD_GENERAL_WEBHOOK_URL': ''},
                       {'DISCORD_MESSAGE_ID': '444'}, {'ANNOUNCEMENT_MODE': 'unknown'}):
            with self.subTest(values=values):
                original = dict(self.env); self.env.update(values)
                with self.assertRaises(announce.AnnouncementError): self.execute()
                self.env = original
        self.assertEqual(self.http.calls, [])

    # @covers AC-RELEASE-DISCORD-003.3
    def test_update_requires_matching_message_and_removes_attachments(self):
        self.enable_create(); self.execute(); self.http.calls.clear()
        self.env.update(ANNOUNCEMENT_MODE='update', DISCORD_MESSAGE_ID='444')
        self.http.existing['attachments'] = [{'id': 'old'}]
        self.execute()
        self.assertEqual([c[0] for c in self.http.calls], ['GET', 'GET', 'PATCH'])
        self.assertEqual(self.http.calls[-1][2]['attachments'], [])
        for existing in (None, dict(self.http.existing, webhook_id='999'), dict(self.http.existing, content='different release')):
            self.http.calls.clear(); self.http.existing = existing
            with self.assertRaises(announce.AnnouncementError): self.execute()
            self.assertFalse(any(c[0] in ('POST', 'PATCH') for c in self.http.calls))

    def test_uncertain_send_never_retries_or_exposes_webhook(self):
        self.enable_create()
        for failure in (URLError(HOOK), IncompleteRead(b'partial'), HTTPError(HOOK, 500, HOOK, {}, io.BytesIO(b'{}'))):
            self.http.calls.clear(); self.http.failure = failure
            with self.assertRaises(announce.AnnouncementError) as result: self.execute()
            self.assertIn('uncertain', str(result.exception).lower())
            self.assertNotIn('test_token', str(result.exception))
            self.assertEqual(sum(c[0] == 'POST' for c in self.http.calls), 1)

    def test_rate_limit_retry_is_bounded_and_rejection_is_not_retried(self):
        self.enable_create()
        def rate(): return HTTPError(HOOK, 429, 'limited', {}, io.BytesIO(b'{"retry_after": 2}'))
        self.http.failure = [rate()]
        self.execute()
        self.assertEqual(self.sleeps, [2])
        self.http.calls.clear(); self.http.failure = [rate(), rate(), rate()]
        with self.assertRaises(announce.AnnouncementError): self.execute()
        self.assertEqual(sum(c[0] == 'POST' for c in self.http.calls), 3)
        for status in (400, 401, 403, 302):
            self.http.calls.clear(); self.http.failure = HTTPError(HOOK, status, HOOK, {}, io.BytesIO(b'{}'))
            with self.assertRaises(announce.AnnouncementError): self.execute()
            self.assertEqual(sum(c[0] == 'POST' for c in self.http.calls), 1)

    def test_webhook_validation_rejects_other_hosts_and_url_credentials(self):
        for url in ('http://discord.com/api/webhooks/111/token', 'https://evil.test/api/webhooks/111/token',
                    'https://discord.com@evil.test/api/webhooks/111/token', HOOK + '?target=evil', HOOK + '#x'):
            with self.subTest(url=url), self.assertRaises(announce.AnnouncementError):
                announce.DiscordClient(url, opener=self.http)
        self.assertEqual(self.http.calls, [])

    def test_commit_match_is_exact_and_unambiguous(self):
        def runner(command, **kwargs):
            return subprocess.CompletedProcess(command, 0, 'a'*40+'\0feat(ui): direct change\n'+'b'*40+'\0fix: direct change later\n', '')
        git = announce.ReleaseGit(runner=runner)
        self.assertEqual(git.find_commit('direct change', TAG), 'a' * 40)
        self.assertIsNone(git.find_commit('direct', TAG))

    def test_entry_point_creates_public_artifacts_without_a_secret(self):
        with patch.dict(os.environ, self.env, clear=True), patch.object(announce.github, 'GitHubApi', return_value=self.api), patch.object(announce, 'ReleaseGit', return_value=self.git), patch('sys.stdout', new=io.StringIO()):
            self.assertEqual(announce.main(), 0)
        self.assertTrue(Path(self.temp.name, 'payload.json').is_file())
        self.assertFalse(Path(self.temp.name, 'receipt.json').exists())
        self.assertEqual(self.http.calls, [])

    def test_invalid_response_is_uncertain_without_a_second_post(self):
        self.enable_create()
        original = self.http.open
        def open_response(request, timeout):
            response = original(request, timeout)
            return Response({'id': '444'}) if request.get_method() == 'POST' else response
        self.http.open = open_response
        with self.assertRaisesRegex(announce.AnnouncementError, 'uncertain'): self.execute()
        self.assertEqual(sum(c[0] == 'POST' for c in self.http.calls), 1)
        self.assertFalse(Path(self.temp.name, 'receipt.json').exists())

    def test_rate_limit_cannot_sleep_past_the_deadline(self):
        self.enable_create()
        self.http.failure = HTTPError(HOOK, 429, 'limited', {}, io.BytesIO(b'{"retry_after": 121}'))
        with self.assertRaisesRegex(announce.AnnouncementError, 'retry budget'): self.execute()
        self.assertEqual(self.sleeps, [])
        self.assertEqual(sum(c[0] == 'POST' for c in self.http.calls), 1)

    def test_entry_point_reports_safe_failure(self):
        with patch.dict(os.environ, {'ANNOUNCEMENT_MODE': 'invalid'}, clear=True), patch('sys.stderr', new=io.StringIO()) as error:
            self.assertEqual(announce.main(), 1)
        self.assertNotIn('Traceback', error.getvalue())


if __name__ == '__main__': unittest.main()
