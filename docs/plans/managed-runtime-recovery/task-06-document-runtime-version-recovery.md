---
id: "06-document-runtime-version-recovery"
title: "Document runtime version recovery"
status: done
wave: 5
depends_on: ["04-add-responsive-version-recovery-ui"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 06: Document runtime version recovery

## Acceptance

- Public agent/profile guidance explains selecting update, rollback, and repair
  targets and that activation follows a successful ACP validation.
- The guide states that the active exact version survives Kandev restarts,
  affects only later host-local probes/utility calls/sessions, and does not
  change active or remote/container sessions.
- Troubleshooting covers incomplete latest releases, failed candidates,
  authentication-required validation, and retrying another stable version.
- The guide does not recommend global npm cache cleanup or imply Kandev owns an
  offline artifact inventory.

## Implementation

1. Replace the current latest-only update steps in the existing how-to section.
2. Add a concise troubleshooting entry for selecting the previous stable
   version when latest cannot initialize.
3. Check all labels and guarantees against the implemented API/UI and spec.
4. Run documentation link and formatting checks available in the repository.

## Files likely touched

- `docs/public/agents-and-profiles.md`

## Verification

```bash
rtk rg -n "Update a managed agent runtime|Roll back runtime|Repair runtime|npm cache" docs/public/agents-and-profiles.md
rtk rg -n "docs/public|markdown" Makefile apps/package.json package.json
```

Run the repository documentation check discovered by the second command and
record the exact command/result.

## Risks

- Describe host scope and persistence precisely; do not promise remote updates
  or cached offline reinstall.
- Keep this as an operator how-to, not an implementation narrative.

## Output contract

Record changed sections, checks, and risks in Results. Update this task and
`plan.md` status.

## Results

Updated `docs/public/agents-and-profiles.md` with exact-version selection,
update/rollback/repair behavior, ACP validation, restart and host-local scope,
active-session and remote/container boundaries, failed-candidate preservation,
and retry guidance. GREEN verification:

- `rtk node --test scripts/validate-public-docs.test.mjs` — 60 tests passed.
- `rtk node scripts/validate-public-docs.mjs` — 41 published docs pages validated.

The guide does not promise global npm cache cleanup or an offline artifact
inventory.
