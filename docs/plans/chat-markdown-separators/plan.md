---
created: 2026-09-17
status: done
requirements:
  - REQ-UI-COMMENT-MARKDOWN-002
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
legacy_specs: []
---

# Implementation Plan: Chat Markdown Section Separators

## Overview

Repair ambiguous section separators at render time while retaining short setext
headings. One sequential work order owns the pure transform and rendered proof.

## Requirement and design reconciliation

UI owns reusable Markdown interpretation independently of task or agent state.
Adjacent tasks and agents own message lifecycle and persistence, which do not change.
The existing GFM criterion, `AC-UI-COMMENT-MARKDOWN-001.1`, does not define this
exception. Requirement 002 adds the missing product behavior and its compatibility
limits in the [existing requirement](../../specs/ui/requirements/comment-markdown.md).
The [paired design](../../specs/ui/system-design/comment-markdown.md#prose-separator-normalization)
contains the delivered extension. Existing file-link behavior remains current.

The [bare-wrapper package](../bare-markdown-wrapper/plan.md) remains blocked.
Its preservation fixtures are gates for this repair, not a dependency to unblock.
The current pair no longer describes tagged-wrapper repair despite that older
plan's historical statement. Source and tests establish the existing fence behavior.
No other markdown-separator requirement or relevant ADR appeared in catalog searches.

## Evidence and root cause

CommonMark treats `---` immediately after a paragraph as a setext underline.
The current normalizer leaves it unchanged, so the parser emits a long `h2`.
CSS correctly styles that heading and is not the defect.

Supplied incident evidence: task `193274d1-9253-4b30-b965-fdf83b1ff218`, session
`770bfc9f-342d-412d-b808-248f1f6e94e7`, message
`f226be5e-bbd7-4730-a30a-5f070033adf1`. The report identifies seven glued rules.
Its full-message counts (12 headings, 3 rules) are supplied evidence, not locally
reproduced evidence. No live-instance access is required.

A read-only local probe on 2026-09-17 imported the current normalizer and rendered
through ReactMarkdown with GFM, breaks, and gemoji. It used this exact body:

```text
After root cause: if the bug is missing specified behaviour, add or correct the requirement before the regression test.
```

| Input | Normalizer changes bytes | Rendered result |
| --- | --- | --- |
| Body plus newline plus `---` | No | One `h2`, no rule |
| Body plus blank line plus `---` | No | One paragraph, one rule |
| `Skills to change` plus newline plus `---` | No | One `h2` |
| Body and rule inside a tilde fence | No | Literal code, no heading or rule |

## Scope

### In scope

- Requirement 002 and criteria `.1` through `.6`.
- Conservative prose eligibility, protected fences and block contexts, exact newline preservation.
- Unit, production-plugin parser, desktop chat, and phone chat evidence.
- Existing normalizer consumer and cache compatibility.

### Out of scope

- Stored-message changes, CSS changes, producer changes, new parser plugins, and dependencies.
- Bare-wrapper repair, new cache keys, metrics, feature flags, or raw HTML support.
- Perfect author-intent detection and repair within nested Markdown containers.
- Commits, pushes, PR creation, and delegated work.

## Technical approach

Extend `normalizeMarkdown` in `apps/web/lib/markdown/normalize-cache.ts` as specified
in the design. The implementation keeps `strengthenMarkdownWrapperFence`, glued-close behavior,
and `normalizeCached` semantics. Helpers must keep the existing complexity limits.
The separator scan recognizes homogeneous backtick and tilde fences, container-owned
fences, definite container boundaries, and HTML termination classes.
Avoid changes to `MemoizedMarkdown`, plugins, or CSS unless test evidence establishes
a necessary integration change and the package is revised first.

The chosen thresholds are 70 characters, or 40 characters with sentence punctuation.
They are the tested heuristic authorized by the user's explicit permission to define one.
They are not a claim that Markdown encodes author intent. Short punctuated labels
remain headings, and long intentional setext headings can change interpretation.

## ASCII UI preview

UI-01: Agent response in `/t/:id`, desktop and phone, completed message.

```text
Before                           After
Skills to change [h2]            Skills to change [h2]
LONG BODY AS ONE HEADING [h2]    Body text with inline formatting [p]
                                 ------------------------------ [hr]
Next section                     Next section

Separate real title              Separate real title
Skills to change [h2]            Skills to change [h2]
```

Both viewports use this content order. Phone text wraps within the existing
full-height chat surface. The transcript retains its scroll owner and navigation.
The nearest phone exemplar is `mobile-markdown-wrap.spec.ts`. Paragraph, heading,
and rule semantics are required by `.1`, `.2`, and `.6`. Spacing is illustrative.
No changed loading, error, or empty state is introduced. Streaming prefixes can
temporarily be paragraphs or headings until the complete candidate arrives.

## Tests

New focused cases belong in `apps/web/lib/markdown/normalize-separators.test.ts`.
The existing normalizer suite already exceeds the normal file-size limit, so keep
its fixtures intact and run it as a compatibility gate.

| AC suffix | Named regression and evidence |
| --- | --- |
| `.1` | `renders supplied prose as a paragraph followed by a rule`: exact byte insertion and production-plugin HTML |
| `.1`, `.2` | `applies prose thresholds at their boundaries`: 39/40 punctuated, 69/70 unpunctuated, closing inline markers, Unicode code points |
| `.1` | `repairs the supplied skills excerpt`: one explicit h2, one hr, all body text outside headings |
| `.2` | `preserves short setext titles`: supplied title, `Done.`, inline-formatted title, multi-line short title |
| `.3` | `preserves protected blocks`: bare/tagged backticks, tildes, mixed marker runs, longer enclosing fences, unmatched fences, wrong-character and short closers, list-owned fences across blank lines |
| `.3` | `preserves structural predecessors`: ATX, list markers, pipe rows, quotes, rules, fence delimiters, indentation, lazy container continuation, block-boundary recovery, and HTML blocks |
| `.4` | `preserves non-candidates and front matter`: rule indent 0-3, trailing whitespace, negative indent 4/tab, spaced hyphens, other rule characters, leading YAML with closed/unclosed boundaries |
| `.5` | `preserves source bytes except inserted blank lines`: LF, CRLF, mixed endings, leading/trailing blanks, no final newline, idempotence, cached/uncached equality |
| `.1`, `.5` | `normalizes successive streaming values independently`: prefixes ending in one/two/three hyphens, repeated values, completed message |
| `.3` through `.5` | Existing `normalize-cache.test.ts` and `markdown-components.test.tsx`: exact bare-wrapper, tagged-wrapper, glued-close, and bounded-LRU expectations |

Boundary inputs must contain ordinary words with spaces, not repeated single letters.
Assert both unchanged bytes and rendered semantics for negative cases. Positive
fixtures must not include the blank line that the repair must insert.

### Supplied skills fixture

Use the following source with each paragraph on one physical line. Preserve the
absence of a blank line before the final rule.

````markdown
## Skills to change
**`/spec-delta` and `/spec-archive`**
Living spec path becomes `docs/specs/<system>/requirements/<capability>.md`, not `go/cmd/<service>/specs/`. Deltas stay under `docs/intake/gsp/<id>/lld/specs/<system>/`. Add a domain column so a GSP that changes item resolution writes `lld/specs/catalogue/item-resolution.md`, not a copy under every consuming service.
**`/lld` Phase 7**
When the LLD touches `go/internal/catalogue` or `go/internal/composable`, emit a domain delta plus service deltas that only cover HTTP projection (status codes, cache headers, error envelope). Service deltas must reference domain REQ IDs, not restating the SHALL.
**`/feature` Phase 3 and 7**
Phase 3 should produce (or update) requirements + system design, not only a file map in chat. Phase 7 already calls `/spec-delta`; point it at `docs/specs/`. Skip only for refactors, perf, and config.
**`/fix`**
After root cause: if the bug is missing specified behaviour, add or correct the requirement before the regression test. If the requirement already exists, the test is the AC.
**`/tdd`**
CLIP's Red-Green-Refactor is fine. Add one line: implement against the current requirement / work order; name the REQ or scenario in the test.
**Leave as-is**
`/hld`, `/record`, `/task-spec`, `/code-review`, `/verify`. `/hld` is CLIP's GSP brief. `/record` already owns ADRs.
---
````

## E2E tests

Add a focused separator scenario to `e2e/tests/chat/markdown-paragraph-breaks.spec.ts`
in `chromium`, plus `e2e/tests/chat/mobile-markdown-separators.spec.ts` in `mobile-chrome`.
Use a sibling `markdown-separators-helpers.ts` for shared fixture and assertions.
Reuse `apiClient.createTaskWithAgent`, the mock `e2e:message` command, direct task
navigation, and `SessionPage.activeChat()` from existing chat specs.

Seed long prose, its glued rule, a separate short setext title, and fenced literal
prose/rule content. Await persisted agent content through `listSessionMessages`.
Scope DOM checks to that agent message in active chat. Assert the body is in a
paragraph, exactly one semantic rule exists, and only the short title is `h2`.
Assert code text is literal. Reload and repeat. Compare API content with raw input
before and after rendering (`.1`-.6`). On phone, also assert no document overflow.
Do not copy the old test's broad DOM scan or arbitrary UI timeouts.

## Work orders

- [x] [Task 01: Repair and verify prose separators](task-01-repair-prose-separators.md) (`done`, sequential)

## Verification results

Read-only renderer probe: passed and confirmed the root cause described above.
Documentation checks on 2026-09-17:

- `python3 scripts/list-docs.py validate`: passed, 286 decisions and 998 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/chat-markdown-separators`: passed.
- `git status --short -- docs/plans/chat-markdown-separators`: both new delivery
  files are present in the untracked package.

Implementation and TDD checks:

- `pnpm exec vitest run lib/markdown/normalize-separators.test.ts lib/markdown/normalize-cache.test.ts components/shared/markdown-components.test.tsx`: passed, 3 files and 94 tests.
- Follow-up review fixup: the same focused suite passed, 3 files and 96 tests, including homogeneous mixed-fence literals, block-boundary recovery, HTML termination classes, loose-list ownership, exact front-matter opening, and lone-CR preservation.
- Targeted ESLint: passed with no warnings.
- `pnpm run typecheck`: passed.
- `pnpm run build:vite`: passed. Existing chunk-size and ineffective dynamic-import warnings remain.
- Desktop Chromium separator spec: passed, 2 tests.
- Mobile Chromium separator spec: passed, 1 test.
- Follow-up review regressions: passed for mixed fence markers, container-boundary recovery, HTML termination classes, and list-owned tilde fences.
- Documentation and specification validation: passed after status reconciliation.
- `git diff --check`: passed.

## Risks

- Identical Markdown can encode a real long heading or a separator. The heuristic
  intentionally cannot preserve every long heading.
- A short final paragraph line can miss repair even when the whole paragraph is long.
- Front-matter protection can preserve a leading rule-delimited prose region.
- Shared review and walkthrough consumers receive the same interpretation change.
- Fence and container state must not weaken existing byte-preserving cases.
- The exact full incident payload is unavailable locally. Do not claim its reported
  seven-rule count as a locally verified result.
