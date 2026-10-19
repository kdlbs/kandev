---
status: current
system: architecture-lint
requirements:
  - REQ-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001
---

# Root-state slice typing system design

## Purpose and boundaries

This design records a narrow compile-time boundary for the Jira and Linear
issue-watch slices and the evidence required when their entries leave the
`ARCH-FRONTEND-ROOT-STATE-CAST` baseline. Architecture-lint owns this boundary
because the baseline rule guards root-store assertions. The integrations
system continues to own Jira and Linear provider behavior, which this change
does not alter.

The Azure DevOps slice requirement is specific to that slice. Its delivery
record and the Features slice plan are implementation precedents, not
acceptance criteria for Jira or Linear. The migrated System Query owner guard
continues to protect its four System snapshots and is independent of this
root-composition contract.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-ARCHITECTURE-LINT-ROOT-STATE-SLICE-TYPING-001` | Slice composition, state preservation, baseline and roadmap evidence |

## Slice composition

`apps/web/lib/state/slices/jira/jira-slice.ts` and
`apps/web/lib/state/slices/linear/linear-slice.ts` each expose six issue-watch
actions and mutate their own state through Immer recipes. Their creator types
shall accept only the recipe setter capability they use. The root creator in
`apps/web/lib/state/store.ts` shall pass its `set` argument directly to each
slice; neither slice needs the root `get` argument, store API, replacement
state overload, or a whole-root type.

The current `types.ts` files already describe the slice state and actions.
Change them only if the smallest setter type cannot be expressed without an
assertion. Do not introduce a shared generic slice factory.

## State preservation

Each slice starts with an empty watch list and `loaded` and `loading` set to
false. Setting a list marks it loaded. The loading action changes only the
loading flag. Add appends, update replaces the matching item without changing
its position and leaves a missing ID unchanged, and remove preserves the order
of remaining items. Reset clears the list and `loaded` flag while preserving
the current loading flag. These actions retain their current signatures.

Tests shall cover defaults in an isolated Immer store, mutation isolation
between independent stores, real `createAppStore` composition, its existing
initial-state merge, and the existing `hydrateState` path. They shall also
verify unchanged references to unrelated root state after a slice action. Do
not add Jira- or Linear-specific boot or hydration routes, change
`createAppStore` merging, or alter generic `hydrateState` behavior. The
defaults-isolation assertion checks that a mutation in one store does not
affect another; it does not require replacing the current default object with
a factory.

## Baseline and roadmap evidence

`config/architecture-lint/frontend_root_state_cast.json` is the authoritative
finding inventory. At main `d7a44e96ede93d50c6277773f4174de716272f02` it has 45
entries, including three Jira and three Linear root-composition entries. Each
slice work order removes only its own three entries. Re-measure at execution
entry and report main drift before changing the expected total.

At implementation entry, current main `64f8830d0d7b1ea98da96d3373d68bc0386cf49e`
still has 45 entries with the same three Jira and three Linear identities. The
intervening main commits changed unrelated plan documentation only.

The architecture-maintenance pages retain their dated historical counts and
add any new snapshot with its date and source commit. Merged work links to its
PR and merge commit. Jira and Linear remain delivery pending while their
combined PR is open. The July source audit files remain untouched.

## Verification

- Slice, root-store, and hydration tests verify state behavior and composition.
- Web typecheck verifies the setter contract at the compiler boundary.
- The architecture scanner, focused root-state-cast rule tests, and exact
  baseline comparison verify that only the intended three entries leave per
  slice.
- Documentation catalog/specification checks and the trusted-main pure PR
  documentation evaluator verify the delivery records.

No rendered UI, touch interaction, or user-visible flow changes. Browser E2E
and mobile-composition checks are not part of this contract.
