---
id: "01-restore-selected-credentials"
title: "Restore selected credentials on remote resume"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-RESUME-REMOTE-AUTH-001
acceptance_criteria:
  - AC-EXECUTORS-RESUME-REMOTE-AUTH-001.1
  - AC-EXECUTORS-RESUME-REMOTE-AUTH-001.2
  - AC-EXECUTORS-RESUME-REMOTE-AUTH-001.3
  - AC-EXECUTORS-RESUME-REMOTE-AUTH-001.4
system_design:
  - ../../specs/executors/system-design/resume-remote-auth.md
---

# Restore selected credentials on remote resume

## Scope and acceptance

1. Add credential resolution to the resume builder using the fresh-launch helper and eligibility rule.
2. Verify selected GitHub and agent credentials reach environment definitions, with recorded Kubernetes identity preserved.
3. Verify Kubernetes resume uses the current recorded profile selection, including replacement, deselection, and managed-broker fallback.
4. Verify other remote executors, local exclusion, owner visibility, missing secrets, and managed-broker compatibility.

The affected sources include `executor_resume.go` and `executor_resume_kubernetes_env.go`; regressions belong in `executor_resume_remote_credentials_test.go` and `executor_resume_kubernetes_env_test.go`. Update the Kubernetes recovery guide and resume design. No dependency, settings, or deployment changes are required.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor -run '^(TestKubernetesResumeResolvesSelectedRemoteCredentials|TestKubernetesResumeUsesCurrentProfileRemoteCredentials|TestResumeRemoteCredentialEligibility|TestResumeRemoteCredentialsRespectSecretVisibility|TestResumeSession_PreparesFreshManagedGitEnvironment|TestKubernetesResumeRestoresRecordedProfileEnvironment|TestKubernetesResumeProfileEnvironmentFailures|TestKubernetesResumeWithoutRecordedProfileSkipsLookup|TestResolveAuthSecrets)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/executor/executor_resume.go apps/backend/internal/orchestrator/executor/executor_resume_kubernetes_env.go apps/backend/internal/orchestrator/executor/executor_resume_remote_credentials_test.go apps/backend/internal/orchestrator/executor/executor_resume_kubernetes_env_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run the required `make fmt` and `make typecheck test lint` checks after the focused regressions pass.

## Lifecycle evidence

Criteria .1-.3 are covered by `TestKubernetesResumeResolvesSelectedRemoteCredentials`, `TestResumeRemoteCredentialEligibility`, `TestResumeRemoteCredentialsRespectSecretVisibility`, and the existing `TestResumeSession_PreparesFreshManagedGitEnvironment`.

Resume and request reconstruction use persisted fixture metadata. The Kubernetes case deliberately changes session executor/profile selection and verifies recorded identity. The scoped-store case pairs an owner read with another user's denied read. API, UI, and event entry points share this builder; they are not changed here. Duplicate delivery, cancellation, and concurrent ownership transitions are excluded because the repair adds no scheduling, persistent effects, or ownership transitions.

Local cluster and remote-process evidence is recorded below. Idle suspension and retained-PVC replacement remain unverified; neither is simulated by the request-construction tests.

## Results

The new regressions failed before the fix with missing GitHub and agent tokens, then passed after the resolver call. All seven focused tests passed with the race detector. Changed-code lint reported zero issues. `make fmt`, `make typecheck`, and `make lint` passed. Typecheck required the documented generated release-note and changelog files and an 8 GiB Node heap. Documentation catalog, specification, public-doc, architecture, and harness checks passed; local PR documentation coverage reported `covered`.

The full executor race run under an isolated home directory and physical temporary path failed these three tests on macOS:

- `TestContinuationNativeStartupFailureRetainsAdmissionReleaseFailure`
- `TestMissingCheckoutRecoveryLaunchAndResume`
- `TestTerminalResumeCleansStaleExecutionBeforeRecoveryAdmission`

All three also failed with the original upstream resume source under the same environment settings. The failures involved checkout recovery through `/dev/fd/3`. The initial run without isolation had additional path-pinning and Git transport failures.

`make test` failed in plugin-fixture networking, launcher containment, lifecycle, journal, and agentctl API/configuration tests. Remaining long-running cases were stopped after those failures; the full repository suite did not pass. These broader failures were not all reproduced on the base. Web, CLI, and script test targets were not reached.

### Manual runtime verification

Manual local Kind testing used an ARM64 worker, a mock agent, dummy secrets, and an isolated backend. Fresh launch and message-driven resume after a controller restart supplied both GitHub token aliases and the agent credential to the worker process and session auth file. Updated fixture values reached the resumed process and a real `/bin/sh -lc` through Kandev's terminal API. Pod UID, workspace marker, environment, executor, and profile identity were preserved; the expected agent reply arrived. The isolated runtime was removed.

Authentication against a real provider, idle suspension, and retained-PVC replacement were not tested.

Two independent implementation reviews found no actionable issues. Additional reviews checked upstream metadata conventions and contribution guidance; their requested wording corrections are included.

### Maintainer follow-up (2026-11-28)

The follow-up loads `remote_auth_secrets` from the current recorded Kubernetes profile before the running-record metadata fallback. An empty selection or missing profile clears the older binding. Regression coverage checks replacement, deselection, both GitHub token aliases, and the managed broker fallback. The local and worktree eligibility cases also assert that session metadata is unchanged.

Focused verification passed:

```bash
(cd apps/backend && go test -trimpath -count=1 -run '^(TestKubernetesResumeUsesCurrentProfileRemoteCredentials|TestKubernetesResumeResolvesSelectedRemoteCredentials|TestResumeRemoteCredentialEligibility|TestKubernetesResumeRestoresRecordedProfileEnvironment|TestKubernetesResumeProfileEnvironmentFailures|TestKubernetesResumeWithoutRecordedProfileSkipsLookup)$' ./internal/orchestrator/executor)
```

The command passed. `bash scripts/lint-go-changed apps/backend/internal/orchestrator/executor/executor_resume.go apps/backend/internal/orchestrator/executor/executor_resume_kubernetes_env.go apps/backend/internal/orchestrator/executor/executor_resume_remote_credentials_test.go apps/backend/internal/orchestrator/executor/executor_resume_kubernetes_env_test.go` reported zero issues. `python3 scripts/list-docs.py validate` validated 468 decisions and 2,135 specifications; `python3 scripts/lint-spec-files.py --all` passed. `node --test scripts/validate-public-docs.test.mjs` passed all 63 tests, and `node scripts/validate-public-docs.mjs` validated 56 published docs pages. `git diff --check` passed.
