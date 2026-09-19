---
id: "03-runtime-ownership-docs"
title: "Document runtime ownership"
status: completed
wave: 2
depends_on: ["01-runtime-state-ownership"]
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 03: Document Runtime Ownership

Document the one-home ownership rule and the safe command for an isolated backend-only instance.

## Acceptance

- Public development docs state that one backend owns a Kandev home at a time.
- Raw backend commands explain their default-home behavior and show the existing isolated
  `KANDEV_HOME_DIR` command.
- The docs do not tell users to remove a stale lock file.

## Verification

Run these commands from the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files Likely Touched

- `docs/public/backend-development.md`
- `docs/public/contributing.md`
- `docs/public/configuration.md`

## Dependencies

Task 01.

## Parallelism

`sequential`. The final error behavior and ownership scope must match Task 01.

## Inputs

- Spec section **Exclusive runtime-state ownership**.
- Plan section **Public Documentation**.
- Docs-maintainer guidance for development and configuration pages.

## Risks

- Keep `make dev` distinct from raw `make dev-backend` and `make -C apps/backend dev` behavior.
- Do not expose the lock file as a user-managed control.

## Output Contract

Report the changed pages, their primary Diataxis type, exact test results, blockers, and risks.
Update this task and `plan.md` in the same conversation.

## Results

- Documented one-backend-per-home ownership in backend development, contribution, and configuration guidance.
- Documented separate home, database, and port requirements for intentional second instances.
- Verification: `node --test scripts/validate-public-docs.test.mjs` passed (58 tests); `node scripts/validate-public-docs.mjs` validated 41 published pages.
