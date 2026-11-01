---
status: current
system: tasks
requirements:
  - REQ-TASKS-QUESTION-IMAGES-001
  - REQ-TASKS-QUESTION-IMAGES-002
  - REQ-TASKS-QUESTION-IMAGES-003
  - REQ-TASKS-QUESTION-IMAGES-004
  - REQ-TASKS-QUESTION-IMAGES-005
---

# Question image previews system design

## Purpose and boundaries

Extend the existing operator clarification bundle with optional workspace image
references. Keep question identity, answer claims, and durable messages in the
task domain. Workspace and runtime code supply constrained reads. Shared
clarification UI supplies presentation across task chat, Quick Chat, Office,
Needs-you Inbox, and coordinator surfaces that already use that UI.

The [requirements](../requirements/question-image-previews.md) own observable
behavior. The [plan](../../../plans/question-image-previews/plan.md) contains
source evidence, assumptions, work orders, and validation.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-QUESTION-IMAGES-001 | Authoring contract, Persistence and projections |
| REQ-TASKS-QUESTION-IMAGES-002 | Authorized preview transport, Executor reader, Failures |
| REQ-TASKS-QUESTION-IMAGES-003 | Presentation and inspection, Loading and memory |
| REQ-TASKS-QUESTION-IMAGES-004 | Persistence and projections, Lifecycle and compatibility |
| REQ-TASKS-QUESTION-IMAGES-005 | Choice image extension |

## Current integration points

Paths below are repository-relative.

| Boundary | Current owner and extension |
| --- | --- |
| Tool registration | `apps/backend/internal/mcp/server/server.go`: `registerInteractionTools`, `buildQuestionSchemaItem`. Add images only to the operator tool schema. |
| Tool validation and result | `apps/backend/internal/mcp/server/handlers.go`: `parseQuestions`, `extractQuestionAnswers`. Preserve keyed answer output. |
| Backend admission | `apps/backend/internal/mcp/handlers/handlers_ask.go` and `internal/clarification/handlers.go`: validate before registering a wait or creating messages. |
| Shared wire model | `apps/backend/internal/clarification/protocol/types.go`: additive `Question.Images`. |
| Durable messages | `apps/backend/internal/backendapp/adapters.go`: `clarificationQuestionData` persists `metadata.question.images`. |
| Recovery and external listing | `apps/backend/internal/clarification/bundle_question.go`: parse saved images and expose optional `QuestionStatus.Images`. |
| Identity and claiming | `internal/clarification/retry.go`, `store.go`, and `resolver.go`: preserve retry identity and resolver ownership. |
| Web question model | `apps/web/lib/types/http-agents.ts`: additive `ClarificationQuestion.images`. |
| Pending and historical UI | `clarification-input-overlay.tsx`, `messages/clarification-request-message.tsx`, and shared panel hosts under `apps/web/components/task/chat/`. |
| Existing preview examples | `hooks/domains/session/use-workspace-file-preview.ts`, rich-output `file-preview-block.tsx`, and `image-preview-dialog.tsx`. Reuse transport architecture and image inspection primitives where compatible. |

## Authoring contract

Add `images?: QuestionImage[]` to each operator question:

```json
{
  "id": "layout_dark",
  "prompt": "Approve this layout?",
  "images": [
    {
      "id": "phone_dark",
      "path": "review/phone-dark.png",
      "repo": "web",
      "alt": "Phone settings screen in dark mode"
    }
  ],
  "options": [
    {"label": "Approve", "description": "Keep this layout."},
    {"label": "Revise", "description": "Apply the changes in my comment."}
  ]
}
```

`repo` is optional. Without it, `path` addresses the task workspace root.
With it, `path` is relative to the named attached repository/source scope.
These are executor workspace identities, not backend-host paths or repository URLs.

