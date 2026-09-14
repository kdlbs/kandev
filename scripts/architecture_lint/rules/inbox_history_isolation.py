"""Isolate the Inbox History read path from operational sinks and UI state.

AC-UI-INBOX-HISTORY-001.5/.17/.22 need two, opposite-direction halves of one
rule:

- The SINK half (AC .5): a closed set of backend files that must never
  reference the history read's exported entry points, because the read
  shares a package with most of its own sinks and there is no import
  statement to forbid within one Go package.
- The SOURCE half (AC .17/.22): the history modules themselves must never
  reference the sidebar/Needs-you count state or an event-stream
  subscription entry point, so History can never (even accidentally) feed
  the sidebar badge or start reacting to live updates.

Both sets and both forbidden-target lists are enumerated explicitly rather
than inferred from a substring match on "history" or "count" -- per the
system design's "The rule's own module shall enumerate all of them rather
than infer them from a substring match", and AC .16 requires the History
tab to render its own bundle count, so the bare identifier "count" can never
be a forbidden target.
"""

from pathlib import Path

from architecture_lint.model import Finding, Rule


RULE_ID = "ARCH-INBOX-HISTORY-ISOLATION"

# AC .5's closed sink set, normative by reference to the system design's
# "Isolation mechanism" > "The closed sink set" table. A rule scanning fewer
# files does not satisfy AC .5.
SINK_FILES = frozenset(
    {
        "apps/backend/internal/task/repository/sqlite/pending_interactions.go",
        "apps/backend/internal/task/repository/sqlite/message.go",
        "apps/backend/internal/task/repository/sqlite/pending_action_projection.go",
        "apps/backend/internal/task/repository/sqlite/message_clarification_response.go",
        "apps/backend/internal/task/service/service_events.go",
    }
)

# The history read's two exported entry points (task/repository/sqlite/
# clarification_history_query.go): forbidden anywhere in SINK_FILES.
HISTORY_READ_ENTRY_POINTS = ("ListInboxHistoryBundles", "CountInboxHistoryBundles")

# AC .17/.22's history-module SOURCE set: every new module Build added for
# the History read/render path. Prefixes end in "/" and match a directory;
# exact entries match one file.
HISTORY_SOURCE_PREFIXES = (
    "apps/web/lib/state/slices/inbox-history/",
    "apps/web/hooks/domains/inbox-history/",
    "apps/web/components/inbox-history/",
)
HISTORY_SOURCE_FILES = frozenset(
    {
        "apps/web/lib/api/domains/inbox-history-api.ts",
        "apps/backend/internal/task/repository/sqlite/clarification_history_query.go",
    }
)

# AC .17's forbidden targets. NOT the bare identifier "count": AC .16
# requires History to render its own bundle count, so that target could
# never pass. These are the Needs-you slice's own symbols instead -- the
# sidebar-badge selector, and the slice's own workspace-state field access
# -- verified round 4 against lib/state/slices/needs-you-inbox/{selectors,
# types}.ts and app-sidebar-primary-nav.tsx:30.
NEEDS_YOU_TARGETS = ("selectNeedsYouInboxCount", "needsYouInbox.byWorkspaceId")

# AC .22's forbidden targets. There is no dedicated event-stream
# subscription entry-point symbol -- client.on is a generic method on a
# shared WS client -- so the enumerable thing is the two event-name string
# literals use-needs-you-inbox-controller.ts:114-115 subscribes to.
EVENT_STREAM_TARGETS = ("session.pending_action_changed", "session.state_changed")

SOURCE_HALF_TARGETS = NEEDS_YOU_TARGETS + EVENT_STREAM_TARGETS


def _matches_source_set(path: str) -> bool:
    if path in HISTORY_SOURCE_FILES:
        return True
    return any(path.startswith(prefix) for prefix in HISTORY_SOURCE_PREFIXES)


def applies_to(path: str) -> bool:
    return path in SINK_FILES or _matches_source_set(path)


def _line_number(source: str, index: int) -> int:
    return source.count("\n", 0, index) + 1


def _find_all(source: str, needle: str) -> list[int]:
    positions: list[int] = []
    start = 0
    while True:
        index = source.find(needle, start)
        if index < 0:
            return positions
        positions.append(index)
        start = index + 1


def scan(path: str, source: str) -> list[Finding]:
    if path in SINK_FILES:
        targets = HISTORY_READ_ENTRY_POINTS
        message = (
            "references the Inbox History read's exported entry point `{target}`; the "
            "additive history read must never be reachable from an operational sink "
            "(AC-UI-INBOX-HISTORY-001.5)"
        )
    else:
        targets = SOURCE_HALF_TARGETS
        message = (
            "references `{target}`, forbidden for the Inbox History read/render path: "
            "History must never write the sidebar badge or Needs-you count state and must "
            "never subscribe to an event stream (AC-UI-INBOX-HISTORY-001.17/.22)"
        )

    findings: list[Finding] = []
    occurrences: dict[str, int] = {}
    for target in targets:
        for index in _find_all(source, target):
            occurrences[target] = occurrences.get(target, 0) + 1
            findings.append(
                Finding.create(
                    RULE_ID,
                    path,
                    _line_number(source, index),
                    {"path": path, "target": target, "occurrence": occurrences[target]},
                    f"{path} " + message.format(target=target),
                )
            )
    return findings


RULE = Rule(
    id=RULE_ID,
    slug="inbox_history_isolation",
    baseline_path=Path("config/architecture-lint/inbox_history_isolation.json"),
    applies_to=applies_to,
    scan=scan,
)
