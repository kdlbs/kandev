---
id: "08-frontend-surfaces"
title: "Frontend team-access surfaces"
status: todo
wave: 4
depends_on: ["05-ws-fanout-and-revocation", "06-human-assignee-takeover", "07-actor-attribution"]
plan: "plan.md"
spec: "../../specs/workspaces/membership.md"
---

# Task 08: Frontend Team-Access Surfaces

## Acceptance

- Workspace settings gains a Visibility control (`Organization` / `Private`)
  with copy that states plainly who gains access, and a Members section: add via
  the user-directory picker with a role, change a role, remove, transfer
  ownership.
- Org settings gains the default workspace visibility, plus a one-time
  "Make my existing workspaces visible to the organization" action that names
  the exact count it will change and requires confirmation.
- Org member management gains the four org roles with their descriptions read
  from `/api/v1/authz/scopes`, so role copy cannot drift from the registry.
- Task detail and kanban show the human assignee with "Assign to me" as the
  takeover affordance; assignee is available to the existing sidebar filter
  dimensions.
- The transcript attributes human messages to their author, visually distinct
  from agent output. An unattributed human message renders neutrally, never as
  the workspace owner.
- **Every control is gated on the `scopes` array from the DTO.** The frontend
  does not re-derive permission by comparing the current user ID to
  `owner_id`, and does not branch on role names.
- A `viewer` sees the board and transcripts with no prompt box, no terminal
  entry point, and no task-edit affordances — hidden, not disabled-with-a-
  tooltip.
- `workspace.access.updated` re-evaluates access in place; a user who lost
  access is routed out with a clear message rather than left on a stalled panel.
- A single-user install with one private workspace shows no new UI: no Members
  section entry point, no visibility control clutter, no role pickers.
- A top-level test proves `scopes`, `viewer_role`, `visibility`, and
  `assignee_user_id` reach the consuming components, not just the hooks.
- All copy through `t()` / `<Trans>` in five locales; no em dashes;
  `pnpm run i18n:check` and `pnpm run i18n:ratchet` pass.
- Mobile parity for the Visibility control, Members section, role picker, and
  assignee control: no hover-only or desktop-only required action.

## Verification

- From `apps/web`: `pnpm test`, `pnpm run typecheck`, `pnpm run lint`,
  `pnpm run i18n:check`, `pnpm run i18n:ratchet`
- `pnpm run i18n:zh-hant` for the Traditional Chinese pair

## Files Likely Touched

- `apps/web/components/settings/workspace/{visibility,members}/`
- `apps/web/components/settings/organization/`
- `apps/web/components/tasks/` assignee control, transcript attribution
- `apps/web/lib/api/domains/{workspace,authz}-api.ts`, `apps/web/lib/types/`
- `apps/web/hooks/domains/workspace/`
- `apps/web/src/locales/*/`

## Inputs

- Spec: API surface, Permissions; `roles-and-scopes.md` (scopes are a UI hint,
  the server is authoritative).
- Patterns: the `/mobile-parity` skill; the slot/prop-chain rule that a mocked
  leaf hides a dropped hop, so the prop chain needs a top-level test.

## Output Contract

Report the top-level prop-chain tests, a grep proving no component branches on
`owner_id` comparison or a role name, the i18n gate results, the mobile parity
evidence, the single-user no-new-UI check, and set this task plus its plan
checkbox to done.
