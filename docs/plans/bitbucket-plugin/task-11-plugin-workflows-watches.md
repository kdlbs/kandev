---
id: "11-plugin-workflows-watches"
title: "Plugin task, Git, linking, and watch workflows"
status: completed
wave: 3
depends_on: ["03-protocol-manifest-actions", "05-dynamic-composer-reference-sources", "06-plugin-owned-task-lifecycle", "07-provider-neutral-git-credentials", "10-cloud-dc-domain-auth"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 11: Plugin task, Git, linking, and watch workflows

## Intent

Use released host seams to implement Bitbucket plugin actions/RPC handlers for task
launching, scoped Git credentials, PR linking/creation, and durable watches.

## Owned paths

- Attached `kdlbs/kandev-plugin-bitbucket` worktree: action/RPC handlers, task and
  Git workflow services, link storage, credential resolver, health poll integration,
  watch persistence/poller/recovery, events, and focused tests.

## Dependencies

Tasks 03, 05, 06, 07, and 10.

## Acceptance

1. Authenticated actions launch tasks from PRs, link/unlink tasks, create PRs from
   task branches, and resolve Git credentials without secrets crossing host boundaries.
2. Plugin `pull_request` search and authorization use the live Cloud/DC adapter for
   both composer search and message submission; they do not bypass host canonicalization.
3. Watches persist definition/cursor/dedupe/reservation/link/recovery state, use keyed
   mutex plus durable `creating` reservation, reconcile crashes, and emit
   `plugin.kandev-plugin-bitbucket.*` events.
4. Reset/delete previews cascade and removes only plugin/watch-owned task trees;
   manually linked and adopted tasks survive.
5. The Cloud/Data Center provider wrapper preserves restart-safe PR paging; a
   watch-created task is returned by both task-scoped PR and workspace-association
   queries after restart.
6. Creating a PR from a verified task persists its association immediately. A remote
   create success followed by local association failure returns a bounded partial
   result rather than a retryable error that could duplicate the remote PR.
7. Review status adapters query the PR source/head commit.
8. Task-scoped refresh performs bounded, exact source-branch PR discovery from the
   host-verified repository checkout and persists the result idempotently; discovery
   errors do not suppress existing associations.

## Verification

```sh
make test
make vet
make build
```

## Risks

Concurrent polls and crash timing can duplicate tasks. Credential refresh and ownership
checks must fail closed; avoid adding Bitbucket logic to host `agentctl` PR automation.

## Completion

- Live Cloud acceptance proved manual linking through the host-native dialog and durable
  association recovery after host/plugin restarts.
- A disposable linked task was explicitly unlinked, then task-scoped refresh rediscovered
  the open pull request by its exact host-verified `kandev-live-test` checkout branch and
  persisted the association again (`0 -> discovered key -> 1`).
- The live watch created exactly one owned task and a subsequent run skipped the same pull
  request. Cloud paging now respects Bitbucket's 50-item maximum.
- Review status reads the source commit: the live `KANDEV-LIVE-ACCEPTANCE` marker resolved
  target `c1c53106bebf`, not the destination commit.
- Final plugin checks passed: 37 UI tests, `go test ./...`, `go test -race ./internal/...`,
  `go vet ./...`, host build, five-platform package build, and checksum verification.
