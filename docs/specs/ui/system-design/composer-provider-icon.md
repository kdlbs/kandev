---
status: current
system: ui
requirements:
  - REQ-UI-COMPOSER-PROVIDER-ICON-001
---

# Composer provider icon design

## Purpose and boundaries

Add a presentation adornment to the existing session model picker, reusing agent
identity and logo infrastructure. No persistence or backend changes are needed.
This complements the [ACP summary](../requirements/acp-model-configuration-summary.md)
without changing its textual summary rules.

## Components and data flow

- `apps/web/components/task/model-selector.tsx`: resolve the current session's
  nonempty string `agent_profile_snapshot.agent_name`, falling back to the
  matching `agentProfiles.items` entry's `agent_name`. Snapshot identity survives
  deleted/edited profiles. Never use a database UUID or infer identity from model text.
- Add an optional `showAgentIcon` prop to `ModelSelector`, default false. Enable
  it in both `chat-input-toolbar-desktop.tsx` and `chat-input-toolbar-mobile.tsx`
  so other consumers do not change accidentally.
- `apps/web/components/model-config-selector.tsx`: accept an optional
  `providerIcon: ReactNode`, thread it into `ModelConfigSelectorTrigger`, and
  render it before the existing truncated text and beside the existing localized Model heading in the popover. Leave it absent by default.
- Use `AgentLogo` at 14px inside a nonshrinking, decorative `aria-hidden` wrapper.
  Reuse existing spacing tokens and theme-aware logo fetching. No new tooltip or
  nested interactive element. Preserve the existing accessible button name.

## Failure behavior

Unknown identity omits the icon. Known identity uses the existing `AgentLogo`
terminal placeholder during loading and on failure. React updates the adornment
from the current session identity, without retaining another session's provider.

## Desktop and phone

The nearest shipped mobile exemplar is the current chat toolbar's `ModelSelector`
inside `chat-input-toolbar-mobile.tsx`, with its `max-w-[56vw]` bounded trigger.
Keep the existing picker/popover and phone toolbar composition. Add only the
leading glyph. Preserve 28px fine-pointer trigger height and coarse-pointer 44px
minimums. Text yields width before either glyph. No new scroll region is added.

## Requirement mapping and evidence

| Criteria | Design / evidence |
| --- | --- |
| AC-UI-COMPOSER-PROVIDER-ICON-001.1 | Session identity resolution; unit coverage for snapshot priority, profile fallback, session changes and same-agent model changes |
| AC-UI-COMPOSER-PROVIDER-ICON-001.2 | Unknown identity and existing logo fallback; component tests and failed-logo browser check |
| AC-UI-COMPOSER-PROVIDER-ICON-001.3 | Optional leading slot; desktop and mobile rendered checks for truncation, hit area, keyboard/touch selection and overflow |

Public agent/profile documentation explains the CLI logo in both picker states. No new user-facing copy is
planned; any copy introduced during implementation must follow localization rules.
