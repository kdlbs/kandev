---
status: draft
created: 2026-08-19
updated: 2026-08-20
owner: tbd
---

# Automations YAML Export

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

## Decisions

### Export is additive and read-only

This card is scoped "no backend change". That is read as: no applier, no schema
migration, and no change to any existing backend behaviour. The export path itself is
new backend code, because:

- Phase 2 is specified as mirroring `internal/workflowsync`, a backend package. An
  exporter living outside the backend would give that mirror nothing to agree with.
- The artifact is a product capability delivered to a user, not a one-off script.

No existing behaviour is modified. The export reads; it never writes to `automations`,
`automation_triggers`, `automation_repositories`, or `automation_runs`.

"Additive" is about behaviour, not about file count, and it does **not** forbid new code in
existing packages. AC-29 requires a single read transaction and the automation store has no seam
for one — every read path issues directly against the reader handle and takes no transaction — so
the export needs **new** transaction-accepting read methods on `Store`. Adding them is in scope.

**Four stores are in scope for that addition, not one.** AC-29 covers the rows a portable
descriptor is resolved from as well as the automation's own rows, and those live in three other
packages, so new exported transaction-accepting read methods are authorized on the **automation
store, the agent-settings store, the workflow repository, and the repository store**. Each new
method is a read, takes the transaction as a parameter, and sits beside the existing signature
rather than replacing it.

What is out of scope, in all four, is changing what any existing method does to any existing
caller: the current signatures stay, the new methods sit beside them, and no firing path,
scheduler path, or WS handler changes behaviour.

*(An earlier revision scoped this sentence to `Store` alone — automation's own — while AC-29
simultaneously required descriptor rows to be read inside the same transaction. Since no existing
method on the other three stores accepts a transaction, the two statements could not both be
honoured, and the builder was left to choose between exceeding the stated scope and writing query
text into `backendapp`. Naming the four stores here is what makes AC-29's mechanism authorized
rather than improvised.)*

### `webhook_secret` needs an explicit exclusion, not an inherited one

`Automation.WebhookSecret` is tagged `json:"-" db:"webhook_secret"`. It carries **no**
`yaml` tag. `gopkg.in/yaml.v3` — the marshaller `workflowsync` already uses — does not
read `json` tags. Verified by marshalling the field layout through yaml.v3:

```text
id: abc
name: "n"
webhooksecret: SENTINEL-SECRET-VALUE
lasttriggered: null
```

Two failures in four lines. The secret is emitted under `webhooksecret`, and
`json:"...,omitempty"` is ignored too, so nil runtime timestamps serialize as explicit
`null` rather than being dropped. All 7 live automations carry a 32-character secret, so
the first is not a hypothetical.

The export type is a separate DTO with explicit `yaml` tags on every field. The domain
struct is never marshalled directly. AC-6 and AC-7 exist to hold this closed.

### Scheduler state is excluded because it is the fire anchor

`CronScheduler.shouldFire` computes the next fire time from an anchor that is
`trigger.LastEvaluatedAt`, falling back to `trigger.CreatedAt` when the trigger has never
been evaluated. Both fields are therefore live scheduler state, not definition. Four of
the seven live triggers have `last_evaluated_at` set; the other three are anchored on
`created_at`.

Excluding both from the export means a future applier physically cannot carry a stale
anchor into the database, which is the drift-and-double-fire failure mode called out for
Phase 2. Their exclusion also keeps the diff quiet: `last_evaluated_at` changes on every
tick, so an export that included it would show a diff every hour for an hourly
automation, and the signal would be lost.

### Foreign keys become portable descriptors

An automation references five things by UUID. Exporting the UUIDs would satisfy nothing:
a UUID is neither reviewable in a diff nor sufficient to recreate the automation by hand.

| Reference | Exported as | Rationale |
|---|---|---|
| `agent_profile_id` | `{agent_name, model, mode}` | Reuses the existing `wfmodels.AgentProfilePortable` triple, already the established portable identity for an agent profile (`backendapp/services.go`). Profile **names are not unique** — the live store has two profiles named `Opus - Low` — so name alone is not viable; the triple separates them (`opus[1m]` vs `default`). |
| `executor_profile_id` | `{executor, name}` | No portable type exists for executor profiles; this spec defines one. `executor_id` alone is insufficient because it is a stable slug for built-ins (`exec-local`, `exec-worktree`) but a UUID for custom executors — the live store has one such profile, `neo`. Both fields are emitted so a reader can match on either. |
| `workflow_id` / `workflow_step_id` | `{name, step}` | Workflows are already git-synced and already identified by name in `ApplySyncedWorkflows`. Name reference is consistent with the entity's own sync identity. |
| `repository_ids` | ordered list of repository names | Repository name is the human handle shown in the UI. |

Where a referenced row cannot be resolved, see AC-19: the export does not fail.

### Trigger config is a generic mapping, not a typed struct

`automation_triggers.config` is stored as raw JSON text and is **not** canonicalized on
write. The live store holds two different serializations of the same shape:

```text
{"cron_expression":"0 9 * * *","timezone":"Asia/Singapore"}
{"cron_expression": "@daily", "timezone": "Asia/Singapore"}
```

Emitting the stored text verbatim would make byte-identical automations diff against each
other. Decoding into the typed per-trigger structs (`ScheduledTriggerConfig` and friends)
would normalize the formatting but would **silently drop any key the struct does not
declare**.

That second failure is precisely the defect shipped by Office's config export/import,
where `reports_to` is present in the DTO and emitted by export but dropped on import. The
lesson is taken rather than repeated: config is decoded to a generic mapping and
re-emitted with keys sorted, which normalizes formatting *and* preserves unknown keys.

A consequence worth stating: a new trigger type needs no exporter change.

### Numbers are copied, not converted

Decoding trigger config into a generic mapping puts every JSON number through Go's `any`, and every
route that *converts* the number corrupts some class of input. Probed against
`{"big":9007199254740993,"huge":9223372036854775808,"round":1000000000,"exp":1e9,"onepoint":1.0}`:

```text
json.Unmarshal -> map[string]any    big  -> 9.007199254740992e+15   VALUE CHANGED
                                    round-> 1e+09                   reformatted

UseNumber + Int64()/Float64()       exp  -> 1e+09    an integer inside int64, now exponential
                                    huge -> 9.223372036854776e+18   VALUE CHANGED
                                    onepoint -> 1                   a float, now an integer

UseNumber, no conversion            big  -> "9007199254740993"      NOW A STRING
                                    round-> "1000000000"
```

Plain decoding makes every number a `float64`, which rounds `9007199254740993` and reformats
`1000000000` as `1e+09`. Emitting the `json.Number` directly fixes the precision loss and
introduces a worse bug: `json.Number` is declared `type Number string`, so yaml.v3 emits it
**quoted** and `max_retries: 3` comes back as `max_retries: "3"`. The `Int64()`-then-`Float64()`
route looks like the fix and is not: it branches on *lexical* form — `strconv.ParseInt` rejects
`1e9` and `1.0` outright, and cannot hold `9223372036854775808` — so three separate input classes
each come out wrong, two of them silently.

The mechanism that survives all of them is not to convert at all. Decode with `UseNumber` to keep
the original characters, then hand yaml.v3 a `*yaml.Node` scalar carrying that exact lexeme.

**Leave `Tag` unset.** An earlier revision of this spec assigned `!!float` when the lexeme
contained `.`, `e` or `E` and `!!int` otherwise, on the theory that each lexeme resolves to its tag
implicitly so nothing would be printed. That is true only inside `[-2^63, 2^64)`. yaml.v3's encoder
drops an assigned tag only when its own `resolve()` of the raw lexeme returns the same tag; for a
20-digit integer `resolve()` falls through `ParseInt` and `ParseUint` to `!!float`, and for `1e400`
`ParseFloat` returns `ErrRange` and it falls through to `!!str`. Either way the assigned tag now
disagrees and is printed. Probed side by side:

```text
lexeme                        Tag unset                     Tag assigned !!int/!!float
1000000000                    1000000000                    1000000000
1e9                           1e9                           1e9
1.0                           1.0                           1.0
9007199254740993              9007199254740993              9007199254740993
9223372036854775808           9223372036854775808           9223372036854775808
18446744073709551616          18446744073709551616          !!int 18446744073709551616
99999999999999999999999999    99999999999999999999999999    !!int 99999999999999999999999999
1e400                         1e400                         !!float 1e400
-12345678901234567890         -12345678901234567890         !!int -12345678901234567890
```

Unset wins on every row and loses on none. It is also safe: a JSON number lexeme can never collide
with YAML's keyword set (`true`, `false`, `null`, `yes`, `on`, `~`) because the JSON grammar
excludes them, and `encoding/json` rejects `NaN`, `Infinity`, a leading `+`, leading zeros and
`0x` forms during scanning, so those never reach the exporter as numbers at all — a config
containing them is not valid JSON and routes to AC-39 instead. Deleting the tag assignment is
therefore strictly simpler than the rule it replaces and removes the last class of altered output.

