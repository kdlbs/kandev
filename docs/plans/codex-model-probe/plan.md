---
created: 2026-09-25
status: complete
requirements:
  - REQ-AGENTS-CODEX-NATIVE-001
  - REQ-AGENTS-CODEX-NATIVE-002
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
legacy_specs: []
---

# Fix plan: Native Codex model discovery

## Evidence and scope

The settings model refresh fails before launching Codex. The generated managed command includes
`--prefix ~/.kandev/managed-npm-runtime`, but `resolveCodexAppServerCommand` accepts only the older five-argument npm command.
The native utility launcher also omits `managedruntime.PrepareNPMProjectPrefix`, which the ACP launcher calls before spawn.

A temporary `TestReproNativeCodexGeneratedProbeCommand` passed the real
`agents.NewCodexAppServer(true).InferenceConfig().Command.Args()` to the utility resolver.
It failed with the exact screenshot error. The existing hand-written allowlist tests passed in the same run.
The temporary test was removed after diagnosis. No provider process or authenticated turn ran.

This violates the existing native model-discovery contract; no product or UI design change is required.
Reuse the existing settings controls, feature flag, managed version selection, and trusted npm isolation helper.
Do not relax the allowlist to arbitrary commands, prefixes, packages, wrappers, or additional arguments.

## Work orders

- [x] [01: Accept and prepare the generated managed command](task-01-managed-command.md)

## Verification results

The real command builder regression covered the default and selected exact versions. A fake `npx` app-server initialized, returned a model catalogue through `Probe`, and recorded the prepared private prefix. The shared launch test confirmed prefix preparation without mutating the original command, and a failed preparation prevented spawn.

Verification passed:

- `(cd apps/backend && go test -race ./internal/agentctl/server/utility ./internal/agent/hostutility ./internal/agent/managedruntime ./internal/agent/agents)`
- `make -C apps/backend lint` (0 issues)
- `python3 scripts/list-docs.py validate` (306 decisions, 1155 specifications)
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

No authenticated Codex request ran. No browser layout changes or new UI controls were needed.

## PR fixup notes

Backend CI identified repeated command literals in the resolver predicates. They are now shared constants; the utility race tests and CI-style changed-code lint pass after this correction. The first PR documentation-coverage run failed when GitHub code search returned HTTP 429 after exhausting its wait budget. The documentation-coverage and backend-static-check jobs passed on the subsequent source-fix push; its full PR snapshot had 60 passed, 0 failed, and 0 pending checks, with no open review threads. A plan-only reconciliation follows, so its own PR check snapshot must be verified separately.

## Risks

Accepting the prefix without preparing it can leave a literal tilde path or npm state in the wrong directory.
Model probing and utility inference share this launch path; both need coverage.
