---
id: "04-host-plugin-docs"
title: "Public plugin docs for the external-chat control additions"
status: pending
wave: 5
depends_on: ["02-host-control-rpcs", "15-host-delegated-human-responses", "16-host-step-approval-rpcs", "17-host-message-source"]
plan: "plan.md"
spec: "../../specs/integrations/discord.md"
---

# Task 04: Public plugin docs

Repository: `kdlbs/kandev`. Use `/docs-maintainer`.

- **Acceptance:**
  1. `docs/public/plugins-authoring.md` covers the following, with Go
     examples in the style of the surrounding sections:
     - **Host API matrix rows:** `CancelTurn`, the linked-account
       human-response flow (`LinkExternalIdentity`,
       `RequestHumanResponseReceipt`), `ApproveStep`/`GetStepApproval`, and
       `Message.Source`.
     - **"Exact execution controls and human interaction responses":** the
       linked-account flow next to the native-UI receipt flow.
  2. `docs/public/plugins-manifest.md` documents:
     - the `human_responses` write resource and its Host v2 grant;
     - that `message.*` event payloads may include `raw_content` with the
       unstripped `<kandev-system>` content, and that plugins that must not
       handle system content use `content` or `Messages().List`.
  Following the maintainers' request, each implementation commit carries its
  own docs; this task closes any remaining gap.
- **Verification:** `git diff --check`, plus the docs check that `/docs-maintainer` names for `docs/public`.
- **Files likely touched:** `docs/public/plugins-authoring.md`, `docs/public/plugins-manifest.md`.
- **Dependencies:** tasks 02, 15, 16, 17.
- **Parallelism:** sequential.
- **Output contract:** summary, files changed, checks run, blockers, risks, and task/plan status update.

## Results
Pending.
