---
status: draft
system: executors
created: 2026-10-28
owners:
  - kandev
---

# Remote executor plugins: profile schema, typed values, and secret references requirements

## Overview

The executor profile schema a provider declares today allows only `type`, `title`, `description`, `enum`, `secret`, `format`,
`minimum`, and `maximum`, and the profile snapshot carries every value as a string and every secret only as a transient value.
A first real provider skeleton had to re-implement defaults, length and pattern checks, and type parsing, and could not
refer to a secret without holding its value. This document closes those gaps as opt-in, additive
[capabilities](remote-executor-plugin-capability-versioning.md). A provider using none keeps today's schema and snapshot.

## Terminology

- **Schema keyword:** A key on a profile property in the manifest `profile_schema`.
- **Typed value:** A profile value carried as a boolean, integer, number, or string rather than a string.
- **Secret reference:** An opaque, non-secret identifier of a stored secret field value.

## Requirements

### REQ-EXECUTORS-PLUGIN-PROFILE-001: Schema defaults, length, and pattern

**Intent:** The host owns the common checks, so providers do not each re-implement them.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-PROFILE-001.1:** The manifest validator shall accept `default` on any non-secret profile property, and `minLength`, `maxLength`, and `pattern` on string properties, in addition to the current keywords. A manifest that does not use them shall validate and behave as before.
- **AC-EXECUTORS-PLUGIN-PROFILE-001.2:** A `default` shall satisfy the property's type, `enum`, `minimum`, `maximum`, length, and pattern, and a secret property shall not have a default. A violation shall reject the manifest and name the property.
- **AC-EXECUTORS-PLUGIN-PROFILE-001.3:** `minLength` and `maxLength` shall be integers from 0 to 4096 with `minLength` at most `maxLength`. A `pattern` shall be at most 256 characters, shall use linear-time (RE2) syntax, shall match the whole value, and shall be compiled at install; a pattern that does not compile shall reject the manifest.
- **AC-EXECUTORS-PLUGIN-PROFILE-001.4:** The host shall fill an absent optional property with its default when it builds the profile snapshot, record the filled values in the operation input digest, and show defaults in the profile editor. A later upgrade that changes a default shall not change an existing environment's recorded snapshot.
- **AC-EXECUTORS-PLUGIN-PROFILE-001.5:** The host shall validate a profile against type, `required`, `enum`, bounds, length, and pattern before any provider call and shall return field errors with stable codes, including the property name, without calling the provider. Provider validation shall remain authoritative for provider-specific rules.
- **AC-EXECUTORS-PLUGIN-PROFILE-001.6:** Field errors shall never echo a secret value.
- **AC-EXECUTORS-PLUGIN-PROFILE-001.7:** An absent property and an empty string shall be the same for every type except `string` with an explicit `minLength` of 0 (where an empty value is valid and kept, even when the property is `required`); otherwise an empty value of a `required` property with a default shall take the default, and one without shall be the field error `required`. Length shall be counted in Unicode code points. A `bool` property shall accept `true` or `false` only, and an `int` or `number` property only decimal text with an optional leading minus sign (no exponent for `int`, no `NaN` or infinity). For a provider that declares any capability, a config key the schema does not declare shall be rejected as the field error `unknown_property`, except the host-owned keys: the existing kandev keys (`remote_credentials`, `remote_auth_secrets`, `agent_config_bundles`, `remote_auth_target_home`, `git_user_name`, `git_user_email`) and the keys of this document set (`launch_policy`, `policy_id`, `checkout_kind`, `checkout_value`, `sparse_paths`, `hint.*`, `migrated_to`). A provider that declares no capability keeps today's handling of undeclared keys.
- **AC-EXECUTORS-PLUGIN-PROFILE-001.8:** For a provider that declares any capability, the marshaled profile config and secret references that the host records in the environment envelope shall total at most 4096 bytes (the budget row of the capability versioning design), and this aggregate limit applies in addition to the per-item limits of this document and of REQ-EXECUTORS-PLUGIN-PREPARATION-001. A profile over it shall be rejected when it is saved and again in launch validation with the profile-level field error `profile_too_large` (no property name, no value), without a provider call. A profile saved before this document and over the limit shall not be rewritten; its next launch shall fail with `profile_too_large`, and an environment already allocated shall keep its recorded snapshot. A provider that declares no capability has no aggregate limit; "declares" means the current manifest's `requires_host_capabilities` is non-empty, and the measure applies to a save, a first launch, and a resume of an environment whose recorded negotiated set is non-empty. A resume of an environment allocated with an empty set keeps its recorded snapshot and is not re-measured, so a plugin upgrade that adds a capability cannot fail an existing environment. The measured value is one shared function used at save and at launch: the length of the compact JSON encoding (object keys sorted, key names, quotes, and separators included) of the filled provider-facing `config` object, which excludes only the six existing kandev keys and `migrated_to` (which is not provider-facing and is not recorded in the snapshot), so the keys `launch_policy`, `policy_id`, `checkout_kind`, `checkout_value`, `sparse_paths`, and every `hint.<key>` are measured and recorded within the same 4096 bytes (a profile whose per-item limits are individually met but whose total exceeds 4096, such as 64 sparse paths of 512 bytes, is `profile_too_large`), plus the length of the compact JSON array of the secret store IDs. The check is local and runs before the capability echo and any provider call.

