"""Pure parsing and bounded rendering of a compact release announcement."""
from __future__ import annotations

from dataclasses import dataclass
import re
from urllib.parse import quote


CATEGORIES = {
    'Features': '✨', 'Fixes': '🐛', 'Performance': '⚡',
    'Refactoring': '♻️', 'Documentation': '📚', 'Other changes': '🛠️',
}
LOGIN = r'[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})(?:\[bot\])?'
CREDIT = re.compile(r'\s+by @(' + LOGIN + r')\s*$')


@dataclass(frozen=True)
class Entry:
    text: str
    prs: tuple[int, ...]
    logins: tuple[str, ...]


def _entry(text: str, repository: str) -> Entry:
    link = re.compile(r'\(\[#(\d+)\]\(https://github\.com/' + re.escape(repository) + r'/pull/\1\)\)')
    prs = tuple(dict.fromkeys(int(m.group(1)) for m in link.finditer(text) if int(m.group(1)) > 0))
    text = link.sub('', text)
    credit = CREDIT.search(text)
    logins = (credit.group(1),) if credit else ()
    if credit:
        text = text[:credit.start()]
    return Entry(' '.join(text.split()), prs, logins)


def parse_notes(notes: str, repository: str) -> dict[str, list[Entry]]:
    raw: dict[str, list[str]] = {}
    category = None
    fence = None
    for line in notes.splitlines():
        marker = re.match(r'^\s*(`{3,}|~{3,})', line)
        if marker:
            token = marker.group(1)
            if fence is None:
                fence = token
            elif token[0] == fence[0] and len(token) >= len(fence):
                fence = None
            continue
        if fence:
            continue
        if line.startswith('### '):
            title = re.sub(r'<!--.*?-->', '', line[4:]).strip()
            title = 'Fixes' if title == 'Bug Fixes' else title
            category = title if title in CATEGORIES else 'Other changes'
            raw.setdefault(category, [])
        elif line.startswith('#'):
            category = None
        elif category and line.startswith('- '):
            raw[category].append(line[2:])
        elif category and raw[category] and line.startswith(('  ', '\t')) and line.strip():
            raw[category][-1] += ' ' + line.strip()
    return {name: [_entry(text, repository) for text in entries]
            for name, entries in raw.items() if entries}


def highlight_candidates(groups: dict[str, list[Entry]]) -> dict[str, list[Entry]]:
    names = [name for name in ('Features', 'Fixes') if name in groups]
    if not names:
        names = list(groups)[:1]
    return {name: groups[name][:3] for name in names}


def escape(text: str) -> str:
    text = re.sub(r'[\x00-\x1f\x7f]', ' ', text).replace('@', '@\u200b')
    return re.sub(r'([\\`*_{}\[\]()<>|~])', r'\\\1', text)


def render_entry(entry: Entry, repository: str, credits: dict[Entry, tuple[str, ...]]) -> str:
    authors = credits.get(entry, entry.logins)
    authors = tuple(dict.fromkeys(login for login in authors if re.fullmatch(LOGIN, login)))
    links = ' '.join(f'[#{number}](https://github.com/{repository}/pull/{number})' for number in entry.prs)
    attribution = ', '.join(f'[@{escape(login)}](https://github.com/{quote(login, safe="")})' for login in authors)
    return '- ' + escape(entry.text) + (f' ({links})' if links else '') + ' by ' + (attribution or 'Contributor unavailable')


def units(text: str) -> int:
    return len(text.encode('utf-16-le')) // 2


def payload_units(payload: dict) -> int:
    return units(payload['content']) + sum(
        units(embed['title']) + units(embed['description']) + units(embed.get('footer', {}).get('text', ''))
        for embed in payload['embeds']
    )


def _payload(header: str, selected: dict[str, list[str]], groups: dict[str, list[Entry]], total: int) -> dict:
    embeds = [{'title': f'{CATEGORIES[name]} {name} ({len(groups[name])})',
               'description': '\n'.join(lines), 'color': 0x5865F2}
              for name, lines in selected.items() if lines]
    shown = sum(len(lines) for lines in selected.values())
    footer = f'{shown} highlights shown · {total} changes in the full release'
    if embeds:
        embeds[-1]['footer'] = {'text': footer}
    else:
        header += '\n\n' + footer
    return {'username': 'Kandev Releases', 'content': header, 'embeds': embeds,
            'allowed_mentions': {'parse': []}, 'attachments': []}


def render_payload(release: dict, groups: dict[str, list[Entry]], credits: dict[Entry, tuple[str, ...]], repository: str) -> dict:
    total = sum(len(entries) for entries in groups.values())
    summary = ' · '.join(f'{CATEGORIES[name]} {len(entries)} {name.lower()}' for name, entries in groups.items())
    label = 'change' if total == 1 else 'changes'
    header = (f'🎉 **Kandev {release["tag_name"]} is available!**\n📅 {release["published_at"][:10]}\n\n'
              f'**{total} {label} in this release**\n{summary}\n\n'
              f'[View the full release on GitHub]({release["html_url"]})')
    selected: dict[str, list[str]] = {}
    for name, entries in highlight_candidates(groups).items():
        selected[name] = []
        for entry in entries:
            selected[name].append(render_entry(entry, repository, credits))
            candidate = _payload(header, selected, groups, total)
            if payload_units(candidate) > 2500:
                selected[name].pop()
    payload = _payload(header, selected, groups, total)
    if payload_units(payload) > 2500 or units(payload['content']) > 2000:
        raise ValueError('The release summary exceeds the Discord message budget.')
    return payload
