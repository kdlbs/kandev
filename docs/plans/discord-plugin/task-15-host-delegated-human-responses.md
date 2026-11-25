---
id: "15-host-delegated-human-responses"
title: "Host: human responses from linked Kandev users"
status: done
wave: 3
depends_on: ["01-host-control-rpcs-adr"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-004
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-005
acceptance_criteria:
  - AC-PLUGINS-CHAT-BRIDGE-HOST-004.1
  - AC-PLUGINS-CHAT-BRIDGE-HOST-004.2
  - AC-PLUGINS-CHAT-BRIDGE-HOST-004.3
  - AC-PLUGINS-CHAT-BRIDGE-HOST-004.4
  - AC-PLUGINS-CHAT-BRIDGE-HOST-004.5
  - AC-PLUGINS-CHAT-BRIDGE-HOST-004.6
  - AC-PLUGINS-CHAT-BRIDGE-HOST-004.7
  - AC-PLUGINS-CHAT-BRIDGE-HOST-005.1
  - AC-PLUGINS-CHAT-BRIDGE-HOST-005.2
  - AC-PLUGINS-CHAT-BRIDGE-HOST-005.3
  - AC-PLUGINS-CHAT-BRIDGE-HOST-005.4
  - AC-PLUGINS-CHAT-BRIDGE-HOST-005.5
  - AC-PLUGINS-CHAT-BRIDGE-HOST-005.6
  - AC-PLUGINS-CHAT-BRIDGE-HOST-005.7
system_design:
  - ../../specs/plugins/system-design/chat-bridge-host-controls.md
---

# Task 15: Human responses from linked Kandev users

Repository: `kdlbs/kandev`. Use `/tdd`. Re-planned for the maintainer
decision on
[#5420](https://github.com/kdlbs/kandev/issues/5420#issuecomment-7083766483):
account linking gated by an admin grant. The design is in
[ADR 2026-11-22-plugin-external-chat-task-control](../../decisions/2026-11-22-plugin-external-chat-task-control.md).

- **Acceptance:**
  1. **Grant.** A manifest that declares `api_write: [human_responses]`
     requests `host.v2.write:human_responses`. A workspace manager grants or
     revokes it in Settings > Plugins > Workspace access through the existing
     approval ledger (reason, revision, audit). A plugin cannot grant itself.
  2. **Link codes.** A signed-in Kandev user creates a link code for one
     installed plugin that declares `human_responses`
     (`POST /api/plugins/:id/identity-links/codes`). Codes are random,
     single-use, valid ten minutes, bound to the user and the installation,
     and stored as digests.
  3. **Linking.** `LinkExternalIdentity(code, provider, subject,
     display_name)` redeems a code for the calling installation only. It
     needs the manifest declaration. It creates a durable link; an existing
     link of the same subject or of the same user for that provider is
     replaced. An unknown, expired, used, or foreign code is denied.
  4. **Link management.** `GET /api/plugins/:id/identity-links` lists the
     caller's links, or every link of the installation for an instance admin.
     `DELETE /api/plugins/:id/identity-links/:link_id` revokes the caller's
     own link, or any link for an instance admin. Uninstalling the plugin
     deletes its links. The plugin settings page shows a "Linked accounts"
     card with the code generator, the list, and revoke actions.
  5. **Receipts.** `RequestHumanResponseReceipt` (workspace, provider,
     subject, kind `permission` | `clarification`, target ID, observed
     revision, exact payload, approval revision, manifest digest):
     - requires the workspace grant (`DENIED` with the ledger reason
       otherwise);
     - requires an active link (`DENIED`, `external_identity_not_linked`);
     - requires the linked user to be active and hold `session.control` on
       the workspace (`DENIED`, `linked_user_not_authorized`);
     - requires the target pending at the observed revision (`CONFLICT`) and
       in the workspace (`NOT_FOUND`);
     - returns a receipt bound to the installation, link, user, workspace,
       target, revision, and payload digest; five minutes; single-use.
  6. **Consumption.** Exact `RespondPermission`/`AnswerClarification` accept
     delegated receipts unchanged. Consumption re-checks the installation,
     the link, the grant, and `session.control`, and runs the response under
     the linked user's identity. A delegated receipt cannot be consumed by
     another installation. Native receipts keep working.
  7. **Attribution.** Each delegated receipt is recorded durably (issued and
     consumed). `Interactions().Get` returns `ResolvedBy {UserID,
     DisplayName, Via}` for an interaction resolved with a delegated receipt.
     The permission resolution audit records the linked user with source
     `plugin`.
  8. **Negative tests:** no grant; revoked grant; no link; revoked link
     (including between issue and consume); a replayed receipt; a payload
     mismatch; a cross-workspace interaction; a manifest without the
     declaration; a receipt consumed by another installation.
- **Verification:**
  - Regenerate protos with the documented proto target.
  - `cd apps/backend && go test ./internal/plugins/... ./internal/backendapp/...`
  - `cd pluginsdk && go test ./...`
  - `make -C apps/backend lint` (or the changed-package lint when the full lint
    is not available).
  - `cd apps/web && pnpm run typecheck`, the new vitest files, and
    `pnpm run i18n:check`.
- **Files likely touched:** `apps/backend/internal/plugins/` (receipts,
  identity links, store migration, handlers, host methods),
  `apps/backend/internal/backendapp/` (linked-user authorizer, permission
  responder actor), `apps/backend/internal/orchestrator/agent_permissions.go`
  and `task/models` (audit actor kind and source), `pluginsdk/` (DTOs, gRPC,
  proto), `apps/web/components/settings/plugins/` and
  `apps/web/lib/api/domains/plugins-api.ts`, locale files.
- **Dependencies:** task-01.
- **Parallelism:** sequential (proto, store).
- **Risks:**
  - Security-sensitive: the Host, never the plugin, chooses the acting user.
  - A restart drops unredeemed codes and unconsumed receipts by design.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Implemented as planned. The grant is `host.v2.write:human_responses`, derived
from `api_write: [human_responses]` and approved in the existing Workspace
access ledger, so no new grant UI was needed (capabilities render by ID).

- **Backend:** `apps/backend/internal/plugins/human_responses.go` (link codes,
  `LinkExternalIdentity`, `RequestHumanResponseReceipt`, consumption
  re-checks, attribution), `identity_link_handlers.go` and routes,
  `state/identity_links.go` (tables `plugin_identity_links` and
  `plugin_delegated_human_responses`, created by the command store),
  installation binding on receipts in `human_interaction_receipts.go`,
  consumption in `host_exact_interactions.go`, `Interaction.ResolvedBy` on
  `Interactions().Get`, exact `GetInteraction`, and exact responses,
  uninstall cleanup, capability discovery. `backendapp` resolves the linked
  user (`plugins_linked_users.go`: auth service identity, or the single local
  user while auth is disabled) and audits permissions with source `plugin`
  and actor kind `linked_identity`.
- **SDK:** `pluginsdk/human_responses.go` (+ gRPC, proto, classification as
  caller-required), `Interaction.ResolvedBy`.
- **Web:** "Linked chat accounts" card on the plugin settings page
  (`plugin-identity-links.tsx`, `use-plugin-identity-links.ts`), API client
  functions, seven locales.
- **Tests:** `human_responses_test.go` covers the end-to-end linked response
  (acting identity, attribution, audit, replay) and the denials: no grant,
  revoked grant, unlinked subject, user without session control,
  cross-workspace interaction, payload mismatch, link revoked or session
  control lost before consumption, and a receipt presented by another
  installation; link-code tests cover foreign installation, reuse, expiry,
  normalization, and an undeclared manifest; `identity_link_handlers_test.go`,
  `state/identity_links_test.go`, the backendapp responder test, the SDK
  round trips, and the web API client and hook tests.
- **Checks:** `go test ./internal/plugins/... ./internal/backendapp/...`,
  `cd pluginsdk && go test ./...`, changed-package lint, web typecheck,
  vitest, oxlint, and `pnpm run i18n:check` passed.
- **Risk:** the permission-resolution audit records the user; clarification
  answers are attributed only through the delegated-response ledger, because
  the clarification store has no actor field.
