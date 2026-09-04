# Task 02: Launch-recogniser registry seam

Spec: §"The launch-recogniser seam", D7. ACs: AC-37, AC-69, D7's own nil/empty
rules (round-5 F14).

## Shape

```go
package recogniser // new package, e.g. internal/orchestrator/parkedwork or a
                    // small standalone package — see "package placement" below

type BackgroundLaunchRecognizer interface {
    AgentID() string
    RecognizesDetachedLaunch(payload *streams.NormalizedPayload) bool
}

func Register(r BackgroundLaunchRecognizer)          // programming-error semantics below
func Lookup(agentID string) (BackgroundLaunchRecognizer, bool)
```

## D7 nil/empty rules (round-5 F14 — spec left these unspecified; Build decides)

- `Register` with an empty `AgentID()` or a nil recogniser: **panic at
  registration time** (init-time programming error, consistent with how Go
  registries like `sql.Register`/`image.RegisterFormat` behave — fails loudly
  at process start, never silently).
- `Register` called twice for the same agent ID: **panic** ("programming
  error, not a runtime merge" per D7) rather than first-wins/replace, so a
  duplicate registration is caught in CI rather than silently shadowed.
- Two concurrent `Register` calls: registry is fixed at process start (D7), so
  guard with a simple mutex; no concurrent-registration behaviour needs to be
  more permissive than "last one to grab the lock wins, but this should never
  happen after init".
- A recogniser whose `RecognizesDetachedLaunch` panics: caught and treated as
  "did not recognise" (fail closed), per D7's explicit requirement.

## Package placement (for F1/F2's import-direction test to be meaningful)

Put the registry in its own leaf package (e.g.
`internal/agentctl/server/adapter/transport/acp/backgroundwork` or similar,
colocated with `stampBackgroundShellWork`) that imports only `streams` types.
It must NOT import the probe (task-03/04), the parked projection
(task-05/06), or anything under `apps/web`. This is what makes AC-69's
import-direction test (see task plan's F1 disposition) non-vacuous.

## Ship-time registration

Register exactly one recogniser at ship time: Claude, whose
`RecognizesDetachedLaunch` reimplements today's condition
(`payload.ShellExec() != nil && payload.ShellExec().Background`, currently
inline in `stampBackgroundShellWork`, `normalize.go:304-308`). Wire the
existing `stampBackgroundShellWork` call sites to also attest through the
registry (or have the registry become the single source of truth and
`stampBackgroundShellWork` consult it — keep `BackgroundWorkPayload`'s shape
unchanged either way, per §I).

## Tests

- Nil / empty-ID / duplicate registration each panic (`recover()` in test).
- A recogniser that panics on `RecognizesDetachedLaunch` is treated as
  "did not recognise" by the caller, not propagated.
- Lookup miss for an unregistered agent ID returns `(nil, false)`.
- **AC-69, part (a) — behavioural.** Register a second recogniser (fake agent
  ID) through `Register` from a test in a different package, drive a session
  through the full pipeline (attestation → probe stub returns `live` → turn
  settles), and assert the session parks and its task row would render the
  background test id (this can be a projection-level assertion; task-07 adds
  the actual DOM-level assertion once rendering lands).
- **AC-69, part (b) — import-direction.** A test (e.g. using `golang.org/x/tools/go/packages`
  or a simple `go list -deps` shell-out wrapped in a Go test) asserts the
  registry package's import list contains nothing from the probe, projection,
  or `apps/web` paths.
- AC-37: an agent with no registered recogniser is never attested (`Lookup`
  returns false) — end-to-end this is asserted again in task-05's projection
  tests.
