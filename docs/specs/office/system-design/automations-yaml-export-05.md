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
# Automations YAML Export System Design Part 5

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

### Portable references

**AC-18** — When an automation references an agent profile, an executor profile, a
workflow and step, or repositories, the system shall emit the portable descriptors defined
in `## Data model` and shall emit no UUID for any of them.

**AC-19** — If a referenced agent profile, executor profile, workflow, workflow step, or
repository **is absent**, then the system shall omit that reference from the
automation, emit a warning naming the automation and the unresolved reference, and still
export the automation. Export shall not fail because a reference is dangling. Partial
resolution is resolved as follows, and each case emits its own warning:

- **Workflow resolves, step does not:** emit `workflow` with `name` only and no `step` key.
  The workflow reference is still true and still useful to a human recreating the automation.
- **Workflow does not resolve:** omit the whole `workflow` descriptor, including any step.
  A step name without its workflow names nothing.
- **Repositories, some members unresolved:** drop only the unresolved members and keep the
  resolved ones in their AC-8 order. If every member is unresolved the list becomes empty
  and is then omitted under AC-4.
- **Agent or executor profile unresolved:** omit that whole descriptor. Neither is
  meaningful partially populated.

**This AC applies only to a reference that is actually made.** A foreign-key column holding the
empty string references nothing, is not "absent", and shall produce **no** warning. Specifically:
when `workflow_id` is non-empty and `workflow_step_id` **is** empty, the system shall emit
`workflow` with `name` only and no `step` key, and shall add no warning — the same document shape
as the unresolved-step case above, but silent. When `agent_profile_id`, `executor_profile_id` or
`workflow_id` is empty, AC-4 omits that descriptor and no warning is emitted. An empty
`repository_ids` list is likewise AC-4's business, not this AC's.

