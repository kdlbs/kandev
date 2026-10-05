---
name: retro
description: Use when the user runs /retro or explicitly requests a session retrospective. Find session lessons and propose concrete shared harness edits for future efficiency. Exclude memory files. Do not run unprompted.
---

# Retro

Review the current session for lessons that improve future agent sessions.
By default, propose edits in the reply and stop. Do not edit files during the retrospective.
If the user explicitly requests edits as part of the retro, apply the qualifying changes within that authorization.
Do not commit, push, open a PR, or create persistent tasks unless the user explicitly requests those actions.

Optional focus: `/retro commit` or `/retro apps/backend/AGENTS.md` limits the analysis to that workflow, topic, or path.

## 1. Gather session evidence

Use the available conversation, tool results, user corrections, and files touched in this session.
If the user identifies another session, use its available primary sources instead.
If context is incomplete, state that limitation. Do not invent missing events or search unrelated private transcripts.

Look for:

- Repeated failed commands, retries, dead ends, or unnecessary exploration.
- Missing navigation pointers, hidden dependencies, or ambiguous workflow instructions.
- Excessive output, repeated reads, lost tool handles, or avoidable context consumption.
- Successful techniques that saved work and can apply to other tasks.
- User corrections that reveal a reusable process gap.

Separate observed events from possible causes. Verify each proposed cause against current files or authoritative documentation.
Do not infer a permanent user preference from a single correction.
Do not mine unrelated PR reviews or perform a general repository audit.

## 2. Filter candidates

Keep a candidate only if it passes every test:

1. **Behavior change:** Identify the earlier decision that the proposed guidance changes and how that change improves the outcome.
2. **Recurrence:** The same gap can affect a normal future session on different code.
3. **Cost:** The gap risks correctness, security, contracts, performance, or substantial wasted tool calls and rework.
4. **Specificity:** State one concrete, testable action with its trigger. Include a fallback or verification where necessary.
5. **No duplication:** Read the proposed target and search related guidance before proposing an edit.

If existing instructions already cover the gap, skip another rule. Tighten unclear wording only if that ambiguity caused the miss.
Remove or shorten ineffective guidance only if session evidence supports that change.
Skip common knowledge, one-off filenames, task IDs, temporary paths, secrets, cosmetic preferences, and rules already enforced by automated checks.
Do not turn a temporary tool workaround into a permanent instruction without evidence that it remains necessary.
Prefer an empty action list over weak lessons. Do not fill a quota.

## 3. Choose the shared source

Use [harness-improvement](../harness-improvement/SKILL.md) for artifact placement and validation.
Read its [session-learnings reference](../harness-improvement/references/session-learnings.md) for normalization and deduplication.
Load only the artifact or platform references relevant to surviving candidates.

Eligible targets are shared repository harness files:

- `.agents/skills/`, shared commands, agents, review guidance, and harness scripts.
- The closest scoped `AGENTS.md`, or root `AGENTS.md` for a rule that applies across the repository.
- Existing repository platform instructions, settings, and hooks in `.claude/`, `.codex/`, `.cursor/`, or `.opencode/`.

Route workflow lessons to the owning skill before considering always-on instructions.
Prefer a small correction to an existing rule over a new file.
Propose a new skill only for a repeatable workflow with no existing owner.
Keep the repository's single-session and delegation policies intact unless the user explicitly authorizes a change.

Exclude memory files, auto-memory, transcript notes, personal settings, application code, application tests, CI, specifications, ADRs, and plans.
Do not save the retrospective to disk.
If a lesson requires product changes, identify the limitation in the reply without adding a harness action.
If the fix belongs to an uneditable tool or hosted service, report it separately under Upstream.
Do not propose a brittle harness workaround for an upstream defect.

Resolve symlinks before choosing an edit path. Use `.agents/skills/` and `AGENTS.md` as this repository's shared sources.
Do not edit `CLAUDE.md` separately when it links to `AGENTS.md`.
Do not copy skill changes into platform mirrors or create new platform trees.
If a real platform file needs a distinct change, read the corresponding harness-improvement platform reference first.
Measure target line, word, and byte counts. Stay within the limits in its [validation reference](../harness-improvement/references/validation.md).
If a target exceeds its limit, propose a replacement or a narrower home instead of appending more text.

## 4. Report and stop

Use [template.md](template.md) for the reply. Use today's date and keep the sections brief.
Connect each proposed action to an observed event and its expected benefit.
Order actions by impact. Each checkbox needs a repository-relative source path and an exact addition, replacement, or removal.
For a replacement, show the current text and the proposed text. Avoid vague actions such as "improve the docs."
If no candidate survives, write `None.` under Action Items.
Include Upstream only if an observed problem belongs there.

If edits lack authorization, return the proposals and stop. State that the user can request all edits or select individual items.
Do not treat silence or an unanswered question as authorization.

## 5. Apply authorized edits

Re-read the accepted targets and re-check duplication before editing.
Apply only accepted changes. Preserve unrelated work and match the target file's voice.
Keep session evidence in the reply and reusable directives in the shared files.
Run the checks in [harness validation](../harness-improvement/references/validation.md) for the changed files.
Report each edited path, the concrete change, and validation results.
If an accepted change is redundant or no longer valid, explain why you skipped it.
