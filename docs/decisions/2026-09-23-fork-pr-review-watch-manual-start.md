# ADR-2026-09-23: Require a manual start for fork PR review tasks

**Status:** accepted
**Date:** 2026-09-23
**Area:** backend, integrations, security, workflow

## Context

GitHub review watches create tasks attached to the PR target repository and may
auto-start them from a configured workflow step. Fork PR tasks check out
contributor-controlled files. Repository setup scripts run against that
checkout and receive executor-profile environment values, including secrets.

The workspace base-resolution contract must support valid fork PR checkouts.
That must not make a background review watch execute contributor-controlled
setup inputs without a user action.

## Decision

GitHub review-watch tasks may auto-start only when provider data confirms that
the PR head and target are the same repository. A different or incomplete head
repository identity leaves the linked task waiting for an explicit manual
start. Manual starts remain available and do not grant later background
auto-starts. Same-repository review-watch behavior remains unchanged. Ordinary
PR-link task creation remains user initiated and is unaffected.

The backend enforces this rule at review-task creation metadata and automated
launch boundaries. Fork PR base resolution and push routing remain unchanged.

## Consequences

- Fork PR reviews remain visible as tasks without running setup scripts in the
  background.
- A user can inspect the task and manually start it when they accept the
  repository setup and execution risk.
- Missing provider head identity fails closed for watcher auto-start.
- The integration system must retain the manual-start marker through task
  persistence and check it in every automated launch path.

## Alternatives Considered

- **Reject fork PR review tasks.** Rejected because the watch can still create a
  useful linked task for a user to start manually.
- **Run setup scripts without profile environment values.** Rejected because
  configured setup commands may require those values, and removing them does
  not sandbox the checked-out code.
- **Add a per-PR approval control or trust label.** Deferred because it adds a
  new provider-specific approval lifecycle. An explicit task start already
  supplies a clear user action.
- **Auto-start all valid fork identities.** Rejected because provider identity
  proves which repository supplied the head; it does not make its code trusted.
