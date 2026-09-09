---
status: draft
system: ci
created: 2026-09-09
owners:
  - kandev
---

# Pull request size label requirements

## Overview

Maintainers use pull request size labels to review smaller changes first. The CI automation system owns this base-controlled pull request metadata.

## Terminology

- **Counted file:** An application source file, test source file, or web translation catalog that the pull request changes.
- **Size label:** One of the exact pull request labels `small`, `medium`, or `big`.
- **Tooling file:** A workflow, script, generated file, dependency file, linter file, or tool settings file.
- **Excluded asset:** A file with a known image, font, or binary asset extension, or a file under one of these asset directories: `apps/backend/internal/notifications/providers/assets/`, `apps/desktop/src-tauri/icons/`, `apps/web/lib/assets/`, `apps/web/public/`, and `apps/web/src/assets/`.

## Requirements

### REQ-CI-PR-SIZE-001: Classify pull requests by changed application files

**Intent:** Give maintainers a stable label that supports a small-first review order.

**User story:** As a maintainer, I want pull requests grouped by size, so that I can review smaller changes first.

#### Acceptance criteria

- **AC-CI-PR-SIZE-001.1:** When a pull request opens, reopens, or receives a new commit, the system shall recalculate its size from the current changed-file list.
- **AC-CI-PR-SIZE-001.2:** When the count is 0 through 10, the system shall apply `small`. For 11 through 50, it shall apply `medium`. For 51 or more, it shall apply `big`.
- **AC-CI-PR-SIZE-001.3:** When the system calculates the count, it shall include application source, test source, and web translation catalogs under `apps/`.
- **AC-CI-PR-SIZE-001.4:** When the system calculates the count, it shall exclude documentation, workflows, scripts, generated files, dependency files, linter files, tool settings, and assets. Assets are files with known image, font, or binary extensions or files under these asset directories: `apps/backend/internal/notifications/providers/assets/`, `apps/desktop/src-tauri/icons/`, `apps/web/lib/assets/`, `apps/web/public/`, and `apps/web/src/assets/`.
- **AC-CI-PR-SIZE-001.5:** After a successful recalculation, the pull request shall have exactly one size label. The system shall preserve all unrelated labels.
- **AC-CI-PR-SIZE-001.6:** When any size label definition does not exist, the system shall create all missing definitions before it changes pull request labels.
- **AC-CI-PR-SIZE-001.7:** When GitHub does not return a complete changed-file list, the system shall fail without changing the pull request size labels.
- **AC-CI-PR-SIZE-001.8:** The workflow shall use trusted base-branch code. It shall not check out or execute pull request content.
- **AC-CI-PR-SIZE-001.9:** After concurrent recalculations complete, the size label shall match the current pull request diff.

## Out of scope

- Size scores that use changed-line totals or language weights.
- Manual overrides for size labels.
- Automatic updates for open pull requests that receive no new pull request event after deployment.
- Changes to review, preview, merge, or walkthrough workflows.
- Pull request sizing for repositories other than `kdlbs/kandev`.