This is the same principle the spec already applies to prompts (AC-15) and to unknown config keys
(AC-11): the export reports what is in the database rather than normalizing it, and AC-34 already
states that outright. A number is not a special case; it only looked like one because the obvious
implementations reach for a Go numeric type on the way past.

### Ordering is explicit because the store's ordering is not stable

`ListAutomations` orders by `created_at DESC` and trigger hydration orders by
`created_at`, neither with a tiebreak. Three of the seven live automations were created
within **5 milliseconds** of each other by a bulk path, so equal timestamps are a
realistic collision, and SQLite's row order for a tie is not defined.

A git-committed artifact whose row order can shuffle between runs produces phantom diffs.
Every ordering in the export is therefore pinned to a named column with a named tiebreak
(AC-8).

### Prompts are emitted faithfully, and degradation is observed rather than predicted

The whole value of this feature is a readable diff of a long prose prompt. yaml.v3 emits a
multi-line string as a literal block scalar (`|`) only when the string qualifies. When it does
not, the entire value collapses to one double-quoted line — applied to the 404-line prompt in the
live store that is a 24 KB single line: syntactically valid, completely unreviewable, and it
happens silently.

**The export therefore observes the emitted form rather than predicting it.** That is a
correction to three earlier revisions of this spec, and the reason is measured rather than
stylistic. Each revision tried to enumerate the conditions under which yaml.v3 declines a block
scalar, and each enumeration was found incomplete by the next review:

| Revision | Enumerated | Missed |
|---|---|---|
| First | trailing-whitespace line, CR, invalid UTF-8 | every astral character, every C0/C1 control, DEL, BOM |
| Second | + C0 controls, DEL, astral characters | the C1 controls U+0080–U+009F, U+FEFF, U+FFFE, U+FFFF |
| Third | + C1 controls, BOM, U+FFFE, U+FFFF, "a U+0020 followed by a line break" | U+2028 and U+2029 |

The third miss settles the argument. `emitterc.go` › `yaml_emitter_analyze_scalar` computes
`block_allowed = !(trailing_space || space_break || special_characters)`, where `space_break` is a
U+0020 followed by any character in `is_break` — `{U+000A, U+000D, U+0085, U+2028, U+2029}`
(`yamlprivateh.go`) — while `is_printable` **admits** U+2028 and U+2029, so `special_characters`
does not catch them either. A prompt containing a space immediately followed by U+2028 thus
matched no enumerated condition and still lost its block scalar. Probed against v3.0.1: it emits
double-quoted on one line.

Each round the list was extended and each round a new gap appeared, because the spec was
maintaining by hand a predicate the library computes from three interacting expressions over five
character classes. The structural fix is to stop maintaining it.

**Mechanism.** The emitted style is directly observable: yaml.v3 records `Style` on the node it
parses, so re-parsing emitted output into a `yaml.Node` and testing `Style & yaml.LiteralStyle`
answers the question exactly, with no predicate to keep in sync. Probed against v3.0.1:

```text
prompt value                        Style          Tag        literal?  bytes preserved?
"line one\nline two\n"              Literal        !!str      yes       yes  (emits |)
"line one\nline two"                Literal        !!str      yes       yes  (emits |-)
"line one\nline two\n\n"            Literal        !!str      yes       yes  (emits |+)
"line one\ta\nline two\n"           Literal        !!str      yes       yes  (TAB does not degrade)
"line one   \nline two\n"           DoubleQuoted   !!str      no        yes  (space before a break)
"line one\nline two "               DoubleQuoted   !!str      no        yes  (trailing space)
"line one\r\nline two\r\n"          DoubleQuoted   !!str      no        yes  (CR)
"line one\na<U+1F389>b\n"           DoubleQuoted   !!str      no        yes  (astral)
"line one\na<U+001B>b\n"            DoubleQuoted   !!str      no        yes  (C0 control)
"line one\na<U+2028>b\n"            DoubleQuoted   !!str      no        yes  (the miss above)
"Do the thing"                      Plain          !!str      no        yes  (single line; not a degradation)
"Do the thing "                     SingleQuoted   !!str      no        yes  (single line; not a degradation)
"line one\nbad\xff\xfebyte\n"       —              !!binary   no        yes  (base64; AC-47)
"\nhello"                           Literal        !!str      yes       NO   -> "hello"        (AC-49)
"\n"                                Literal        !!str      yes       NO   -> ""             (AC-49)
"\n\nhello"                         Literal        !!str      yes       NO   -> "\nhello"      (AC-49)
"\n\n"                              Literal        !!str      yes       NO   -> "\n"           (AC-49)
```

**The last four rows are the reason this spec has an AC-49.** A prompt that *begins* with a
newline loses that newline, and the emitted scalar is a perfectly ordinary literal block scalar,
so a style check sees nothing wrong. Style answers "is this readable"; it does not answer "are
these the same bytes". The two questions are independent and the export must ask both.

