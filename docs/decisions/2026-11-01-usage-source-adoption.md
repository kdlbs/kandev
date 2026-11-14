# ADR-2026-11-01-usage-source-adoption: Adopt existing sessions as usage sources once

**Status:** accepted
**Date:** 2026-11-01
**Area:** backend, protocol

## Context

Retained usage only trusts a transcript as a source when evidence was captured at session creation.
Every session that existed before this capture is permanently `unknown`, so historical import cannot read the history it exists to import.
Review of the retained usage PRs reproduced this: one such session also stopped all collection and failed the whole import.

The creation rule exists because a transcript's cumulative totals can include work from another scope, such as a forked session or a transferred task.
Discovery must not be able to promote a session simply because a collector asked about it.

## Decision

Core runs one bounded migration step that adopts existing sessions as eligible sources when all of these hold:

- The session has a recorded transcript ID.
- The session is not forked.
- The existing source guards find no use of the transcript in another workspace or organization.
- The task has no `task_transfer_audit` row.
- The workspace and organization are not being erased, and have not been erased.

Adoption records `evidence='migration'`, so it stays distinguishable from creation evidence.
It never assigns an actor. Discovery and current-target lookups still cannot create trust, and sessions that fail a rule stay `unknown`.

## Consequences

Existing installs can import history for most existing sessions, and those sessions regain composer cost without a collector workaround.
Adoption reads current ownership, so it is weaker evidence than capture at creation. A move that left no audit row cannot be detected.
The risk is limited because unsafe transcript sharing is still blocked, actors stay unknown, and the evidence value allows a later correction to target adopted rows.

## Alternatives Considered

- **Keep creation evidence only.** This is the safest attribution, but import cannot serve existing history, which is its purpose.
- **Promote sessions during discovery or at import time.** This lets any collector call create trust, which the retained usage design forbids.
- **Trust everything in single-tenant mode.** This is simple, but forks can still mix totals, and the rule would change with deployment mode.
- **Ask the user to confirm each session.** This adds work for a decision users cannot verify from a session list.
