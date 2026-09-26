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
# Automations YAML Export System Design Part 4

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Acceptance criteria

### Export content

**AC-1** — When a user requests the export for a workspace, the system shall return a
YAML document whose `version` is `1` and whose `type` is `kandev_automations`.

**AC-2** — When a user requests the export for a workspace, the system shall include
exactly the automations belonging to that workspace, and no automation belonging to any
other workspace.

**AC-3** — When an automation is exported, the system shall emit `name`, `enabled`,
`max_concurrent_runs`, and `triggers` unconditionally, including when `triggers` is empty.

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
`name`, `description`, `enabled`, `max_concurrent_runs`, `task_title_template`, `prompt`,
`agent_profile`, `executor_profile`, `workflow`, `repositories`, `triggers`, and each
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
