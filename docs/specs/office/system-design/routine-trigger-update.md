---
status: current
system: office
requirements:
  - REQ-OFFICE-ROUTINE-TRIGGER-UPDATE-001
created: 2026-10-29
owners:
  - kandev
---

# Office Routine Trigger Update System Design

## Purpose and boundaries

Office owns routine triggers and their `next_run_at` cursor. This design adds an
in-place schedule update that preserves trigger identity and does not race the
scheduler's claim or re-arm. It does not change trigger creation, deletion,
dispatch, or retention.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-ROUTINE-TRIGGER-UPDATE-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow) |

## Data and contracts

`PATCH /api/v1/office/routine-triggers/:triggerId` accepts optional
`cron_expression`, `timezone`, and `enabled`. The response is the existing
`TriggerResponse` envelope with the secret cleared. A missing trigger returns
404, an invalid cron expression or timezone returns 400, and a claimed or
concurrently edited trigger returns 409.

Only the trigger's schedule fields are writable. The trigger ID, routine, kind,
webhook authentication fields, creation time, last fire, and past runs are not
part of the update.

## Control flow

The service reads the current trigger from the writer connection, applies the
requested fields, prepares the next fire with the existing trigger helper, and
writes the next row with a compare-and-set on the trigger identity plus
`cron_expression`, `timezone`, `enabled`, `next_run_at`, and `updated_at`. When
the observed `next_run_at` is null, the predicate requires the stored value to
still be null (`next_run_at IS NULL`); otherwise it requires the stored value to
equal the observed value. A re-armed non-null value therefore cannot match a null
observation, and a changed non-null value cannot match the old value. The
`updated_at` comparison also rejects a competing edit that has since restored the
same schedule values.

When the requested fields already equal the stored fields, the service writes
nothing and returns the stored row, leaving `next_run_at` and `updated_at`
untouched. A fire that is due but not yet claimed is therefore never skipped by a
repeated enable or an otherwise unchanged update.

A scheduler claim clears `next_run_at`; a stored value that no longer equals the
observed value does not match and the update returns 409 rather than overwriting
the claimed fire. An enabled cron trigger observed with no next fire is refused
before the write. Disabling clears the next fire but does not cancel a fire
already claimed.

## Failure and recovery

- The caller reloads after the dispatch re-arms and applies the edit again.
- An invalid expression or timezone leaves the stored trigger unchanged.
- A lost comparison leaves the stored trigger unchanged.
- Replacing or deleting the trigger to work around a 409 is not part of this
  contract.

## Testing

The endpoint is covered by the existing Office route scope inventory. Regression
tests live with the existing routine handler and repository test files: a no-op
update preserves a pending fire, a schedule edit preserves identity and history,
an invalid schedule returns 400, a webhook toggle disables the trigger, and a
claimed or stale revision returns 409.
