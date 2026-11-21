---
id: "01-repair-delivery"
title: "Align attachment materialization and adapter delivery"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-001.1
  - AC-TASKS-PROMPT-ATTACHMENTS-001.6
  - AC-TASKS-PROMPT-ATTACHMENTS-001.7
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 01: Align attachment materialization and adapter delivery

## Summary

Materialize files where the active adapter reads them after a live repository
rescan. Reject unresolved descriptors before provider dispatch and settle the
matching prompt through the existing error path.

## In scope

- Add a failing regression for upload after real single-to-multiple-repository rescan.
- Resolve the active adapter root under process lifecycle synchronization.
- Propagate content-construction errors for claimed descriptors through prompt and steer delivery.
- Verify byte content, duplicate names, root replacement, and recovery after error.

## Out of scope

No live-instance changes, provider updates, UI changes, persistence changes,
host version reporting, or workspace migration.

## Acceptance

1. A valid PNG uploaded before or after rescan reaches ACP with identical bytes.
   Duplicate names remain distinct. Repeated rescans preserve delivery.
   Path-delivered resources resolve from the active agent directory.
2. A missing or unreadable claimed attachment stops the entire submission before
   provider dispatch. Cover images, audio, resources, mixed attachment sets,
   ordinary prompts, and steers. The failure uses existing owned settlement.
   A later valid prompt succeeds without a stuck gate.
3. A replacement adapter after explicit stopped-process rebind uses its new
   directory. An in-flight materialization cannot silently select a replacement
   generation. Inline content and present large-file fallback remain compatible.

## Verification

Run from the repository root. Use synthetic images, temporary repositories,
and a scripted ACP peer. Do not submit the user's image to a provider.

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/api -run 'TestMaterializeAttachment|TestHandleRescanWorkspace' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp -run 'TestBuildPromptContentBlocks|TestAttachmentDelivery' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run 'TestAttachmentWorkDir|TestRescanRepositories|TestRebindWorkspace' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agentctl/server/adapter/transport/acp -run 'TestMaterializeAttachment|TestHandleRescanWorkspace|TestAttachmentWorkDir|TestRescanRepositories|TestRebindWorkspace|TestBuildPromptContentBlocks|TestAttachmentDelivery|TestSteer|TestPrompt.*Handoff|TestWakeup|TestFireWakeup' -count=1 -timeout=120s)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/api/attachments.go apps/backend/internal/agentctl/server/api/attachments_rescan_test.go apps/backend/internal/agentctl/server/process/manager_attachment_workdir.go apps/backend/internal/agentctl/server/process/manager_attachment_workdir_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_test.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_attachment_delivery_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Name new tests with the prefixes above so these commands include them.
Run the relevant test before implementation and record its expected failure.
Update the lint path list if the final file set changes. Inspect process-stop
and rebind tests before changing synchronization.

## Files likely touched

- `apps/backend/internal/agentctl/server/api/attachments.go`
- `apps/backend/internal/agentctl/server/api/attachments_rescan_test.go` (new)
- `apps/backend/internal/agentctl/server/process/manager_attachment_workdir.go`
- `apps/backend/internal/agentctl/server/process/manager_attachment_workdir_test.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_attachment_delivery_test.go` (new)

## Dependencies

None. Execute this work order only after an explicit implementation request.

## Risks

Hold lifecycle admission for the operation without holding a broad mutex across
network work. Follow the existing lock order. Test root replacement and error
settlement so the repair cannot redirect bytes or strand an active prompt.

## Parallelism

`sequential`

## Inputs

- [Plan and diagnostic evidence](plan.md)
- [Prompt attachment requirements](../../specs/tasks/requirements/prompt-attachments.md)
- [Active directory design](../../specs/tasks/system-design/prompt-attachments.md#active-agent-directory-and-live-workspace-rescans)
- [Live-rescan decision](../../decisions/2026-07-27-legacy-add-branch-live-rescan.md)
- Existing `TestRescanRepositories_TransitionsSingleToMultiRepo` and
  `TestMaterializeAttachment_StreamsIntoSessionDirectory` patterns.

## Results

Implemented on 2026-11-06 after the user requested the fix.

- The materialization API uses the active adapter directory under lifecycle
  synchronization. Multipart network parsing finishes before synchronization.
- Claimed attachments must resolve to readable, nonempty regular files before
  ACP dispatch. Failure preserves pending context and an existing generating
  turn. Legacy inline delivery and large-file path fallback remain supported.
- Red phase: permanent regressions failed on the root mismatch, missing-file
  dispatch, and generating-turn completion after an invalid steer.
- Green phase: all three targeted test commands above passed. The combined race
  command passed for API (1.549 s), process (2.315 s), and ACP (5.032 s)
  after the final native-content helper extraction.
- Changed-package lint: passed with zero issues using golangci-lint 2.9.0.
  The command used the available binary in `PATH` and `GOFLAGS=-trimpath`.
- Documentation validation: 436 decisions and 1,931 specifications validated;
  all specification files passed. `git diff --check` passed.
- Local PR documentation coverage preflight: `covered`, no errors, using the
  actual changed paths, work order, and linked requirements/design files.
  The user subsequently requested PR publication.

The tests use temporary repositories, synthetic files, and a scripted ACP peer.
They do not establish every provider's decoder behavior. No live instance was
changed. This package has not been deployed.

### Review remediation

Resolved the base-document date conflict without dropping either design section.
Review found an inverse shell-startup lock order and an agent-state error reported
as storage failure. Added failing regressions, aligned shell startup with
attachment installation, and mapped typed agent-unavailable errors to HTTP 503.
The rescan regression now captures the actual manager-owned adapter through a
scripted subprocess, rather than a separately constructed adapter.

Focused verification includes `TestMaterializeAttachment`, `TestAttachmentWorkDir`,
`TestManager_StartShell`, `TestBuildPromptContentBlocks`, and
`TestAttachmentDelivery` across API, process, and ACP packages with `-trimpath -race`. All passed.
Remote CI and review evidence remain pending until checks for the new head finish.
