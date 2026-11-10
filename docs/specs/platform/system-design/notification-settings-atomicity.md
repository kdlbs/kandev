---
status: current
system: platform
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
---

# Notification Settings Atomicity

## Boundaries

Platform owns the notification configuration saved through
`internal/notifications/controller`, `service`, and `store`. Provider adapter
validation, semantic delivery, and the settings UI remain their existing contracts.
This design covers provider/subscription persistence and mounted Save
acknowledgement without a schema migration, new public payload, or new
authorization boundary.

## Requirement mapping

| Requirement | Criteria | Design section |
| --- | --- | --- |
| `REQ-PLATFORM-NOTIFICATIONS-001` | `AC-PLATFORM-NOTIFICATIONS-001.6`, `AC-PLATFORM-NOTIFICATIONS-001.10` | Event selection and ownership |
| `REQ-PLATFORM-NOTIFICATIONS-001` | `AC-PLATFORM-NOTIFICATIONS-001.9` | Validation and persistence |
| `REQ-PLATFORM-NOTIFICATIONS-001` | `AC-PLATFORM-NOTIFICATIONS-001.12` | Mounted Save acknowledgement |

## Validation and persistence

The service prepares the proposed provider and validates its type/config and
every supplied event before calling a write operation. Update reads the owned
provider first. Validation failure must never reach provider or subscription SQL.

The repository exposes explicit atomic create-with-subscriptions and
update-with-optional-subscriptions operations. Each operation begins one database
transaction, writes the provider, applies the supplied subscription replacement,
and commits. Statement failures roll back the transaction and propagate an error.
A confirmed transaction abort preserves the previous configuration. If the
connection fails during PostgreSQL commit, the server may have committed before
its acknowledgement was lost; an error then has an unknown persisted outcome.
Deferred rollback cannot undo a transaction already committed by the server.
Subscription delete and inserts share that transaction with the provider write.
Existing low-level provider and subscription methods may remain for seed/migration
callers and tests, sharing SQL helpers with the atomic operations.

The dialect-rebinding SQL path is retained for SQLite and PostgreSQL. No tables,
columns, schema history, or delivery deduplication behavior changes. Default Local
and System creation also uses the atomic create operation so a failed subscription
write cannot leave an incomplete default provider that later startup skips.

## Event selection and ownership

Controller create defaulting continues to distinguish omitted/null `events` from
an explicit empty array. Update passes the existing optional event selection to
the store: no selection preserves subscriptions; a supplied empty selection clears
them. The store must not infer omission from slice length.

Atomic update SQL retains both provider ID and owner ID predicates and checks
affected rows before replacing subscriptions. An absent/foreign row returns
`ErrProviderNotFound` and rolls back without touching subscriptions. Atomic create
derives subscription ownership from the new provider's user ID.

## Mounted Save acknowledgement

`AC-PLATFORM-NOTIFICATIONS-001.12` specializes the shared
[manual Save contract](../../ui/requirements/settings-manual-save.md) for the
mounted notification contributor. At request admission, capture the provider
snapshot, pending deletion IDs, raw Apprise buffers and create-form identity.
On success, publish canonical responses over that submitted snapshot to the
existing saved-provider owner and advance the baseline. Reconcile the current
draft by provider identity: apply canonical fields only where the current value
still equals the submitted value. Preserve subsequent event-membership changes,
provider fields, raw name/URL buffers, removed providers and deletion intentions.
Remove only acknowledged deletion IDs from the pending set. New canonical
providers join the baseline and draft, while only the unchanged submitted create
form is cleared; a replacement or edited form survives. Reset uses the advanced
baseline. The existing shared coordinator retains Save admission and error policy.

This does not migrate cache ownership, change the API, introduce a generic
coordinator, or promise batch atomicity, exactly-once creation, recovery from
ambiguous transport outcomes, or cross-request server ordering. Partial batch
progress on failure remains the existing limitation.

The shipped notification event desktop table and native phone stacked list use
the same state/actions. This repair changes only draft reconciliation; layout,
navigation, touch handlers, scrolling and copy remain unchanged. Transport-held
integration tests exercise the real provider, API and coordinator; managed
real-backend browser checks use the existing controls and disposable data only.

## Failure and verification

An error is returned through the existing controller/handler path. A successful
save exposes the entire selected configuration. A create rolled back before commit
leaves no orphan provider; an aborted update preserves fields, timestamps, and
subscription rows. After an ambiguous commit
error, reload the saved configuration before retrying, especially creation.
Either the entire configuration was committed or none of it was; partial provider
and subscription state is never a valid outcome. This guarantee applies to one
provider save, not a batch of independent provider requests.

Permanent service tests use the real repository and a private temporary SQLite
database. Invalid events and test-only `BEFORE INSERT` triggers exercise rejected
updates and failed subscription insertion. Store tests exercise atomic ownership
and rollback independently; controller tests retain create default/empty coverage
and exercise update omission/empty semantics. No live instance or data is used.

## Delivery record

- [Draft preservation plan and work order](../../../plans/notification-save-drafts/plan.md)
- [Plan and work order](../../../plans/notification-settings-atomicity/plan.md)
