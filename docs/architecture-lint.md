# Architecture lint

Kandev turns a small set of accepted architecture boundaries into fast repository checks. The
pre-commit hook runs them automatically; use `make lint-architecture` to run them directly. CI runs
the same dependency-free Python linter against tracked files and reports `path:line` diagnostics.
Executable tooling lives under `scripts/`, durable rule configuration lives under `config/`, and
the GitHub Actions workflow only invokes those repository-owned entry points.

## Enforced boundaries

| Rule | Boundary | Intended seam |
| --- | --- | --- |
| `ARCH-RUNTIME-IMPORT` | Production Go outside `internal/agent/runtime/` must not add direct imports of `runtime/lifecycle` or `runtime/agentctl`. | Depend on `internal/agent/runtime` or an explicitly approved low-level adapter. |
| `ARCH-TASK-OFFICE-IMPORT` | Production Go under `internal/task/` must not add imports of `internal/office`. | The shared task model owns task concepts; Office consumes or adapts them. |
| `ARCH-FRONTEND-ROOT-STATE-CAST` | `apps/web/lib/state/store.ts` must not add `as any` or `as unknown as` escapes. | Derive typed domain state, actions, and defaults. |

Each rule owns its exact grandfathered finding set under `config/architecture-lint/`. A current
finding absent from that rule's baseline fails. When cleanup removes a finding, the now-stale entry
also fails, so the same change must delete the exemption. CI compares each file with the pull
request base and rejects additions; a rule's baseline can only shrink after its initial rollout.
Normal lint never rewrites baselines.

To reduce the baseline:

1. Remove the architecture violation.
2. Run `python3 scripts/lint-architecture.py --all` to see the stale entry.
3. Delete that exact entry from the rule's `config/architecture-lint/<rule>.json` file.
4. Run `make lint-architecture` and the script tests.

## Adding a rule

Rules are intentionally modular so progressive migrations do not grow one central linter or
baseline file. A new rule adds:

1. One scanner module under `scripts/architecture_lint/rules/`, exporting a `Rule` with
   its stable ID, slug, baseline path, path predicate, and scanner.
2. One focused test module under `scripts/architecture_lint_tests/`.
3. One exact baseline under `config/architecture-lint/`.
4. One explicit import in the small `rules/__init__.py` registry and one boundary row above.

The scanner owns its actionable migration message. It must name the intended replacement seam,
not only report that a violation exists. Shared code owns git discovery, comparison, annotations,
and ledger validation; it must not accumulate rule-specific conditions. A baseline absent from the
target branch is treated as a new rule's reviewed bootstrap. After that first merge, CI permits only
removals from that rule's baseline.

## Compatibility ledger

Intentional compatibility aliases and fallbacks belong in
`config/architecture-lint/compatibility-ledger.json`. Add an entry in the same change that introduces
the compatibility behavior. Every entry has:

- a stable lowercase `id`;
- a tracked `locator.path` and exact, stable `locator.marker` near the compatibility behavior;
- a non-empty `reason` and accountable `owner`;
- exactly one introduction date (`introduced_on`) or SemVer (`introduced_version`);
- a testable `removal_condition`;
- exactly one target removal date (`target_removal_date`) or SemVer
  (`target_removal_version`).

Date targets fail after their target day. The linter also rejects invalid dates or versions,
duplicate IDs, missing fields, untracked paths, and markers that disappeared. Removing the
compatibility code therefore requires removing its ledger entry. Version targets remain explicit
review checkpoints; use a date target when automatic calendar expiry is required.

The ledger is intentionally explicit. The linter does not guess from words such as `legacy`,
`fallback`, or `alias`, because those terms also describe permanent domain behavior and would
create noisy false positives.
