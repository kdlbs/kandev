# Architecture lint

Kandev turns a small set of accepted architecture boundaries into fast repository checks. The
pre-commit hook runs them automatically; use `make lint-architecture` to run them directly. CI runs
the same dependency-free Python linter against tracked files and reports `path:line` diagnostics.
Executable tooling lives under `scripts/`, durable rule configuration lives under `config/`, and
the GitHub Actions workflow only invokes those repository-owned entry points.

The linter's system boundary is documented in the
[architecture-lint specification](specs/architecture-lint/README.md).

## Enforced boundaries

| Rule | Boundary | Intended seam |
| --- | --- | --- |
| `ARCH-RUNTIME-IMPORT` | Production Go outside `internal/agent/runtime/` must not add direct imports of `runtime/lifecycle` or `runtime/agentctl`. | Depend on `internal/agent/runtime` or an explicitly approved low-level adapter. |
| `ARCH-TASK-OFFICE-IMPORT` | Production Go under `internal/task/` must not add imports of `internal/office`. | The shared task model owns task concepts; Office consumes or adapts them. |
| `ARCH-FRONTEND-ROOT-STATE-CAST` | `apps/web/lib/state/store.ts` must not add `as any` or `as unknown as` escapes. | Derive typed domain state, actions, and defaults. |
| `ARCH-RUN-SCHEDULER-OWNER` | Only the backend composition root under `apps/backend/internal/backendapp/` may import `internal/runs/scheduler`. | Construct the one backend-wide scheduler in `internal/backendapp`; other packages consume runs services or interfaces. |
| `ARCH-RUNS-OFFICE-IMPORT` | Production Go under `internal/runs/` must not add imports of `internal/office` or its subpackages. | Office adapters may depend on generic runs; generic runs must not depend on Office implementations. |
| `ARCH-FRONTEND-STATE-UI-IMPORT` | Production files under `apps/web/lib/state/` must not import `apps/web/components/` or `apps/web/app/`. | Components and routes consume state; state remains below UI/app layers and shared values belong in dependency-neutral modules. |
| `ARCH-INBOX-HISTORY-ISOLATION` | A closed set of backend pending-action sinks must not reference the Inbox History read's exported entry points, and the Inbox History read/render modules must not reference the Needs-you sidebar-badge state or subscribe to an event stream. | Keep the additive History read (AC-UI-INBOX-HISTORY-001.5/.17/.22) out of the operational pending-action path in both directions. |
| `ARCH-DEPRECATION-LEDGER` | Handwritten production Go `Deprecated:` comments and TypeScript JSDoc `@deprecated` annotations must be registered or present in the exact legacy baseline. | Give every newly deprecated declaration an owner, reason, introduction record, removal condition, and target in the compatibility ledger. |

Each rule owns its exact grandfathered finding set under `config/architecture-lint/`. A current
finding absent from that rule's baseline fails. When cleanup removes a finding, the now-stale entry
also fails, so the same change must delete the exemption. CI compares each file with the pull
request base and rejects additions; a rule's baseline can only shrink after its initial rollout.
For `ARCH-DEPRECATION-LEDGER`, a valid declaration-level ledger registration satisfies the finding;
the baseline contains only current unregistered declarations.
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
- an optional `locator.declaration` for a specific source declaration; it is required by the
  deprecation rule and must match the scanner's normalized symbol identity;
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

### Explicit declaration deprecations

The deprecation rule recognizes these attached forms:

- Go line comments whose paragraph begins `Deprecated:` and is attached to a top-level `type`,
  `func`, `const`, or `var`, including individual declarations in grouped `type`, `const`, and
  `var` blocks; a struct or interface field, including embedded fields; or an interface method. A
  trailing `// Deprecated:` field comment is also recognized.
- TypeScript JSDoc blocks with an `@deprecated` tag attached to a top-level type, class, interface,
  enum, function, or variable, or to a class/interface/type-literal member. Decorators between the
  JSDoc and declaration are supported, as are nested type literals and identifier, string, numeric,
  or computed member keys.

The exact finding identity is `(path, declaration, marker)`. A declaration identity is a normalized
kind and qualified symbol name followed by an occurrence number such as `#1`. Go names use forms
such as `type:Old`, `field:Wrapper.Old`, and `method:Service.Close`; each name in a grouped
declaration has its own identity. TypeScript member identifiers use forms such as
`property:Options.config.old`, while non-identifier keys use canonical brackets such as
`property:Keys["old-key"]`, `property:Keys[4]`, or
`method:Keys[Symbol.iterator][signature=( )]`. TypeScript functions and methods also include their
normalized generic and parameter signature, for example
`method:Service.oldApi[signature=( value : string )]#1`. This gives overloads distinct identities
independent of declaration order. Whitespace and comments in a signature do not affect its
identity, and single- and double-quoted string literal types use the same canonical form.

The identity does not contain line numbers or explanatory comment text. The marker is exactly
`Deprecated:` or `@deprecated`. If multiple annotations in one file have the same path, declaration,
and marker identity, the linter reports the ambiguity and rejects ledger registration. This keeps
an occurrence number from silently selecting one of two indistinguishable annotations.

Generated headers and generated path/file names, test and fixture paths/files, and third-party
`vendor`, `third_party`, or `node_modules` paths are excluded. Ordinary comments and strings are
ignored. The scanner does not search for compatibility words or infer deprecation from prose.
