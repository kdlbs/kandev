# Voice webhook evidence

## Historical observation

The retained backend log at `/root/.kandev/logs/backend-logs-2026-10-05-000006.log:32894` records plugin `kandev-plugin-voice` returning HTTP 503 with origin `plugin_response`. The host correctly retained the plugin-supplied status. The event does not contain the response body, plugin version, config-read result, or provider response. No audio, transcript, credential, or live plugin state was inspected.

## Pinned source and behavior

The dedicated repository was inspected read-only at `ea92f43f8aae2568ff5e37faf7ea0922b59f2930`, manifest version `0.1.1`. Its manifest declares the `transcribe` webhook as authenticated. No repository-local `AGENTS.md` was present.

At this revision, the plugin returns 503 when the host is unavailable, configuration cannot be read, or the configured API key is absent or blank. All three produce the same public error. An upstream HTTP error, including credential refusal or rate limiting, becomes a generic 502. A non-HTTP transport failure such as a timeout becomes a generic 500. Successful synthetic transcription returns 200.

Therefore, the observed 503 is consistent with the host/configuration branches in this pinned version and inconsistent with its upstream HTTP-error and transport-timeout branches. The installed version is absent from the retained event, so the pinned source cannot establish which branch ran in production.

## Regression evidence

The existing plugin tests cover missing configuration, host/config read failure, successful transcription, upstream credential refusal, internal handling of a synthetic 429 response, and request cancellation. Two temporary tests in a disposable archive additionally verified the webhook-level mapping for a synthetic 429 (502) and provider timeout (500). The archive and temporary tests were not applied to either repository.

The host regression passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/plugins -run 'TestWebhook' -count=1)
```

`TestWebhookFailureOriginLogsSafeFields` verifies that the host relays a plugin-supplied 503, records plugin ID/status/origin, and omits response body, request body, query, headers, and raw errors.

The pinned plugin's documented `make test` suite passed in an isolated test archive: Go tests passed and all 57 UI tests passed. The test environment required `GOFLAGS=-mod=mod` for the current local SDK and a test-only `packages: [.]` entry because the available pnpm 9 rejects the checkout's pnpm-11 workspace configuration. Neither adjustment was made to the pinned checkout. The plugin build also passed in that archive with VCS stamping disabled because `git archive` has no `.git` metadata.

## Classification and proposal

Classification: **production cause unresolved; no plugin or core defect established for this 503**. Candidate branches at the pinned revision are missing/blank configuration, unavailable host injection, or a configuration read failure. Distinguishing these requires the installed plugin version and a safe plugin-owned outcome signal, which are not present in the host event. No live configuration or installation data was read.

If further diagnosis is authorized, a versioned plugin-owned regression package should test and emit a closed, non-sensitive class for `host_unavailable`, `config_read_failed`, and `not_configured`, while keeping the public 503 response unchanged. It should also retain the existing tests that map upstream 401/429 to 502, transport timeout to 500, and success to 200. No provider call, code change, release, or deployment was performed.
