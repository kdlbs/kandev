# Launcher signal fixture CI follow-up

## Failure evidence

The first CI run after merging main into PR #3585 tested head
`bb4e1851081e9d60d0baf60a9b29c485d3341650`, containing base
`f817de3df5444cc344b3090c43c8db898e98b5be`.
[Backend shard 2](https://github.com/kdlbs/kandev/actions/runs/34548757427/job/103107045718)
failed `TestManagedProcessKillSignalsOnlyRootBeforeForceKill` while waiting
for the `root-term` marker. The structured `backend-test-results-2` artifact
recorded that assertion after 5.24 seconds. The required backend aggregate
failed because of this leaf failure, not a second failing test.

The launcher source and test were unchanged from the previous passing PR
head and the merged base. Twenty focused local runs before the correction
passed, so local success alone did not dismiss the recorded scheduling race.

## Correction

Both process-tree helpers wrote their readiness markers before calling
`signal.Notify`. A parent observing readiness could therefore send SIGTERM
before the helper registered its handler, terminating it without writing its
signal marker. Register handlers before publishing readiness in both the
root and descendant helpers. Existing assertions, timeouts, process cleanup,
and production shutdown behavior remain unchanged. This fixture-only
correction does not change product requirements, UI, or public documentation.

## Local validation

Both signal-targeting cases passed 20 times each with race detection and
atomic coverage. The full launcher package then passed three uncached race
runs. Changed-code Go lint against the merged base and Go 1.26 formatting
checks passed.

From `apps/backend`:

```sh
go test -race -covermode=atomic -coverprofile=/tmp/kandev-3585-launcher-signal.cover ./internal/launcher -run '^TestManagedProcessKill(SignalsOnlyRootBeforeForceKill|TargetsBackendPIDFile)$' -count=20
go test -race ./internal/launcher -count=3
golangci-lint run ./... --new-from-rev=f817de3df5444cc344b3090c43c8db898e98b5be --timeout=5m
```

Fresh exact-head CI and review confirmation remain required after pushing
this correction; prior-head successful checks do not satisfy that gate.
