---
status: current
system: ui
requirements:
  - REQ-UI-COMMENT-MARKDOWN-001
  - REQ-UI-COMMENT-MARKDOWN-002
---

# Comment Markdown Rendering System Design

## Purpose and boundaries

UI owns the reusable interaction contract that distinguishes web links from file references in
rendered task messages and routes authorized file references into the active desktop or phone file
viewer. The task system remains authoritative for task-repository membership, session worktrees,
and the effective `workspace_path`; the workspace file service remains authoritative for final
filesystem containment and file reads.

An absolute path beneath a registered repository's source checkout is an alias for the matching
file in the active task worktree. It is never authority to read or edit the source checkout. This
design extends the task-workspace behavior in
[Attach Workspace Sources](../../tasks/system-design/attach-workspace-sources.md) without changing
its persistence, materialization, or file-service contracts.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-COMMENT-MARKDOWN-001` | [Trusted file roots](#trusted-file-roots), [Link resolution](#link-resolution), [Workspace-scoped task identifier links](#workspace-scoped-task-identifier-links), [Responsive behavior](#responsive-behavior), [Failure behavior](#failure-behavior) |
| `REQ-UI-COMMENT-MARKDOWN-002` | [Prose separator normalization](#prose-separator-normalization) |

## Prose separator normalization

This section defines the delivered extension. The existing file-link design remains current.
The [implementation package](../../../plans/chat-markdown-separators/plan.md)
records delivery. This extension adds no backend or storage contract.

### Render boundary

`AgentMessageContent` uses `MemoizedMarkdown`, which passes `normalizeCached(content)`
to `ReactMarkdown`. The production remark plugins are GFM, breaks, and gemoji.
The render-time boundary is `apps/web/lib/markdown/normalize-cache.ts`; its pure
separator helper lives in `apps/web/lib/markdown/normalize-separators.ts`.
Direct consumers include review finding bodies and walkthrough steps. They receive
the same deterministic semantics. No task, provider, viewport, or message identity
enters the transform or its raw-string LRU key. The cache limit remains 500.

### Algorithm and protected contexts

Keep tagged-wrapper strengthening first. Extend the existing line scan with a
small separator predicate and protected-context state. Do not add a remark plugin.
Before emitting an eligible hyphen line, emit exactly one empty line.
Use the preceding emitted source line for the requirement's length and punctuation
test. Do not count rendered text or combine earlier paragraph lines.

Track homogeneous backtick and tilde fence runs and opener lengths. A fence closer
must use the same character and at least the opener length. Mixed-character runs,
shorter runs, or opposite-character runs do not close the protected region. An
unclosed fence protects the remaining input. Fence openers after list or blockquote
markers keep their container-owned region protected through blank lines and
indented content. A list remains owned through a blank only for an indented
continuation; an unindented line ends that ownership before prose eligibility is
evaluated. Keep glued-close repair limited to its existing backtick behavior.
Do not broaden tagged-wrapper eligibility or interpret bare fences as wrappers.

Use conservative block guards before the prose predicate. Reject indentation of
four spaces or a tab, ATX markers, ordered/unordered list markers, blockquote
markers, table rows (including pipe rows without outer pipes), and rule/fence lines.
Reset list or blockquote protection only at definite block boundaries, such as a
top-level heading, rule, fence, or HTML block, so lazy continuations remain protected.
Do not repair list/blockquote continuation regions or HTML blocks. Track these
contexts conservatively when their source can resemble ordinary prose. Prefer a
missed repair to a change in literal or nested content.

Model HTML blocks by termination class. Basic and complete block tags, including
void tags such as `hr`, remain protected until a blank line. Recognize arbitrary
basic open and closing tags, not only a fixed allowlist, so custom elements and
standalone closing tags receive the same protection. Raw tags such as `pre`,
`script`, `style`, and `textarea` remain protected until their matching closing
tag, including a close on the opening line. Comments, CDATA, processing
instructions, and declarations remain protected until their explicit terminators.
Closing tags do not end a basic block early. A leading front-matter-like region is
also protected: when the first nonblank line is exactly
`---`, preserve lines through the next standalone `---` or `...`. Indented
lookalikes do not start that region. If there is no closer, preserve the remaining
input. This is a conservative exclusion, not YAML parsing or support for front
matter as a product feature.

Preserve each original line terminator, including lone CR. For an inserted empty
line, use the terminator immediately before the candidate. Uniform CRLF stays
CRLF, and mixed input keeps its existing terminators. Preserve leading/trailing
blanks and absence of a final newline. Do not globally trim, normalize newlines,
or rewrite rule bytes.
Existing wrapper and glued-fence expectations remain regression gates.

### Rationale and limits

The supplied incident identifies false-positive source lines of 70 to 292
characters. A 70-character threshold includes the shortest reported example.
The 40-character punctuation path admits medium-length sentences while retaining
short labels such as `Done.`. Boundary fixtures at 39/40 and 69/70 make the choice
explicit. Terminal punctuation alone is too aggressive for short headings.
Length alone misses shorter sentences. Neither heuristic proves author intent.

This is a local rendering exception within the existing pure-transform boundary.
Requirements, this design, and boundary tests preserve its rationale. Under the
record skill's criteria, a separate ADR is unnecessary. A parser plugin remains
an alternative only if focused counterexamples defeat a maintainable string scan.
Such a change requires revising this package before implementation.

### Rendering and verification

Use the existing transcript on both viewports. The closest phone exemplar is
`e2e/tests/chat/mobile-markdown-wrap.spec.ts`, with `SessionPage.activeChat()`.
The correction changes paragraph/heading semantics inside the current scroll owner.
It adds no controls, navigation, strings for localization, or responsive branch.

Unit tests assert exact normalized bytes, idempotence, protected contexts, and
threshold boundaries. Parser integration uses the three production remark plugins
and asserts paragraph text, heading text, and horizontal rules together.
Desktop and phone browser checks seed an agent response, inspect the active chat,
reload, and assert the same semantics. The message API must still return raw input.
No new telemetry is needed for this deterministic transform.

## Components and responsibilities

- `TaskChatPanel` is the task/session boundary. It supplies the shared Markdown renderer with the
  active session's effective workspace root, file-open action, and repository-source aliases.
- `resolveSessionWorktrees` combines restored session worktrees with live worktree state. Alias
  construction uses these repository-qualified worktrees rather than directory-name inference.
- `repositories.itemsByWorkspaceId` supplies each registered repository's canonical `local_path`.
  The active task's repository links restrict which workspace repositories can become aliases.
- `MarkdownFileLinkContext` carries one stable file-link context through the transcript so message
  renderers do not independently subscribe to repository and worktree state.
- Shared Markdown image rendering uses the session file reader only when a session context exists.
  It preserves browser image rendering outside task sessions and blocks rejected local paths.
- `MemoizedMarkdown` preserves an inherited task-level file-link context while allowing an explicit
  message renderer to override the workspace root or open action.
- The shared Markdown anchor resolver classifies and normalizes a link, produces a task-workspace-
  relative file target when authorized, and calls the existing `onOpenFile` action after preventing
  browser navigation.
- Existing desktop `useFileEditors` and phone `MobileFileViewerPanel` flows fetch and display the
  resolved task-workspace file. They do not receive the registered checkout path.

## Trusted file roots

The task chat boundary builds repository-source aliases from authoritative identities already in
frontend state. An alias has this logical shape:

```text
repository_id
source_root               canonical Repository.local_path
workspace_relative_root   matching session worktree path relative to workspace_path
```

An alias is eligible only when all of these conditions hold:

1. The repository is linked to the rendered message's task.
2. A worktree in the rendered message's session has the same `repository_id`.
3. The repository has a non-empty canonical `local_path`.
4. The matched worktree path equals the effective workspace root or is contained beneath it on a
   path-segment boundary.

For a legacy single-worktree session that has no `worktrees` collection, the session's
`repository_id` and `worktree_path` provide the same identity-qualified fallback. An empty,
ambiguous, stale, or cross-task association produces no alias. Nested source roots are evaluated
most-specific-first and must match on a complete path segment.

The effective workspace root remains `workspace_path`, falling back to `worktree_path` for legacy
session projections. Repository aliases supplement that runtime root; they never replace it.

## Link resolution

The shared resolver performs these steps in order:

1. Preserve fragment-only links and links with an explicit web or application URL scheme as normal
   navigation targets.
2. Decode the path once, remove query/fragment data used only by Markdown navigation, normalize
   supported path separators, strip a supported trailing source-location selector, and reject
   malformed encoding, home-relative syntax, or parent traversal. Windows drive roots accept an
   optional leading slash and use case-insensitive containment after normalization.
3. Resolve an absolute path already beneath the effective workspace root directly to a
   workspace-relative target. This remains the preferred path for single- and multi-repository
   tasks.
4. Otherwise, match the absolute path against an eligible registered source root. Replace only
   that root with its `workspace_relative_root`, append the file suffix, and return the resulting
   workspace-relative target.
5. Preserve an unmatched host absolute path for the current-file read under ADR 0016.
   Otherwise retain the existing relative/root-relative file-shape check and open action.

For example:

```text
source_root:             /home/user/projects/example
active workspace root:  /home/user/.kandev/tasks/task-a
active worktree:         /home/user/.kandev/tasks/task-a/example
message href:            /home/user/projects/example/ui/bundle.js:61
file-open target:        example/ui/bundle.js
```

The mapping is based on `repository_id`, not a shared basename such as `example`. This prevents two
same-named repositories or an unrelated sibling directory from becoming accidental authority.

## Workspace-scoped task identifier links

Task identifiers use the configured prefix and sequence of their workspace. The task record stores
the identifier. The workspace record stores `task_prefix` and `task_sequence`. The link and route
use these values. This feature adds no backend state.

The flow is:

1. `TaskChat` passes its `workspaceId` to each `CommentEntry`.
2. `CommentEntry` selects that workspace and gives its `task_prefix` and ID to `MarkdownComment`.
   It uses the active workspace only when the task has no workspace context.
3. `remarkTaskLinks` matches identifiers for that prefix. It creates a link to
   `/office/tasks/<identifier>?workspaceId=<workspace-id>`.
4. The task route keeps UUIDs on the canonical task ID path. An identifier-shaped path mounts
   `TaskIdentifierRedirect`. The redirect uses an explicit `workspaceId` query when present.
   It accepts the old `workspace` query name for existing links. It uses the active workspace when
   neither query is present.
5. `resolveTaskIdentifier` sends a GET request to
   `/api/v1/office/workspaces/<workspace-id>/tasks/by-identifier/<identifier>`. The backend
   resolves the workspace ID and task identifier and returns the task ID.
6. The redirect replaces the identifier URL with `/office/tasks/<task-id>`. It carries the
   workspace as `workspaceId` and preserves other query parameters.

The UI link matcher uses the exact workspace prefix. The route accepts an identifier with a numeric
suffix. This lets every generated identifier reach the resolver. It includes identifiers that use
a prefix that starts with a digit. UUIDs continue to use the existing task route.

The redirect component holds the resolver status. A successful response changes the URL. A 404
shows the task-not-found state. Other API errors show a retry action. If the link and active
workspace have no workspace ID, the route shows the workspace-unavailable state. These states do
not change task or workspace storage.

## Failure behavior

- An unmatched host absolute path invokes the existing file-open action with that absolute
  path; its anchor prevents browser navigation to a same-origin filesystem URL.
- A path beneath a trusted root that is not recognized as a file target is non-actionable. An
  unmatched host absolute path keeps its original path, including an extensionless path; the
  current-file service decides whether it names a readable file and does not provide directory
  listing.
- Missing alias evidence never invents a repository mapping. Ambiguous eligible aliases fail
  closed. Unmapped absolute reads follow ADR 0016 using the exact requested path.
- External HTTP, HTTPS, mail, issue, and application links keep their existing navigation behavior.
- If a resolved task-workspace file no longer exists, the existing desktop or phone file-open flow
  surfaces its localized failure feedback. Resolution does not fall back to the registered source
  checkout.
- Backend path containment and symlink checks remain the final authorization boundary even after a
  frontend alias has produced a workspace-relative target.

Task identifier lookup has its own failure states. An unknown identifier shows the localized
not-found state. Other lookup errors show a retry action. If the link and active workspace have no
workspace ID, the route shows its workspace-unavailable state.

## Responsive behavior

This change does not alter chat composition, link styling, scroll ownership, navigation, or touch
geometry. The inline Markdown link remains the entry point at every viewport.

On desktop and compact desktop, selecting a resolved link opens or focuses the existing Dockview
file tab. On phone, the same semantic action switches to the existing Files surface and renders the
file in `MobileFileViewerPanel`; back/close behavior, the single viewer scroll owner, dynamic
viewport handling, and safe-area behavior remain unchanged. The nearest shipped phone exemplar is
the absolute attached-worktree link flow in `mobile-add-workspace-sources.spec.ts`, which already
proves the native file viewer rather than a compressed desktop editor.

Because the repair changes target normalization rather than layout or controls, no new mobile
surface is introduced. Desktop and phone end-to-end scenarios use the same repository identity
mapping and prove the same file content is opened from the active workspace.

Task identifier links use the same workspace-scoped route on desktop and phone. They add no
viewport-specific lookup state or navigation controls.

## Testing

- Pure unit coverage builds eligible aliases for single- and multi-repository sessions and rejects
  missing task membership, mismatched repository IDs, paths outside the active workspace, ambiguous
  roots, stale worktrees, traversal, unsupported contained targets, and unmatched Windows paths.
- Shared Markdown component coverage reproduces a registered source checkout link with a trailing
  line selector and asserts the workspace-relative open target. An unrelated host path uses the
  existing file-open action with the exact absolute path, without browser navigation.
- Memoization/context coverage proves a task-level alias context reaches nested agent and user
  message Markdown without replacing explicit open actions.
- The existing desktop attach-workspace-sources scenario cites the attached repository through its
  original `local_path` and asserts the active worktree file content and tab while the task URL
  remains unchanged.
- The existing phone scenario taps the same form of source-path link and asserts the native viewer,
  expected active-worktree path/content, close control, and absence of document horizontal
  overflow.

Task identifier tests verify that comment rendering uses its task workspace, that custom workspace
prefixes resolve to canonical task IDs, and that not-found and retryable failures show their
separate states.

## Observability

Resolution is deterministic frontend behavior and adds no log or metric. Existing file-open errors
remain observable through the localized failure notification and browser diagnostic history. Tests
cover the fail-closed paths directly.

## Related decisions

- [ADR-2026-07-22: Runtime-Mutable Task Workspace Sources](../../../decisions/2026-07-22-runtime-mutable-task-workspace-sources.md)
- [ADR-2026-07-23: Workspace Source Root Move Boundary](../../../decisions/2026-07-23-workspace-source-root-move-boundary.md)

## Local absolute reads and Markdown images

ADR [0016](../../../decisions/0016-observed-external-file-reads.md) owns absolute
current-file read authority. This governs ordinary absolute file references
(AC-UI-COMMENT-MARKDOWN-001.10).
Keep workspace/source alias remapping first; otherwise preserve the absolute path
instead of stripping its leading separator or blocking known host roots. Malformed
encoding, relative traversal and ambiguous repository mappings remain rejected.

For AC-UI-COMMENT-MARKDOWN-001.11, shared Markdown images use the same resolver,
then `requestFileContent` (`workspace.file.get`) with the rendered session ID from
`TaskMarkdownFileLinkProvider`. `useMarkdownImage` ignores superseded responses.
Reuse existing binary/text image encoding and `ImagePreviewDialog`, including its
phone inspection surface. No session means no local read. Loading, request errors
and decode errors use existing localized states. No backend endpoint, publication,
file copy, new storage model, or changed write authority is required.

Implementation: [local file media correction](../../../plans/codex-local-file-media/plan.md).
