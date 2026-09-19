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
# Automations YAML Export System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
