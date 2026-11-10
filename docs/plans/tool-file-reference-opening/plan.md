---
created: 2026-10-27
status: implemented
requirements:
  - REQ-UI-TOOL-FILE-OPEN-001
system_design:
  - ../../specs/ui/system-design/tool-file-reference-opening.md
legacy_specs: []
---

# Plan: Tool file reference opening

## Outcome and evidence

Open extensionless normalized read and search file references through the
existing desktop and phone actions. The independently supplied real-consumer
reproduction at `a2e13dcf5584e3462ea995791e245177249a9845` reported four
extensionless failures and four controls passing in happy-dom, with the actual
consumer and hook. It did not prove browser or editor-network behavior. The
source cause is the basename-dot guard in `FilePathButton`; only read links and
expanded search entries use it. Search scope is separate inert text.

The [requirement](../../specs/ui/requirements/tool-file-reference-opening.md)
and [design](../../specs/ui/system-design/tool-file-reference-opening.md)
keep presentation eligibility separate from the existing
[file-tree path-scope contract](../../specs/ui/system-design/file-tree-path-scope.md).
The normalized listing has no per-entry type: unmarked directories remain
ambiguous and may produce the existing file-open failure. Preserve explicit
markers, selector behavior, exact references, line/session ownership and the
existing path service. No filesystem probe or parser overhaul.

## ASCII UI preview

`UI-01: Tool references`, entry: task chat; read header and expanded search.
Before: extensionless references are inert text. After: they are the existing
inline buttons. The scope and explicitly marked directory remain inert.

```text
Desktop: task chat                      Phone: task chat
+-------------------------------+       +-----------------------+
| Read [Dockerfile] :20-39      |       | Read [Dockerfile] :20 |
| Found files  (expand)         |       | Found files (expand)  |
|  [LICENSE]                    |       |  [LICENSE]            |
|  folder/                      |       |  folder/              |
+-------------------------------+       +-----------------------+
       | click                                   | tap
       v                                         v
  existing file editor                    existing file viewer
```

Brackets indicate activation, not literal new copy. Chat owns its existing
scrolling, and the search result list retains its own existing bounded scroll;
the desktop editor and phone viewer retain their current layouts and navigation.
The phone exemplar is `mobile-file-viewer.spec.ts`'s chat-to-viewer flow; no
new overlay or touch interaction is introduced. Structure and actual
activation are required, ASCII spacing is illustrative. Covers criteria 001.1
through 001.4; rendered desktop and mobile checks verify both file paths.
The phone verifies the visible read line; the component test verifies the
session-scoped cursor request.

## Work order and verification

- [x] [Task 01: Open tool file references](task-01-open-tool-files.md)

One sequential work order, no dependency. It specifies independent permanent
component RED/GREEN, desktop and mobile Playwright RED/GREEN and narrow checks.
Implementation and the exclusive heavy lease were separately released.
Install, tests, builds, lint and hooks run serially with joined handles.
No broad-suite replay is planned.

## Risks and exclusions

- A normalized listing entry without `/` can denote a directory; the existing
  opener handles failure. No reliable type metadata exists in this contract.
- Read selector ambiguity for comma-joined extensionless names is unchanged.
- Desktop line 300 visibility failed for both the extensionless read and a
  matched dotted control after opening. Its deeper cause is unknown and deferred;
  see the [design](../../specs/ui/system-design/tool-file-reference-opening.md).
- No new feature, text, public API, editor routing, provider normalization,
  file probing, workspace authority, or unrelated path classification changes.
- Public docs describe existing file workflows; no new user-facing feature or
  terminology is introduced. Reassess only if implementation expands scope.