*(Stated because `Automation.WorkflowStepID` is a plain `string` and
`internal/automation/models.go:79` documents the pair outright: "WorkflowID / WorkflowStepID are
optional: no automation run is placed…". A workflow without a step is therefore an ordinary,
service-supported configuration, not a damaged row. AC-4's key list is flat and names `workflow`
as a whole, so it cannot express the nested `step`, and this AC's own list covered only a step
that was referenced and could not be found — leaving the common case matching neither. A builder
resolving that silence toward "warn" would emit `unresolved workflow step` for every correctly
configured step-less automation, filling `warnings` with false positives in an artifact whose
entire value is a quiet, reviewable diff. The distinction is between "you asked for something that
is not there", which is worth a human's attention, and "you asked for nothing", which is not.)*

**"Absent" and "the lookup failed" are different outcomes and this AC covers only the first.**
Every descriptor lookup shall report three outcomes distinguishably — **resolved**, **absent**, and
**failed**. Absent takes the path above. **Failed does not**: an error from a descriptor lookup is
an infrastructure failure and returns `500` under AC-45, with no partial document and no warning.

*(Stated because the spec applies exactly this discipline to the workspace authorizer and the
workspace lookup — AC-44 steps 1 and 2, both classifying on `repoerrors.ErrWorkspaceNotFound` —
and applied nothing equivalent one level down, leaving "cannot be resolved" to cover both. Without
the distinction a database error while reading `agent_profiles` is silently reported as a dangling
reference: the export returns `200`, the committed artifact loses a reference that was never
actually dangling, and a later applier recreates the automation without it. The precedent a builder
would reach for makes this concrete rather than theoretical —
`buildAgentProfileResolver` (`backendapp/services.go`) is written
`if err != nil || profile == nil { return nil }`, collapsing both outcomes into one nil and calling
`context.Background()` besides. **It must not be reused for this export**, and the lookups defined
in AC-29 exist partly to replace it. A two-valued `(value, ok)` signature cannot express this AC and
is non-conforming; `(value, found bool, err error)`, or a `(value, error)` whose absence is a
distinguishable sentinel the caller matches with `errors.Is`, both can. Which of the two is the
builder's choice — what this AC fixes is that absence and failure must be separable by the caller
without inspecting an error string.)*

**AC-20** — The system shall carry warnings inside the exported artifact itself, not in a
transport-level sidecar: as a top-level `warnings` sequence in the single-document form,
and as `.kandev/automations/WARNINGS.txt` in the zip form. Where there are no warnings the
`warnings` key shall be omitted and the file shall be absent.

*(The body is `application/yaml`, so there is no envelope to put a sidecar in, and a
response header cannot carry multi-line text. Keeping warnings in the artifact also means
a dangling reference shows up in the committed diff, which is where the reviewer will
actually see it.)*

**AC-21** — The system shall order warnings by automation name ascending, tiebroken by
`automations.id` ascending, then by the warning text ascending, so that AC-9 holds when
warnings are present.

**AC-42** — Each warning shall be a single-line YAML **string scalar** — a string, never a
mapping — of the form `<automation name>: <message>`, containing no newline as `## Definitions`
defines it. The `warnings` value
is a sequence of those strings. `WARNINGS.txt` shall contain the same strings in the same AC-21
order, one per line, UTF-8, each line terminated by `\n` including the last.

**The emitted style is yaml.v3's choice and is not a plain scalar.** Every warning contains
`": "`, which makes a plain scalar impossible, so yaml.v3 single-quotes it. Probed:
`- 'Daily Sync: unresolved agent profile'`, and an entirely benign name is quoted identically
(`- 'plain name: unresolved workflow'`). A test for this AC shall assert on the **decoded string
value** and on the node being a scalar rather than a mapping; it shall **not** assert a plain
style, which no compliant export can produce for any warning this spec defines.

**Name and type rendering.** `automations.name` is unconstrained `TEXT`, and
`automation_triggers.type` is `TEXT NOT NULL` with no CHECK constraint
(`internal/automation/store.go`), so both are unconstrained in practice regardless of what the
service accepts. Before interpolation, **both** the automation name and the trigger type shall be
rendered by applying all four of the following, and nothing else:

1. U+000A becomes `\n` and U+000D becomes `\r`.
2. Every other character in U+0000–U+001F, U+007F, **and U+0085** becomes `\x` followed by
   exactly two lowercase hex digits.
3. **U+2028 and U+2029 become `\u` followed by exactly four lowercase hex digits** — that
   is, the six ASCII characters `\u2028` and `\u2029` respectively.
4. **Every byte that is not part of a well-formed UTF-8 sequence becomes `\x` followed by exactly
   two lowercase hex digits.** After this step the rendered value is valid UTF-8 by construction.

No other character is altered.

The four rules operate on disjoint inputs, so the order in which they are applied cannot change
the result: rules 1 and 2 touch only characters below U+0080 plus U+0085, rule 3 touches exactly
two characters, and rule 4 touches only bytes that are part of no well-formed sequence — which are
all at or above 0x80 and, U+0085 being well-formed `C2 85`, never the characters rule 2 names.

*(Rules 2 and 3 gained U+0085, U+2028 and U+2029 when `## Definitions` fixed **newline** as YAML's
five-character line-break set. Before that, this AC required each warning to contain "no newline"
while its rendering rules preserved three characters that are newlines under that definition, and
closed with "No other character is altered" — so the AC forbade the only edit that could satisfy
it. The failure was not theoretical in either output form. Probed: a name carrying U+2028 emits as
`- 'Daily<U+2028>  Sync: unresolved agent profile'` — the raw character survives into the file and
yaml.v3 folds the line around it — and the same raw bytes are written to `WARNINGS.txt`, where a
reader that honours the YAML and Unicode line-break set sees two lines where this AC promised one.
Escaping them keeps one warning on one line under every reading.*

*Rules 1 and 2 otherwise exist because a stored newline in either value makes "containing no
newline" and "of the form `<automation name>: <message>`" jointly unsatisfiable. Rule 4 exists
because both
columns are `TEXT` and SQLite does not validate encoding, so a row written by direct SQL can carry
an invalid byte — and without it that byte reaches the warning string, at which point yaml.v3
emits the **whole warning** as `!!binary` by exactly the mechanism AC-47 documents for prompts.
That would break this AC twice over: a base64 blob is neither a string scalar of the stated form
nor a line of the UTF-8 `WARNINGS.txt` promised above. Escaping the bytes keeps the warning
readable, keeps both output forms well-formed, and keeps the byte sequence recoverable by a human
reading the diff. Note the asymmetry with AC-47, which is deliberate: an invalid-UTF-8 **prompt**
is preserved exactly and allowed to become `!!binary`, because the prompt is the payload and
fidelity outranks readability; an invalid-UTF-8 **name inside a warning** is escaped, because the
warning is diagnostic text whose whole job is to stay readable.)*

**De-duplication is scoped to the entity the message describes**, per the `Scope` column of the
vocabulary table below. Two identical messages for the same **automation** are emitted once; two
automations that produce byte-identical warning strings each keep their own. The AC-39 messages
describe a **trigger**, so their scope is the trigger: two triggers on one automation that
produce the same string each keep their own line.

*(The scope of the dedup is the whole point, at both levels. Automation names are not unique —
neither `automations` nor `Service.CreateAutomation` enforces it — so two automations named "Daily
Sync" that both have an unresolved agent profile produce the same string, and a global dedup would
silently drop the second, contradicting AC-19's promise that each unresolved reference emits its
own warning. The identical argument applies one level down: the AC-39 messages name a trigger
**type**, never a trigger identity, and `automation_triggers` has no UNIQUE constraint on
`(automation_id, type)` — AC-8 tiebreaks triggers by `automation_triggers.id` precisely because two
triggers can share a type. An automation-scoped dedup would therefore collapse two malformed
same-type triggers into one line and report one problem where there are two, which is the exact
silent loss this feature exists to prevent. Per-trigger scope costs a repeated line and keeps the
count. AC-9 is unaffected: identical strings sort identically under AC-21, so the bytes stay
deterministic. The prompt messages are automation-scoped and AC-17 now emits at most one of them
per prompt, so for those the dedup is a no-op.)*

**Message vocabulary.** The message half is fixed text, because AC-21 sorts warnings by it and
AC-9 makes it part of the artifact's bytes; two reasonable wordings would otherwise produce two
different committed files. The messages are exactly:

| Condition | Scope | Message |
|---|---|---|
| Agent profile unresolved (AC-19) | automation | `unresolved agent profile` |
| Executor profile unresolved (AC-19) | automation | `unresolved executor profile` |
| Workflow unresolved (AC-19) | automation | `unresolved workflow` |
| Workflow resolved, step unresolved (AC-19) | automation | `unresolved workflow step` |
| Repository unresolved (AC-19) | automation | `unresolved repository at position <position>` |
| Trigger config not valid UTF-8 (AC-39) | trigger | `trigger <type>: config is not valid UTF-8` |
| Trigger config not valid JSON (AC-39) | trigger | `trigger <type>: config is not valid JSON` |
| Trigger config valid JSON but not an object (AC-39) | trigger | `trigger <type>: config is not a JSON object` |
| Prompt not emitted as a block scalar (AC-17) | automation | `prompt not emitted as a block scalar` |
| Prompt not valid UTF-8 (AC-47) | automation | `prompt not emitted as a block scalar: invalid UTF-8` |
| Prompt re-quoted to preserve bytes (AC-49) | automation | `prompt re-quoted to preserve bytes` |

*(The repository message names a `position` rather than an id because AC-18 keeps UUIDs out of the
artifact and the unresolved repository has no name to give; `automation_repositories.position` is
the stable handle that survives. The `Scope` column is what the dedup rule above keys on. The three
prompt-degradation rows this table previously carried — one each for trailing space, carriage
return and non-printable character — are replaced by the single row above: AC-17 no longer
classifies **why** a block scalar was lost, only **that** it was, so there is exactly one message
and it cannot drift out of step with the library. `invalid UTF-8` survives as its own row because
AC-47 detects it from the emitted tag rather than from any character list, so it carries no drift
risk and is worth telling a human apart from an ordinary degradation.)*

### Round-trip completeness

**AC-22** — The system shall hold a Go disposition table — a literal in the test package listing
every field of `Automation` and `AutomationTrigger` with its disposition — and a test that
enumerates both structs by reflection and asserts each field is accounted for by exactly one entry
in that table. Adding a field to either domain struct without adding it to the Go table shall fail
the test. The markdown table in `## Data model` › "Field disposition" is documentation **of** the
Go table and must be updated alongside it; the Go table is the test oracle.

*(This is the anti-`reports_to` guard. Office's config export declares `reports_to` in its DTO,
emits it, and drops it on import; nothing failed. A field that is neither exported nor consciously
excluded is the defect. The oracle is a Go literal rather than the markdown table parsed at
runtime because a test that reads a docs path is fragile in a way that gets it deleted; the cost
is that markdown drift is a documentation bug rather than a test failure, which is the right
trade for a guard whose job is preventing silent data loss. The match is against an explicit
disposition list, not against DTO field names, because five domain fields are deliberately renamed
on the way out and one concept is carried by two struct fields — a name-identity test cannot
express either, and would fail on first run for bookkeeping reasons, which is the surest way to
get the guard loosened.)*

*(Division of labour, stated because it is easy to assume this AC does more than it does: AC-22
proves every field is consciously **classified**. It does not prove the DTO actually marshals an
exported field to its key — that is AC-23 — and it cannot see a column that never becomes a
struct field — that is AC-43. All three are required.)*

**AC-43** — The system shall hold a test over a fixture in which every excluded column
(`webhook_secret`, `last_evaluated_at`, `last_triggered_at`, `created_at`, `updated_at`,
`workspace_id`, `automation_id`, `id`, `execution_mode`, `repository_id`, `legacy_board_card`)
holds a distinctive sentinel value where its type permits one, and shall assert **both** of the
following against both output forms:

1. **No sentinel value appears** anywhere in the serialized bytes.
2. **No key resolves to an excluded column**, checked structurally: parse the output, walk every
   mapping key at every depth *outside the `config` subtrees*, normalize each key by lowercasing
   it and removing `_` and `-`, and assert none equals the same normalization of any excluded
   column name. Every name in that list is written **bare**, with no table qualifier: the
   normalization strips only `_` and `-`, so a qualified `automations.repository_id` would
   normalize to `automations.repositoryid`, retain its `.`, and be unable to match any YAML key
   this export can produce — leaving the one column half 2 exists for with no coverage at all
   while the check appeared to pass.

*(Both halves are load-bearing and neither subsumes the other. Half 2 exists because the failure
this backstops is yaml.v3 **mangling key names**: marshalling the domain struct emits
`webhooksecret` and `lasttriggered`, so a fire-anchor leak would appear under `lastevaluatedat`
and a test grepping for the literal string `last_evaluated_at` would pass while leaking. Half 1
exists because a leak can also arrive under a key nobody predicted, where only the value gives it
away. The `config` subtrees are excluded from half 2 because AC-11 requires an unknown config key
to survive verbatim, and a user's config key legitimately named `webhook_secret` is preserved
data, not a leak; the sentinel check in half 1 still covers the values. Normalizing keys before
comparison is what makes half 2 robust to the mangling instead of blind to it, and matching on
normalized keys rather than raw substrings is what stops an ordinary prompt or description
containing the words "created at" from failing a compliant export.)*

*(`legacy_board_card` is on the list for half 2 only, and the "where its type permits one" clause
above is what exempts it from half 1. It is a Go `bool` derived from the withdrawn `execution_mode`
column, and a bool has no distinctive sentinel value — `true` appears legitimately throughout the
document as `enabled`. Half 2 still applies in full and is the half that matters here: AC-7 forbids
this key under any name, and marshalling the domain struct would emit it as `legacyboardcard`,
which normalizes to exactly what half 2 compares against. Omitting it from this list entirely, as
an earlier revision did, left the only structural guard in the spec blind to a column AC-7 names
explicitly — AC-22 proves it was consciously classified, which its own parenthetical is careful to
say is not the same thing.)*

*(AC-22 cannot cover all of these. `automations.repository_id` and `execution_mode` are **not
fields on the `Automation` struct at all** — `repository_id` is touched only by raw SQL in the
legacy backfill path, and `execution_mode` reaches Go solely as the derived `legacy_board_card`
alias. A reflection pass over struct fields structurally cannot enumerate a column that never
becomes a field, so the two exclusions this spec singles out by name would have had zero coverage
from the mechanism named as their enforcer, and any future SQL-only column would slip the same
way.)*

**AC-23** — The system shall hold a test that exports a fully-populated automation — every
optional field set, at least two triggers, at least two repositories — parses the exported
document back with `yaml.Unmarshal` into a generic `yaml.Node` tree, and asserts that for every
row in the AC-22 disposition table marked `exported`, the value reachable at its YAML key equals
the **expected value declared literally in the test**. Absence of a key whose source value was
non-empty shall fail the test.

The expected value is defined per row, because six rows are transforms and equality with the raw
source field is false by design for all six:

- **Untransformed rows** (`name`, `description`, `prompt`, `task_title_template`, `enabled`,
  `max_concurrent_runs`, and each trigger's `type` and `enabled`): the expected value is the
  source field itself.
- **The five reference rows** (`WorkflowID`, `WorkflowStepID`, `AgentProfileID`,
  `ExecutorProfileID`, `RepositoryIDs`): the expected value is the **descriptor the fixture's
  referenced rows imply**, written into the test as a literal — e.g. the agent profile fixture's
  `{agent_name, model, mode}` triple spelled out, not read back from the profile row and not
  produced by calling any exporter helper. AC-18 forbids emitting the UUID, so comparing against
  the raw foreign key would fail against a correct implementation.
- **`Config`**: compared as a tree, with every scalar compared on **both** its **lexeme** — the
  `yaml.Node.Value` string — **and its re-parsed `yaml.Node.Tag`**, against the corresponding
  number or string as it appears in the stored JSON. The expected tag is `!!str` for every JSON
  string and every mapping key; for a JSON number the assertion is that the tag is **not**
  `!!str`, which is the observable form of AC-41's "emitted as an unquoted YAML number". The
  fixture shall include at least one string value whose text would resolve to another YAML type
  if emitted bare — `"true"`, `"null"`, `"1.0"`, `"0755"`, `"12"` or `"~"`.
  This is why the parse target is `yaml.Node` and not `map[string]any`: a
  `map[string]any` decode discards the emitted characters (`1.0` reads back as `float64(1)`) and
  would silently pass an AC-41 violation.

  *(The tag half is not redundant with the lexeme half and the fixture requirement is not
  decoration — together they are what makes this test able to fail. A JSON string `"true"`
  emitted bare re-parses with `Value == "true"` and `Tag == "!!bool"`: the lexeme matches the
  stored string exactly, so a lexeme-only comparison passes on corrupted output. And a fixture
  built only from ordinary strings never produces a scalar whose bare form is ambiguous, so the
  tag assertion would pass vacuously. The defect this guards is a type change with no textual
  trace, which is precisely the class the lexeme comparison was chosen to catch for numbers and
  is structurally blind to for strings.)*

*(Recovery is pinned to a generic node parse rather than to a typed decode on purpose: a typed
decode would be a second implementation of the exporter's own field list and would inherit its
blind spots — a field dropped from both sides would still pass. For the same reason the expected
descriptors must be literals in the test rather than computed by exporter code; a shared
projection reproduces the exporter's omission and the guard passes for the wrong reason. The
applier is out of scope, so "recoverable" has no other reader to define it.)*

### File layout and naming

**AC-24** — When the zip export is requested, the system shall write one file per
automation at `.kandev/automations/<slug>.yml`, each containing the full envelope
(`version`, `type`, `automations`) with a single-element `automations` list.

**AC-25** — The system shall derive `<slug>` from the automation name by lowercasing,
replacing every character outside `[a-z0-9]` with `-`, collapsing consecutive `-`,
trimming leading and trailing `-`, and truncating to 64 characters followed by a further
trailing-`-` trim.

*(Verified by running this algorithm over all 7 live names: every one yields a distinct
slug, the longest being 53 characters, so the 64-character truncation is not exercised by
the current corpus. `Daily Review — @kegmil/offline-first` yields
`daily-review-kegmil-offline-first`; `Daily km-mobile-app-v2 repo drift --all` yields
`daily-km-mobile-app-v2-repo-drift-all`. Office's config export writes
`.kandev/agents/<Name>.yml` using the raw name with no sanitization at all; **3 of the 7**
live automation names contain `/`, which under that approach silently creates a nested
path instead of a file.)*

**AC-26** — If the derived slug is empty, then the system shall use `automation` as the
slug. *(Reachable and verified: the name `日次レビュー` collapses to the empty string under
AC-25 and must fall back.)*

**AC-27** — If two automations in one export derive the same slug, then the system shall
append `-` followed by the first 8 characters of each colliding automation's `id`, applied
to every member of the colliding group so no member keeps the bare slug. If any resulting
name still collides with another entry in the same export — whether with a suffixed or an
unsuffixed one — the system shall lengthen the suffix for the still-colliding entries, 8
characters at a time, up to the full `id`, until every entry path in the archive is unique.
*(Automation names are not unique: neither `automations` nor `Service.CreateAutomation`
enforces uniqueness — only non-emptiness. Two `id` values sharing their first 8 characters
is vanishingly unlikely in a real workspace, and `id` is unique, so widening to the full
value always terminates. This is specified for the same reason AC-26 specifies the
empty-slug fallback for a case the live corpus does not contain: AC-24 promises one file
per automation, and a silent overwrite would break that promise by losing an automation
entirely rather than by failing.)*

**AC-28** — When the zip export is requested, the system shall order zip entries by entry
path ascending, and shall leave each entry's modification time unset so no wall-clock value
enters the archive. *(Entry order alone does not make a zip reproducible — timestamps do
too. Probed: `zip.Writer.Create` leaves `FileHeader.Modified` zero and produces
byte-identical archives across runs for identical content, and is byte-identical to
`CreateHeader` with `Modified` unset. Office's export is therefore reproducible by accident
rather than by contract; AC-9 now binds the archive bytes, so this is pinned deliberately.)*
