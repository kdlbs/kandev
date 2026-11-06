---
status: done
created: 2026-10-23
requirements:
  - REQ-PLATFORM-I18N-001
system_design:
  - ../../specs/platform/system-design/i18n.md
---

# feat(i18n): localize feature toggle metadata

Localize feature toggle labels, descriptions and restart metadata through existing catalog keys in every shipped human locale, preserving technical IDs.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Changing flag defaults, feature behavior, locale selection, or handwritten pseudo generation. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.

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
