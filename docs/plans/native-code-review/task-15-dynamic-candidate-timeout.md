---
id: 15-dynamic-candidate-timeout
title: Reuse utility routing after native reviewer request timeouts
status: building
wave: 3
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-NATIVE-REVIEW-TIMEOUT-001
acceptance_criteria:
  - AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.1
  - AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.2
  - AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.3
  - AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.4
system_design:
  - ../../specs/agents/system-design/native-review-timeout.md
legacy_specs: []
---

# Native reviewer request timeout recovery

Base: upstream 5302566674b5db307a5d03fed79f4b62f884fe79. Move the duplicate
utility profile loop, connect native review identity and route state, preserve
typed parent/child context ownership and reuse the existing failure writer.
Use disposable existing runner/HTTP/profile fixtures for before/after evidence;
then run affected existing checks and scoped lint. Integration owns Git.

The earlier read-only map demonstrated that native review never called the
dynamic failure resolver: a live parent plus candidate deadline ended the run,
and session errors fell back to the same host profile. The old fork's separate
retry-run/preview picker is not copied. Abandoned-run recovery (G1/PR4513),
operating endpoint health and child capacity/provider admission remain separate.

## Implementation and preserved boundaries

The shared utility-service helper replaces both handler loops. Review profile
lookup uses the existing ProfileExecutionResolver, with a transient run-local
route passed through ReviewerIdentity across batches. The callback still executes
the concrete profile through native session/host utility. The initial version
treated a typed deadline with a live parent as transient; the follow-up below
supersedes that assumption because it does not prove child termination. Existing retry
deadlines resume through ResumePendingRoute. No global deadline text rule, new
run, ledger, candidate ordering or timeout value was introduced.

The existing failure writer retains its detached ten-second context. Its storage
error now remains reachable from the returned/logged error, alongside the original
execution cause. Partial output and parent cancellation stop before another
candidate. A child request deadline does not retry the same candidate through
the host. Bound utility/default profiles keep the original native default-model
case; explicit reviewer profiles keep their prior model validation.

Integration supplied the frozen canonical agentctl inference consumer contract.
This change forwards that exact contract through the existing runtime facade and
attaches stored call/profile or verified task/session/review-run relationships at
dispatch. Candidate transitions update concrete execution identity only. A typed
capacity wait/refusal stops before host fallback or provider policy, even if its
wrapped cause is a deadline. The lifecycle/host-utility files and actual admission
implementation were not modified. Thus this standalone base prepares the callback
contract; actual child admission and unknown-predecessor settlement require the
separate C2b composition. A request timeout itself never releases a lease.

## Verification

The existing read-only baseline proof observed one failed inference call with a
live parent and no next-candidate transition (backendapp 0.737s, review 0.103s).
That baseline's PASS means the missing behavior was observed, not that recovery
was implemented. The corrected disposable proof uses the actual profile resolver,
dynamic engine, review runner and shared callback path; profile/store/inference
boundaries are synthetic and no real provider or operating database was used.

```text
go test -trimpath -p 2 -race -overlay OVERLAY ./internal/backendapp ./internal/review -run '^TestG2Review' -count=1 -timeout 60s -v
PASS: backendapp 2.291s; review 1.129s
```

Observed: same run A-to-B success; A/B exhaustion with one live-context terminal
write; A-to-B then B on the next batch with unchanged route ID; cancellation
before/during inference without another dispatch; partial output without replay;
saved retry policy A, one-second policy wait, A, B; failed terminal write retaining
both the storage error and wrapped deadline. No six-minute/600-second wait or model
call was made. An eight-attempt cap remains in the moved source loop.

After the frozen identity/refusal branches were added, only those branches ran:

```text
go test -trimpath -p 2 -race -overlay OVERLAY ./internal/backendapp ./internal/utility/service -run '^TestG2(ReviewCapacityContextNoBypass|UtilityOriginContextPreserved)$' -count=1 -timeout 60s -v
PASS: backendapp 1.420s; service 1.283s

go test -trimpath -p 2 -race -overlay OVERLAY ./internal/review -run '^TestG2ReviewBoundProfileKeepsNativeDefaultModel$' -count=1 -timeout 60s -v
PASS: review 1.135s
```

