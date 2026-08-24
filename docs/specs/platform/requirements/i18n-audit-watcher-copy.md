---
status: draft
system: platform
created: 2026-08-12
owners:
  - Kandev
---
# Watcher And Task Fallback Localization Requirements

## Overview

Watcher dialogs and inter-task message badges expose several English-only fallback labels even when another locale is active. Obsolete catalog entries also obscure whether the current UI is fully covered by localization checks.

## Requirements

### REQ-PLATFORM-I18N-AUDIT-WATCHER-COPY-001: Watcher And Task Fallback Localization

**Intent:** Watcher dialogs and inter-task message badges expose several English-only fallback labels even when another locale is active. Obsolete catalog entries also obscure whether the current UI is fully covered by localization checks.

#### Acceptance criteria

- **AC-PLATFORM-I18N-AUDIT-WATCHER-COPY-001.1:** Watcher profile selectors display the localized equivalent of `(use step default)` while their stored sentinel and submitted empty profile ID remain locale-independent.
- **AC-PLATFORM-I18N-AUDIT-WATCHER-COPY-001.2:** Watcher repository and branch selectors localize the no-repository option, default-branch option, repository-first prompt, and loading prompt at render time. Repository IDs, branch names, and sentinel values remain untranslated.
- **AC-PLATFORM-I18N-AUDIT-WATCHER-COPY-001.3:** Inter-task sender badges display a localized unknown-task fallback when neither a live title nor a snapshot title exists.
- **AC-PLATFORM-I18N-AUDIT-WATCHER-COPY-001.4:** English catalog entries with no current source reference and no current user-interface owner are removed from every catalog. Current behavior is not changed merely to retain an obsolete key.
- **AC-PLATFORM-I18N-AUDIT-WATCHER-COPY-001.5:** Translation lookup does not occur at module scope.
- **AC-PLATFORM-I18N-AUDIT-WATCHER-COPY-001.6:** **GIVEN** any GitHub issue/review, GitLab issue/review, Jira, or Linear watcher dialog is rendered in a non-English locale, **WHEN** no agent or executor profile override is selected, **THEN** the step-default option and placeholder use that locale while saving still submits an empty profile ID.
- **AC-PLATFORM-I18N-AUDIT-WATCHER-COPY-001.7:** **GIVEN** a watcher repository picker is rendered in a non-English locale, **WHEN** repository and branch selection move through empty, loading, and loaded states, **THEN** only UI fallback copy is translated and all repository/branch domain values remain unchanged.
- **AC-PLATFORM-I18N-AUDIT-WATCHER-COPY-001.8:** **GIVEN** an inter-task message has no resolvable live or snapshot task title, **WHEN** its sender badge renders, **THEN** the badge and tooltip use the localized unknown-task fallback.

## Migrated source detail

## Why

Watcher dialogs and inter-task message badges expose several English-only fallback labels even
when another locale is active. Obsolete catalog entries also obscure whether the current UI is
fully covered by localization checks.

## What

- Watcher profile selectors display the localized equivalent of `(use step default)` while their
  stored sentinel and submitted empty profile ID remain locale-independent.
- Watcher repository and branch selectors localize the no-repository option, default-branch option,
  repository-first prompt, and loading prompt at render time. Repository IDs, branch names, and
  sentinel values remain untranslated.
- Inter-task sender badges display a localized unknown-task fallback when neither a live title nor a
  snapshot title exists.
- English catalog entries with no current source reference and no current user-interface owner are
  removed from every catalog. Current behavior is not changed merely to retain an obsolete key.
- Translation lookup does not occur at module scope.

## Scenarios

- **GIVEN** any GitHub issue/review, GitLab issue/review, Jira, or Linear watcher dialog is rendered
  in a non-English locale, **WHEN** no agent or executor profile override is selected, **THEN** the
  step-default option and placeholder use that locale while saving still submits an empty profile ID.
- **GIVEN** a watcher repository picker is rendered in a non-English locale, **WHEN** repository and
  branch selection move through empty, loading, and loaded states, **THEN** only UI fallback copy is
  translated and all repository/branch domain values remain unchanged.
- **GIVEN** an inter-task message has no resolvable live or snapshot task title, **WHEN** its sender
  badge renders, **THEN** the badge and tooltip use the localized unknown-task fallback.
- **GIVEN** the catalogs contain the 12 audit-reported unreferenced keys, **WHEN** the i18n checker
  runs after cleanup, **THEN** it reports no orphan entries from that set and pseudo remains in sync.

## Out of scope

- Resolving the existing Portuguese and Simplified Chinese catalog parity advisories.
- Changing watcher persistence, payload contracts, repository selection behavior, dialog layout, or
  mobile interaction behavior.
- Restoring UI that was previously removed or replaced solely to consume an old catalog key.