**Order of operations, because the naive reading is circular.** The warning lives inside the
document (AC-20), but the style is only knowable once something has been emitted. The export
therefore determines each prompt's disposition **before** building the final document: for each
automation it marshals that prompt alone through an encoder configured identically to the real one
(AC-12's 2-space indent, block context), re-parses that throwaway output, and reads **three**
things from the resulting node — its `Tag`, its `Style`, and its `Value`. The final document is
then built once, warnings included, and marshalled once. It shall **not** be marshalled,
inspected, amended and re-marshalled — a two-pass emit makes AC-9's byte-determinism depend on
the two passes agreeing, and gives AC-48 a body that existed in an earlier form.

**Classification is ordered, and the order is normative.** Exactly one branch is taken per
prompt, so exactly one prompt warning is possible per automation:

1. **`Tag` is `!!binary`** → AC-47. Do not override the style; do not apply the fidelity test in
   step 2, whose comparison is meaningless here (the re-parsed node's `Value` is the base64 text,
   not the prompt, so a naive comparison reports a loss that has not occurred). Fidelity for this
   branch is established by base64-decoding the emitted value, which is probed byte-identical.
2. **`Value` differs from the stored prompt** → AC-49. Emit this prompt in the final document as
   a `*yaml.Node` carrying `Tag: "!!str"` and `Style: yaml.DoubleQuotedStyle`, and warn.
3. **`Style` is not literal and the prompt contains a newline** (as `## Definitions` defines it)
   → AC-17. Warn; do not override the style.
4. **Otherwise** → no prompt warning.

Steps 1 and 2 are ordered relative to each other by measurement, not by preference: forcing
`Tag: "!!str"` and a double-quoted style onto an invalid-UTF-8 prompt produces output that
**fails to re-parse at all**, so applying step 2 before step 1 would turn a handled case into a
broken document.

The standalone probe is faithful because `yaml_emitter_analyze_scalar` derives `block_allowed`
from the scalar's own bytes, and `encode.go` selects the literal style for any string containing a
newline **except in flow context**. This export emits block context throughout and never sets flow
style, so the probe and the real emission cannot disagree. A conforming implementation may instead
inspect the final document and assert agreement, but shall not emit a body twice.

**The mechanism for step 2's override, and why it disturbs nothing else.** The DTO's `prompt`
field is declared `any`. In branches 1, 3 and 4 it holds the prompt as a plain Go `string`, which
is what the export does today; only in branch 2 does it hold a `*yaml.Node`. This matters because
a field whose type changed would put every prompt on a new code path: probed across twelve prompt
classes — leading newline, bare newline, ordinary multi-line, trailing space, CR-only, NEL-only,
LS-only, C0 control, single line, and a single-line prompt reading `true` — an `any`-typed field
holding a `string` emits **byte-identical output to a `string`-typed field in all twelve**. The
override is therefore inert everywhere except the branch that needs it.

`Tag: "!!str"` is required on that node and is not decoration. A `*yaml.Node` scalar with `Tag`
unset is re-resolved by the encoder against its own value, which is exactly the mechanism AC-41
exploits for numbers and exactly the mechanism that would retype a prompt whose entire text is
`true` or `null`. AC-41's "leave `Tag` unset" rule is scoped to JSON numbers in trigger config
and applies nowhere else; see AC-8 for the same distinction on config strings.

Probed on all twelve classes: the forced double-quoted form round-trips **byte-identical in every
one**, including the four leading-newline cases the default emission loses. The forced form is
less readable than a block scalar, which is why AC-49 warns; it is not less faithful, which is
why AC-49 is a fix rather than only a diagnostic.

Three things fall out of that table and each is load-bearing. All three chomping indicators —
`|`, `|-`, `|+` — are `LiteralStyle`, so the test is a style check and never a prefix match on
the emitted text. `!!binary` is identifiable by its **tag**, which is what lets AC-47 take
precedence without re-testing the bytes. And a single-line prompt is `Plain` or `SingleQuoted`
rather than `Literal`, so "not a literal block scalar" is not by itself a degradation — a block
scalar is unreachable for a string with no newline, which is why AC-17's antecedent still
requires one.

**Byte fidelity survives every *degraded* case, but degradation is not the only way to lose
bytes.** Probed: every non-literal value above, plus the BOM, C1-control and U+10FFFD cases,
decodes back byte-identical — for those, degradation costs readability and never data.

**An earlier revision of this spec generalized that into "AC-15 is never in tension with AC-17",
and that claim was false.** It was falsified by measurement, not by argument: a prompt beginning
with a newline loses that newline, and does so while emitting as a literal block scalar, which is
precisely the case AC-17 is defined not to warn about. The 13-case table this claim was drawn
from contained no prompt that *starts* with a newline, so the generalization was made over a
sample that excluded the counterexample.

The correction is structural rather than a patch to the sentence. **Style and fidelity are two
independent observables and the export checks both** — style answers "will a human be able to
read this diff" (AC-16, AC-17), fidelity answers "are these the same bytes" (AC-15, AC-49). A
value can fail either without failing the other. Where the default emission would lose bytes,
AC-49 requires the export to emit that prompt in a form probed to preserve them, so **AC-15 holds
as an outcome and not merely as a promise about the exporter's own restraint**.

**What causes degradation, non-normatively.** No acceptance criterion depends on the following
list and no test may be written against it; it exists so a human reading a warning has somewhere
to start looking. As of v3.0.1 the usual causes are a trailing space, a space before a line break,
a carriage return (a CRLF prompt), a control character, an emoji or other astral character, and
the byte-order mark. **This list is illustrative and is permitted to be incomplete** — that is
precisely the point of observing the emitted style instead of predicting it. If it drifts out of
date, nothing breaks.

Two options are rejected:

- **Normalizing on export** (stripping trailing whitespace, folding CRLF, dropping astral
  characters) would make the export an inexact copy. A later applier would then rewrite the stored
  prompt on the first sync. An export that quietly changes the thing it exports is the same class
  of defect as Office dropping `reports_to` — different direction, same betrayal.
- **Emitting as-is and saying nothing** leaves the user with an unreadable diff and no
  explanation.

The export therefore emits the prompt byte-faithfully and adds exactly one warning when the
emitted form is not a literal block scalar (AC-17). Fidelity is preserved; the degradation is
visible rather than mysterious.

Measured against the live corpus: **0 of 7 prompts** degrade — every one serializes as a block
scalar today. All 7 lack a final newline and so use `|-`, which is equally readable and is still
`LiteralStyle`. The corpus is clean; an emoji pasted into a prompt is a likelier way to break it
than a trailing space is.

### The encoder is pinned

yaml.v3's package-level `Marshal` defaults to a 4-space indent; `yaml.Encoder.SetIndent`
changes it. Office's `writeYAMLFile` uses the default and never pins it. Byte-determinism
(AC-9) requires the indent to be part of the contract rather than a library default that
a dependency bump can move, so the export pins 2-space indentation explicitly.

## Data model

The exported document. Field names are the YAML keys.

```yaml
version: 1
type: kandev_automations
automations:
  - name: Daily Review — @kegmil/offline-first
    enabled: false
    max_concurrent_runs: 1
    continuation_policy: reuse_thread
    task_title_template: Daily Review — offline-first ({{trigger.timestamp}})
    prompt: |
      ...
    agent_profile:
      agent_name: Claude Code
      model: opus[1m]
      mode: auto
    executor_profile:
      executor: exec-worktree
      name: Worktree
    workflow:
      name: Kanban
      step: In Progress
    repositories:
      - kegmil-offline-first
    triggers:
      - type: scheduled
        enabled: true
        config:
          cron_expression: 0 9 * * *
          timezone: Asia/Singapore
```

**Included, always emitted:** `name`, `enabled`, `max_concurrent_runs`,
`continuation_policy`, `triggers`
(possibly empty), and per trigger `type`, `enabled`, `config` (possibly empty).

**Included, omitted when empty:** `description`, `prompt`, `task_title_template`,
`agent_profile`, `executor_profile`, `workflow`, `repositories`, and the document-level
`warnings` (AC-20).

**Excluded — secret:** `webhook_secret`.

**Excluded — runtime state:** `continuation_task_id`, `last_triggered_at`, `created_at`, `updated_at` (automation);
`last_evaluated_at`, `created_at`, `updated_at` (trigger); everything in `automation_runs`.

**Excluded — instance identity:** `id`, `workspace_id`, `automation_id`, `trigger.id`.

**Excluded — withdrawn or legacy columns:** `execution_mode` (withdrawn; live values are
`''` on six rows and `'run'` on one, and no firing path reads it), `automations.repository_id`
(inert legacy column superseded by `automation_repositories`), and the derived
`legacy_board_card` field.

### Field disposition

This table is the contract AC-22 tests against. Every field of `Automation` and
`AutomationTrigger` appears exactly once. `exported` names the YAML key it becomes;
`excluded` names why.

| Struct | Field | Disposition |
|---|---|---|
| `Automation` | `Name` | exported → `name` |
| `Automation` | `Description` | exported → `description` |
| `Automation` | `Prompt` | exported → `prompt` |
| `Automation` | `TaskTitleTemplate` | exported → `task_title_template` |
| `Automation` | `Enabled` | exported → `enabled` |
| `Automation` | `MaxConcurrentRuns` | exported → `max_concurrent_runs` |
| `Automation` | `ContinuationPolicy` | exported → `continuation_policy` |
| `Automation` | `Triggers` | exported → `triggers` |
| `Automation` | `WorkflowID` | exported → `workflow.name` (renamed; resolved to a descriptor) |
| `Automation` | `WorkflowStepID` | exported → `workflow.step` (renamed; resolved to a descriptor) |
| `Automation` | `AgentProfileID` | exported → `agent_profile` (renamed; resolved to a descriptor) |
| `Automation` | `ExecutorProfileID` | exported → `executor_profile` (renamed; resolved to a descriptor) |
| `Automation` | `RepositoryIDs` | exported → `repositories` (renamed; resolved to names) |
| `Automation` | `WebhookSecret` | excluded — secret |
| `Automation` | `ID` | excluded — instance identity |
| `Automation` | `WorkspaceID` | excluded — instance identity |
| `Automation` | `ContinuationTaskID` | excluded — instance runtime state |
| `Automation` | `LastTriggeredAt` | excluded — runtime state |
| `Automation` | `CreatedAt` | excluded — runtime state / fire anchor |
| `Automation` | `UpdatedAt` | excluded — runtime state |
| `Automation` | `LegacyBoardCard` | excluded — derived from a withdrawn column |
| `AutomationTrigger` | `Type` | exported → `type` |
| `AutomationTrigger` | `Enabled` | exported → `enabled` |
| `AutomationTrigger` | `Config` | exported → `config` (the decoded form; see AC-11, AC-39, AC-41) |
| `AutomationTrigger` | `ConfigJSON` | excluded — raw storage of `Config`, same value, not a second concept |
| `AutomationTrigger` | `ID` | excluded — instance identity |
| `AutomationTrigger` | `AutomationID` | excluded — instance identity |
| `AutomationTrigger` | `LastEvaluatedAt` | excluded — runtime state / fire anchor |
| `AutomationTrigger` | `CreatedAt` | excluded — runtime state / fire anchor |
| `AutomationTrigger` | `UpdatedAt` | excluded — runtime state |

Two notes a builder needs. `AutomationTrigger` carries **two** fields for one concept —
`Config json.RawMessage` (tagged `db:"-"`) and `ConfigJSON string` (tagged `db:"config"`) —
and the DTO has a single `config`; the table settles which is which rather than leaving it
to a tag convention. And the columns excluded above that are **not** struct fields
(`execution_mode`, `automations.repository_id`) are deliberately absent from this table,
because reflection cannot see them; they are covered by AC-43 instead.

## API surface

Two read-only endpoints, mirroring the shape Office's config export already established
(`GET /workspaces/:wsId/config/export` and `.../export/zip`):

| Method | Path | Response |
|---|---|---|
| `GET` | `/api/v1/workspaces/:wsId/automations/export` | `200`, `Content-Type: application/yaml`, body is the single YAML document |
| `GET` | `/api/v1/workspaces/:wsId/automations/export/zip` | `200`, `Content-Type: application/zip`, `Content-Disposition: attachment; filename=kandev-automations.zip` |

REST rather than a WebSocket action: every other automation operation is a WS action because
it is interactive state, but an export is a bulk byte stream a client retrieves at a URL — a plain
HTTP `GET` a `fetch` can read a status and a `Blob` from, and that a human or a script can also
curl directly. `Content-Disposition` is set for that direct-access case; it is **not** the
mechanism the in-app control uses, which is `fetch` + `Blob` per AC-37. Office set the REST
precedent for the same job, though not the frontend one.

## Acceptance criteria

### Export content

**AC-1** — When a user requests the export for a workspace, the system shall return a
YAML document whose `version` is `1` and whose `type` is `kandev_automations`.

**AC-2** — When a user requests the export for a workspace, the system shall include
exactly the automations belonging to that workspace, and no automation belonging to any
other workspace.

**AC-3** — When an automation is exported, the system shall emit `name`, `enabled`,
`max_concurrent_runs`, `continuation_policy`, and `triggers` unconditionally, including when
`triggers` is empty.

**AC-4** — When an automation is exported, the system shall omit `description`, `prompt`,
`task_title_template`, `agent_profile`, `executor_profile`, `workflow`, and `repositories`
when the underlying value is empty, and emit them otherwise.

**AC-5** — When a trigger is exported, the system shall emit its `type`, its `enabled`
flag, and its `config`, and shall emit `config` as a mapping even when the stored config
is `{}`.

**AC-39** — If a trigger's stored `config` **is not valid UTF-8**, is not valid JSON, or is valid
JSON that is not an object (`null`, an array, a string, a number, or a boolean), then the system
shall emit `config` as an empty mapping, add a warning naming the automation, the trigger type, and
which of the three conditions occurred, and still export the trigger and its automation.
Export shall not fail.

The UTF-8 check shall be made **on the raw stored bytes, before decoding**, and takes precedence
over the other two conditions, so exactly one warning is emitted for such a trigger.

*(`automation_triggers.config` is raw TEXT; nothing enforces that it is UTF-8 or that it decodes to
an object. Probed: `null`, `[1,2]`, `"str"`, `42` and `true` all decode without error and none is a
`map[string]any`, so a type check is required, not just an error check.*

*The UTF-8 condition is separate and is the subtle one, because the failure is silent rather than
loud: `encoding/json` does **not** reject invalid UTF-8 in a string value — it substitutes U+FFFD
and returns no error. Without this clause a config string containing an invalid byte decodes
"successfully", passes the JSON check and the object check, and is emitted with its bytes changed,
while the export reports nothing. That is a direct contradiction of AC-34, which states the export
reports what is in the database rather than normalizing it, and it is the same class of silent
alteration the spec already refuses for prompts (AC-47) and for numbers (AC-41, character-for-
character). Checking the raw bytes first is what keeps the three conditions to one warning apiece
and keeps this one detectable at all — after decoding, the evidence is gone. Emitting an empty
mapping rather than the corrupted values is deliberate: a partially-mangled config in a committed
artifact is worse than an absent one, because a human reviewing the diff cannot tell which values
are real.)*

### Redaction and exclusion

**AC-6** — The exported YAML shall not contain the automation's `webhook_secret` under
any key, in either output form. *(Test with a non-empty secret containing a
recognizable sentinel and assert the sentinel is absent from the serialized bytes, not
merely that a named field is unset — the leak this guards against is a mis-keyed field,
not an unset one.)*

**AC-7** — The exported YAML shall not contain `id`, `workspace_id`, `automation_id`,
`created_at`, `updated_at`, `last_triggered_at`, `last_evaluated_at`, `execution_mode`,
`legacy_board_card`, or `automations.repository_id`, for any automation or trigger.

### Determinism

**AC-8** — The system shall order the export deterministically:

- automations by `name` ascending, tiebroken by `automations.id` ascending;
- triggers within an automation by `automation_triggers.type` ascending, tiebroken by
  `automation_triggers.id` ascending;
- repositories by `automation_repositories.position` ascending, tiebroken by
  `automation_repositories.repository_id` ascending;
- keys within a trigger `config` mapping by key ascending, at every nesting depth.

Sort order is byte-wise ascending on the UTF-8 encoding of the key.

**The `config` mapping shall be emitted as an explicitly ordered `*yaml.Node` of kind
`MappingNode` whose key/value pairs the exporter has already sorted byte-wise, at every nesting
depth.** It shall **not** be emitted by handing a Go `map[string]any` to yaml.v3 and relying on the
marshaller to sort. Sequence order is the stored JSON array order and is never sorted.

**Every JSON string in `config` — every key, and every value whose JSON type is string — shall be
emitted as a `*yaml.Node` scalar carrying an explicit `Tag: "!!str"`, at every nesting depth.**
This is the exact opposite of AC-41's rule for numbers, and the two are stated together here so
neither is applied to the other's type.

**Every JSON type has exactly one node form, and the table below is the whole rule.** Once AC-8
requires an explicit `MappingNode`, every descendant is builder-constructed, so leaving any type
unstated forces a guess:

| JSON type | Go type after `UseNumber` decode | Node form |
|---|---|---|
| object | `map[string]any` | `MappingNode`, keys sorted byte-wise per this AC |
| array | `[]any` | `SequenceNode`, stored order preserved, never sorted |
| string (and every key) | `string` | `ScalarNode`, `Tag: "!!str"` |
| number | `json.Number` | `ScalarNode`, **`Tag` unset**, `Value` the stored lexeme (AC-41) |
| `true` / `false` | `bool` | `ScalarNode`, `Tag: "!!bool"`, `Value` `true` or `false` |
| `null` | `nil` | `ScalarNode`, `Tag: "!!null"`, `Value` `null` |

*(The Go column is measured, not assumed: probed against
`{"s":"true","n":12,"b":true,"z":null,"arr":[...],"o":{...}}` decoded with `UseNumber`, the six
types land as `string`, `json.Number`, `bool`, `nil`, `[]any` and `map[string]any` respectively.
The `!!bool` and `!!null` rows are the only two where the explicit tag changes nothing — probed,
a bool or null node emits and re-parses identically with the tag set or unset, because their
lexemes resolve to themselves. They are stated anyway so the table is total: a builder who finds
four of six types specified reasonably infers the other two are someone else's problem, and the
one type where that inference is catastrophic is `string`.)*

*(Pinned because the natural implementation corrupts data silently and the spec's own round-trip
test does not catch it. A `*yaml.Node` container's children must themselves be `*yaml.Node`s, so
once AC-8 requires an explicit `MappingNode` **every** scalar is builder-constructed, strings
included — and AC-41 is otherwise the only node-construction rule this spec states. Applied
uniformly, its `Tag`-unset instruction hands yaml.v3 a bare lexeme to re-resolve. Probed:*

```
stored JSON string   emitted     re-parsed as     with Tag: "!!str"
"true"            -> true     -> !!bool          -> "true"   (!!str)
"null"            -> null     -> !!null          -> "null"   (!!str)
"1.0"             -> 1.0      -> !!float         -> "1.0"    (!!str)
"0755"            -> 0755     -> !!int           -> "0755"   (!!str)
"12"              -> 12       -> !!int           -> "12"     (!!str)
"~"               -> ~        -> !!null          -> "~"      (!!str)
"yes" / "on" / "no" / "ordinary"     already !!str, and stay unquoted with the tag set
```

*So a trigger config of `{"mode":"true"}` exports as `mode: true` and a later applier reads a
boolean. `Tag: "!!str"` fixes every row and over-quotes none: yaml.v3 quotes only the values whose
bare form would resolve to another type, and YAML 1.2's core schema does not treat `yes`/`on`/`no`
as booleans, so those stay bare. Numbers keep AC-41's `Tag`-unset treatment for the separate
reason given there — an assigned numeric tag gets printed whenever the encoder's own resolution
disagrees. The rule is therefore per-JSON-type, never uniform.*

*AC-23 is the test that ought to catch this and, as originally written, could not: it compared
config scalars by lexeme, and the re-parsed `true` node's lexeme is still `true`. That gap is
closed in AC-23 itself.)*

*(Pinned because the natural implementation silently violates the clause above it. yaml.v3 sorts
map keys with its own comparator (`sorter.go`), which is digit-aware and letter-aware rather than
byte-wise. Probed: yaml emits `v1, v2, v10` where byte-wise is `v1, v10, v2`; `_beta, Alpha` where
byte-wise is `Alpha, _beta`; `step1, step2, step10` where byte-wise is `step1, step10, step2`.
AC-11 guarantees an unknown key survives verbatim, so digit-bearing and punctuation-leading keys
are squarely in scope, and a test fixture using only lowercase-letter keys passes against a
non-compliant export. The same node-based container is what AC-41's scalars are placed into, so
this costs nothing extra.)*

**AC-40** — The system shall emit the document's top-level keys in the fixed order
`version`, `type`, `automations`, `warnings`, and each automation's keys in the fixed order
`name`, `description`, `enabled`, `max_concurrent_runs`, `continuation_policy`,
`task_title_template`, `prompt`, `agent_profile`, `executor_profile`, `workflow`, `repositories`,
`triggers`, and each
trigger's keys in the fixed order `type`, `enabled`, `config`. Keys omitted under AC-4 are
skipped without disturbing the order of the rest.

*(Every other ordering in this spec is pinned to a named column, but key order within a
mapping was not, and AC-9 makes it part of the artifact's bytes. Declaring it here means the
determinism contract does not rest on the marshaller emitting DTO struct fields in
declaration order — true of yaml.v3 today, but an undeclared dependency on a library
detail is exactly what AC-12 already refuses to accept for indentation.)*

**AC-9** — When the same workspace is exported twice with no intervening database change,
the system shall produce byte-identical output both times. This binds **both** output
forms: the single YAML document, and the complete bytes of the zip archive.

**AC-10** — When two automations differ only in the whitespace of their stored trigger
`config` JSON, the system shall produce identical `config` YAML for both. *(The live store
contains both a compact and a space-separated serialization of the scheduled trigger
config.)*

**AC-11** — When a trigger's stored `config` contains a key the exporter has no typed
field for, the system shall emit that key and its value in the exported `config`.

**AC-41** — When a trigger's stored `config` contains a JSON number at any nesting depth,
the system shall emit it as an unquoted YAML number whose **characters are exactly the
characters of the number as stored in the JSON**, with no explicit tag. It shall not reformat,
round, re-base, add or remove an exponent, add or remove a trailing `.0`, or emit the number as a
quoted string. A stored `1000000000` emits as `1000000000`; a stored `1e9` emits as `1e9`; a
stored `1.0` emits as `1.0`; a stored `9223372036854775808` emits as `9223372036854775808`; a
stored `18446744073709551616` emits as `18446744073709551616`.

**This AC's `Tag`-unset rule applies to JSON numbers and to nothing else.** JSON strings in the
same mapping take the opposite treatment — an explicit `Tag: "!!str"` — under AC-8. Applying the
rule below to a string retypes it; applying AC-8's rule to a number prints an explicit tag. The
two rules are per-JSON-type by construction, and a builder who unifies them breaks one or the
other.

*(Mechanism, pinned because four implementations were probed and three corrupt or annotate the
output — see `## Decisions` › "Numbers are copied, not converted". Decode with
`json.Decoder.UseNumber()`, then emit each `json.Number` as a `*yaml.Node{Kind: ScalarNode,
Value: n.String()}` with **`Tag` left unset**. Two details are load-bearing and were each probed:
the node must be a **pointer** — a bare `yaml.Node` value inside a generic container panics with
`interface conversion: interface {} is *interface {}, not *yaml.Node` — and the **`Tag` must not be
assigned**. Setting `Tag` to `!!int`/`!!float` makes the encoder compare it against yaml.v3's own
re-resolution of the lexeme and print the tag explicitly whenever they disagree, which happens for
every integer outside `[-2^63, 2^64)` and every exponent outside float64 range: `!!int
18446744073709551616`, `!!float 1e400`. Leaving `Tag` unset skips that comparison and emits the
lexeme plain in every case. Do **not** convert through `int64` or `float64`: that path branches on
lexical form while this AC is framed by characters.)*

**AC-12** — The system shall serialize with an explicitly configured 2-space indent
rather than relying on the marshaller's default.

**AC-13** — When two exports of the same workspace run concurrently **and no mutation of
that workspace's automations commits between the start of the first and the start of the
second** — *start* as `## Definitions` fixes it, the moment each export's read transaction
establishes its snapshot — each shall produce byte-identical output to the other. *(The qualifier is
load-bearing and matches AC-9's. Without it this AC contradicts AC-30: AC-30 gives each
export the snapshot open at its own start and explicitly permits a concurrent mutation, so
a commit landing between the two snapshots makes differing output required by AC-30 and
forbidden by AC-13. What AC-13 actually guards is that concurrency itself introduces no
nondeterminism — no map-iteration order, no shared buffer, no racing sort.)*

**AC-14** — The export shall be free of side effects, so a client may retry a failed or
interrupted request without consequence. *(It is a `GET`; this records that no counter,
timestamp, or run row is touched — see `## Persistence guarantees`.)*

### Prompt fidelity

**AC-15** — When an automation's prompt is exported, the system shall emit it
byte-for-byte, applying no trailing-whitespace stripping, no line-ending conversion, and
no newline insertion or removal. This binds the **emitted document**, not merely the exporter's
own restraint: for every prompt that is valid UTF-8, parsing the exported document back shall
yield the stored prompt exactly. Where the marshaller's default choice would not satisfy this,
AC-49 governs. Where the prompt is not valid UTF-8, AC-47 governs and fidelity is established by
base64-decoding the emitted value.

*(The second sentence was added because the first, alone, was satisfied by an export that lost
data. "The system applies no conversion" is a claim about the exporter; "the bytes survive" is a
claim about the artifact, and only the second is what a human relying on this diff needs. yaml.v3
drops a prompt's leading newline while emitting a flawless block scalar, so an exporter that did
nothing wrong still produced a document that failed to round-trip. Stating the outcome makes the
AC testable against the artifact rather than against the implementation's intentions.)*

**AC-16** — When a prompt contains at least one newline and is valid UTF-8, the system shall emit
it through the pinned marshaller **without selecting or overriding the scalar style — except
where AC-49 requires an override to preserve the prompt's bytes** — and shall add **no** warning
when the emitted scalar is a literal block scalar. All three chomping indicators — `|`, `|-`,
`|+` — are literal block scalars and satisfy this AC equally. "Contains at least one newline" is
`## Definitions`' newline.

*(The exception is narrow and its boundary is observable, not a matter of judgement: it applies
only on AC-49's branch, which is entered only when the marshaller's own output has already been
shown to differ from the stored prompt. This AC's purpose is unchanged — the export does not
second-guess the marshaller's readability decisions, and does not cry wolf when a block scalar
was produced. What it never meant to say is that the export must accept a silently altered
prompt in order to keep its hands off the style, which is what the unqualified wording required.)*

*(This AC no longer mandates a block scalar, and that change is deliberate. Whether a block scalar
is producible is the marshaller's decision given the bytes, not the exporter's; three earlier
revisions mandated the outcome and were each falsified by an input the spec's condition list did
not name — see `## Decisions` › "Prompts are emitted faithfully". What the export can promise, and
what this AC now states, is that it does nothing to prevent one and does not cry wolf when one was
produced. All 7 live prompts satisfy this; the readable diff is the feature.)*

**AC-17** — If a prompt contains at least one newline (as `## Definitions` defines it), is valid
UTF-8, the emitted scalar is **not** a literal block scalar, **and AC-49 has not been triggered
for that prompt**, then the system shall add **exactly one** warning naming the automation,
carrying the message AC-42's vocabulary table defines for this condition. Export shall
not fail, and the prompt shall not be modified to make it qualify. AC-42 owns the exact message
bytes; this AC owns when the message is emitted, and the two are stated in one place each so they
cannot drift.

*(The AC-49 clause keeps "exactly one" true. AC-49's branch deliberately emits a double-quoted
scalar, which is not a literal block scalar, so without the exclusion both ACs would fire on the
same prompt and the automation would carry two prompt warnings describing one problem. AC-49's
message already tells the human the prompt is not in block form; the ordered classification in
`## Decisions` is what makes the exclusion mechanical rather than a judgement call.)*

**The condition shall be determined by inspecting the style of the emitted scalar — not by testing
the prompt's characters against any list of degradation conditions.** A conforming implementation
observes what the marshaller produced; it does not predict it.

*(One warning with one fixed reason replaces the previous three-condition table and its
one-warning-per-condition rule. The table was the defect: it was rewritten in three consecutive
review rounds and found incomplete each time, most recently because U+2028 is simultaneously a
line break to `is_break` and printable to `is_printable`, so it satisfied no listed condition
while still costing the block scalar. A fixed single reason cannot drift, keeps AC-42's vocabulary
finite, and keeps AC-9 deterministic. The diagnostic value given up is real but small: the human
reading the warning knows the prompt is unreadable in the diff and can see the prompt, and
`## Decisions` carries a non-normative list of usual causes that no test may depend on.)*

*(Testability, stated because the obvious test re-implements the bug: a test for this AC shall
parse the **exported document**, locate the automation's `prompt` node, and assert the biconditional
— a warning with this reason is present **if and only if** that node's style is not literal, given
the prompt has a newline and is valid UTF-8. Asserting instead that a particular input character
produces a warning rebuilds the enumeration this AC exists to delete, and will drift the same way.)*

**AC-46** — When a **non-empty** prompt contains no newline at all (as `## Definitions` defines
it) and is valid UTF-8, the system shall emit it in whatever single-line form YAML requires —
plain, single-quoted, or double-quoted as the value demands — and shall **not** add a warning.

*(Probed against yaml.v3: `"Do the thing"` emits as the plain scalar `prompt: Do the thing`, and
`"Do the thing "` as `prompt: 'Do the thing '`. Neither is a literal block scalar, and neither is a
degradation: a block scalar is unreachable for a string with no newline, so reporting one would
misdescribe a structural impossibility as a fault. This is why AC-17's antecedent requires a
newline — not, as an earlier draft of this rationale claimed, to stop AC-17 firing on every
single-line prompt in the store; AC-17's newline clause already prevents that on its own.
The **non-empty** qualifier is load-bearing in the other direction: the empty string has no newline
and is valid UTF-8, so without it this AC would require emitting `prompt: ""` for a prompt AC-4
requires be omitted entirely, and the two would mandate different documents. AC-4 wins — an empty
prompt is omitted and no prompt AC applies to it. The valid-UTF-8 qualifier is load-bearing too:
without it a single-line invalid-UTF-8 prompt would satisfy this AC and AC-47 simultaneously and
they mandate different bytes. AC-47 wins, and says so.)*

**AC-47** — When a prompt is not valid UTF-8, the system shall emit it as the `!!binary` value
yaml.v3 produces, add a warning naming the automation and carrying the `invalid UTF-8` message
AC-42's vocabulary table defines for this condition, and still return `200`. This applies **whether or not the prompt contains a newline**; it takes precedence
over both AC-46 and AC-17; and `invalid UTF-8` shall be the **only** reason emitted for that
prompt — AC-17's style check is not additionally applied, because yaml.v3 has already left the
text representation entirely. This condition shall **not** be treated as a serialization failure
under AC-36.

*(Probed: `"line one\nbad\xff\xfebyte\n"` does not error — yaml.v3 emits
`prompt: !!binary bGluZSBvbmUKYmFk//5ieXRlCg==` — and the no-newline form
`"bad\xff\xfebyte no newline"` emits `prompt: !!binary YmFk//5ieXRlIG5vIG5ld2xpbmU=`. The emitted
node carries **tag `!!binary`**, which is how this case is told apart from AC-17's without
re-examining the bytes; `utf8.ValidString` on the source is equivalent and either is conforming.
That second case is why the precedence is stated outright: `!!binary` is none of the three
single-line forms AC-46 permits, so without an explicit winner the two ACs mandate different bytes
for one input. The prompt column is SQLite TEXT read into a plain Go `string` and nothing enforces
UTF-8 validity, so a row written by direct SQL or an external tool is representable. Byte fidelity
under AC-15 is preserved by the base64 — probed, it decodes back byte-identical; what is lost is
readability, which is exactly the AC-17 degradation class. Failing the whole workspace's export
over one bad row would be the wrong trade.)*

**AC-49** — If a prompt is valid UTF-8 and the scalar the pinned marshaller emits for it does
**not** decode back to the stored prompt byte-for-byte, then the system shall emit that prompt in
the final document as a double-quoted scalar carrying an explicit `!!str` tag, shall verify that
this form decodes back byte-for-byte, and shall add **exactly one** warning naming the automation
and carrying the `prompt re-quoted to preserve bytes` message AC-42's vocabulary table defines.
Export shall not fail, and the stored prompt shall not be altered to make the default form work.

Both the default form and the double-quoted form are checked **in the probe phase**, on throwaway
single-prompt marshals, before the final document is built. `## Decisions`' prohibition on
marshalling, inspecting, amending and re-marshalling binds the **final document body** only; the
per-prompt probes are not that body and a second probe of the re-quoted form is required rather
than merely permitted.

If the double-quoted form also fails to decode back byte-for-byte, that is a serialization
failure under AC-36 and the export shall return `500` with no partial document. No prompt whose
bytes could not be preserved is ever emitted.

*(This AC exists because style and fidelity are independent, and four review rounds checked only
style. Probed: `"\nhello"` exports and decodes back as `"hello"`, `"\n"` as `""`, `"\n\nhello"` as
`"\nhello"`, `"\n\n"` as `"\n"` — and in every one of those the emitted scalar is a **literal
block scalar**, so AC-16 is satisfied and AC-17's antecedent is false. Nothing warned, nothing
failed, and the committed artifact silently differed from the database. A prompt pasted with a
leading blank line is ordinary user behaviour, so this is not an edge case reachable only by
direct SQL.*

*The remedy preserves rather than merely reports, because the card requires the export be "enough
to recreate the automation by hand" and a lost leading newline defeats that no matter how loudly
it is announced. Probed across twelve prompt classes, the double-quoted form round-trips
byte-identical in **all twelve**, including the four the default emission loses; the `!!str` tag
is required so a prompt whose entire text is `true` or `null` is not re-resolved to another type.*

*The `500` clause is a backstop that no probed input reaches. It is stated because the
alternative to failing, in a case where both forms lose bytes, is committing corrupted data to a
git repository — and the whole feature exists to stop that. It is deliberately narrower than
AC-47, which returns `200`: an invalid-UTF-8 prompt is losslessly representable as `!!binary`, so
there is nothing to fail about.*

*Testability: a test for this AC shall parse the **exported document**, decode the automation's
`prompt` node, and assert equality with the stored prompt, over a fixture corpus that includes
`"\nhello"`, `"\n"`, `"\n\nhello"` and `"\n\n"` alongside prompts that round-trip under the
default emission. Asserting only that a warning appears for a named input would re-create the
enumeration problem AC-17 exists to avoid; the observable is the round trip itself.)*

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
(`webhook_secret`, `continuation_task_id`, `last_evaluated_at`, `last_triggered_at`, `created_at`, `updated_at`,
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
  `max_concurrent_runs`, `continuation_policy`, and each trigger's `type` and `enabled`): the expected value is the
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

### Consistency, permissions, failure

**AC-29** — When an export is served, the system shall read the automations, their triggers,
their repository links, and every row it resolves a portable descriptor from, within a **single
read transaction opened on the store's reader handle**, so a concurrent create, update, or delete
cannot produce a document containing a trigger whose automation is absent, an automation missing
triggers it holds, or a descriptor resolved against a different snapshot than the row referencing it.

**The export shall establish that transaction's snapshot immediately upon opening it**, by issuing
its first read inside the transaction before performing any other work. This is what makes
`## Definitions`' *start* a single well-defined moment rather than an interval, and it is what
AC-13 and AC-30 are stated against.

*(SQLite's deferred transaction takes its snapshot at the first read, not at `BEGIN`. Left to the
builder, the gap between the two is unbounded — descriptor-lookup wiring, slug precomputation and
warning-buffer setup could all run inside it — and any commit landing in that gap is visible to an
export that had already "started". AC-13 was falsifiable that way by an implementation doing
nothing wrong; see `## Definitions` for the trace. Closing the gap costs one read.)*

Satisfying this **requires new read methods on the automation store** that accept the transaction,
and that work is in scope for this card. `Store` today splits `db` (writer) and `ro` (reader),
every read path takes only a `ctx` and issues against `s.ro` directly, `BeginTxx` is called only on
the writer, and no read transaction on the reader handle exists anywhere in the backend — so there
is nothing to reuse. See `## Decisions` › "Export is additive and read-only" for why adding them
does not contradict that decision.

**Descriptor rows are covered by this AC too, and the existing lookup seams cannot carry them.**
The automation package's four injected lookups return the wrong data for AC-18 and none accepts a
transaction:

| Existing seam | Returns | Why it cannot serve the export |
|---|---|---|
| `AgentProfileLookup.AgentProfileExists(ctx, id)` | `(bool, error)` | AC-18 needs `{agent_name, model, mode}` |
| `WorkflowLocator.WorkflowWorkspaceID(ctx, id)` | `(string, error)` | AC-18 needs the workflow **name** and the **step name** |
| `RepositoryLookup.GetRepository(ctx, id)` | `(workspaceID, defaultBranch string, ok bool)` | AC-18 needs the repository **name**; also two-valued, which AC-19 forbids |
| *(none)* | — | **No executor-profile lookup exists on the automation service at all** |

The export shall therefore define **new, transaction-accepting descriptor lookups** — one each for
agent profile, executor profile, workflow-and-step, and repository — following the established
`SetWorkflowLocator` / `SetRepositoryLookup` injection pattern: the interfaces are declared in the
automation package, each method takes the export's read transaction — the `*sqlx.Tx` opened on the
store's reader handle — as its first argument after `ctx`, and each reports the three outcomes
AC-19 requires.

**Where the SQL lives is part of this contract.** Each new lookup shall be satisfied by a **new
exported, transaction-accepting read method on the store that owns the table** — the
agent-settings store, the workflow repository, and the repository store respectively — and the
adapter constructed in `backendapp` shall do nothing but call that method, passing the transaction
through. **No SQL text shall be written in `backendapp`.**

*(This is stated because the AC is otherwise satisfiable two ways and only one of them is
consistent with the codebase. Verified: `internal/backendapp` today contains no raw SQL and no
`*sqlx.Tx` usage, every existing `Set*Lookup` adapter delegates to the owning package's exported
method, and the only `*sqlx.Tx` parameters in `internal/workflow/repository` and
`internal/agent/settings/store` are unexported helpers operating inside those packages' own
transactions. An adapter issuing its own `SELECT` against `agent_profiles`, `workflows`,
`workflow_steps` or `repositories` would be the first of its kind in the tree, and would put query
text for four tables in a package that owns none of them — with no compile-time signal when an
owning package renames a column. That is precisely the silent drift AC-22 and AC-43 exist to
prevent for this export's own fields, reintroduced one layer out. Delegation keeps each query
beside the schema it reads and keeps the seam doing what every other seam here does.*

*The seams exist so the automation package does not import the task, agent-settings or repository
services and create a cyclic import — AC-44 step 2 relies on the same rationale — so moving the
SQL into the automation package is not available either. Passing the transaction across the seam
costs nothing new: the automation package already depends on `sqlx` for its own `Store`, every one
of these tables lives in the same SQLite database, and one read transaction can read all of them.*

*The alternative considered and rejected was to exempt descriptor rows from this AC, which would
mean a descriptor resolved against a different snapshot than the automation referencing it —
exactly the inconsistency the AC exists to prevent, and the one a git-committed artifact would
preserve forever.)*

This AC is observed by a unit test, not by a race: the export path shall obtain its snapshot once
and pass it to every read — store reads and descriptor lookups alike — and the test asserts, with a
store double and four lookup doubles that each record the handle they were given, that every
recorded handle is the same one and that no read arrived with no handle.

*(Stated because a race is not deterministically testable, and an AC whose only verification is
"run it and hope" is one a builder can satisfy by doing nothing. Making the shared handle the
observable turns a timing property into a structural one. The lookup doubles are named explicitly
because a test that instruments only the store passes while every descriptor read runs outside the
transaction, which was true of an earlier draft of this AC.)*

**AC-30** — While an export is in flight, a concurrent mutation of the same workspace's
automations shall neither block nor be blocked by it; the export reflects the snapshot
open at its *start*, as `## Definitions` fixes that term — the moment its read transaction
establishes its snapshot, which AC-29 requires to happen immediately upon opening the
transaction.

*(Probed, not reasoned, because an earlier round cleared this claim on general grounds that turned
out to be incomplete. The probe replicates both DSNs from `internal/db/sqlite.go` exactly — writer
`_foreign_keys=on&_mode=rwc&_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL&_cache=shared`,
reader `_foreign_keys=on&_mode=ro&_busy_timeout=5000&_cache=shared` — opens a read transaction on
the reader pool, establishes its snapshot with a first read, and then writes the same table on the
writer pool. Result: `journal_mode = wal`; the `UPDATE` and a subsequent `INSERT` each **succeeded
immediately with no block**; and the still-open read transaction continued to see the pre-write
value, so snapshot isolation holds. A read transaction opened after the write also did not block.
AC-30 stands, with evidence.*

*One dependency is worth naming, because it is not what it appears to be. Both DSNs carry
`_cache=shared`, and SQLite shared-cache mode uses table-level locks under which this AC would
fail. It does not fail, because **`_cache=shared` is inert**: SQLite's URI parameter is `cache`,
and the driver's own parameters are `_busy_timeout`, `_journal_mode` and friends — neither honours
`_cache`, so shared cache is never enabled. Probed side by side against the same code path:
`_cache=shared` and no cache parameter at all behave identically (write succeeds), while a true
`cache=shared` fails the write immediately with `database table is locked: automations` — and
immediately, because `busy_timeout` does not retry `SQLITE_LOCKED`. **If that DSN is ever
"corrected" to `cache=shared`, this AC breaks at once and hard.** That is a pre-existing
observation about the database configuration, not a defect this card introduces or fixes; it is
recorded here so the dependency is visible rather than accidental.)*

**AC-31** — When a user without access to the target workspace requests an export, the system
shall deny the request through the same workspace authorizer used by every other automation
operation (`Service.authorizeWorkspace`, wired to `taskSvc.AuthorizeWorkspaceAccess`), return
`404`, and return no automation data and no error text distinguishing denial from absence.

*(`404` and not `403`, deliberately. `ErrAutomationNotFound` is documented as covering "both a
missing automation and one in a workspace the caller cannot reach, so authorization never leaks
which of the two it is". A `403`/`404` split on this endpoint would reintroduce exactly the
existence leak that sentinel exists to prevent: a caller could enumerate which workspace IDs are
real by reading the status code.)*

*(Testing note, because the obvious fixture observes the opposite: the authorizer returns `nil`
without checking anything when the caller context carries no user scope, which is Kandev's default
single-user mode. A test for this AC must supply a **scoped** caller context; run unscoped it
observes `200` and the AC looks wrong.)*

**AC-44** — The system shall evaluate the request in this order, and shall emit no automation
data unless every step passes:

1. **Authorize** the workspace through `Service.authorizeWorkspace`. Classify the result by
   sentinel, not by position: `nil` → continue; an error satisfying
   `errors.Is(err, repoerrors.ErrWorkspaceNotFound)` → `404` (AC-31); **any other non-nil error**
   → `500` (AC-45).
2. **Resolve the workspace** through a workspace lookup injected into the automation service at
   construction, following the existing `SetWorkflowLocator` / `SetRepositoryLookup` injection
   precedent — the seam exists so the package does not import the task service and create a cyclic
   import. `repoerrors.ErrWorkspaceNotFound` → `404` (AC-35); any other non-nil error → `500`
   (AC-45); lookup not wired → `500`, since the endpoint cannot honour AC-35 without it and an
   unwired lookup is a construction error rather than a deployment mode.
3. **Export.** An authorized, existing workspace holding no automations → `200` with an empty list
   (AC-32).

*(The sentinel rule in step 1 is spelled out because the wired authorizer makes the naive reading
unsafe. `taskSvc.AuthorizeWorkspaceAccess` returns **the same `repoerrors.ErrWorkspaceNotFound`
for a denied workspace and for a workspace that does not exist**, and returns any infrastructure
error from its own workspace read bare on the same path. A builder who reads "denial" as "the
authorizer said no" and treats a not-found error as "a failure that is not a denial" returns `500`
where AC-31 and AC-35 both mandate `404` — which re-opens, with the status code as the oracle,
precisely the enumeration leak AC-31 exists to close. Classifying on the sentinel makes denial and
absence indistinguishable, which is the required behaviour, and leaves `500` for the errors that
genuinely are failures.)*

*(Why step 2 exists, stated precisely because the general claim is false: for a **scoped** caller
step 1 already answers existence, since the authorizer's own workspace read produces
`ErrWorkspaceNotFound` for a workspace that is not there. Step 2 is the path that carries AC-35
for an **unscoped** caller — Kandev's default mode — where step 1 returns `nil` without reading
anything and `Service.ListAutomations` cannot help: the store returns an empty slice for a
workspace ID that never existed, indistinguishable from a real but empty workspace. The lookup is
therefore required and is not dead code, even though a scoped-context test will never reach it.
The automation package holds no workspace lookup of any kind today. Office's config export, which
this endpoint otherwise mirrors, never returns `404` at all — it returns `500` on any error — so
the precedent is not usable here.)*

*(Step 2 and step 3 read from different snapshots and the gap between them is not closed. If the
workspace is deleted after step 2 succeeds, the export returns `200` with whatever the AC-29
snapshot holds, which for a cascaded delete is an empty list. This is an accepted race: the
existence check is a gate against a workspace ID that was never real, not a guarantee that the
workspace outlives the request, and paying for the stronger guarantee would mean holding the
workspace row in the export's own transaction across a service boundary the injection seam exists
to avoid.)*

**AC-32** — When a workspace has no automations, the system shall return a valid document
with `version`, `type`, and an empty `automations` list, and the zip form shall be a valid
archive containing no automation files. This is not an error.

**AC-33** — When an automation's stored `name` is the empty string, the system shall emit
`name: ""` and derive its filename through the AC-26 fallback. *(`Service.CreateAutomation`
rejects an empty name, but the `automations.name` column permits one, so a row written by
any other path is representable.)*

**AC-34** — When an automation's stored `max_concurrent_runs` is zero or negative, the
system shall emit the stored value unchanged rather than substituting the service's
`1` default. *(Export reports what is in the database; it is not a validation pass.)*

**AC-35** — If the target workspace does not exist, then the system shall return `404` and
no partial document, indistinguishable in status and body from the AC-31 denial. The
existence answer comes from the lookup named in AC-44 step 2.

**AC-36** — If serialization of any automation fails, then the system shall return `500`
and no partial document. A partially-written body is never emitted.

**AC-45** — If the read transaction cannot be opened, any query fails, the workspace lookup
fails for a reason other than not-found, the authorizer fails for a reason other than
denial, or the zip archive cannot be written, then the system shall return `500` with no
automation data and no partial body.

**AC-48** — The system shall build each response body completely in memory before writing
any of it to the response, and shall write the status line only once the body is known to
be complete.

*(AC-36 and AC-45 both promise "no partial body", and that promise is unimplementable in
the streaming shape Office uses: `exportConfigZip` sets `200` and the `Content-Disposition`
header and then `io.Copy`s the archive to the writer, so a failure mid-copy has already sent
`200` and cannot retract it — the client receives a truncated archive under a success
status. Buffering is what makes the guarantee real. The measured corpus is ~78 KB of prompt
across 7 automations, so the memory cost of buffering the whole artifact is not a concern at
this scale.)*

### User surface

**AC-37** — When a user opens a workspace's automations settings page, the system shall offer an
export control that downloads the zip form. The control shall be present when the page itself is
(a user who can open the page has passed the same workspace check the export authorizes against),
shall be disabled from the moment the request is issued until the download has been handed to the
browser or an error has been surfaced, and shall surface a non-blocking error message if the
response status is not `200`, leaving the page otherwise unchanged. A workspace with no automations
still shows the control enabled — AC-32 makes that a valid export, not an error.

The control shall issue the request with `fetch`, check the response status, read the body to a
`Blob`, and trigger the download from an object URL with an explicit filename of
`kandev-automations.zip`. It shall **not** navigate to the endpoint with `window.open` or an anchor
`href`.

*(The mechanism is pinned because the two halves of this AC are unimplementable without it, and the
precedent this spec otherwise mirrors gets it wrong. Office's equivalent control is
`window.open(officeApi.exportConfigZipUrl(activeWorkspaceId), "_blank")`: a raw navigation gives the
calling code no completion signal and no access to the response status, so there is nothing to
disable on and nothing to test for. Worse, a navigation to a non-`200` renders the error body in a
blank tab, which is the opposite of "leaving the page otherwise unchanged". Under `fetch` the
filename comes from the object URL's `download` attribute rather than from `Content-Disposition`;
the header stays on the response for anyone hitting the URL directly, but it is not what names this
download. "In flight" is defined above as the whole span from issue to hand-off precisely because
"until headers arrive" and "until the body is read" are different windows and only the wider one
prevents the second click AC-37 is guarding against.)*

**AC-38** — All copy introduced by the export control shall be resolved through `t()` or
`<Trans>` and shall be present in `en`, `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`.

## Failure modes

Every row cites the AC that makes it testable. A row without one is a shadow path.

| Condition | Behaviour |
|---|---|
| Workspace has no automations | `200`, empty `automations` list (AC-32) |
| Workspace does not exist | `404`, body indistinguishable from denial (AC-35, AC-44) |
| Caller lacks workspace access | `404`, no data, no distinguishing text (AC-31, AC-44) |
| Workspace lookup not wired | `500` (AC-44 step 2) |
| Agent/executor profile or repository row is **absent** | Reference omitted, warning emitted, export succeeds (AC-19) |
| A descriptor lookup **fails** rather than reporting absence | `500`, no partial document, **no** warning — absence and failure are distinct outcomes (AC-19, AC-45) |
| Workflow resolves but its step does not | `workflow.name` emitted without `step`, warning emitted (AC-19) |
| Some repositories resolve, others do not | Unresolved members dropped, resolved members kept in order, warning emitted (AC-19) |
| Trigger `config` is not valid UTF-8 | Empty `config` mapping, `config is not valid UTF-8` warning; checked on raw bytes before decoding and takes precedence over the next two rows, so exactly one warning is emitted (AC-39) |
| Trigger `config` is not valid JSON | Empty `config` mapping, warning naming automation + trigger type + condition; export succeeds (AC-39) |
| Trigger `config` is valid JSON but not an object | Same as above (AC-39) |
| Trigger `config` holds a number | Emitted unquoted and untagged, character-identical to storage; never reformatted, rounded, quoted, or given an explicit `!!int`/`!!float` tag (AC-41) |
| Trigger `config` keys contain digits or punctuation | Sorted byte-wise via an explicit `MappingNode`, not by yaml.v3's map sorter (AC-8) |
| Trigger `config` holds a **string** whose text would resolve to another YAML type (`"true"`, `"null"`, `"1.0"`, `"0755"`, `"12"`, `"~"`) | Emitted with an explicit `!!str` tag, so yaml.v3 quotes it and it re-parses as a string; never retyped to bool, null, int or float (AC-8, AC-23) |
| Multi-line valid-UTF-8 prompt, emitted scalar **is** a literal block scalar (any chomping indicator) | Emitted as-is, **no** warning (AC-16) |
| Multi-line valid-UTF-8 prompt, emitted scalar is **not** a literal block scalar | Emitted faithfully in whatever form yaml.v3 chose, exactly one `prompt not emitted as a block scalar` warning; export succeeds. The condition is read from the emitted node's style, never from the prompt's characters (AC-17) |
| Multi-line prompt contains a TAB | Block scalar retained, **no** warning — a TAB does not clear `block_allowed`, and this row is a consequence of the style check rather than a special case in it (AC-16) |
| Prompt contains invalid UTF-8 | Emitted as `!!binary`, `invalid UTF-8` warning added, `200` — not a serialization failure; wins over AC-46 and AC-17 whether or not the prompt has a newline, and is the only reason emitted (AC-47) |
| Single-line non-empty prompt, valid UTF-8 | Emitted as YAML requires, **no** warning — a block scalar is unreachable without a newline, so this is not a degradation (AC-46) |
| Prompt is the empty string | `prompt` omitted entirely; no prompt AC applies and no warning is emitted (AC-4, AC-46) |
| Valid-UTF-8 prompt whose default emission does **not** decode back byte-for-byte (a prompt beginning with a newline is the reachable case) | Re-emitted as a double-quoted `!!str` scalar, which is probed to round-trip; exactly one `prompt re-quoted to preserve bytes` warning; AC-17 suppressed for that prompt so only one prompt warning is emitted; `200` (AC-49, AC-15, AC-16, AC-17) |
| A prompt that round-trips under neither the default nor the double-quoted form | `500`, no partial document — corrupt bytes are never committed. No probed input reaches this (AC-49, AC-36) |
| Prompt is invalid UTF-8 **and** would fail the fidelity test | AC-47 wins and is evaluated first; fidelity comes from base64-decoding, not from comparing the re-parsed scalar, whose value is the base64 text (AC-47, AC-49) |
| Multi-line prompt whose only line break is CR, U+0085, U+2028 or U+2029 | Treated as multi-line, since `## Definitions` fixes *newline* as YAML's five-character break set; emits non-literal, so exactly one `prompt not emitted as a block scalar` warning (AC-17, AC-46) |
| `workflow_id` set, `workflow_step_id` empty | `workflow` emitted with `name` only, **no** warning — nothing was referenced, so nothing is unresolved (AC-19, AC-4) |
| Two automations derive the same slug | Suffixed with `id` prefix, lengthened until unique (AC-27) |
| Authorizer returns a non-`ErrWorkspaceNotFound` error | `500` — not `404`; the sentinel is what distinguishes denial (AC-44 step 1, AC-45) |
| Workspace deleted between the existence check and the snapshot | `200` with an empty list; accepted race (AC-44) |
| Two same-named automations emit an identical warning | Both kept — dedup is never global (AC-42, AC-19) |
| Two same-type triggers on one automation both have bad `config` | Both warnings kept — AC-39 messages dedup per trigger, not per automation, so the count is not lost (AC-42, AC-39) |
| Automation name **or trigger type** contains a newline or control character | Escaped before interpolation so the warning stays one line. "Newline" is the five-character set, so U+0085 escapes as `\x85` and U+2028/U+2029 as `\u2028`/`\u2029`; without this a raw U+2028 survives into both the YAML scalar and `WARNINGS.txt`, where a reader honouring YAML's break set sees two lines (AC-42) |
| Automation name **or trigger type** contains an invalid UTF-8 byte | Escaped as `\xNN` before interpolation, so the warning stays a UTF-8 string scalar and does not become `!!binary` (AC-42) |
| A concurrent mutation lands while an export is in flight | Neither blocks the other; the export keeps its snapshot. Probed against both real DSNs (AC-30) |
| A mutation commits between two exports opening their transactions | Each export's *start* is its snapshot establishment, and AC-29 requires that to happen immediately on opening, so the window is one read wide and AC-13 is stated against a defined moment (AC-13, AC-29, AC-30) |
| Read transaction, query, or zip write failure | `500`, no partial body (AC-45, AC-48) |
| Serialization error | `500`, no partial body (AC-36, AC-48) |

## Persistence guarantees

The export performs no writes. No table is modified, no `last_evaluated_at` is touched,
no run row is created. Exporting an automation has no effect on when it next fires.

## Out of scope

Named exclusions, not omissions:

- **The applier (Phase 2).** Reading exported YAML back into the database is not
  specified here. It is additionally gated on an unresolved upstream question — whether
  `kdlbs/kandev` considers automations a git-syncable entity at all — tracked in Office
  discovery task `ba70553d`. If the answer is no, Phase 2 is a carried topic branch
  rather than an upstream contribution.
- **Two-way sync** (Kandev writing back to git).
- **A poller.** Nothing is fetched from a repository on an interval.
- **Migrating agent or executor profiles into git.** They stay database-resident; the
  export references them by descriptor, which is why AC-19 needs a warning path.
- **`automation_runs`.** Run history is observability, not definition, and is never
  exported.
- **Importing Office `ConfigBundle` routines.** A different entity in a different package.
- **Fixing the Office `reports_to` import defect.** Referenced as a caution; it has its
  own card.
- **Cross-workspace export.** Every endpoint is scoped to one workspace.
- **Secret rotation or a secret-bearing export variant.** `webhook_secret` has a
  dedicated reveal endpoint and stays there.
