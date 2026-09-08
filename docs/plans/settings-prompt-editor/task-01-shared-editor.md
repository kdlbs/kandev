---
id: "01-shared-editor"
title: "Build shared prompt editor"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-prompt-editor.md"
---

# Task 01: Build shared prompt editor

## Acceptance

- `SettingsPromptEditor` provides controlled plain text, configured placeholders, optional saved-prompt references, exclusions, accessibility, dirty state, and localized hints.
- Two mounted Monaco editors return completion items only for their own model.
- Provider updates and unmounts dispose the correct Monaco registrations without changing existing script-editor behavior.

## Verification

Follow TDD. Add the model-isolation and shared-component tests first. Make sure that they fail against the language-wide provider ownership.

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/settings/settings-prompt-editor.test.tsx components/settings/profile-edit/script-editor.test.tsx components/settings/profile-edit/script-editor-completions.test.ts && cd web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet && pnpm exec eslint components/settings/settings-prompt-editor.tsx components/settings/settings-prompt-editor.test.tsx components/settings/profile-edit/script-editor.tsx components/settings/profile-edit/script-editor.test.tsx components/settings/profile-edit/script-editor-completions.ts components/settings/profile-edit/script-editor-completions.test.ts
```

## Files likely touched

- `apps/web/components/settings/settings-prompt-editor.tsx`
- `apps/web/components/settings/settings-prompt-editor.test.tsx`
- `apps/web/components/settings/profile-edit/script-editor.tsx`
- `apps/web/components/settings/profile-edit/script-editor.test.tsx`
- `apps/web/components/settings/profile-edit/script-editor-completions.ts`
- `apps/web/components/settings/profile-edit/script-editor-completions.test.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pt-pt/settings.json`
- `apps/web/src/locales/zh-cn/settings.json`
- `apps/web/src/locales/zh-hk/settings.json`
- `apps/web/src/locales/zh-tw/settings.json`

## Dependencies

None.

## Parallelism

Sequential. All later tasks depend on this component and its provider ownership contract.

## Inputs

- Spec: `What`, `Failure modes`, and the model-isolation scenarios.
- Plan: `Shared prompt editor`, `Monaco completion ownership`, and `Risks`.
- Existing patterns: `ScriptEditor`, `useCustomPrompts`, and `script-editor-completions.test.ts`.

## Output contract

Report the RED error, final API, model-isolation method, files changed, exact command results, blockers, risks, and synchronized task and plan status.

## Results

- The initial browser run exposed a lifecycle race: saved prompts loaded before Monaco mounted, but the one-shot `onMount` callback still held an empty prompt list. The editor now keeps the latest registration callbacks in refs and registers per-model providers.
- `ScriptEditor` owns one primary and one saved-prompt registration per mounted model, scopes both through `scopeCompletionProviderToModel`, and disposes them independently.
- `SettingsPromptEditor` owns the controlled plain-text contract, localized completion hints, dirty attributes, stable editor attributes, prompt loading, and current-prompt exclusions.
- Focused shared-editor/component/completion tests pass: 3 files, 13 tests; the regression test covers prompt data loading before Monaco mount.
- `pnpm run typecheck` and `pnpm exec tsc --noEmit --incremental false` pass.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` pass.
