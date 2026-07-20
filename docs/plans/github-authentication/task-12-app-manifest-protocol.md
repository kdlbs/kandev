---
id: "12-app-manifest-protocol"
title: "GitHub App manifest protocol"
status: done
wave: 8
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 12: GitHub App Manifest Protocol

## Inputs

- Spec: GitHub App permissions, deployment registration API/state machine, URL failure modes.
- Official contracts: GitHub App Manifest registration, conversion, setup URL, and App webhook docs
  linked from the spec/ADR.
- Existing patterns: `apps/backend/internal/github/app_client.go` bounded responses and
  `oauth_flow.go` random state/hash/expiry behavior.

## Acceptance

1. A versioned manifest generator emits the exact GitHub.com permissions, events, callback, setup,
   and webhook URLs for personal and organization owners without operator-editable JSON. It sets
   public installability, OAuth-on-install, and setup-on-update exactly as required by the spec.
2. Public-origin validation rejects non-HTTPS, userinfo, query/fragment, loopback, and private or
   link-local literal addresses; requires every DNS result to be globally routable; and never
   performs an outbound reachability fetch.
3. The bounded conversion client parses App identity plus generated credentials, uses stable
   sanitized errors, and tests one-hour state expiry/replay helpers without logging secrets.

## Files Likely Touched

- `apps/backend/internal/github/deployment_app_manifest.go` (new)
- `apps/backend/internal/github/deployment_app_manifest_test.go` (new)
- `apps/backend/internal/github/deployment_app_conversion_client.go` (new)
- `apps/backend/internal/github/deployment_app_conversion_client_test.go` (new)

## Verification

```bash
rtk go test ./internal/github -run 'Test(DeploymentAppManifest|PublicGitHubBaseURL|ManifestConversion)' -count=1
```

Run from `apps/backend`.

## Dependencies

None. Keep protocol types local so this can land beside Task 11; Task 13 integrates both outputs.

## Output Contract

Report the manifest revision/fields, URL rules, conversion bounds, secret-redaction tests, files
touched, commands run, blockers, and GitHub contract risks. Mark this task `done` and update
`plan.md` only after targeted tests pass.
