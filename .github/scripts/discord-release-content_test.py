#!/usr/bin/env python3
"""Behavior tests for compact Discord release content."""
import importlib
import json
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).parent))
SCRIPT = Path(__file__).with_name('discord-release-content.py')
content = importlib.import_module('discord-release-content') if SCRIPT.exists() else None
REPOSITORY = 'kdlbs/kandev'
RELEASE = {'tag_name': 'v1.2.3', 'published_at': '2026-10-25T12:00:00Z',
           'html_url': 'https://github.com/kdlbs/kandev/releases/tag/v1.2.3'}


class ContentTest(unittest.TestCase):
    def parse(self, notes):
        self.assertIsNotNone(content, 'The compact changelog formatter must exist.')
        return content.parse_notes(notes, REPOSITORY)

    def render(self, groups, credits=None):
        return content.render_payload(RELEASE, groups, credits or {}, REPOSITORY)

    # @covers AC-RELEASE-DISCORD-002.1, AC-RELEASE-DISCORD-002.4
    def test_counts_cover_complete_release_before_selection(self):
        notes = '### Features\n' + ''.join(f'- feature {i}\n' for i in range(157))
        notes += '### Bug Fixes\n' + ''.join(f'- fix {i}\n' for i in range(208))
        payload = self.render(self.parse(notes))
        self.assertIn('365 changes in this release', payload['content'])
        self.assertIn('157 features', payload['content'])
        self.assertIn('208 fixes', payload['content'])
        self.assertEqual([e['title'] for e in payload['embeds']], ['✨ Features (157)', '🐛 Fixes (208)'])
        self.assertEqual(sum(e['description'].count('\n') + 1 for e in payload['embeds']), 6)
        self.assertIn('6 highlights shown', payload['embeds'][-1]['footer']['text'])

    # @covers AC-RELEASE-DISCORD-002.2, AC-RELEASE-DISCORD-002.3
    def test_grouped_entries_and_links_with_resolved_credits(self):
        groups = self.parse('### Features\n- new mode ([#12](https://github.com/kdlbs/kandev/pull/12)) by @someone\n')
        entry = groups['Features'][0]
        payload = self.render(groups, {entry: ('jcfs', 'dependabot[bot]')})
        text = payload['embeds'][0]['description']
        self.assertIn('[#12](https://github.com/kdlbs/kandev/pull/12)', text)
        self.assertIn('https://github.com/jcfs', text)
        self.assertIn('https://github.com/dependabot%5Bbot%5D', text)
        self.assertNotIn('@someone', text)
        self.assertIn(RELEASE['html_url'], payload['content'])

    def test_parser_preserves_continuations_and_ignores_warning_and_code_bullets(self):
        groups = self.parse('> [!WARNING]\n> unsigned builds\n\n## 1.2.3\n'
                            '### Features\n- first\n  continued text\n  - nested detail\n'
                            '```text\n- not an entry\n```\n- second\n'
                            '### Bug Fixes\n### Something new\n- another\n')
        self.assertEqual({k: len(v) for k, v in groups.items()}, {'Features': 2, 'Other changes': 1})
        self.assertIn('continued text', groups['Features'][0].text)
        self.assertIn('nested detail', groups['Features'][0].text)
        self.assertNotIn('not an entry', repr(groups))

    # @covers AC-RELEASE-DISCORD-002.4, AC-RELEASE-DISCORD-002.5
    def test_unicode_budget_drops_whole_entries_without_changing_counts(self):
        groups = self.parse('### Features\n- ' + '🦊' * 2000 + '\n- fits\n### Bug Fixes\n- fix\n')
        payload = self.render(groups)
        self.assertIn('3 changes in this release', payload['content'])
        self.assertNotIn('🦊', json.dumps(payload, ensure_ascii=False))
        self.assertIn('fits', repr(payload))
        self.assertLessEqual(content.payload_units(payload), 2500)
        self.assertEqual(payload['allowed_mentions'], {'parse': []})
        self.assertEqual(payload['attachments'], [])

    def test_credits_fallback_and_markdown_escape(self):
        groups = self.parse('### Features\n- **bold** <@123> @everyone by @trusted-user\n- plain\n')
        payload = self.render(groups)
        text = payload['embeds'][0]['description']
        self.assertIn(r'\*\*bold\*\*', text)
        self.assertNotIn('<@123>', text)
        self.assertIn('https://github.com/trusted-user', text)
        self.assertIn('Contributor unavailable', text)

    # @covers AC-RELEASE-DISCORD-002.6, AC-RELEASE-DISCORD-002.7
    def test_compact_format_and_emojis_without_unwanted_copy(self):
        groups = self.parse('### Performance\n- faster\n### Documentation\n- docs\n')
        payload = self.render(groups)
        self.assertEqual(payload['embeds'][0]['title'], '⚡ Performance (1)')
        self.assertIn('📚 1 documentation', payload['content'])
        for excluded in ('kandev.ai', 'discord.com/channels', 'unsigned', 'attached', '(feat)', '(fix)'):
            self.assertNotIn(excluded, json.dumps(payload))

    def test_repeated_groups_are_combined_and_empty_notes_are_valid(self):
        groups = self.parse('### Features\n- one\n### Features\n- two\n### Documentation\n')
        self.assertEqual(len(groups['Features']), 2)
        empty = self.render(self.parse('No changes.'))
        self.assertIn('0 changes in this release', empty['content'])
        self.assertEqual(empty['embeds'], [])

    def test_multiple_prs_do_not_multiply_entry_counts(self):
        groups = self.parse('### Features\n- joint ([#1](https://github.com/kdlbs/kandev/pull/1)) ([#2](https://github.com/kdlbs/kandev/pull/2))\n')
        self.assertEqual(groups['Features'][0].prs, (1, 2))
        self.assertIn('1 change in this release', self.render(groups)['content'])


if __name__ == '__main__':
    unittest.main()