`QuestionImage` has exactly `id`, `path`, optional `repo`, and `alt`. Require a
nonempty description of at most 300 Unicode code points. Require an image ID
matching `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`. Limit path and repo values to 4096 UTF-8
bytes each. Reject unknown image-object keys to catch unsupported URL fields.

Validate canonical slash-separated relative paths through
`internal/common/workspacepath.ValidateTreePath`, with additional nonempty,
URI-scheme, and leading-home-shortcut rejection. Do not clean traversal into a
valid path, expand a home shortcut, decode URI escapes, or strip line selectors.
Allow literal filenames only. Require a case-insensitive `.png`, `.jpg`, or
`.jpeg` suffix. Recheck actual content during reads.

An absent or empty image list means no images. Normalize empty lists to omitted
storage. Reject null, non-array values, malformed entries, and duplicate image
IDs within one question. Allow at most four images per question and four distinct
IDs per bundle. A repeated bundle-wide image ID must carry identical path, repo,
and alt values. Repeating a reference in two questions associates both questions
with that image. Do not merge those questions or alter their navigation order.

Keep stable question IDs recommended, not newly mandatory. Questions retain the
existing options and `allow_custom_text` fields. Image IDs identify previews only.
They are never option IDs or answer keys. No bundle-level image field is introduced.
Choices can carry the optional field defined in the choice image extension below.

## Choice image extension

`protocol.Option` and `ClarificationOption` gain `images?: QuestionImage[]`.
The list accepts zero or one item. This reuses strict image decoding and avoids
a second reference shape. Question-level `images` remains shared visual context.
Each question has at most four references across its own list and its options.
The bundle still has at most four distinct image IDs. Identical references may
appear in different placements. Duplicate IDs within one list remain invalid.
Counting placements bounds simultaneous reads even when an ID repeats.

Only `buildOperatorQuestionSchemaItem` adds option-image schema metadata. The
shared base schema remains image-free. Both MCP parent entry points reject even
empty choice-image fields. Native providers remain image-free producers.
`ValidateQuestionImages` checks all placements before admission. `HasQuestionImages`
includes choice images so task/session ownership checks cannot be bypassed by a
choice-only request. Optional empty lists remain omitted from JSON.

`messageCreatorAdapter` preserves `options[].images` in durable question metadata.
`optionsFromQuestionData` restores those references for bundle reconstruction and
external pending lists. Retry hashing includes serialized option images, and
`optionsEqual` includes their contents. No database migration is required.

The preview URL remains keyed by pending ID, question ID, and image ID.
`savedQuestionImage` searches question and choice references only within the
authorized question. Bundle-wide ID consistency makes an additional option ID
unnecessary for file access. It does not authorize a reference on another question.
The existing agentctl endpoint, file containment, binary validation, byte/pixel
limits, unavailable-runtime behavior, and local/remote boundaries remain unchanged.

`ClarificationOptions` accepts a render slot for option previews. Image-bearing
options use a labelled group containing the existing selection button and a sibling
`QuestionImages` control. Never nest the inspector button inside the selection
button. Use the same vertical order on desktop and phone: choice label/description,
then compact preview and Inspect. Keep the existing question-body scroller and
96px/128px thumbnail ceilings. No carousel, hover-only action, or new comparison mode.
Image-free options keep their existing button markup. Existing localized Inspect,
loading, failure, Retry, and viewer strings are reused. Agent labels remain data.

Pass inspection state to the existing question shortcut guard. The existing draft
retention predicate must include option images. Historical messages group each
choice label with its lazy image disclosure. Failed previews remain separate from
the selectable choice. Inspecting an option must not activate its selection callback,
including when a single question normally submits on selection.

This is an additive extension to the same task-owned lifecycle. A separate ADR,
file-serving route, or image archive would duplicate established boundaries.

The operator tool needs a schema wrapper around the shared question schema.
`ask_parent_question_kandev` uses that same base today. Do not expose images there.
Explicitly reject image-bearing parent requests rather than silently accepting
unsupported references. Native provider adapters remain image-free producers.
The shared backend validator also checks images on REST and WS admission paths.

