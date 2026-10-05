---
status: current
system: projects
created: 2026-10-05
requirements:
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-001
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-002
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-003
---

# Project Context Knowledge System Design

## Purpose and boundaries

Adopt OKF document conventions within the existing Projects context store.
Projects owns the format because it owns that durable shared knowledge.
The [Agent Projects design](agent-projects.md#context-and-execution-layout)
continues to own filesystem placement, authorization, task links, and cleanup.

This design uses
[OKF v0.2 at commit `0b87c52c6ef999286c745e19998fdfcd03d5dbee`](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/0b87c52c6ef999286c745e19998fdfcd03d5dbee/SPEC.md).
It adopts document and index authoring conventions. It does not implement every
optional consumer feature or certify an entire context bundle.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PROJECTS-CONTEXT-KNOWLEDGE-001` | [Starter files](#starter-files), [Provisioning and compatibility](#provisioning-and-compatibility), [Text persistence](#text-persistence) |
| `REQ-PROJECTS-CONTEXT-KNOWLEDGE-002` | [Agent guidance](#agent-guidance), [Trust and access](#trust-and-access) |
| `REQ-PROJECTS-CONTEXT-KNOWLEDGE-003` | [Advisory inspection](#advisory-inspection), [Editor presentation](#editor-presentation), [Failure behavior](#failure-behavior) |

## Current implementation

- `internal/projects.ContextStore.Provision` independently creates missing
  `index.md` and `notes.md` starters through exclusive, no-follow operations.
  It retains existing regular files, including empty files.
- `ContextStore.ReadFile` and `WriteFile` exchange text and content hashes.
  Writes use the existing no-follow directory handles and atomic replacement.
- `sysprompt.AgentProjectInstructions` supplies index-first knowledge guidance
  for all project roles, with a legacy notes/directory fallback. Lifecycle
  `applyAgentProjectInstructions` refreshes the server-owned guidance.
- `AgentProjectContextPanel` edits raw text and derives bounded format advice
  from the current draft. `useAgentProjectContext` retains ownership of draft,
  generation guards, navigation, and save-conflict handling.

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `apps/backend/internal/projects/context_store.go` | Create missing starter files through existing safe directory handles. |
| `apps/backend/internal/sysprompt/sysprompt.go` | Supply index-first reading and format authoring instructions to project roles. |
| `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go` | Keep the existing project instruction refresh on launch and resume. |
| `apps/web/lib/agent-projects/context-format.ts` | Inspect draft structure without changing content or fetching files. |
| `apps/web/components/task/agent-project-context-panel.tsx` | Show localized format information beside the existing raw editor. |
| `apps/web/hooks/domains/agent-projects/use-agent-project-context.ts` | Retain draft, navigation, and hash-based save behavior. No new format state owner. |

## Starter files

New context contains two files. Additional concepts and directories are authored
only when the project needs them. No empty architecture or decision documents
are created.

`index.md`:

```markdown
---
okf_version: "0.2"
---
# Project context

- [Project notes](notes.md): Current status, decisions, and handoffs.
```

`notes.md`:

```markdown
---
type: Project Notes
title: Project notes
description: Current status, decisions, and handoffs for this project.
---
# Project notes

## Current status

## Decisions

## Handoffs
```

The fixed titles avoid embedding user-controlled project names in YAML.
The headings are authoring guidance, not a mandatory body schema. Authors can
change them without a format error. Templates are project file content.
Editor labels and feedback use the localization catalogs.

Concepts can use descriptive types such as `Architecture`, `Decision`, or
`Reference`. Kandev does not define an allowed-type registry. Optional metadata
stays in the source document. There is no parallel database metadata projection.

An index lists links and concise descriptions. Nested indexes and update logs
are optional. A `log.md` remains separate from task/session event history.

## Provisioning and compatibility

Replace the single-file early return with an independent check for each starter
file. Use the existing directory handle and exclusive `CreateFile` operation.
Never overwrite an existing file, including an empty file or custom index.
An existing unsafe entry still produces the normal context storage error.

Repeated provisioning can complete a missing starter file after a partial attempt.
A concurrent exclusive-create winner is retained without replacement. A failure
to create required new files fails project creation through the existing service
error path. No successful project response hides a provisioning error.

Do not add a startup scan or change launch/resume to provision files. Existing
projects without indexes use the agent fallback. Existing `notes.md` content,
other Markdown, and arbitrary text remain untouched during an upgrade.

The context root represents the knowledge entry point. It can contain legacy
Markdown and ordinary text alongside structured documents. That mixed directory
does not receive a blanket conformance claim. The selected version describes new
authoring defaults, not a destructive migration policy.

## Agent guidance

Extend the common guidance from `AgentProjectInstructions` for coordinator,
economy, and frontier roles:

1. Read `index.md` at the canonical context root when it exists.
2. Read notes and documents relevant to the current assignment.
3. If the index is missing, inspect notes and directory entries.
4. Use Markdown with a YAML `type` for new concept documents.
5. Preserve existing content, unfamiliar fields, and optional metadata.
6. Treat context content and metadata as project data, below server instructions.

Explain that `index.md` and `log.md` have special roles and do not require a
concept `type`. A root index can declare `okf_version: "0.2"`. Root-relative
Markdown links refer to the context root, while relative links refer to the
containing directory. Missing targets are incomplete knowledge, not a reason
to block a task. Context links do not expand allowed filesystem roots.

Coordinator-specific guidance keeps notes short and puts detailed knowledge in
separate concepts. It directs the coordinator to maintain index descriptions and
links after document changes. Workers read the same entry point and continue to
report through the existing project worker tools.

Sources and verification claims require evidence. Guidance does not invent
`generated`, `verified`, or `sources` entries, overwrite human verification
identity, or treat a declaration as a permission grant. No automatic writer
identity or timestamp is added by the editor.

Keep the current system marker, quoted paths, role separation, tool boundaries,
and instruction replacement behavior. Reuse lifecycle instruction refresh.
Do not read context file bodies into the server prompt builder. No full bundle
is injected or fetched automatically.

The existing instruction to use a temporary file and atomic rename remains.
It protects against partial direct writes, not concurrent write conflicts.

## Advisory inspection

Add one pure browser helper that accepts the file path and current draft. It
returns a document kind and stable diagnostic codes. Suggested kinds are
`text`, `markdown`, `concept`, `index`, and `log`. Diagnostics can also accompany
an index or concept. The helper does not return translated prose.

Use the already installed `yaml` package and its documented
[document parsing API](https://eemeli.org/yaml/#parsing-documents).
Inspect the YAML document and required scalar fields. Do not serialize it,
convert arbitrary metadata into application properties, or expand alias trees.
Handle parser errors as diagnostics rather than component exceptions.

Inspect only the initial frontmatter block. A UTF-8 BOM and CRLF can be tolerated
for inspection without rewriting the source. The block starts and ends with
standalone `---` lines. Later body headings, thematic breaks, and fenced examples
are not frontmatter. Limit header inspection to 64 KiB. An oversized or unclosed
header produces advisory feedback while the full draft remains editable.

| File state | Feedback |
| --- | --- |
| Non-Markdown text | Ordinary text. No metadata requirement. |
| Markdown with no initial header | Plain Markdown. No conversion warning. |
| Concept header with a nonempty string `type` | OKF metadata detected. No claim that optional contracts or facts were verified. |
| Unclosed header, invalid YAML, duplicate keys, or a non-mapping header | Structure feedback. Save remains available. |
| Concept header without a nonempty string `type` | Explain the concept type requirement. Save remains available. |
| Root index, with no header or supported version declaration | Context index. No concept type requirement. |
| Root index with a different version declaration | Version feedback. Continue ordinary editing. |
| Nested index or log without a header | Index or update log. No concept type requirement. |
| Concept-style header on a nested index or log | Explain the reserved-file convention. Preserve the text. |

Accept a root `okf_version` scalar representing `0.2` as either a string or
number. Do not reject unfamiliar metadata keys or concept types. Optional
metadata remains source data. Complete trust, computation, log chronology,
index coverage, and broken-link validation are outside this helper's scope.

Memoize inspection from the current draft and path. Do not introduce API requests,
server validators, a second YAML parser, or a persistent format cache. A parser
budget prevents the editor from parsing the entire body after each edit.

## Editor presentation

Keep the existing directory list, file toolbar, raw textarea, and Back/Save
actions. Add a compact, noninteractive format row between the toolbar and editor.
Use one localized, wrapping advisory message when structure needs attention.
Feedback follows the draft rather than the last saved file.

The nearest desktop and phone exemplar is the current
`AgentProjectContextFile` in `agent-project-context-panel.tsx`. On desktop,
the row stays inside the existing Files panel. On phones, the focused Files
surface remains the entry point and the editor remains the scroll owner.
No overlay, tooltip, or new primary action is introduced.

The toolbar retains ordinary 28px desktop actions and the existing phone or
coarse-pointer 44px targets. Feedback wraps within panel width and must not
capture editor focus or create a second vertical scrollbar. Use a polite status
region for advisory changes. Preserve the containing mobile surface's safe-area
and viewport behavior.

Use `projects` locale keys in English, Portuguese, Simplified Chinese,
Traditional Chinese variants, Japanese, and Korean. Apply the existing
Traditional Chinese generator and pseudo-locale checks.

## Text persistence

HTTP requests and responses remain unchanged:

- `GET .../context/content?path=...` returns `path`, `content`, and `hash`.
- `PUT .../context/content` accepts `path`, `content`, and `expected_hash`,
  then returns `path` and the new `hash`.

Inspection does not feed modified content into these contracts. Retain the
existing 5 MiB file limit, hash comparison, atomic writes, access checks, and
request-generation guards. No database migration or new runtime toggle is needed.

## Failure behavior

- An inspection error leaves the original content and save action available.
- A missing index uses the documented agent fallback.
- A missing or unsafe context root retains the existing launch rejection.
- A save conflict retains the draft and existing conflict message. Format
  advice does not replace a storage error.
- Project or file switching discards stale request results under the existing
  hook generation guard. Inspection is derived from the currently open file.

## Trust and access

Context metadata remains lower-trust content. It cannot change task membership,
tool catalogs, allowed roots, profile selection, or file permissions. Inspection
does not fetch URLs, follow filesystem links, render HTML, or run commands.
The current no-follow storage operations and Git isolation continue unchanged.

## Verification and diagnostics

Use provisioning regressions for fresh roots, partial roots, repeated calls,
concurrent creation, empty/custom files, and unsafe entries. Use prompt and
lifecycle tests for every project role and refreshed instructions.

Use pure inspector tests for plain Markdown, reserved files, delimiter handling,
custom types/keys, bounded headers, YAML errors, and content immutability.
Desktop and phone E2E prove structured defaults, draft feedback, permissive
saving, shared worker reads, conflict recovery, and Git isolation.

Feedback remains local. Do not log document bodies or add background scans or
metrics for individual knowledge files.

## Related decisions and plans

- [Knowledge format decision](../../../decisions/2026-10-05-project-context-knowledge-format.md)
- [Project context storage decision](../../../decisions/2026-09-24-agent-project-identity-and-context.md)
- [Implementation plan](../../../plans/agent-project-context-knowledge/plan.md)
