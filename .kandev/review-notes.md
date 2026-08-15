## Fixed during review

- `.github/workflows/e2e-tests.yml:315,494` — the new `global-setup.ts` backend-binary
  freshness guard would have fired on every CI E2E shard. Both the `e2e` (14 shards) and
  `e2e-containers` (6 shards) jobs check out the repo first and then download
  `apps/backend/bin/kandev` from the `build` job's artifact, so every `apps/backend` source
  file carries a checkout mtime later than the binary's — even though both come from the
  same commit and `needs: build` already guarantees they agree. Set the documented
  `KANDEV_E2E_SKIP_FRESHNESS=1` opt-out on both run steps; `apps/web/e2e/README.md` already
  advertised it for exactly this "CI stages artifacts out of band" case, but nothing wired
  it up. (commit `8ce472739`)

- `apps/web/e2e/global-setup.ts:74` — the freshness guard walked *every* file under
  `apps/backend`, including Go build/test output that lands in the source tree rather than
  in the excluded `bin/` or `.build/`. `make -C apps/backend test-coverage` writes
  `coverage.out` and `coverage.html` next to `go.mod`, and `go test -c` leaves `*.test`
  binaries, so the everyday "run backend tests with coverage, then run E2E" sequence
  aborted global setup demanding a rebuild over a coverage profile that changes nothing
  the binary contains. Now skips the three patterns the repo `.gitignore` already lists as
  Go output; none is `//go:embed`-ed and no tracked file under `apps/backend` matches one,
  so the guard cannot miss a genuinely stale binary. Covered by three unit cases (10/10) and
  confirmed on the real tree. Beyond showing the three patterns are ignored, two cases pin the
  properties that keep the exclusion safe: a coverage run alongside a genuinely newer `.go`
  still aborts and names the `.go` (the skip removes files from consideration rather than
  short-circuiting the check), and the `$` anchors hold, so `notes.output` is still counted.
  Both were confirmed to fail against a deliberately loosened pattern list. (commits
  `f21a1a631`, `5bb4d38d7`)

- `apps/web/e2e/global-setup.ts:41`, `apps/web/e2e/global-setup.test.ts:95` — both comments
  justified excluding `internal/webapp/embedded/generated/` with "it is written *after* the
  backend builds". Every `sync-embedded-web` call site runs it *before* the backend build
  (`Makefile:264`, `:279`, `:333`), so the stated mechanism was wrong even though the
  exclusion itself is right: E2E serves `apps/web/dist`, and refreshing the copy does not
  require relinking the binary. Rewritten to the accurate rationale so a future maintainer
  does not re-derive the exclusion from a false premise. (commit `f21a1a631`)