## Persistence and projections

Persist validated references in each existing message's `metadata.question`.
No image bytes, data URLs, blob URLs, absolute roots, or bearer URLs enter metadata.
No new database table or row migration is required. JSON metadata works for
SQLite and PostgreSQL. The message's task, session, turn, pending ID, and question
ID remain authoritative.

Update manual serializers and parsers together. Include `QuestionStatus.Images`
in authorized external pending-question listings when nonempty. Existing inbox
and coordinator projections carry messages and reuse the same metadata. A malformed
stored image list degrades to an unavailable attachment area. It must not hide
or invalidate an otherwise usable historical question.

`PendingIDForRequest` already hashes normalized questions. The optional field
must use `omitempty` so image-free retry payloads retain their existing bytes.
Extend `questionsEqual` to compare ordered image references. For image-bearing
bundles, also compare question IDs so content deduplication cannot remap answers.
Keep legacy image-free equality behavior. A different path, image ID, repo, alt,
or order changes the image-bearing request identity.

Page reload and backend recovery reload references from durable messages. Image
bytes remain live workspace content. A fresh read can show changed file content.
The viewer uses the thumbnail's loaded bytes so enlargement cannot silently show
a different revision during the same inspection. There is no watcher or archive.
Document that agents should write a new filename for a revised screenshot.

Answer drafts retain the existing lifecycle. Inspection, question navigation,
panel collapse, and responsive changes must not clear them. This feature does
not add durable storage for unsent drafts across a full page reload.

## Authorized preview transport

Add a feature-owned, authenticated read route:

`GET /api/v1/clarification/:pendingID/images/:imageID?question_id=<questionID>`

Both preview HTTP clients refuse redirects. A preview cannot cause a request to
an external location through a redirect response.

The question ID is a query value because existing callers can use slashes in IDs.
Gin decodes path separators before route matching. Do not tighten existing
question-ID rules or change router-wide path handling to support previews.

The browser sends identities only. It cannot supply a path, repo, session ID,
task ID, or external URL. Do not put image bytes in WS events or tool results.
Use the normal configured backend origin and authenticated fetch conventions.

1. Use `Resolver.AuthorizeBundleAccess` to resolve and authorize the bundle.
   Preserve its non-disclosure behavior and exclusion of parent-question bundles.
2. Read the matching durable question from that same authorized bundle. Require
   consistent task/session ownership across its records. Select the saved image ID.
   Reject mismatched or malformed records before runtime access.
3. Resolve the source session through the lifecycle's `GetOrEnsureExecution`
   authorization path. This retains `session.exec`, workspace ownership, and
   workspace-admission checks. Do not copy an identity-free context or use the
   currently selected UI session. The normal workspace-only ensure operation may
   restore runtime access. It must not dispatch a prompt or resolve a clarification.
4. Acquire the source execution's authenticated agentctl client. Call a new strict
   image endpoint, `GET /api/v1/workspace/image-preview?path=...&repo=...`.
   Encode query values exactly once. Use a bounded 15-second context.
5. Return validated raster bytes with the detected `Content-Type`, fixed inline
   disposition, `X-Content-Type-Options: nosniff`, and `Cache-Control: private, no-store`.
   Use the existing authenticated origin. Do not expose an executor port to the browser.

Use a small injected preview-reader interface in clarification handlers. Wire it
through `internal/backendapp` to lifecycle and agentctl. The clarification package
must not import the backend application or concrete runtime implementation.
Recheck access on every read. Authorizing an answer alone does not grant file access.

## Executor reader

The generic `workspace.file.get` route is unsuitable for automatic image reads.
`WorkspaceTracker.GetFileContent` supports external absolute paths, home expansion,
and read-selector fallbacks. Its size check precedes an unbounded read. Leave that
existing editor contract unchanged.

