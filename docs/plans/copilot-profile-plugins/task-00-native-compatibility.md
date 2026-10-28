---
id: "00-native-compatibility"
title: "Qualify Copilot inventory and mount compatibility"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-001.1
  - AC-AGENTS-COPILOT-PLUGINS-003.2
  - AC-AGENTS-COPILOT-PLUGINS-003.3
  - AC-AGENTS-COPILOT-PLUGINS-004.1
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-007.2
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
---

# Task 00: Qualify native compatibility

## Summary

Produce a reproducible compatibility matrix before production adapters depend on private metadata or native policy behavior.
This is an evidence work order. It changes documentation and disposable fixtures only.

## In scope

- Recheck official references linked in the design and the current managed selection/default.
- Use isolated runtime packages and homes. Start with the checkout's `1.0.88` pin.
- Verify package metadata and actual reported binary version. A mismatch must remain unqualified.
- Observe flat/envelope inventory, missing/disabled entries, direct installs, local marketplace/live roots, JSONC state, and manifest precedence.
- Exercise repeated `--plugin-dir` arguments with a harmless fixture plugin and native MCP disables.
- Establish explicit-mount behavior under managed prohibitions from primary-source evidence and native fixtures where available.
- Use `/acp-debug` for native ACP sessions. Capture raw frames privately, and record sanitized outcomes and exact commands.
- Record supported versions/platforms/source mappings in `compatibility-evidence.md` and reconcile the design matrix.

## Out of scope

Production code, permanent test changes, runtime pin updates, global plugin management, real credentials, login, and remote executors.
Do not infer a managed prohibition from `enabled: false`.
Do not treat a successful handshake as proof of plugin loading or a successful tool call.

## Acceptance

1. Evidence identifies exact package, binary version, platform, inventory shape, root mapping, and manifest behavior for every proposed supported adapter.
2. Repeated mounts and native veto behavior have explicit results. Missing evidence excludes that runtime/source rather than silently weakening the gate.
3. The matrix distinguishes reproduced results, supplied observations, unsupported cases, and authentication limitations without changing managed selection.

## Verification

Run from the repository root after implementation is authorized.
The following baseline probe uses no developer home or credentials.
It does not qualify installation paths, plugin loading, or managed policy by itself.

```bash
copilot_fixture_dir="$(mktemp -d /tmp/kandev-copilot-plugins.XXXXXX)"
mkdir -p "$copilot_fixture_dir/runtime" "$copilot_fixture_dir/home" "$copilot_fixture_dir/config" "$copilot_fixture_dir/work"
npm install --prefix "$copilot_fixture_dir/runtime" --no-audit --no-fund --package-lock=false --save-exact @github/copilot@1.0.88
node -p 'require(process.argv[1]).version' "$copilot_fixture_dir/runtime/node_modules/@github/copilot/package.json"
env HOME="$copilot_fixture_dir/home" COPILOT_HOME="$copilot_fixture_dir/config" "$copilot_fixture_dir/runtime/node_modules/.bin/copilot" --version
env HOME="$copilot_fixture_dir/home" COPILOT_HOME="$copilot_fixture_dir/config" "$copilot_fixture_dir/runtime/node_modules/.bin/copilot" plugin list --json
test -x apps/backend/bin/acpdbg || make -C apps/backend build-acpdbg
env HOME="$copilot_fixture_dir/home" COPILOT_HOME="$copilot_fixture_dir/config" apps/backend/bin/acpdbg probe --timeout 45s --exec "$copilot_fixture_dir/runtime/node_modules/.bin/copilot --acp" --workdir "$copilot_fixture_dir/work" --out "$copilot_fixture_dir/frames"
git diff --check -- docs/plans/copilot-profile-plugins docs/specs/agents
```

Preserve inherited proxy and CA variables. Use command-scoped home overrides only for the disposable child.
If the package cannot run, record the reason before trying another version.
Do not copy an existing auth store or initiate login to make this probe pass.

Extend the evidence document with exact commands for fixture installation and repeated-mount/policy cases after observing their supported CLI syntax.
All installation commands must target the disposable home, never the developer's configuration.
Use two fixture plugin roots, including a path with spaces, and a local sentinel MCP tool.
Use `acpdbg prompt` with flags before the agent argument for a read-only sentinel call when existing authorized authentication is available.
Missing native authentication remains a limitation, not success evidence.
Inspect captures, keep secrets out of repository files, and remove only the exact temporary directory after extracting safe evidence.

These adaptive qualification commands cannot be fixed before their native syntax is observed.
They must be recorded exactly before marking this work order done.
If no runtime meets the policy boundary, record the blocker and stop dependent production loading work.

## Files likely touched

- `docs/plans/copilot-profile-plugins/compatibility-evidence.md` (new during execution).
- `docs/specs/agents/system-design/copilot-profile-plugins.md`, compatibility/source tables only.
- This work order and the plan status/results.
- Disposable fixtures under the exact temporary directory only.

## Dependencies

None. Production or permanent test work requires a later explicit implementation request.

## Risks

- Package metadata can differ from the executable's reported version.
- Authentication can prevent a full native tool call.
- Managed policy may require an environment that cannot be recreated here. Do not invent enforcement evidence.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/copilot-profile-plugins.md#compatibility-qualification).
- `apps/backend/internal/agent/agents/copilot_acp.go` and `managed_npm_runtime_versions.json`.
- `.agents/skills/acp-debug/SKILL.md` and official documentation linked from the design.

## Results

Pending 2026-10-28. Inventory/schema evidence is recorded in [compatibility-evidence.md](compatibility-evidence.md); native policy qualification is not.

- Reproduced package/binary identity, flat inventory, external rows, live rows with `installedFrom`, installed rows with `cache_path`, JSONC state, manifest precedence, `--config-dir` precedence, and repeated ACP mounts for 1.0.88.
- Reproduced flat inventory, built-in rows, live rows, installed rows with `cache_path` (after registering the non-default marketplace), and repeated ACP mounts for 1.0.104.
- 1.0.103 remains unqualified because only supplied observations exist.
- Authenticated `session/new`, plugin tool calls, plugin MCP loading, native veto observation, Windows, and macOS remain unverified. Inventory discovery is qualified for 1.0.88 and 1.0.104; explicit mount permission is not. Nonempty starts return `policy_unverified`, and other versions return `runtime_unsupported`.
- `git diff --check -- docs/plans/copilot-profile-plugins docs/specs/agents` passed.
