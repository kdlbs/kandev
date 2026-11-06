---
id: "04-report-coverage-and-e2e"
title: "Report coverage and prove desktop/mobile counting"
status: done
wave: 4
depends_on:
  - "03-project-durable-counts"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-001
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-002
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-003
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-004
  - REQ-UI-CONTEXT-COMPACTION-COUNT-001
acceptance_criteria:
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.2
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.3
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.1
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.3
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.1
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-003.2
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.1
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.2
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.3
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.4
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.5
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.6
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.7
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.1
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.6
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.7
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.8
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.13
system_design:
  - ../../specs/agents/system-design/copilot-native-compaction.md
  - ../../specs/ui/system-design/context-compaction-count.md
---

# Task 04: Report coverage and prove desktop/mobile counting

## Summary

Expose source/coverage through the existing count surface and prove real native-event counting through the whole stack.
Use a Copilot-scoped synthetic wire peer, not a seeded browser count, for desktop and phone acceptance.

## In scope

- Extend parser, session-runtime types, WS metadata handling, boot hydration, and session hook for optional reporting metadata.
- Preserve count-only native increments when the gauge is absent, and preserve them when its first root sample arrives.
- Reject stale reporting revisions and prevent delayed gauge/HTTP patches from lowering a newer lifetime count. Use controlled deferred responses in tests.
- Render supported help and partial/unavailable statuses without describing the aggregate as wholly confirmed.
- Keep legacy inferred-provider help and historical totals. Current availability must not clear sticky partial coverage.
- Add seven-language copy, generated Traditional Chinese/pseudo, and public session documentation updates during implementation.
- Add a fixture-only raw JSON-RPC mode in `cmd/mock-agent` that emits the exact vendor method using one serialized writer.
- Register the synthetic peer as `copilot-acp` using existing mock-provider identity support. Require the real initialize subscription.
- Cover two child calls with zero increments, one applied parent completion, duplicate completion, parent-only gauge, reload, and isolated backend restart.
- Cover legacy total 2 plus one new event yielding 3, partial/unavailable status, and completion before the first gauge sample.
- Use the existing context-window and compaction timeline patterns for desktop and phone; preserve touch toggling, focus, and containment.

## Out of scope

- Store-bridge injection as counter evidence, production provider validation exceptions, a new overlay/navigation, or a second mobile business-logic path.
- Native-history backfill, real customer sessions, extra RPC permissiveness, or changing existing UI control sizes.

## Acceptance

1. Unit/component tests preserve source/coverage/count across live and hydrated metadata, including count-only updates and historical totals.
2. Real raw vendor notifications traverse the scoped adapter, durable journal/projector, SQL, WS, and visible count, retaining exactly one increment after duplicate/reload/restart.
3. Desktop and phone expose the same honest coverage and localized explanation without horizontal overflow. All documentation/spec lifecycle results match actual evidence.

## UI preview

**UI-01: Context-window count with native coverage.** See the [full package preview](plan.md#ui-preview).

| State | Shared desktop/phone region |
| --- | --- |
| Available, accepted parent event | Existing token/source row; Compactions: 1; info |
| Known missing evidence | Existing token/source row; Compactions: N; Partial; info |
| Unsupported collection | Existing token/source row; Compactions: N; Unavailable; info |
| Historical 2 plus native 1 | Compactions: 3; explanation retains historical-estimate disclosure |

Desktop uses hover/focus. Phone taps the current pinned surface and info control.
No separate phone composition, overlay, or scroll owner is introduced.
The existing `mobile-context-window-source.spec.ts` is the nearest phone exemplar.
Structural requirements are the current hierarchy, visible status, and accessible explanation. Copy/spacing are illustrative.

## Verification

Run from the repository root, installing workspace dependencies once before package commands.
Managed E2E rebuilds current assets and uses one shard. Run projects sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -tags fts5 ./cmd/mock-agent -run '^TestCopilotNative' -count=1)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm exec vitest run lib/state/slices/session-runtime/context-window.test.ts lib/ws/handlers/agent-session.test.ts hooks/domains/session/use-session-context-window.test.ts components/task/chat/token-usage-display.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/copilot-native-compaction.spec.ts tests/chat/context-window-source.spec.ts tests/chat/context-compaction.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-copilot-native-compaction.spec.ts tests/chat/mobile-context-window-source.spec.ts tests/chat/mobile-context-compaction.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run focused Oxlint for every changed TS/TSX file and record the full invocation.
Record test discovery counts and screenshots of the existing region in each coverage state.
A passing fixture that writes a count directly into the browser store does not satisfy acceptance.
Rerun the package's documentation-coverage preflight after recording results.
Promote paired draft requirements/designs and the plan only after all four work orders and qualification gates pass.

