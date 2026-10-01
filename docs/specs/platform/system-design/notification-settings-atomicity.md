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
This design repairs the approved provider/subscription persistence defect without
a schema migration, new public payload, or new authorization boundary.

## Requirement mapping

| Requirement | Criteria | Design section |
| --- | --- | --- |
| `REQ-PLATFORM-NOTIFICATIONS-001` | `AC-PLATFORM-NOTIFICATIONS-001.6`, `AC-PLATFORM-NOTIFICATIONS-001.10` | Event selection and ownership |
| `REQ-PLATFORM-NOTIFICATIONS-001` | `AC-PLATFORM-NOTIFICATIONS-001.9` | Validation and persistence |

## Validation and persistence

The service prepares the proposed provider and validates its type/config and
every supplied event before calling a write operation. Update reads the owned
provider first. Validation failure must never reach provider or subscription SQL.

The repository exposes explicit atomic create-with-subscriptions and
update-with-optional-subscriptions operations. Each operation begins one database
transaction, writes the provider, applies the supplied subscription replacement,
and commits. Any statement or commit failure propagates an error and rolls back.
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

## Failure and verification

An error is returned through the existing controller/handler path. A successful
save exposes the entire selected configuration. A failed create can be retried
without an orphan provider; failed update preserves fields, timestamps, and
subscription rows. This guarantee applies to one provider save, not a batch of
independent provider requests.

Permanent service tests use the real repository and a private temporary SQLite
database. Invalid events and test-only `BEFORE INSERT` triggers exercise rejected
updates and failed subscription insertion. Store tests exercise atomic ownership
and rollback independently; controller tests retain create default/empty coverage
and exercise update omission/empty semantics. No live instance or data is used.

## Delivery record

- [Plan and work order](../../../plans/notification-settings-atomicity/plan.md)
