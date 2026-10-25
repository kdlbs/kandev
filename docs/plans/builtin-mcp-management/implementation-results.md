# Built-in MCP implementation results

Status: all three work orders are complete, 2026-10-25. This record supersedes
only the completion claims in the historical partial publication checkpoint.

## Delivered behavior

- Five separately registered native packages, with Core required and explicit
  human Configuration consent for regular tasks. Fixed configuration-chat and
  restricted-surface contracts remain distinct.
- Versioned workspace policy and task choices, two-revision CAS, atomic initial
  creation consent, next-process-start binding, reserved identities, and dispatch
  guards. Existing retained processes keep their accepted packages.
- Prompt discovery follows the binding. Accepted manifests, desired selection,
  connection status, and observed catalogs remain separate. Foreign and retired
  execution evidence cannot replace the current owner's evidence.
- Typed Walkthrough intent survives queues, merges, retries and handoffs, and is
  checked again at dispatch. UI requests stop after owner/authentication changes.
- Workspace Built-in detail, task Customize and New Task choices, phone controls,
  localized copy, and a public package guide.

## Completed checks

- Twenty focused backend runtime packages passed, plus normal application wiring,
  atomic creation integration and final resolver refactor checks.
- Affected Go lint against `cc687bdce96bc22fb8a9ca87abe571b8309d3125`: zero issues.
- Eight frontend files: 102 tests passed, including ownership changes, CAS draft
  recovery, collision identity, creation payloads, and acceptance reconciliation.
- Full frontend typecheck and changed-file ESLint with zero warnings passed.
- Seven-language keys, placeholders, pseudo catalog, copy rules and i18n ratchet
  passed. Unrelated generator changes to existing pseudo and Traditional Chinese
  task/startup entries were excluded.
- Public documentation validator: 55 pages. Documentation catalog and all
  specification structure/size checks passed.

- Five focused backend race packages passed (streams, policy, orchestrator,
  queue handlers, and task handlers). Creation regression/error checks passed
  after the missing-primary-session and HTTP error classification fixes.
- Existing MCP explorer: three desktop/tablet and four phone cases passed.
- New desktop journeys: workspace save, task consent/pruning, denied draft,
  initial Configuration consent, and fixed configuration-chat catalog passed.
- Final phone run: workspace save, task consent/pruning, initial consent with a
  real Configuration read, and translated responsive detail passed. Layout checks
  cover 320/393/767/768/900 px, 44 px touch targets, horizontal containment, Back
  focus, one task customization scroller, and light/dark states.
- Six fresh PR screenshots cover desktop/phone workspace policy, task choices,
  and initial consent. The disposable capture tests passed and were removed.

Browser runs were sequential, one worker, retries zero. No full E2E suite or
protected runtime was used. Controlled mock-agent transport exercises real native
catalog and dispatch; it does not certify every installed provider.

## Reproduction and environment

Run the managed focused E2E commands from the work orders with
`KANDEV_SERVER_HOST=127.0.0.1`. This host has an operator config that binds to its
network interface, while the isolated fixture probes localhost. The test process
also needs the Nix Chromium shared-library paths in `LD_LIBRARY_PATH`; the
Playwright-downloaded headless shell cannot resolve them by itself. These are
command-local environment corrections, not repository or main-runtime changes.

The initial browser attempt stopped on stale backend artifacts after a source edit.
A fresh `make -C apps/backend build` resolved that. Later fixture-only attempts
confirmed the bind and Chromium library issues before any UI assertions ran.

A broad handler test attempt timed out at 600 seconds during a large existing
suite. The focused built-in handler tests pass. The separate native subprocess
cancellation fixture also failed because its fixed `/usr/bin:/bin` PATH cannot
find `sleep` on this Nix host. Neither is reported as a passing full-package audit.
The earlier SQLite/PostgreSQL store conformance run, including upgrades, passed
with `-race` in 618 seconds; this continuation did not alter its schema.

## Final browser evidence

The last desktop Configuration/catalog and creation checks passed in 29.9 seconds
(`/tmp/kandev-run.e2e.lH9xH44D.log`). The final mobile run passed all five cases,
including its capture case, in 55.5 seconds (`/tmp/kandev-run.e2e.65LcYDX3.log`).
Desktop capture passed separately in 23.8 seconds. Successful assets and their
merged manifest are under ignored `apps/web/.pr-assets/` and preserved in the
PR-scoped `/tmp/kandev-pr4762-captures-h0m6a9tg/` directory.

An initial test used a Core-only tool on the Configuration endpoint; the runtime
correctly rejected it. The final real read uses `list_shared_prompts_kandev`.
The phone test uses the native Add task control and Send button. Failed test-only
selectors and incorrect catalog paths are not treated as product regressions.

Final visual inspection found content beside the sticky Back control during
scrolling. A full-width opaque header fixes this. Fresh desktop and phone capture
checks passed in 21.1 and 21.5 seconds, including header geometry and task
selection. All six screenshots were refreshed after this correction. Final
assets are backed up in `/tmp/kandev-pr4762-final-captures-pohidv5d/`.

The first commit attempt found two newly staged orchestrator imports that
bypassed the public runtime seam. Both now use the existing runtime history
loader. The staged architecture check exposed this gap in the earlier check of
tracked files; all other hooks passed on that attempt.
