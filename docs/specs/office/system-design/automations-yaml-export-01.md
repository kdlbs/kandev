---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001
created: 2026-08-19
updated: 2026-08-20
owners:
  - tbd
---
# Automations YAML Export System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

A workspace's automations are operational prose with no version control. Measured
against the live store on 2026-08-19: **7 automations, 78,589 bytes of prompt across
1,338 lines, spread over 3 workspaces**. The largest single automation prompt is 404
lines, longer than most workflow steps. All 7 carry a `webhook_secret`.

None of it is reviewable. There is no diff, no history, no rollback, no way to see what
a prompt said last week. Changing a 56-line automation prompt required dumping the row
out of SQLite to a temp file by hand purely to produce something a human could read as a
diff.

Workflows already solved this: `internal/workflowsync` keeps workflow definitions in a
git repository and applies them on a poll. Automations, which carry far more prose than
workflows do, got nothing.

This spec covers **export only**: rendering a workspace's automations as YAML a human
can commit, diff, review, and roll back by hand. The applier that reads that YAML back
is deliberately out of scope (see `## Out of scope`).

## What

A user can export a workspace's automations as YAML, in two forms:

- **A single YAML document** containing every automation in the workspace, for reading
  and for API consumers.
- **A zip of one file per automation**, laid out as `.kandev/automations/<slug>.yml`,
  which is the shape a human unpacks into a repository so each automation gets its own
  diff.

The exported YAML is *round-trip readable*: everything needed to recreate the automation
by hand is present. Secrets and runtime state are not.

Decisions taken (rationale in `## Decisions`):

- **`webhook_secret` is never exported**, and the export type is a purpose-built DTO
  rather than the `Automation` domain struct, because the existing `json:"-"` tag does
  not redact under a YAML marshaller.
- **Scheduler state is never exported.** `last_evaluated_at`, `created_at`,
  `last_triggered_at`, and `updated_at` are excluded. The first two are the cron
  scheduler's fire anchor.
- **Foreign keys are exported as portable descriptors, not UUIDs.** A UUID is not
  "enough to recreate the automation by hand".
- **Trigger config is carried as an order-normalized generic mapping**, not decoded
  into per-type structs, so a config key the exporter does not know about survives.
- **Output is byte-deterministic** for a given database state, with every ordering
  resolved by a named column. This binds the zip archive's bytes as well as the YAML.
- **Numbers in trigger config are copied character-for-character**, never converted through a Go
  numeric type, because every conversion route corrupts some class of input — rounding large
  integers, adding exponents to plain ones, or quoting all of them.
- **Prompt style and prompt fidelity are checked separately.** Whether a prompt is readable in the
  diff and whether its bytes survived are independent questions; a prompt beginning with a newline
  fails the second while passing the first. Where the marshaller's default choice would lose
  bytes, the export re-quotes that prompt into a form probed to preserve them, and says so.
- **Every JSON type in trigger config has one stated node form.** Numbers are untagged so their
  lexeme survives; strings are explicitly `!!str` so a value reading `true` stays a string.
- **Denial and absence are the same observable response** (`404`), so the endpoint cannot
  be used to enumerate which workspaces exist.

## Definitions

Two terms are load-bearing in more than one acceptance criterion. They are defined once,
here, and every AC that uses them means exactly this and nothing else.

**newline** — any character in the set `{U+000A, U+000D, U+0085, U+2028, U+2029}`. A string
"contains a newline" when at least one of its characters is in that set; it "contains no
newline" when none is. Used by AC-17, AC-46 and AC-42.

*(This is YAML 1.2's own line-break set, and it is exactly the `is_break` set in yaml.v3's
`yamlprivateh.go` that `## Decisions` already quotes. Two properties make it safe to name here
where the deleted degradation-condition list was not. It is **normative and fixed** — it comes
from the YAML specification rather than from an attempt to predict an emitter's behaviour, so it
cannot drift out of date the way a list of "causes of degradation" did across three revisions.
And it answers a different question: not "will this emit as a block scalar" (which stays an
observation, per AC-17) but "is this prompt multi-line at all", which is a property of the string
and of nothing else.*

*The narrower reading — U+000A alone — was rejected and the reason is measured. Probed, a
CR-only, NEL-only, LS-only or PS-only prompt all emit **non-literal**: a prompt a human wrote as
several lines collapses to one quoted line. Under the narrow reading those prompts satisfy AC-46,
which mandates **no** warning, so the single most useful diagnostic this feature offers would be
withheld from exactly the prompts that need it. Under this definition they reach AC-17 and warn.*

*Consequence for AC-42, made explicit rather than left to a reader: because the set is wider than
`{LF, CR}`, AC-42's rendering rules escape all five, and its "No other character is altered"
clause is stated against the completed four-rule list below rather than against a two-rule one.)*

**start** (of an export) — the moment the export's read transaction **establishes its snapshot**,
which is the moment it issues its first read inside that transaction, not the moment `BeginTxx`
returns. AC-29 requires the export to establish its snapshot immediately upon opening the
transaction, before any other work, so the two moments are adjacent and the term is unambiguous.
Used by AC-13 and AC-30.

*(SQLite's deferred transaction takes no snapshot at `BEGIN`; the first read does. Left undefined,
AC-13 was falsifiable by a correct implementation: export A opens at t0 and first-reads at t0.5,
export B opens at t1 and first-reads at t3, a write commits at t2. Nothing committed between the
two `BEGIN`s, so AC-13's antecedent held and demanded byte-identical output, while A and B held
different snapshots and were required by AC-30 to differ. Pinning "start" to snapshot
establishment, and requiring establishment to happen immediately, makes both ACs true together.)*
