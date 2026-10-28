---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-PLUGIN-PROFILE-001
  - REQ-EXECUTORS-PLUGIN-PROFILE-002
  - REQ-EXECUTORS-PLUGIN-PROFILE-003
---

# Remote executor plugins: profile schema, typed values, and secret references system design

## Purpose and boundaries

Extends the scalar-only schema validated by `validateExecutorSchemaProperty` in `internal/plugins/manifest/executor_provider.go`
and the `ExecutorProfileSnapshot` message (`config`, `secret_values`). Gated by `typed_profile_values` and `secret_references`
in the [capability registry](remote-executor-plugin-capability-versioning.md). Schema keywords are manifest data and need no
capability: an older host rejects an unknown keyword at install, which `min_kandev_version` already turns into a clear message
(see the versioning design). Names marked **new** do not describe existing APIs.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-EXECUTORS-PLUGIN-PROFILE-001 | Schema keywords; Host validation |
| REQ-EXECUTORS-PLUGIN-PROFILE-002 | Typed values |
| REQ-EXECUTORS-PLUGIN-PROFILE-003 | Secret references |

## Schema keywords

The per-property allowed-keyword list gains `default`, `minLength`, `maxLength`, and `pattern`. Checks run at install in
`validateExecutorSchemaProperty`: the default is validated by the same function that validates a user value (one code path, so
the manifest check and the runtime check cannot drift); `pattern` is compiled with Go `regexp` (RE2, linear time) and wrapped
as `\A(?:...)\z`. `resource_state_schema` keeps its closed form and does not accept the new keywords.

## Host validation

**New** `validateProfileAgainstSchema(schema, config)` runs when a profile is saved and again in `validateLaunch`, before any
provider call. It fills absent optional values from `default`, converts types, checks `required`, `enum`, bounds, length, and
pattern, and returns `ExecutorProviderFieldError` values with codes `required`, `invalid_type`, `not_in_enum`, `out_of_range`,
`too_short`, `too_long`, `pattern_mismatch`. The filled snapshot is what the operation digest covers, and the allocation
records it in the inventory snapshot, so a default change on upgrade does not touch existing environments. The editor reads
defaults from the same schema.

## Typed values

`ExecutorProfileSnapshot` gains additive `map<string, ExecutorConfigValue> typed_config` (oneof bool, int64, double, string).
It is built from the validated, filled config by schema type and is omitted unless the capability is in the negotiated set.
`config` is unchanged and canonical (`strconv` forms), so both representations are derived from one value and cannot disagree.

## Secret references

`ExecutorProfileSnapshot` gains additive `map<string, ExecutorSecretRef> secret_refs` (`id` only). The ID is the secret store identifier that the profile already holds
(`models.ExecutorProfileSecretID`), and the inventory already records property-to-ID pairs in `secret_references`, which this design reuses (no `secret_ref_ids` field). Replacing a secret
creates a new store entry and a new ID, and there is no revision counter, so a reference has no version. An environment keeps its allocation-time ID; the existing retention check
(`retainedPluginExecutorReferences`) stops the profile service from deleting a secret that a retained environment still names. On recovery and destroy the host reveals the recorded
ID through `loadRecordedExecutorProviderSecrets`: a missing secret is the named failure `secret_reference_missing` before any provider call, otherwise the value goes in `secret_values` only for the duration of the call.

**State check.** After every call that received `secret_values`, the host compares the returned state and checkpoint bytes with each revealed value of at least 8 bytes and, on an exact match, rejects the response as `secret_in_state` without persisting and runs the existing cleanup. Transformed copies (encoded or split) are a provider obligation, not detected.

**Config rules.** Absent and empty are equal except for a `string` with `min_length` 0; an empty required value takes its default; lengths are code points; `bool` is `true|false`, numbers are decimal text; undeclared keys are `unknown_property` except the host-owned keys named in AC 001.7.

## Data model

No new table and no DDL. The envelope gains `profile_snapshot_digest`; `secret_references` already exists. Decode tolerates absence.

## Interleavings and failing-first tests

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Secret replaced between allocation and recovery (003.3) | recorded ID kept alive by retention; its value sent; new allocations see the new ID | lifecycle unit with fake store |
| Secret deleted before recovery (003.3) | named failure, no provider call, record retained | lifecycle unit |
| Provider upgrade changes a default (001.4) | existing environment keeps its recorded snapshot | lifecycle unit |
| Hostile pattern input (001.3) | linear-time rejection or match at a 1 MiB input | validator test with a time bound |
| Unparseable stored integer (002.2) | `invalid_type`, no provider call | lifecycle unit |
| Provider without the capability returns `typed_config` use (002.5) | `conflict` | lifecycle unit |
| Provider returns a value-bearing state field (003.5) | rejected by closed schema | validator test |

Each test is written first and fails on main. Remaining ACs are validator table tests and the unchanged fixture conformance test.

## Resolution of the plugin skeleton findings

The first provider skeleton (2026-10-28) reported seven gaps. Each is closed here or ruled out of scope.

| # | Finding | Resolution |
| --- | --- | --- |
| 1 | No `default`, `maxLength`, or `pattern` | Closed: REQ-EXECUTORS-PLUGIN-PROFILE-001 |
| 2 | Secrets only in `SecretValues` | Closed: REQ-EXECUTORS-PLUGIN-PROFILE-003 (references beside values) |
| 3 | Every field is a string | Closed: REQ-EXECUTORS-PLUGIN-PROFILE-002 (additive `typed_config`) |
| 4 | `bootstrap_json` carries no repository intent, workspace path, or agent environment | Closed: REQ-EXECUTORS-NATIVE-WORKER-002 and 003 (R1, R7) |
| 5 | No minimum host version for executor providers | Closed: AC-EXECUTORS-PLUGIN-CAPABILITY-002.6 |
| 6 | No "refused, nothing allocated" provision outcome | Closed: REQ-EXECUTORS-PLUGIN-PLACEMENT-002 |
| 7 | State versioning and size unchecked | Closed in core: AC-EXECUTORS-PLUGIN-CAPABILITY-003.7 and 003.8 (budget, rejection, reporting) and the state schema version (003.2 to 003.4). Whether the real state fits is a vendor-repository check; D7 stands. |

Two open manifest questions are ruled as follows. The provider's declared capabilities (terminal, files, git, editor, preview,
retention) are provider data validated by the existing capability rules, not a core decision. `prefer_local` and `exclude_local`
do not exist on main and are not part of the plugin profile contract: choosing local, SSH, or remote transport is the provider's
placement, expressed as ordinary profile fields with the schema keywords above. No core field is added for them.

## Observability

`executor_plugin_profile_invalid_total` (`code`). No profile, property, or secret label.

## Security

Defaults are never applied to secret properties. References are opaque and non-secret by construction; the value path is unchanged
and transient. Error messages are built from stable codes and property names, never from values.

## Related decisions

- [Remote executor plugin boundary](../../../decisions/2026-09-26-remote-executor-plugin-boundary.md)