Reuse the executor connection, manager's current workspace/source ownership, and
relative-path validation. `resolveWorkspacePreviewRoot` and
`currentWorkspaceSourceRoots` supply existing source-root resolution patterns.
Do not reuse the HTML publisher, create a preview server, or accept its permissive
content types. Existing prompt attachments are uploads with separate ownership
and retention. Do not create synthetic uploads for workspace references.

Workspace rebind, rescan, and rollback update the image root together with the
tracker root under `repoTrackersMu`. Failed changes retain the previous root.
An unchanged workspace path retains its original resolved image scope, so a
rescan cannot authorize a replacement symlink. Image reads hold this lock only
while selecting and opening the authorized root handle. Reads admitted before
a workspace change finish through that confined handle; later reads use the
updated policy. File I/O and decoding do not block workspace updates.

The new reader must enforce all of these conditions:

- Select only the source execution's task root or a currently attached source.
  Resolve repository scope from trusted current workspace mappings. Reject stale,
  unknown, absolute, or escaped scopes. An attached source may live outside the
  worktree only when the existing workspace manifest authorizes it.
- Anchor the read to the authorized root with Go's `os.Root` APIs. Open relative
  to that handle. Do not use a separate symlink check followed by an unconstrained
  `os.Open`. A symlink within the selected root may resolve inside it. Escaping
  links, including root/scope replacement races, must fail closed. Reuse the
  repository's root-handle patterns where suitable.
- Require a regular file. Reject devices, FIFOs, sockets, and directories without
  blocking. Apply a bounded open/read strategy for platforms where opening a FIFO
  can block before `Stat`. Test the implementation on supported platforms.
- Enforce a 10 MiB byte cap while reading, not only through a pre-read stat. Read
  at most the cap plus one byte. Apply the same bound in the backend HTTP client.
- Identify PNG or JPEG by signature and decoder format. Require the suffix to match
  the detected format, accepting both `.jpg` and `.jpeg` for JPEG. Validate dimensions before
  full decoding. Require positive dimensions and at most 16,000,000 pixels, using
  overflow-safe arithmetic. Decode the bounded content to reject corrupt images.
  Follow the existing screenshot-validation pattern in `task/service/attachment_service.go`.
- The server returns only the detected PNG/JPEG MIME type. Never return HTML, SVG,
  text, or a MIME type derived only from the extension. Do not run transformations,
  embedded instructions, scripts, or external resource resolution.

The limits align with existing preview-feedback screenshot limits. Keep the
question reader independent of the task attachment service. No new package is
needed for PNG/JPEG decoding. Bound image validation to two concurrent decodes per
agentctl instance. Waiting reads honor cancellation and the request deadline.

A retained older agentctl will return unsupported/not-found for the new endpoint.
Map that result to unsupported preview. Never retry through `file/content`, a
shell command, a port proxy, or a backend-host filesystem read.

## Presentation and inspection

Use a small `QuestionImages` component above the prompt in `ClarificationCard`.
It receives the durable pending ID, question ID, source session identity, and
validated image descriptions. Reuse it in historical clarification messages.
The active question card mounts one image group. A repeated image stays visible
for each associated question. The UI does not change option selection or automatic
single-question completion behavior.

Desktop keeps the current clarification panel and question carousel. Each image
has a visible description, an uncropped thumbnail bounded to 128px high on desktop
and 96px high below 768px, and an explicit inspection button. Express these bounds
in rem units so root-font scaling remains available. The image and Inspect label
form one button. The label does not need a separate control-height row. Keep the
whole button at least 44px in each dimension on phones and coarse pointers.
Use `object-contain`, the available width as a ceiling, and a
reserved bounded loading area. Long descriptions wrap. Long file paths may truncate
visually, but their full accessible text remains available.

