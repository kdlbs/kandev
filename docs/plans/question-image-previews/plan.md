---
created: 2026-10-24
status: complete
requirements:
  - REQ-TASKS-QUESTION-IMAGES-001
  - REQ-TASKS-QUESTION-IMAGES-002
  - REQ-TASKS-QUESTION-IMAGES-003
  - REQ-TASKS-QUESTION-IMAGES-004
  - REQ-TASKS-QUESTION-IMAGES-005
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
legacy_specs: []
---

# Implementation plan: Question image previews

## Overview

Show workspace screenshots beside blocking agent questions. Keep the existing
answer lifecycle. Deliver the additive metadata contract, constrained image
transport, accessible interface, and focused integration proof in that order.

Read the [requirements](../../specs/tasks/requirements/question-image-previews.md)
and [system design](../../specs/tasks/system-design/question-image-previews.md).
The user authorized implementation after the design handoff, then extended the
package with optional choice images in Task 05. All five work orders have complete
implementation and feature verification. Backend CI and real Docker/SSH image
cases passed on `8745e76a84536a5fff3f238d2fcdb4bfa23029b2`; Task 04 records the
workflow links and distinguishes that feature evidence from the aggregate E2E
run's four additional flaky cases.

[PR #4766](https://github.com/kdlbs/kandev/pull/4766) is open and ready for review.
The user authorized publication and continued CI/review follow-up on 2026-10-25.
That follow-up corrected workspace preview roots, bounded browser reads, released
workspace locks before decoding, and preserved the mounted task layout when draft
ownership changes. Task 04 records the remaining fixture verification and PR gates.
Windows/macOS file handling was compile-checked, not executed natively.

## Evidence and assumptions

Investigation baseline: `595df36ed5c1cf02fd90cb972b623ad6edc53df8`, 2026-10-24.

1. [Issue #4748](https://github.com/kdlbs/kandev/issues/4748) matches the parent's
   interpretation. It requests one or more ordinary questions beside each image,
   enlargement without losing position, keyed answers, and desktop/mobile support.
   Its one comment confirms maintainer intent to implement. The issue and comment
   contain no image attachments. Its timeline has no linked implementation PR.
2. The issue permits either extending questions or adding questions to rich output.
   This package selects question-level images and now includes optional choice images.
   Multiple questions repeat the same stable image reference.
   During UI review, the user explicitly requested planning followed by implementation
   of optional choice images. Task 05 owns that extension. This explicit sequence
   authorizes implementation after preparing the updated design package in this turn.
3. [Issue #3707](https://github.com/kdlbs/kandev/issues/3707) is closed and links
   [PR #3708](https://github.com/kdlbs/kandev/pull/3708). Its task-owned web feedback
   workflow concerns page annotations and uploaded captures. It does not implement
   blocking image questions. Existing attachment storage is not the reference store.
4. [Issue #4667](https://github.com/kdlbs/kandev/issues/4667) remains open with no
   comments or linked implementation in its timeline. Current results contain
   opaque generated option IDs. Preserve those IDs and document their one-based
   mapping. Do not make image association depend on option positions.
5. Searches of requirements, designs, plans, tool schema, and renderers found no
   existing image-question package or implementation. PR search also returned
   historical clarification lifecycle work #2669, which is a dependency contract,
   not overlapping image support. Recheck upstream before implementation.

The user delegated routine scope decisions. The following are design choices,
not claims that the issue author specified them: preserve four questions per
bundle, allow four distinct images per bundle, support PNG/JPEG first, retain
live file references, and use 10 MiB/16-million-pixel limits. The limits match
existing file and screenshot constraints. Twelve images therefore require at
least three calls. Two questions per image reduce the number of images per call.
No unresolved product question blocks this package.

Tasks owns this vertical contract because it owns clarification messages,
question identity, answer claims, and recovery. Agents owns rich-output presentation.
Workspaces owns source roots. UI owns shared primitives. Neither requires a duplicate
feature specification.

## Scope

In scope: optional image references on operator questions, durable projections,
authorized image-only reads through local/remote agentctl, thumbnails, inspection,
loading/error states, localization, regression tests, and public usage documentation.

Out of scope: larger bundles, multiple images on one choice, generic galleries, image annotations,
answer uploads, archives, additional formats, native provider image schemas,
parent-question images, rich-output forms, and a separate fix for #4667.

## Technical approach

### Metadata expansion

Extend `clarification/protocol.Question`, the operator MCP schema, validators,
`clarificationQuestionData`, and `bundleQuestions` reconstruction together.
Keep image-free JSON omitted to preserve retry hashes. Include image references
in `questionsEqual`. No schema migration is needed. The same data must reach live
messages, history, inbox, coordinator, and external pending-question listings.
Keep the parent tool's shared schema image-free.

### Constrained transport

Add the feature-owned read route from the system design. Resolve image identity
from an authorized durable bundle. Reuse `Resolver.AuthorizeBundleAccess` and
`GetOrEnsureExecution` for task and `session.exec` checks. Introduce a strict
agentctl image endpoint and bounded client method. Do not use `GetFileContent`:
that reader intentionally supports external absolute paths and home shortcuts.

Reuse workspace root/source selection and Go root handles. Follow the existing
PNG validation pattern. Enforce byte and pixel limits before rendering. No new
file upload, image retention job, public URL, shell fallback, or runtime flag.

### Presentation

Add a small image group and hook. Place it above the existing prompt. Preserve
`useClarificationGroup` and the answer endpoint. Add a feature viewer using shared dialog primitives for fit/actual-size
inspection. Keep existing attachment dialog callers unchanged.
The pending overlay and historical clarification messages use durable IDs.
Raw MCP tool transcript rendering can show image labels without fetching bytes
when it lacks the pending ID. It must not invent authorization from tool arguments.

### Compatibility matrix

| Surface | Identity/transport | Expected behavior | Evidence |
| --- | --- | --- | --- |
| Operator MCP through any agent | Session-bound MCP to WS to durable bundle | Images optional, existing blocking result | Task 01 schema and request tests, Task 04 live mock flow |
| REST question creation | Authorized source session to durable messages | Same reference validation | Task 01 admission and round-trip tests |
| Native questions | Existing normalized provider model | Image-free behavior unchanged | Existing native question and choice-only tests |
| Autopilot parent | Parent protocol, shared schema base | No image field or preview access | Parent schema and authorization regression |
| External answer client | Pending ID and question IDs | Additive references, unchanged claims | Bundle reconstruction and cross-entry race tests |
| Current local/worktree runtime | Authenticated agentctl HTTP | Constrained file read | Task 02 integration and desktop/phone E2E |
| Docker and SSH runtime | Existing runtime tunnel/client | Same result, no browser localhost access | Task 04 containers-project real executor tests |
| Sprites, Remote Docker, Kubernetes | Same agentctl API when current and reachable | Same contract, unavailable state on transport failure | Shared client/API tests. Provider-specific deployment is not newly certified by this package. |
| Retained older agentctl | New endpoint absent | Unsupported preview, no broad fallback | Task 02 client test and Task 03 UI state |
| Older UI | Unknown optional metadata | Existing text question and answer behavior | Image-free wire fixtures and compatibility assertions |

## UI preview

The host requires conversation previews through `show_rich_output_kandev` and
prohibits ASCII/SVG/HTML substitutes. The review preview uses structured layout
records below and the matching native rich-output presentation. These records
replace ASCII sketches for this package. Copy is illustrative and must be localized.

### UI-01: Active question on desktop

Entry: current clarification panel above the task composer. State: question 2 of 3.

| Order | Region | Content and behavior |
| --- | --- | --- |
| 1 | Existing panel header | Question count, collapse, existing skip action |
| 2 | Scrolling question body | Image description, uncropped thumbnail, Inspect |
| 3 | Same body | Prompt, radio-style choices, permitted custom text |
| 4 | Existing navigation/actions | Back, Next, existing completion action when eligible |

The thumbnail has a 128px desktop height ceiling and a 96px phone height ceiling.
The image and Inspect label share one activation target. Header/navigation retain existing panel
placement. The question body owns vertical scrolling. One image group remains
adjacent to its prompt. No image click selects an option.

### UI-02: Active question and inspection on phone

Entry: same pending-question panel. State: one focused question.

| Surface | Top | Main region | Bottom/action |
| --- | --- | --- | --- |
| Question | Question position and title | Image, Inspect, prompt, choices, text in one vertical sequence | Existing Back/Next and completion controls, above keyboard |
| Inspection | Description and Close | Fit image, or contained actual-size canvas | Fit / Actual size, safe-area clearance |

Inspection occupies a focused `100dvh` surface. It has one internal canvas scroller.
Closing returns to the same question and draft. Controls measure at least 44px.
The phone breakpoint is 768px, including narrow fine-pointer windows. Normal
question composition remains the existing inline panel, not a compressed desktop modal.

### UI-03: Loading and unavailable image

The image area reserves space while loading. It announces a localized loading
status. Failure shows the image description, localized reason, and Retry when
retry can help. Unsupported formats show supported formats. Oversized files show
limits. Prompt and answer controls remain usable. Historical previews start as
labelled controls and load on explicit expansion.

### UI-04: Desktop inspection

A bounded dialog contains the image description, Close, Fit, Actual size, and
one scrollable canvas. Escape closes only this dialog. Focus returns to Inspect.
The thumbnail and inspection share loaded bytes. Viewing never submits an answer.

These structures implement AC-003.1 through AC-003.5. Dimensions outside the stated
limits are illustrative. Use existing components, density, colors, and radius tokens.
Task 03 includes the same view labels and rendered checks.

## Tests

All test names below are implementation targets unless identified as existing.
Use TDD in each work order. Do not add permanent tests during the design turn.

| Criteria | Target evidence |
| --- | --- |
| AC-001.1, .2, .5 | `server/ask_user_question_images_test.go::TestQuestionImagesSchemaAndValidation`, `clarification/question_images_test.go::TestQuestionImagesValidation` |
| AC-001.3, .4 | Existing `server/ask_user_question_test.go`, `clarification/native_question_test.go`, new keyed-answer fixture with repeated image |
| AC-002.1, .2 | `clarification/image_handlers_test.go::TestQuestionImageAccess` and `backendapp/adapters_question_images_test.go::TestQuestionImageSourceSession` |
| AC-002.3, .4 | `agentctl/server/process/question_images_test.go::TestQuestionImageContainedRead`, `TestQuestionImageLimits` |
| AC-002.5 | `agent/runtime/agentctl/client_question_images_test.go::TestQuestionImageTransport`, agentctl API tests, real Docker/SSH tests |
| AC-003.1, .5 | `hooks/domains/session/use-question-images.test.ts`, the live desktop/phone image fixtures |
| AC-003.2, .3, .4 | `question-image-viewer.test.tsx`, desktop and phone E2E with keyboard, geometry, and screenshots |
| AC-004.1, .2 | Metadata round-trip tests plus live/reloaded history, different selected session, removed-file E2E |
| AC-004.3 | `clarification/store_test.go`, `retry_test.go`, existing `handlers_ask_retry_test.go`, cross-entry race tests, image-bearing retry additions |
| AC-004.4 | Existing rich-output, native question, and parent-question tests. Public docs and prompt/schema synchronization checks. |

## E2E tests

New `e2e/tests/chat/question-image-previews.spec.ts` exercises a real bundled
clarification with two questions sharing one image and a third missing image.
Loader and viewer tests separately exercise multiple concurrent image controls.
Assert keyed answers and custom text, source-session access, explicit rejection,
inspection without submission, page reload, missing files, and historical display.
The fixture must prove the agent stays blocked while images load or fail.

New `e2e/tests/chat/mobile-question-image-previews.spec.ts` runs on the configured
Pixel 5 project. Complete the same bundle, inspect a tall screenshot at actual size,
close it, verify the draft, and submit once. Measure hit targets, viewport containment,
scroll ownership, safe-area clearance, and absence of document horizontal overflow.
Include focused desktop screenshots and phone screenshots in light/dark themes.

Add narrow fine-pointer checks at 767px and a desktop check at 768px in the desktop
suite. Keep shared answer state through desktop-to-phone-to-desktop resizing.
Use causal HTTP/WS waits. Do not add arbitrary sleeps or all-worker overrides.

Task 04 adds real Docker and SSH image reads using repository fixtures. Those
files run only in the `containers` project with `KANDEV_E2E_CONTAINERS=1`.
Do not claim that stubbed runtime tests prove executor connectivity.

### UI-05: Visual choices on desktop and phone

Entry: pending question with two visual alternatives. Shared vertical composition:
prompt, then one labelled group per choice. Each group contains the existing
selection button followed by its thumbnail and Inspect control. The controls are
siblings. Inspect opens the existing bounded desktop or full-height phone viewer.
The question body owns scrolling. Phone hit targets remain at least 44px.
Error and loading states occupy only the preview region. The selection button stays
available. History uses the same label association with explicit image expansion.
The native rich-output preview replaces ASCII sketches under the host instruction.

Task 05 maps AC-005.1–.5 to protocol/admission tests, durable projection and retry
tests, authorized preview reads, and desktop/phone Playwright selection flows.
Public reference changes belong in `docs/public/agent-communication.md`.
The user guide explains inspection versus selection in `docs/public/use-kandev.md`.

## Work orders

1. [Task 01: Persist question image references](task-01-question-image-contract.md), wave 1.
2. [Task 02: Read authorized question images](task-02-authorized-image-reads.md), after 01.
3. [Task 03: Render and inspect question images](task-03-question-image-interface.md), after 02.
4. [Task 04: Prove integration and document usage](task-04-integration-and-docs.md), after 03.
5. [Task 05: Add choice images](task-05-choice-images.md), after 01–03. Task 04's
   remote validation is recorded independently. Task 05 does not change agentctl.

Each task owns a real verification boundary. Task 01 expands an optional wire
contract. Task 02 proves the filesystem boundary before any automatic UI read.
Task 03 completes the operator flow. Task 04 proves remote delivery and shared
lifecycle regressions. Run sequentially. This plan does not authorize delegation.

## Public documentation

Task 04 extends `docs/public/agent-communication.md` as the existing how-to guide.
Add one operator image-question example and the answer mapping. Cover limits,
formats, live-file persistence, remote availability, and image-free fallback.
Add a short usage note to `docs/public/use-kandev.md`. Search README and the screenshot
catalog for stale wording. Do not publish proposed behavior during this design turn.

The external pending-question design receives a link to the new optional metadata
contract. No answer API or permission contract moves to a new owner.

## Verification results

Design validation completed on 2026-10-24:

- `python3 scripts/list-docs.py validate`: passed, 384 decisions and 1618 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `.github/scripts/pr-docs.cjs::validateCoverage`: the actual documentation-only
  diff is exempt. A prospective implementation-path input validates all four work
  orders as covered, with zero reference errors. This check did not change code or
  publish a GitHub status.
- Relative-link checks across the new package: passed.
- `git diff --check` and package status inspection: passed. All new files remain
  untracked, and the existing integration-design amendment remains unstaged.

The temporary coverage runner is `/tmp/check-question-image-package.cjs`. The design
check used a prospective production path. During implementation it was updated to
read the complete tracked and untracked changed-file set. All four work orders
have coverage with zero reference errors. It does not stage files or publish a status.
The design checkpoint ended when the user requested implementation. Current
implementation and verification results are recorded in the work orders. Task 05
records the later choice-image extension and its targeted checks.

## Risks

- Generic file reads permit external paths. Any fallback would violate this design.
- Root/scope symlink races and non-regular file opens need platform-aware proof.
- Existing pending-call recovery depends on exact hashes and deduplication behavior.
- Full-page images can consume memory even at small CSS sizes. Bound bytes, pixels,
  concurrency, and retained blob URLs. The first version does not generate thumbnails.
- A live reference is not a historical snapshot. A changed file changes future previews.

## Next implementation step

No product implementation remains. Complete the ready PR's CI and automated-review
follow-up on the latest head. The user authorized publication on 2026-10-25;
that instruction replaces the earlier local-only delivery restriction.
