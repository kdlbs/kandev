---
id: "04-publish-plugin-action-contract"
title: "Publish the plugin action contract"
status: done
wave: 4
depends_on:
  - "03-prove-native-desktop-and-phone-flows"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
acceptance_criteria:
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.2
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.3
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.4
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.5
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.7
system_design:
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
---

# Task 04: Publish the Plugin Action Contract

## Summary

Document the manifest declaration, request, response, validation, failure, and
security rules for server-side repository inspection. Keep public and internal
author contracts aligned with the verified implementation.

## In scope

- Add a public `repositories.inspect` section beside the persisted repository
  branch action.
- Explain that frontend `inspectURL` supports browser display and URL entry,
  while the backend action is required for first-use task persistence.
- Document the preferred nested repository response and any bounded direct
  response compatibility.
- Document verified workspace context, manifest owner selection, timeout and
  size limits, no-match behavior, typed action errors, and credential-safe URL
  handling.
- Update the internal plugin API contract and any manifest examples that show a
  repository provider capable of native first-use task creation.
- Run public documentation and specification validators.

## Out of scope

- General plugin tutorials unrelated to repository providers.
- External plugin release notes.
- A new public REST task-create field.

## Acceptance

- A plugin author can implement the inspect action without reading Kandev
  source code.
- The docs clearly separate untrusted browser selection from authoritative
  server resolution.
- Examples do not include credentials and do not imply that the request URL is
  an allowed credential origin.
- Public and internal action shapes match production tests.

## Verification

Run the relevant documentation checks from their documented working
directories, then run:

```bash
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Also run the public docs link, heading, and build commands required by
`docs/public/README.md`.

## Files likely touched

- `docs/public/plugins-authoring.md`
- `docs/public/plugins-manifest.md` if action examples need manifest context
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/specs/plugins/requirements/repository-provider-task-creation.md`
- `docs/specs/plugins/system-design/repository-provider-task-creation.md`
- `docs/decisions/2026-08-26-server-owned-plugin-repository-task-resolution.md`

## Dependencies

- Task 03 confirms the final request, response, errors, and limits.

## Risks

- Calling frontend `inspectURL` authoritative would reintroduce the browser
  trust error. Name each boundary explicitly.
- Publishing a response shape before compatibility behavior is tested can lock
  in ambiguity. Document only the shapes accepted by focused tests.
- Copying provider-specific Bitbucket rules into the host contract would make
  the action less reusable. Keep the contract provider-neutral.

## Parallelism

`sequential`

## Inputs

- Verified implementation and E2E results from Tasks 01 through 03.
- `docs/public/plugins-authoring.md` persisted branch action section.
- `docs/plans/plugins/PLUGIN-API.md` repository provider contract.
- `ADR-2026-08-26-server-owned-plugin-repository-task-resolution`.

## Results

Updated the public manifest and authoring references plus the internal plugin
API contract. The docs distinguish browser display hints from authoritative
server inspection and document the request body, preferred nested response,
compatibility response, validation rules, limits, and safe failure behavior.

Verification passed:

```text
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```