Phone entry is the same pending-question panel above the composer. The nearest
shipped exemplars are `ClarificationPanelSection`, `mobile-clarification.spec.ts`,
and `ImagePreviewDialog`. Keep one question at a time, with image, prompt, choices,
and custom input in that order. The existing question body owns vertical scrolling.
Keep navigation and completion actions reachable within the panel and above the
keyboard. Do not add a gallery route or place choices beside a narrow image.

Inspection uses the shared `@kandev/ui` dialog primitives with an opt-in inspection
mode in `ImagePreviewDialog`, or a small feature wrapper if that keeps the current
attachment interface stable. Desktop uses a bounded modal. Phone uses a full-height
viewer because screenshot inspection is dense content. Use `useResponsiveBreakpoint`
for the 768px phone boundary. Keep one mounted viewer and shared answer state.

The viewer has a visible description, Close, Fit, and Actual size. Fit contains
the whole image. Actual size renders one image pixel per CSS pixel and supports
contained scrolling/panning. Touch pinch zoom must not be disabled, but it is not
the only way to inspect details. Only the viewer canvas may scroll horizontally.
Use `100dvh`, safe-area padding, `min-h-0`, and `overscroll-behavior: contain`.
Do not change existing attachment viewers unless they opt into the new mode.

Opening inspection stores the trigger and scroll position. Closing returns focus
to that trigger. While inspection is open, suspend question shortcuts and prevent
Escape from collapsing the parent question or Quick Chat dialog. Closing the viewer
never calls answer, skip, or resolve. A bundle replacement closes stale inspection
and discards stale file responses. Backdrop dismissal follows the shared primitive.

Apply `better-ui` within the current design system. Use a 1px inset image outline
in pure black at 10% opacity for light mode and pure white at 10% for dark mode.
Keep structural borders and visible forced-colors focus. Use existing radius tokens
with consistent nested insets. Phone/coarse-pointer controls use 48px nominal targets
and must measure at least 44px. Ordinary desktop controls keep 28px density.
Use existing icon vocabulary and `currentColor`. Hover effects require hover support.
Keep frequent question navigation instantaneous. Any added press scale uses 0.96
for 150ms with ease-out, only when enabled and reduced motion permits it.
No animation is required to understand state.

All application labels use `t()`. Add keys in all seven locales. Generate the
Traditional Chinese catalogs with `pnpm run i18n:zh-hant`. Treat alt descriptions
as untrusted plain text, never HTML. Announce loading/errors through one translated
status region per image group. Use a stable accessible name on Retry and Inspect.

### Drafts across responsive layouts

The task page keeps a bounded, memory-only draft context above its responsive
layout. Image-bearing active bundles use a key composed of source session, pending
ID, and form mode. The context restores the text, keyed choices, and current
question when the desktop/phone layout remounts. It holds at most eight bundles
and removes locally settled bundles. A task change replaces only the draft cache;
the provider preserves the mounted task layout and its pending panel-focus state.
A page reload discards the context.
It contains no image bytes and does not persist unsent answers. Image-free forms
and late-answer forms keep their existing local state behavior. The answer hook
and server claim remain authoritative; restoring a draft never submits it.

## Loading and memory

The browser fetches through a new question-image hook, following the generation
fence in `useWorkspaceFilePreview`. Do not reuse that hook's broad file-read request.
Use authenticated `fetch`, `AbortController`, and revocable blob URLs. Scope all
state by source session, pending ID, question ID, and image ID. Clear it on auth
change, bundle replacement, or unmount. Ignore stale completions.

Load only images on the active expanded question. The clarification host passes
preview enablement to both question and choice images. Collapse disables previews,
aborts active and queued reads, and revokes loaded URLs. Resource cleanup also
clears the loaded-state snapshot so reopening cannot reuse revoked URLs. The form stays mounted
to preserve answers and drafts. Expansion starts fresh reads. Preview enablement
is independent of keyboard shortcuts, which pause during image inspection.