## Files likely touched

- `apps/web/lib/state/slices/session-runtime/types.ts`, `context-window.ts`, and focused tests.
- `apps/web/lib/ws/handlers/agent-session.ts` and its tests.
- `apps/web/hooks/domains/session/use-session-context-window.ts` and its tests; hydration merge path if needed.
- `apps/web/components/task/chat/token-usage-display.tsx` and its tests.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/common.json` (or the existing owning namespace for added status copy).
- `apps/backend/cmd/mock-agent/copilot_native_wire.go` and `copilot_native_wire_test.go` (new fixture mode), plus fixture-only launch selection.
- `apps/web/e2e/tests/chat/copilot-native-compaction.spec.ts`, `mobile-copilot-native-compaction.spec.ts`, and shared helpers (new).
- Existing context-window/timeline E2E helpers and worker fixtures for the isolated Copilot provider and restart.
- `docs/public/sessions-and-review.md`, paired specs, and this package's result/status sections.

## Dependencies

Task 03: durable count projection and reporting metadata. Tasks 01/02 supply transport qualification and normalization.

## Risks

Count-only metadata and gauge hydration can overwrite each other without explicit merge coverage.
The raw fixture must preserve the exact method and provider identity rather than bypass transport or attribution.
Long translated coverage/help copy must remain usable on the phone surface.

## Parallelism

`sequential`

## Inputs

- [Provider design](../../specs/agents/system-design/copilot-native-compaction.md), reporting, security, and verification sections.
- [Shared count design](../../specs/ui/system-design/context-compaction-count.md), live/hydrated count semantics.
- Listed requirements and [UI-01](plan.md#ui-preview).
- `NewMockAgentWithID`, scoped mock-agent guidance, existing context/timeline fixtures, `/mobile-parity`, `/e2e`, `/docs-maintainer`, and `docs/i18n.md`.

## Results

Done.

- Parser, session-runtime types, WS handler, boot hydration, and session hook preserve optional reporting metadata.
- Count-only native updates patch an existing gauge without inventing a zero-token sample. Delayed gauge/HTTP updates cannot lower a newer lifetime count; stale reporting revisions keep the newer metadata.
- `ContextCompactionCount` renders Available without a badge, Partial, and Unavailable, with distinct localized explanations. All seven languages, Traditional Chinese, and pseudo catalogs pass `i18n:check` and the new-code ratchet.
- `docs/public/sessions-and-review.md` explains native counting, older runtimes, partial coverage, retained historical estimates, and missed events.
- `cmd/mock-agent` has an opt-in serialized raw JSON-RPC fixture that emits the exact unprefixed method, parent/child usage, one applied parent completion, a duplicate, and a failed completion.
- Desktop Playwright passed 6 tests: native compaction 2, context-window-source 3, context-compaction 1.
- Phone Playwright passed 5 tests: native compaction 1, mobile context-window-source 3, mobile context-compaction 1.
- The E2E assertions use the real adapter, durable journal/projection, SQL, WS, reload, and backend restart. They do not seed the counter through the browser store.

Review remediation added: explicit gauge clears retain the lifetime count/reporting; deferred snapshots cannot lower the shared session count or reporting revision; a late first gauge inherits the retained count; hydration repairs a missed live notification and survives cache invalidation. Focused WS, merge, and hook regressions pass.

Limitations: unavailable collection is covered by unit/component and orchestrator tests rather than a live native-disabled Copilot E2E. Automatic compaction was not captured live.
