---
id: "05-frontend-disposition-control"
title: "Disposition control in the CI popover (withdrawn)"
status: withdrawn
wave: 4
depends_on: ["04-disposition-endpoint"]
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 05: Disposition control in the CI popover — WITHDRAWN 2026-08-15

This task was built and then cut from scope alongside
[task-04](task-04-disposition-endpoint.md). See that file for the reason and the
spec's Amendment history for the contract change.

**One decision from this task survives and must not be lost**, because it is
about the five retained fields and it cost a 24-file blast radius to discover:

> The five outcome fields on the frontend `TaskPR` type are declared
> `field?: T | null` — optional *and* nullable — matching this type's existing
> convention for fields added after its original shape (e.g.
> `required_reviews?: number | null`). Declaring them strictly required broke 24
> pre-existing frontend test files that construct `TaskPR` literals without every
> field. The backend never omits the key (spec AC-30), so the wire guarantee
> holds regardless; `?:` only relaxes what a hand-written test literal must
> include. This is documented inline in `lib/types/github.ts` so a future reader
> does not "fix" it back to required and reopen that blast radius.

Everything else this task produced — `patchTaskPRDisposition`,
`pr-disposition-row.tsx` and its test, the `PRCIPopover` mounting point, the
`TaskPRDisposition` union, the three disposition fields on `TaskPR`, the 10
translation keys across four catalogs, and the `i18nGuardFiles` entry — is
removed by
[task-07-narrow-to-upstream-scope](task-07-narrow-to-upstream-scope.md).