Question and choice hooks share
one scheduler keyed by bundle, question, and owner. Cancel queued work on cleanup
and remove idle groups. Use at most two concurrent reads
in a visible group. Do not preload the next question. Historical messages initially
show labelled preview controls and fetch only when explicitly expanded. A failure
requires explicit Retry. No polling or automatic retry loop is introduced.

Keep only the active question's bounded image bytes and the open viewer's bytes.
Reuse those bytes for inspection. Revoke URLs when their owner releases them.
Do not add global caches or local-storage image payloads. Thumbnail means bounded
visual presentation here, not a generated lower-resolution file. Lazy reads and
byte/pixel limits bound the first version.

## Failures

| Failure | Result |
| --- | --- |
| Invalid authoring shape or forbidden path syntax | Tool/HTTP validation error before acceptance. No wait or durable question is created. |
| Unauthorized, unknown bundle/question/image, inconsistent ownership | Non-disclosing 404. No runtime read. |
| Missing file or removed source | Unavailable image with explicit Retry. Keep question controls usable. |
| Invalid image or unsupported format | Unsupported/invalid image state. Never render returned text as content. |
| More than 10 MiB or 16 million pixels | Image-too-large state with the limits. No partial image response. |
| Missing runtime endpoint | Unsupported preview state. No generic file-read fallback. |
| Offline executor, timeout, cancelled read | Unavailable/retry state. A changed target ignores the stale response. |
| Submission failure or concurrent answer | Existing resolver and retry UI remain authoritative. Preview status has no effect on the winner. |

Use machine-readable bounded error codes: `not_found`, `unavailable`,
`unsupported`, `invalid_image`, and `too_large`. Do not expose absolute paths,
credentials, or upstream error bodies to the browser. Existing logs can record
bounded failure reasons with task/session correlation. Do not log bytes or alt text.
No new metrics subsystem is required.

## Lifecycle and compatibility

Preserve [active clarification ownership](clarification-active-lifecycle.md),
[bounded answer recovery](clarification-response-reliability.md), and
[external resolution](../../integrations/system-design/external-question-answering-02.md).
The new route is read-only and also serves authorized terminal history. It does
not require an active waiter and does not claim or revive a bundle.

| Producer or consumer | Behavior |
| --- | --- |
| Current operator MCP tool, any provider using Kandev MCP | Optional images supported through the same request and wait lifecycle. |
| Native provider question adapters | No new image authoring contract. Existing choice/free-text behavior remains. |
| Autopilot parent-question tool | Images remain unsupported. Parent ownership and tool exposure remain unchanged. |
| Existing image-free callers | Omitted field, unchanged hash and answer shape. |
| Older UI or external answer client | Can ignore images and answer through the existing contract. No new mandatory response field. |
| Current external pending-question listing | Optional image references survive reconstruction. No public image URL or byte payload. |
| Local/worktree and remote executors | Same authenticated image endpoint on the source agentctl. No localhost URL in browser data. |
| Older retained agentctl | Unsupported preview. Questions remain answerable. |

The response remains `{question_id: {selected_option, custom_text}}` with existing
answered/rejection fields where applicable. Generated option IDs currently use
`q<question position>_opt<option position>`, with both positions starting at one.
Document that mapping and preserve it. Do not mix image IDs into that numbering.
Issue #4667 remains a separate answer-envelope concern. Rebase its eventual fix
without replacing that fix with a second implementation.

## Design choices

The issue permits either question images or interactive rich output. Extending
questions reuses their barrier, persistence, ownership, and answer delivery.
Rich output remains presentation-only. Question-level references also avoid a
second top-level image catalog and preserve the current request structure.

Live references match existing file previews and avoid new storage, cleanup,
retention, and upload contracts. The consequence is explicit: history cannot
reproduce an image after its file changes or disappears.

A strict image endpoint is necessary because the generic editor reader intentionally
allows external paths. Reusing the transport does not justify inheriting that
permission boundary. This feature-specific rationale is fully captured here.
No repository-wide policy or separate ADR is introduced.
