---
id: "04-document-service-identity-recovery"
title: "Document service identity recovery"
status: done
wave: 3
depends_on:
  - "01-preserve-system-service-user"
  - "02-harden-origin-reconciliation"
  - "03-resolve-resume-repositories-once"
plan: "plan.md"
spec: "../../specs/cli/requirements/native-kandev-cli.md"
---

# Task 04: Document Service Identity Recovery

## Acceptance

- Public service documentation explains preserved identity, `--run-as`, first root-shell install
  behavior, and safe recovery from a service/home owner mismatch.
- CLI reference documents the flag and rejects the implication that it applies to user services or
  non-install actions.
- GitHub integration documentation identifies managed-checkout ownership errors and does not
  recommend `safe.directory=*` or automatic chown.
- Public-doc metadata/links remain valid and both documentation validators pass.

## TDD Sequence

1. Update public service, CLI, and integration pages to match the implemented command and error
   wording.
2. Add or update validator assertions if the public CLI contract has structured coverage.
3. Run both validators and fix only issues caused by these documentation changes.
4. Mark every task and the parent plan complete only after implementation verification is recorded.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files Likely Touched

- `docs/public/run-as-a-service.md`
- `docs/public/cli.md`
- `docs/public/integrations.md`
- `scripts/validate-public-docs.test.mjs` only if contract coverage requires it
- this task file and `plan.md`

## Dependencies

Tasks 01-03, so documentation uses the final CLI and diagnostic wording.

## Parallelism

Sequential after behavior tasks.

## Recorded Results

- Updated `docs/public/run-as-a-service.md`, `docs/public/cli.md`, and
  `docs/public/integrations.md` with identity preservation, `--run-as`, ownership preflight, and
  dubious-ownership recovery guidance.
- `node --test scripts/validate-public-docs.test.mjs` passed all 58 structural tests.
- `node scripts/validate-public-docs.mjs` validated all 41 published pages.
- Final backend verification passed: `cd apps/backend && make test` and `cd apps/backend && make lint`.
- Operator migration caveat: existing homes with an owner mismatch require explicit operator
  reconciliation before reinstall; Kandev does not auto-chown or add Git trust exceptions.

## Output Contract

Report pages changed, both validator results, final backend verification results from the parent
plan, and any operator migration caveat. Mark the plan complete only when all task acceptance
criteria are satisfied.
