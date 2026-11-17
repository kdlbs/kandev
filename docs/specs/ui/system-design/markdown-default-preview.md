---
status: current
system: ui
requirements:
  - REQ-UI-MARKDOWN-DEFAULT-PREVIEW-001
---

# Markdown default Preview system design

## Ownership and baseline

UI owns the initial editor presentation. Platform supplies settings discovery. User settings retain persistence authority.
This design uses fetched main commit `8b46ceba098d`. It imports no parent workspace changes or PR3113 changes.

## Requirement mapping

`REQ-UI-MARKDOWN-DEFAULT-PREVIEW-001` maps to persistence (.1-.2), opening (.3-.5), and settings presentation (.6) below.

## Persistence and propagation

Add boolean `markdown_preview_by_default` to the existing user settings JSON contract and `markdownPreviewByDefault` to frontend state.
Use the current model, DTO, update request, service patch application, JSON store, event publication, and boot payload paths.
Use a pointer boolean in stored JSON decoding to distinguish a missing field from explicit false. Missing values normalize to true.
Fresh settings also initialize true. PATCH omission preserves the saved value. Explicit false must survive serialization and unrelated updates.
No database schema migration or new storage mechanism is required.

Follow `lsp_status_location` through `apps/backend/internal/user/`, `internal/backendapp/boot_state_routes.go`, and `internal/settingscatalog/defaults.go`.
Include HTTP and WebSocket request conversion wherever the existing preference traverses it.
Frontend projections include `lib/types/http-user-settings.ts`, `lib/ssr/user-settings.ts`, settings slice types/defaults, and WS settings updates.
Use nullish fallback, never boolean OR, to preserve false. Extend generated settings contracts using their existing generator.
Use existing permissions, validation, concurrent-update handling, and settings broadcasts. No new telemetry is needed.

## Settings presentation and save lifecycle

Extend `editors-settings-state.tsx` and `editors-settings.tsx` with a draft and accepted baseline.
Include the field in dirty comparison, save revision, payload capture, successful baseline updates, and Reset.
Use the existing route save coordinator described in [Settings Manual Save](settings-manual-save.md) and [ADR 0046](../../../decisions/0046-settings-route-save-coordinator.md).
A failed request leaves the accepted value unchanged. Successful completion must not clear a newer local draft.
Register the setting with settings discovery and reuse existing switch/setting-row primitives and spacing.
Suggested help: “Choose the initial mode for newly opened Markdown files. You can switch modes at any time.”
Localize through the settings namespace in English, Portuguese, Simplified Chinese, both Traditional Chinese locales, Japanese, and Korean.
Generate the Traditional Chinese pair with the existing script.

The closest surface is the existing Editors preference group and its LSP setting rows.
Desktop retains the settings layout. Phone retains direct settings navigation and its single scrolling content region.
Use `SettingsRow` with `touchTarget="switch"` and a linked control ID. It connects help with `aria-describedby` and registers the discovery target.
Phone layout places the touch control below the wrapping label and description. Save/Reset remain in the existing safe-area-aware fixed action surface.
Use at least 44px for the phone/coarse-pointer activation area without enlarging fine-pointer desktop controls.
Reuse existing focus, disabled, and pressed states. Add no new animation or overlay.

## Opening and state precedence

Use `getFilePreviewKind` from `lib/utils/file-types.ts`, including its binary exclusion, as the Markdown eligibility authority.
Reuse `MarkdownPreviewContent` and existing `renderedPreview` state. Do not change rendering, editing, sanitization, or comment ownership.

Read the saved preference when an initial read completes. A settings update during loading must not request the same file again.
For desktop, initialize newly created file state in `hooks/file-editor-open-actions.ts` and the fallback loader in `components/task/file-editor-panel.tsx`.
Keep initial-mode policy small and shared if needed. Do not read the store inside a generic file-content conversion helper.
An existing buffer keeps its mode. Explicit Preview takes precedence. Current tab restoration in `hooks/use-file-editors.ts` remains authoritative.
Preserve the distinction between ordinary opens and restored tabs, including old saved tabs without a Preview flag.
Preserve request generation/session/repository checks. Ensure the fallback loader cannot overwrite the initiating open's mode when both reads resolve.

For mobile, `mobile-file-viewer-panel.tsx` initializes mode from explicit `initialRenderedPreview` or the saved default, gated by Markdown eligibility.
Apply the same initialization when repository/path identity changes. A settings broadcast or render of the same file must not reset a manual mode choice.
Keep `session-mobile-layout.tsx` explicit Preview routing intact. The existing source viewer remains read-only.

## Verification design

Backend tests cover absent JSON, fresh settings, true/false round trips, PATCH omission, unrelated patches, DTO, boot, and event projection.
Frontend tests cover missing-value hydration, explicit false, Save/Reset/failure, and concurrent draft preservation.
Opening tests cover normal opens, explicit Preview, existing buffers, fallback loading races, restored tabs, and non-Markdown/binary exclusions.
Mobile tests cover mounting, identity changes, manual toggles, and preference broadcasts.
Browser tests operate the real settings switch, Save, reload, file opening, and mode switching on desktop and phone.
Update existing tests whose ordinary Markdown opening expectations intentionally change.
