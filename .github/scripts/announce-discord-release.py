#!/usr/bin/env python3
"""Preview, create, or update one compact Discord release announcement."""
from __future__ import annotations

from datetime import datetime
from http.client import HTTPException
import importlib
import json
import math
import os
from pathlib import Path
import re
import sys
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.request import HTTPRedirectHandler, Request, build_opener

content = importlib.import_module('discord-release-content')
github = importlib.import_module('notify-release-contributors')
STABLE_TAG = re.compile(r'v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)')
SNOWFLAKE = re.compile(r'[1-9]\d{0,24}')
WEBHOOK = re.compile(r'https://discord\.com/api/(?:v10/)?webhooks/([1-9]\d{0,24})/[A-Za-z0-9_.-]+')


class AnnouncementError(Exception):
    """An operator-facing failure that excludes credentials and transport URLs."""


class ReleaseGit(github.GitRepository):
    def find_commit(self, text: str, tag: str) -> str | None:
        result = self._run(['git', 'log', f'refs/tags/{tag}', '--format=%H%x00%s', '--fixed-strings', f'--grep={text}'])
        if result.returncode:
            raise AnnouncementError('Could not read commit attribution from the release history.')
        matches = []
        for line in result.stdout.splitlines():
            sha, separator, subject = line.partition('\0')
            subject = re.sub(r'^[a-z]+(?:\([^)]*\))?!?:\s*', '', subject)
            if separator and subject == text and github.COMMIT_RE.fullmatch(sha):
                matches.append(sha)
        return matches[0] if len(matches) == 1 else None


def resolve_release(api, repository: str, tag: str) -> dict:
    if not STABLE_TAG.fullmatch(tag):
        raise AnnouncementError('An exact Stable release tag such as v1.2.3 is required.')
    raw = api.get_release_by_tag(tag)
    release = github._release_from_payload(raw, repository)
    if release.tag != tag:
        raise AnnouncementError('GitHub returned a different release tag.')
    try:
        datetime.fromisoformat(release.published_at.replace('Z', '+00:00'))
    except (TypeError, ValueError):
        raise AnnouncementError('The release publication date is invalid.') from None
    return {'tag_name': tag, 'published_at': release.published_at, 'html_url': release.url, 'body': release.body}


def _login(value) -> tuple[str, ...]:
    login = value.get('login') if isinstance(value, dict) else None
    return (login,) if isinstance(login, str) and re.fullmatch(content.LOGIN, login) else ()