### REQ-EXECUTORS-PLUGIN-PROFILE-002: Typed profile values

**Intent:** A provider receives booleans and numbers as such.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-PROFILE-002.1:** When a provider requires `typed_profile_values`, the profile snapshot shall carry `typed_config` beside `config`, with a boolean, int64, finite double, or string for each non-secret property according to its schema type.
- **AC-EXECUTORS-PLUGIN-PROFILE-002.2:** The host shall convert from the stored value by the schema type. A value that does not parse, is out of int64 range, or is not finite shall be a field error with code `invalid_type` before any provider call.
- **AC-EXECUTORS-PLUGIN-PROFILE-002.3:** `config` shall keep its canonical string form (decimal digits, `true` or `false`) so a provider that ignores `typed_config` still works, and both forms shall be covered by the operation input digest.
- **AC-EXECUTORS-PLUGIN-PROFILE-002.4:** `typed_config` shall never contain a secret property.
- **AC-EXECUTORS-PLUGIN-PROFILE-002.5:** A provider not requiring the capability shall receive no `typed_config`, and a response or callback referring to it shall be rejected as `conflict`.

### REQ-EXECUTORS-PLUGIN-PROFILE-003: Secret references

**Intent:** A provider can name and track a secret without holding its value.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-PROFILE-003.1:** When a provider requires `secret_references`, each operation that carries a profile snapshot shall also carry `secret_refs`, mapping each declared secret property that has a stored value to an opaque reference ID. The ID is the stored secret's identifier that the profile already records. The values shall still be sent only in `secret_values`.
- **AC-EXECUTORS-PLUGIN-PROFILE-003.2:** Reference IDs are non-secret. The host shall record, in the existing `secret_references` field of the plugin inventory, the property and ID pairs that it already records at allocation (at most 16), so recovery and destroy name the same secret. The host shall never record a value.
- **AC-EXECUTORS-PLUGIN-PROFILE-003.3:** Replacing a secret stores a new secret and a new ID; an environment keeps referencing the ID it was allocated with, and the existing retention rule keeps that secret while the environment is retained. Recovery shall send the value of the recorded ID in `secret_values`, so a provider sees a changed ID only on a new allocation. The host shall reject a recovery whose recorded ID no longer exists with a named reason and without a provider call.
- **AC-EXECUTORS-PLUGIN-PROFILE-003.4:** A reference shall exist only for a property the manifest declares secret, for the same profile, and shall never be sent for a property without a stored value.
- **AC-EXECUTORS-PLUGIN-PROFILE-003.5:** No value shall appear in a reference, error, log, checkpoint, state, or event, and a provider that returns a value-bearing field in state shall be rejected by the closed state schema. The state schema cannot tell a secret from other text, so the host shall also reject, with the code `secret_in_state`, any provider response whose state or checkpoint contains the exact bytes of a secret value the host revealed for that call, and a provider shall not store a secret value in state, which the host cannot detect in other forms. The exact-bytes check shall cover revealed values of at least 8 bytes; shorter values are not scanned.

## Out of scope

- JSON Schema keywords beyond those named here, nested objects, and arrays.
- Changing how secrets are stored or the secret store itself.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
