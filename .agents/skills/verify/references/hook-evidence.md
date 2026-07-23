# Hook Evidence

Use a commit hook receipt only to remove exact duplicate work from
`mode=changed`. The planner supplies the receipt from `/commit` plus the last
successfully verified SHA. Do not infer hook success from commit history.

## Eligibility

All conditions are required:

- authoritative PR CI exists for delivery;
- mode remains `changed`;
- `receipt.commit_sha` equals current `HEAD`;
- `receipt.parent_sha` equals both `HEAD^` and the supplied last verified SHA
  (or the PR base for the first verified commit);
- pre-commit and commit-msg hooks were active;
- `receipt.bypass` is false and `receipt.commit_result` is `pass`;
- the receipt names each hook as `passed` or `skipped`;
- the worktree, index, and untracked set are clean; and
- `.pre-commit-config.yaml`, hook wiring, root build/toolchain files, or hook
  implementation did not change in the receipt range.

If any condition is unknown or false, run the normal impact-matrix commands.
Full mode and delivery without PR CI never use hook omissions.

## Covered Checks

Omit a row only when the named hook reported `passed` for at least one matching
path in the receipt range.

| Successful hook | Changed-scope work it replaces |
| --- | --- |
| `harness-lint` | Targeted harness lint for committed harness/instruction files |
| `gofmt` | Formatting of committed `apps/backend/**/*.go` files |
| `go-lint` | Changed-code backend lint for the receipt range |
| `prettier-format` | Formatting of committed matching web/CLI/packages files |
| `web-lint` | Changed-code web lint for committed matching JS/TS files |
| `commitlint` | Commit-message validation only; `verify` has no equivalent |

`skipped` is not `passed` and grants no coverage.

## Never Covered

Hooks do not replace `git diff --check`, TOML parsing, generated metadata,
typechecking, tests, builds, public-doc validation, action pinning, script
syntax/tests, CLI tests, desktop/Rust checks, or any full-mode command. Run
every applicable uncovered command from the impact matrix.

Report the receipt decision, every omitted check and covering hook, and every
remaining command. A pass using eligible omissions is still
`changed-scope PASS`; it is not a full pass.