def resolve_credits(groups, api, git, repository: str, tag: str) -> dict:
    credits, cache = {}, {}
    for entries in content.highlight_candidates(groups).values():
        for entry in entries:
            authors = []
            for number in entry.prs:
                if number not in cache:
                    try:
                        pull = api.get_pull_request(number)
                    except github.ApiError as error:
                        if error.status != 404:
                            raise
                        pull = {}
                    sha = pull.get('merge_commit_sha')
                    base = pull.get('base')
                    base_repo = base.get('repo') if isinstance(base, dict) else None
                    valid = (pull.get('number') == number and pull.get('state') == 'closed' and pull.get('merged_at')
                             and isinstance(base_repo, dict) and base_repo.get('full_name') == repository
                             and isinstance(sha, str) and github.COMMIT_RE.fullmatch(sha))
                    cache[number] = _login(pull.get('user')) if valid and git.is_ancestor(sha, tag) else ()
                authors.extend(cache[number])
            if not authors:
                authors.extend(entry.logins)
            if not authors and not entry.prs:
                sha = git.find_commit(entry.text, tag)
                if sha and git.is_ancestor(sha, tag):
                    commit = api._request('GET', f'repos/{repository}/commits/{sha}')
                    if not isinstance(commit, dict):
                        raise AnnouncementError('GitHub returned invalid commit attribution.')
                    authors.extend(_login(commit.get('author')))
            credits[entry] = tuple(dict.fromkeys(authors))
    return credits


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class DiscordClient:
    def __init__(self, url: str, *, opener=None, sleep=time.sleep, clock=time.monotonic):
        match = WEBHOOK.fullmatch(url)
        if not match:
            raise AnnouncementError('DISCORD_GENERAL_WEBHOOK_URL must be a Discord HTTPS incoming-webhook URL without query parameters.')
        self.url, self.webhook_id = url, match.group(1)
        self.opener = opener or build_opener(NoRedirect())
        self.sleep, self.clock = sleep, clock

    def request(self, method: str, suffix: str = '', payload: dict | None = None) -> dict:
        data = json.dumps(payload, ensure_ascii=False).encode() if payload is not None else None
        deadline = self.clock() + 120
        for attempt in range(3):
            request = Request(self.url + suffix, data=data, method=method,
                              headers={'User-Agent': 'Kandev-Release-Announcements/1.0', 'Content-Type': 'application/json'})
            try:
                with self.opener.open(request, timeout=20) as response:
                    body = response.read(1024 * 1024 + 1)
                if len(body) > 1024 * 1024:
                    raise ValueError('oversized response')
                result = json.loads(body)
                if not isinstance(result, dict):
                    raise ValueError('invalid response')
                return result
            except HTTPError as error:
                if error.code == 429:
                    try:
                        delay = float(json.loads(error.read(4096)).get('retry_after'))
                    except (TypeError, ValueError, AttributeError):
                        delay = float('inf')
                    if attempt < 2 and math.isfinite(delay) and 0 <= delay <= deadline - self.clock() - 20:
                        self.sleep(delay)
                        continue
                    raise AnnouncementError('Discord rate limit exceeded the retry budget. Inspect the channel before recovery.') from None
                if error.code >= 500 and method in ('POST', 'PATCH'):
                    raise AnnouncementError('Discord delivery is uncertain. Inspect the channel before recovery; no automatic resend occurred.') from None
                raise AnnouncementError(f'Discord {method} failed (HTTP {error.code}). No automatic resend occurred.') from None
            except (OSError, URLError, HTTPException, ValueError):
                if method in ('POST', 'PATCH'):
                    raise AnnouncementError('Discord delivery is uncertain. Inspect the channel before recovery; no automatic resend occurred.') from None
                raise AnnouncementError('Could not read Discord metadata. No message was sent.') from None
        raise AnnouncementError('Discord request attempts were exhausted.')

    def deliver(self, payload: dict, release: dict, mode: str, message_id: str) -> dict:
        destination = self.request('GET')
        if (destination.get('id') != self.webhook_id or destination.get('type') != 1
                or any(not SNOWFLAKE.fullmatch(str(destination.get(key, ''))) for key in ('channel_id', 'guild_id'))):
            raise AnnouncementError('Discord returned an invalid incoming-webhook destination.')
        if mode == 'update':
            previous = self.request('GET', '/messages/' + message_id)
            if (previous.get('id') != message_id or previous.get('webhook_id') != self.webhook_id
                    or previous.get('channel_id') != destination['channel_id']
                    or f'Kandev {release["tag_name"]} ' not in previous.get('content', '')
                    or f']({release["html_url"]})' not in previous.get('content', '')):
                raise AnnouncementError('The existing Discord message does not match this webhook and release.')
            body = {key: value for key, value in payload.items() if key != 'username'}
            message = self.request('PATCH', '/messages/' + message_id, body)
        else:
            message = self.request('POST', '?wait=true', payload)
        confirmed_id = message.get('id', '')
        if (not isinstance(confirmed_id, str) or not SNOWFLAKE.fullmatch(confirmed_id)
                or message.get('channel_id') != destination['channel_id'] or message.get('webhook_id') != self.webhook_id
                or message.get('content') != payload['content']
                or (mode == 'update' and confirmed_id != message_id)):
            raise AnnouncementError('Discord delivery is uncertain because the returned message did not match. Inspect the channel before recovery.')
        return {'message_id': confirmed_id, 'channel_id': destination['channel_id'], 'release_tag': release['tag_name'],
                'url': f'https://discord.com/channels/{destination["guild_id"]}/{destination["channel_id"]}/{confirmed_id}'}


