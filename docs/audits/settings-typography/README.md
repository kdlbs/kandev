# Settings typography audit

Status: Active implementation audit
Last reviewed: 2026-08-16

This audit covers the first-party settings routes, shared settings components, the settings navigation, workspace and integration settings, system and account settings, and the existing settings E2E coverage.

The goal is a role-based typography system. Settings should not use one font size for every piece of content, but the same semantic role must render with the same family, size, weight, and line height across pages. Technical values such as commands, paths, identifiers, tokens, and logs are intentional exceptions and are tracked separately.

The audit rollout notes are tracked in [plan.md](plan.md). The formal design
package is [the spec](../../specs/ui/requirements/settings-typography.md), [the canonical
implementation plan](../../plans/settings-typography/plan.md), and its linked
task files.

## Finding inventory

| ID | Finding | Priority | Status |
| --- | --- | --- | --- |
| [TYPO-001](001-define-settings-typography-contract.md) | Define a shared settings typography contract | P0 | Resolved |
| [TYPO-002](002-consolidate-settings-page-shells.md) | Consolidate duplicated page-shell typography | P1 | Resolved |
| [TYPO-003](003-normalize-card-and-section-headings.md) | Normalize card and section heading roles | P1 | In progress |
| [TYPO-004](004-normalize-settings-field-labels.md) | Normalize field labels and label emphasis | P1 | In progress |
| [TYPO-005](005-normalize-settings-helper-text.md) | Normalize descriptions, helpers, and status copy | P1 | In progress |
| [TYPO-006](006-normalize-settings-control-typography.md) | Normalize input, select, and textarea typography | P1 | In progress |
| [TYPO-007](007-remove-agent-profile-compact-drift.md) | Remove agent profile compact typography drift | P1 | In progress |
| [TYPO-008](008-document-settings-micro-type-exceptions.md) | Define limits for micro-type and metadata | P2 | Resolved |
| [TYPO-009](009-centralize-settings-font-family-roles.md) | Centralize UI and technical font-family roles | P1 | Resolved |
| [TYPO-010](010-align-mobile-settings-type-scale.md) | Align mobile and desktop settings type roles | P1 | In progress |
| [TYPO-011](011-align-workspace-and-provider-settings.md) | Align workspace and provider form typography | P1 | In progress |
| [TYPO-012](012-expand-settings-typography-verification.md) | Expand settings typography verification beyond notifications | P2 | In progress |
| [TYPO-013](013-normalize-settings-copy-punctuation.md) | Normalize settings description punctuation | P2 | Resolved |

## Status vocabulary

- `Open`: identified and not implemented.
- `In progress`: implementation is actively changing the affected surface.
- `Resolved`: implementation and focused verification are complete.
- `Won't fix`: reviewed and intentionally retained with a recorded reason.

## Suggested role map for the follow-up plan

The exact values should be agreed before implementation, then encoded in shared settings primitives or semantic utility classes.

| Role | Current examples | Direction |
| --- | --- | --- |
| Page title | `text-2xl font-bold` | One page-title role across every settings shell |
| Page description | `text-sm text-muted-foreground` | One relaxed, readable description role |
| Section title | `text-lg font-semibold` | One section-heading role |
| Card title | `text-sm` or `text-base` | One settings-card-title role |
| Field label | bare `Label`, `text-xs`, or `text-sm` | One primary field-label role, with a documented compact exception |
| Helper/description | `text-xs`, `text-sm`, and custom leading | Separate page, field-help, status, and error roles |
| Control value | `text-sm`, `text-xs`, and responsive overrides | One settings-control role with a deliberate mobile rule |
| Technical value | `font-mono` plus several sizes | Preserve mono only for code, identifiers, paths, tokens, and logs |

## Audit method and boundaries

The audit used a source scan of the settings route map, shared UI primitives, settings components, and mobile/desktop E2E tests. Five read-only subagents independently covered top-level pages, agents and executors, workspace and integrations, shared/mobile behavior, and system/account surfaces. Delayed passes added Utility Agents, legacy Integrations, executor-card, marketplace, badge, mobile-action, copy-punctuation, provider credential, workflow-label, automation-helper, Sentry-heading, semantic-card-heading, anti-zoom-control, navigation-search, editor-token, notification-coverage, SSH-card, system/account-card, dense-table, storage-policy, and editor-route evidence tracked above. No product code was changed during the audit.

The next implementation pass should include rendered desktop and Pixel 5 checks. Source evidence alone cannot prove wrapping, perceived hierarchy, or whether translated labels fit the selected scale.
