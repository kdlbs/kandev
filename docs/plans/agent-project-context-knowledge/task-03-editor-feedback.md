---
id: "03-editor-feedback"
title: "Advisory editor feedback and browser evidence"
status: complete
wave: 3
depends_on:
  - "01-starter-files"
  - "02-agent-guidance"
plan: "plan.md"
requirements:
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-001
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-003
acceptance_criteria:
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-001.1
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-001.4
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-001.5
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-003.1
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-003.2
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-003.3
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-003.4
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-003.5
  - AC-PROJECTS-CONTEXT-KNOWLEDGE-003.6
system_design:
  - ../../specs/projects/system-design/context-knowledge.md
---

# Task 03: Advisory Editor Feedback and Browser Evidence

## Summary

Show current-draft format information inside the existing text editor. Prove
desktop and phone editing, shared visibility, preservation, and conflict recovery.

## In scope

- Use TDD for a pure, bounded inspector with the installed `yaml` library.
- Cover legacy text, reserved files, custom types/keys, invalid headers, duplicate
  keys, versions, BOM/CRLF, fenced examples, and the 64 KiB inspection limit.
- Add localized, wrapping feedback without altering content or save eligibility.
- Extend current Projects desktop/mobile E2E with structured defaults, current
  draft feedback, permissive saves, shared reads, and conflict recovery.
- Measure toolbar actions, viewport containment, and editor focus.
- Update public explanation and scoped project guidance after behavior exists.
- Record this work order's results. The coordinator promotes the package's
  draft specs after all five work orders and final review pass.

## Out of scope

- YAML serialization, new file APIs, a preview renderer, and trust scoring.
- New overlays, controls, navigation, or a persistent format state/cache.
- A broad QA audit, full test suite, commit, or push as part of this work order.

## Acceptance

- Inspection is derived from path and draft, handles bounded/error cases, and
  preserves arbitrary submitted text and metadata.
- All file formats can be saved under the existing dirty/busy/conflict contract.
  Advice is localized, accessible, and usable at desktop and phone dimensions.
- Focused tests and both browser projects pass against fresh builds, with public
  docs and final delivery/spec statuses accurate.

## ASCII UI preview

The [plan](plan.md#ascii-ui-preview) owns the full preview and geometry notes.
This work implements `UI-01` and `UI-02`, covering the six `003` criteria.

`UI-01`: desktop, Files -> Context -> `notes.md`.

```text
[Back]  notes.md                              [Save]
Structured Markdown
Type is missing. You can still save.
+-------------------------------------------------+
| Raw Markdown editor                             |
+-------------------------------------------------+
```

`UI-02`: focused phone Files -> Context -> `notes.md`.

```text
[Back]  notes.md             [Save]
Structured Markdown
Type is missing.
You can still save.
+--------------------------------+
| Raw Markdown editor            |
+--------------------------------+
```

Advice is noninteractive and wraps. The textarea remains the content scroll
owner. Keep desktop actions at 28px and phone/coarse-pointer hit targets at least
44px. Labels are illustrative, not literal production copy.

## Verification

Run from the repository root. The managed E2E runner builds and cleans isolated
instances. Run browser commands sequentially with one shard.

```bash
pnpm --dir apps/web exec vitest run lib/agent-projects/context-format.test.ts hooks/domains/agent-projects/use-agent-project-context.test.tsx lib/api/domains/agent-projects-api.test.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec eslint lib/agent-projects/context-format.ts components/task/agent-project-context-panel.tsx e2e/tests/projects/agent-projects.spec.ts e2e/tests/projects/mobile-agent-projects.spec.ts
pnpm --dir apps/web run i18n:zh-hant
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/projects/agent-projects.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/projects/mobile-agent-projects.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the existing dependencies. If a new checkout lacks them, first run
`pnpm --dir apps install --frozen-lockfile`.

## Files likely touched

- Proposed `apps/web/lib/agent-projects/context-format.ts` and `.test.ts`.
- `apps/web/components/task/agent-project-context-panel.tsx`.
- `apps/web/hooks/domains/agent-projects/use-agent-project-context.test.tsx`,
  only for additional preservation/conflict coverage that is not already proven.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/projects.json`.
- `apps/web/e2e/tests/projects/agent-projects.spec.ts`.
- `apps/web/e2e/tests/projects/mobile-agent-projects.spec.ts`.
- `docs/public/tasks-and-workflows.md`.
- `apps/backend/internal/projects/AGENTS.md`.
- New requirement/design statuses, this plan, and linked work-order Results.

## Dependencies

[Task 01](task-01-starter-files.md) supplies starter files.
[Task 02](task-02-agent-guidance.md) supplies the authoring contract documented
by the browser-facing explanation.

## Risks

- Parsing a large body on every keypress or expanding arbitrary aliases.
- Showing stale advice from saved content instead of the current draft.
- Accidentally changing comments or optional metadata through serialization.
- Turning advisory structure feedback into a blocking save error.
- Wrapping advice clips the editor or creates document-level phone overflow.
- E2E runs test stale binaries or fixtures instead of the changed implementation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/projects/requirements/context-knowledge.md)
- [Inspection and presentation design](../../specs/projects/system-design/context-knowledge.md#advisory-inspection)
- Current `AgentProjectContextFile`, raw API contracts, and hook regressions.
- `apps/web/AGENTS.md`, `/mobile-parity`, `/e2e`, `/tdd`, and
  `/docs-maintainer`.
- The existing Projects E2E request/event waits and mock-worker scenario.

## Results

Implemented the bounded raw-draft inspector, localized advisory feedback, and
desktop/phone context coverage. The desktop test verifies the seeded index,
current-draft feedback, exact raw-content save and shared worker read, then
forces a hash conflict and confirms the worker draft stays editable. The phone
test verifies wrapped advice, editor focus, no horizontal overflow, 44px touch
targets for file and directory Back actions, and shared raw-content visibility.

Focused verification passed:

- 32 Vitest tests across the context inspector, context hook, and project API.
- Web typecheck and changed-source ESLint with `--max-warnings=0`.
- `i18n:zh-hant`, `i18n:check`, and `i18n:ratchet`.
- Managed Chromium Projects E2E: 4/4 tests passed.
- Managed mobile-chrome Projects E2E: 1/1 test passed.
- Public-doc tests and validation, spec validation/lint, and `git diff --check`.

Stable phone context evidence: `/tmp/kandev-projects-refinements-evidence/task03-phone-context-advice.png`.
The coordinator also captured desktop advice, phone sizing, and conflict states
in `/tmp/kandev-iso-48430-Y3Brv1/`.