def validate_inputs(env) -> tuple[str, str]:
    mode = env.get('ANNOUNCEMENT_MODE', 'preview')
    message_id = env.get('DISCORD_MESSAGE_ID', '').strip()
    if mode not in ('preview', 'create', 'update'):
        raise AnnouncementError('Announcement mode must be preview, create, or update.')
    if mode == 'update' and not SNOWFLAKE.fullmatch(message_id):
        raise AnnouncementError('Update requires the existing Discord message ID.')
    if mode != 'update' and message_id:
        raise AnnouncementError('Only update accepts a Discord message ID.')
    if mode == 'create' and env.get('ACKNOWLEDGE_CREATE') != 'true':
        raise AnnouncementError('Create requires acknowledgement that publication succeeded and the channel contains no announcement for this release.')
    if mode != 'preview' and not env.get('DISCORD_GENERAL_WEBHOOK_URL'):
        raise AnnouncementError('Set DISCORD_GENERAL_WEBHOOK_URL in the release environment before sending.')
    return mode, message_id


def write_preview(output: Path, payload: dict, groups, warnings: list[str]) -> str:
    output.mkdir(parents=True, exist_ok=True)
    (output / 'payload.json').write_text(json.dumps(payload, ensure_ascii=False, indent=2) + '\n')
    metadata = {'counts': {name: len(entries) for name, entries in groups.items()}, 'warnings': warnings}
    (output / 'metadata.json').write_text(json.dumps(metadata, indent=2) + '\n')
    markdown = payload['content'] + '\n\n'
    for embed in payload['embeds']:
        markdown += '### ' + embed['title'] + '\n\n' + embed['description'] + '\n\n'
        if 'footer' in embed:
            markdown += embed['footer']['text'] + '\n\n'
    (output / 'preview.md').write_text(markdown)
    return markdown


def execute(env, api, git, discord_factory=DiscordClient) -> dict | None:
    mode, message_id = validate_inputs(env)
    repository = env.get('GITHUB_REPOSITORY', 'kdlbs/kandev')
    tag = env.get('RELEASE_TAG', '').strip()
    try:
        release = resolve_release(api, repository, tag)
        git.resolve_tag(tag)
        groups = content.parse_notes(release['body'], repository)
        credits = resolve_credits(groups, api, git, repository, tag)
        payload = content.render_payload(release, groups, credits, repository)
    except (github.ApiError, github.GitCommandError, github.ReleaseNotificationError, ValueError) as error:
        raise AnnouncementError(str(error)) from None
    warnings = ['A selected entry has no resolvable contributor.' for entry, authors in credits.items() if not authors]
    output = Path(env.get('ANNOUNCEMENT_OUTPUT_DIR') or tempfile.mkdtemp(prefix='kandev-discord-'))
    markdown = write_preview(output, payload, groups, warnings)
    summary = env.get('GITHUB_STEP_SUMMARY')
    if summary:
        with open(summary, 'a') as stream:
            stream.write('## Discord announcement preview\n\n' + markdown)
            for warning in warnings:
                stream.write('- ' + warning + '\n')
    if mode == 'preview':
        print('Discord announcement preview created. No message was sent.')
        return None
    receipt = discord_factory(env['DISCORD_GENERAL_WEBHOOK_URL']).deliver(payload, release, mode, message_id)
    (output / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    if summary:
        with open(summary, 'a') as stream:
            stream.write(f'\nConfirmed {mode}: [{receipt["message_id"]}]({receipt["url"]})\n')
    print(f'Discord announcement confirmed: {receipt["url"]}')
    return receipt


def main() -> int:
    try:
        validate_inputs(os.environ)
        repository = os.environ.get('GITHUB_REPOSITORY', 'kdlbs/kandev')
        execute(os.environ, github.GitHubApi(repository), ReleaseGit())
        return 0
    except (AnnouncementError, github.ReleaseNotificationError, OSError) as error:
        message = str(error) if isinstance(error, (AnnouncementError, github.ReleaseNotificationError)) else 'Could not read or write announcement artifacts.'
        print('Discord announcement failed: ' + message, file=sys.stderr)
        if os.environ.get('GITHUB_STEP_SUMMARY'):
            try:
                with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as stream:
                    stream.write('\n## Discord delivery failed\n\n' + message + '\nPackage publication is unchanged.\n')
            except OSError:
                pass
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
