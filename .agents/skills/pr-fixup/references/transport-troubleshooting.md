# PR Fixup Transport Troubleshooting

Load this reference when CI setup fails before tests, or when GraphQL evidence
is unavailable but authenticated REST calls still work.

**Corepack/pnpm bootstrap transport failures:** If `pnpm install --frozen-lockfile`
fails while Corepack downloads pnpm, before repository tests start, and the log
shows a Node/undici transport or parser assertion, classify it as runner or
registry infrastructure. Verify the exact job head and terminal parent run,
then rerun the failed job once with `gh run rerun <run-id> --failed`; recheck
`scripts/pr-state --summary` against that same head. If it repeats, inspect the
runtime image/Corepack toolchain instead of changing application code.

**GraphQL/rate-limit degradation:** If `pr-await` or `pr-state` cannot read
GraphQL while REST remains available, query the REST pull request for state,
mergeability, and head SHA, then `/commits/<head_sha>/check-runs?per_page=100`
for checks. Require the REST head to equal the check snapshot head. REST cannot
prove review-thread resolution, so keep thread evidence unknown until
`scripts/pr-resolve` or a connector succeeds; never call the PR clean from
partial REST evidence. A queued job with `runner_id=0`, no runner name, and no
completed steps is hosted-runner backlog, not a test hang; do not rerun or
cancel solely because it is queued. Finish with a fresh state, thread, and
mergeability audit.
