---
id: "01-url-install-response-deadline"
title: "Reserve the write deadline for the install result"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGINS-001.5
  - AC-PLUGINS-PLUGINS-001.22
system_design:
  - ../../specs/plugins/system-design/plugins-01.md
---

# Task 01. Reserve the write deadline for the install result

## Summary

Let an authorized URL install complete while the client waits for the final
result. Apply the configured server write timeout when the handler sends
that result.

## In scope

- Add `TestInstallHandlerURLResponseSurvivesWriteTimeout`. Start with the
  failing HTTP regression before editing the handler.
- Reuse `newAdminTestRouter` and `testPackage`. Start a real HTTP test server
  with a 200 ms write timeout and a package server with a 500 ms delay.
- Test a valid package whose first byte arrives before the delay. Verify
  `201` and its active record, then repeat the same URL with an immediate
  origin response and verify `409`.
- Test a delayed upstream error and verify the existing JSON error response.
  Check that storage has no plugin from that request.
- Apply the deadline change after administrator authorization and JSON
  validation. Restore the configured write window on return from the install
  call, before the handler writes its success or error response.
- Document the URL download and response windows in the public plugin guide
  and configuration reference.

## Out of scope

- Multipart upload and the deadlines of other routes.
- Browser presentation, client retries, and reverse proxy behavior.

## Acceptance

- The delayed success case fails with EOF on the current handler. It returns
  `201` and an active plugin record after the fix. A repeat returns `409`.
- A delayed upstream error returns the mapped `500` with an error body.
  The plugin store remains empty for that attempt.
- The URL handler uses the positive server timeout for the final write.
  The existing tests cover administrator access, bad input, and multipart upload.

## Verification

Run these commands from the repository root.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/plugins -run '^(TestInstallHandler|TestServiceInstallSpawnFailureLeavesRecordInErrorStatus$)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/plugins/handlers.go apps/backend/internal/plugins/handlers_install_deadline_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- apps/backend/internal/plugins docs/specs/plugins docs/plans/plugin-install-write-deadline docs/public
```

## Files likely touched

- `apps/backend/internal/plugins/handlers.go`
- `apps/backend/internal/plugins/handlers_install_deadline_test.go`
- `docs/public/plugins.md`
- `docs/public/configuration.md`

## Dependencies

None.

## Risks

- Give the client time to read the result after the delayed download.
- The test fixture needs a delay long enough to exceed the server deadline
  even when CI scheduling varies.

## Parallelism

`sequential`

## Inputs

- [Issue #5519](https://github.com/kdlbs/kandev/issues/5519)
- [Plugin requirements](../../specs/plugins/requirements/plugins.md#req-plugins-plugins-001-plugin-system)
- [URL install response design](../../specs/plugins/system-design/plugins-01.md#url-install-response-deadline)
- [Existing install handler tests](../../../apps/backend/internal/plugins/handlers_test.go)

## Results

Implemented the URL install response deadline and public documentation. The
new real HTTP regression failed with EOF in both delayed cases before the
handler change and passed afterward.
The test-only response writer now records the active deadline at each JSON
write and checks the delayed success, duplicate conflict, and delayed error
responses. A temporary removal of the deferred reset made the delayed success
and error assertions fail.

- `go test -trimpath -tags fts5 ./internal/plugins -run '^(TestInstallHandler|TestServiceInstallSpawnFailureLeavesRecordInErrorStatus$)' -count=1` — passed.
- `GOCACHE=/tmp/kandev-5519-review-gocache go test -trimpath -tags fts5 ./internal/plugins -run '^TestInstallHandlerURLResponseSurvivesWriteTimeout$' -count=1` — passed after the shared cache returned missing archive errors.
- `bash scripts/lint-go-changed apps/backend/internal/plugins/handlers.go apps/backend/internal/plugins/handlers_install_deadline_test.go` — passed, 0 issues.
- `make -C apps/backend build-kandev` — passed.
- `python3 scripts/list-docs.py validate` — passed.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `node --test scripts/validate-public-docs.test.mjs` — passed, 63 tests.
- `node scripts/validate-public-docs.mjs` — passed, 56 pages.
- `git diff --check` for changed implementation, spec, plan, and public-doc paths — passed.
- Reviewer follow-up lint rerun was blocked by concurrent `golangci-lint` and Go compiler work; the targeted test and `gofmt` checks passed for the response-writer capture change.
