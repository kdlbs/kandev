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
- **Secret reference:** An opaque, non-secret identifier and version for a stored secret field value.

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

- **AC-EXECUTORS-PLUGIN-PROFILE-003.1:** When a provider requires `secret_references`, each operation that carries a profile snapshot shall also carry `secret_refs`, mapping each declared secret property that has a stored value to an opaque reference ID and an integer version. The values shall still be sent only in `secret_values`.
- **AC-EXECUTORS-PLUGIN-PROFILE-003.2:** Reference IDs and versions are non-secret. The host may record at most 16 of them in the plugin inventory at allocation so recovery and destroy can name the same secret. The host shall never record a value.
- **AC-EXECUTORS-PLUGIN-PROFILE-003.3:** Rotating a secret shall change its version and keep its ID. Recovery shall receive the current value under the recorded ID in `secret_values`, and the host shall reject a recovery whose recorded ID no longer exists with a named reason and without a provider call.
- **AC-EXECUTORS-PLUGIN-PROFILE-003.4:** A reference shall exist only for a property the manifest declares secret, for the same profile, and shall never be sent for a property without a stored value.
- **AC-EXECUTORS-PLUGIN-PROFILE-003.5:** No value shall appear in a reference, error, log, checkpoint, state, or event, and a provider that returns a value-bearing field in state shall be rejected by the closed state schema.

## Out of scope

- JSON Schema keywords beyond those named here, nested objects, and arrays.
- Changing how secrets are stored or the secret store itself.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
