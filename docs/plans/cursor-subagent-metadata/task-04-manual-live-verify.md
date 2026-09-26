---
id: "04-manual-live-verify"
title: "Manual live verification against a real Cursor subagent"
status: pending
wave: 3
depends_on: ["02-parse-correlate-cursor-task"]
plan: "plan.md"
spec: "../../specs/agents/requirements/cursor-subagent-metadata.md"
---

# Task 04: Manual live verification against a real Cursor subagent

`cursor/task` needs a live Cursor login and cannot run in CI, so this is a
manual end-to-end check with `acpdbg` and/or a real Kandev session.

## Acceptance
- `acpdbg` no longer records a `<<< SENT-reply ... err=method not found:
  cursor/task` frame; instead a success reply is sent for `cursor/task`.
- In a live Kandev session using Cursor, launching a subagent renders a card
  showing the subagent's description, prompt, and model; a background subagent
  shows the background affordance.
- No regression for a Cursor non-subagent tool call, and no error surfaced to
  the user.

## Verification
Run the recorded reproduction and inspect frames:
`apps/backend/bin/acpdbg prompt --workdir /tmp/kandev-cursor-subagent --model "composer-2.5[fast=true]" --timeout 60s --file acp-debug/cursor-verify-$(date -u +%Y%m%d-%H%M%S).jsonl --prompt "Launch a subagent to summarize the JS files under src/." cursor-acp`
then confirm the `cursor/task` reply carries a result (not error) and the
metadata was captured. Also verify visually in a live Cursor session.

## Files likely touched
- None (verification only). Records artifacts under `acp-debug/`.

## Dependencies
Task 02 (behavior must be implemented). Task 01 (seam) transitively.

## Parallelism
parallel-safe with Task 03.

## Inputs
- Spec: "Scenarios", "Notes".
- Plan: "E2E Tests".
- Skill: `.claude/skills/acp-debug/SKILL.md`.
- Pinned: use `composer-2.5[fast=true]`; workspace `/tmp/kandev-cursor-subagent`.

## Output contract
Summary, artifact paths (JSONL + any screenshots), pass/fail per acceptance
bullet, blockers, risks; update this task's status and `plan.md` Wave 3 checkbox
and Verification Results. Note the temporary capture files created.

## Results
Pending. Not run in this workspace because it requires a real Cursor-authenticated live session and manual visual confirmation.
