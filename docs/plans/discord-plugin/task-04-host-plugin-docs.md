---
id: "04-host-plugin-docs"
title: "Public plugin docs for the external-chat control additions"
status: done
wave: 5
depends_on: ["02-host-control-rpcs", "15-host-delegated-human-responses", "16-host-step-approval-rpcs", "17-host-message-source"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-008
acceptance_criteria:
  - AC-PLUGINS-CHAT-BRIDGE-HOST-008.1
  - AC-PLUGINS-CHAT-BRIDGE-HOST-008.2
system_design:
  - ../../specs/plugins/system-design/chat-bridge-host-controls.md
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
Each implementation commit carried its docs, as the maintainers asked:

- `docs/public/plugins-authoring.md`: Host API matrix rows for `CancelTurn`,
  `GetStepApproval`/`ApproveStep`, and the linked human-response flow;
  "Exact execution controls" covers `CancelTurn`, "Responses from linked chat
  accounts", and "Approving workflow review steps" with Go examples; the
  `Messages().List` section documents `Source` and that `message.*` payloads
  may carry `raw_content`.
- `docs/public/plugins-manifest.md`: the `human_responses` write resource and
  its `host.v2.write:human_responses` grant, turn cancellation under
  `execution`, `source` in the `messages` resource, and the `raw_content`
  note under the event vocabulary.
- **Checks:** `git diff --check`; `pnpm run i18n:check` (public copy em-dash
  guard) passed with the web changes.