The context proof rejects a mismatched stored task/session/run and confirms the
logical profile/run/call identity reaches one concrete candidate. A typed capacity
deadline reaches neither the host fallback nor candidate B. The utility proof
keeps its existing parent call identity. The first temporary blank-model fixture
had a pointer-receiver mismatch; correcting that fixture required no production
change. A first compile check found one stale import after moving the loops; it
was removed. These failed attempts are not reported as passes.

Existing focused handler routing, resolver and cancellation checks passed
(handlers 0.061s, review 0.063s); backendapp compiled with no selected existing
tests. Catalog validation passed (384 decisions, 1618 specifications), as did
specification structure/size and architecture checks. The initial seven-package
lint reported only two complexity limits. Splitting the session-response and
fallback predicates resolved those; the final affected three-package delta was:

```text
golangci-lint run --allow-serial-runners ./internal/backendapp ./internal/utility/service ./internal/review --new-from-rev=HEAD --timeout=5m --concurrency=2
exit 0; 0 issues.
```

No frontend, generated value, new permanent test or operating setting changed.
Disposable source and original command output remain outside collection for
integration readback, then cleanup. Abandoned-run G1/PR4513 and actual child
capacity/provider enforcement remain separate; this does not complete all three
original queue symptoms or claim operating acceptance.

Independent source review found a partial-response loss at the review adapter:
when the session returned both visible response content and a typed deadline,
runConcrete discarded the response before the shared no-retry guard. A single
new disposable case reproduced actual calls `[A, B]` instead of `[A]` (RED,
backendapp 0.597s). The adapter now returns the original response and error
together, including before host fallback, so the existing guard sees the partial
content. The same case alone passed under race (GREEN, backendapp 1.436s). The
current concrete host utility methods return nil result whenever error is nonnil;
no mixed host-result/error source path was found and those leaf files remain
unchanged. The earlier passing matrix was not repeated.

```text
go test -trimpath -p 2 -race -overlay OVERLAY ./internal/backendapp -run '^TestG2ReviewPartialWithDeadlineNoRetry$' -count=1 -timeout 60s -v
before: FAIL, calls [A, B], 0.597s
after: PASS, 1.436s

golangci-lint run --allow-serial-runners ./internal/backendapp --new-from-rev=HEAD --timeout=5m --concurrency=2
exit 0; 0 issues. (natural completion)
```

Final actual changed-path documentation coverage: 15 paths, `covered`,
`errors: []`. `git diff --check` passed. No execution handle remains active.

The final moved-loop comparison also found a boundary introduced during this
implementation: checking the eight-attempt cap before reporting the last failure
would omit its policy/circuit update. A narrow callback-boundary counterexample
observed eight actual callbacks but seven failure-resolver calls (RED, 0.059s).
The limit check now follows that last failure report, as in the original loop,
and still starts no ninth invocation or additional wait. Only that case ran after
the correction, under race: eight callbacks/eight failure reports (GREEN, 1.123s).

```text
go test -trimpath -p 2 -race -overlay OVERLAY ./internal/utility/service -run '^TestG2UtilityAttemptBoundRecordsFinalFailure$' -count=1 -timeout 60s -v
PASS, 1.123s

golangci-lint run --allow-serial-runners ./internal/utility/service --new-from-rev=HEAD --timeout=5m --concurrency=2
exit 0; 0 issues. (natural completion)
```

## Unconfirmed transport deadline follow-up

A disposable actual ExecuteProfilePrompt callback returned a wrapped
DeadlineExceeded while its parent remained active. The previous classifier
called candidate A and then B and returned nil error (RED, service 0.113s), even
though the first child's termination was unconfirmed. That deadline now remains
unclassified, with no automatic retry/fallback; the original wrapped error is
retained. The same case passed under race (GREEN, service 1.416s).

This supersedes the earlier synthetic claims that a plain typed deadline alone
proved a safe fallback. AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.2 remains incomplete
until the native child owner supplies exact terminal proof. Requirements are
preserved, not weakened to match this safe intermediate result. Parent stop,
empty transport response and live parent context do not supply that proof.

The original uncommitted ordered-success controller/service follow-up remains
preserved in this worktree outside this atomic public deadline guard. Its separate
integration evidence does not establish public child-deadline recovery. Final
normal hooks and independent review remain required before public delivery.
