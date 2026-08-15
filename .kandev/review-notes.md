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
