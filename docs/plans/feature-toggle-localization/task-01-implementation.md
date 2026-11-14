---
id: "01-implementation"
title: "feat(i18n): localize feature toggle metadata"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-I18N-001
acceptance_criteria:
  - AC-PLATFORM-I18N-001.1
  - AC-PLATFORM-I18N-001.3
  - AC-PLATFORM-I18N-001.5
system_design:
  - ../../specs/platform/system-design/i18n.md
---

# feat(i18n): localize feature toggle metadata

## Outcome and scope

Localize feature toggle labels, descriptions and restart metadata through existing catalog keys in every shipped human locale, preserving technical IDs.

Out of scope: Changing flag defaults, feature behavior, locale selection, or handwritten pseudo generation.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4717 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/platform/system-design/i18n.md)
- [Plan](plan.md)

## Verification and results

Validation: 17 existing feature-card/settings tests passed. Three disposable mounted checks passed for catalog resolution, unknown-flag fallback and actual Korean text/accessibility. ESLint, locale-key validation and all normal commit hooks passed; temporary checks were removed and no standing tests or CI were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Settings > Features
  [ ] Localized feature label
      Localized explanation
      Localized restart notice

Desktop and phone use the same existing rows; only displayed copy changes.
```

This sketch records the affected structure; spacing is illustrative. It does not add a new control or approval requirement.
