# Finding ledger: GitHub Actions security

## Evidence identity

- Source: current `main`, `6677802efa4fac6c2ecc909cb433477c90f7fb2d`.
- Date: 2026-10-25. Scanner: Zizmor 1.25.2, default persona and repository configuration.
- Command: `zizmor .github/workflows`. Exit 14. The text report contains 271 findings:
  229 suppressed, 2 ignored, and 40 unsuppressed. Unsuppressed severity totals are
  18 informational, 3 low, 8 medium, and 11 high.
- The parent reported 38 unsuppressed findings at `595df36ed5c`. The two added
  findings are CMD launchers in `isolated-windows-tests.yml`. No finding names
  `announce-discord-release.yml` or its added release jobs.
- The source checkout was fast-forwarded to the independently read remote SHA.
  Fetch reported an unrelated broken shared branch ref after transferring objects.
  The target commit was available and the fast-forward succeeded. No shared ref was repaired.

The baseline commands ran before authoring this package. No workflows or tests
were changed. Scanner severity is not a finding of exploitability. `Remediate`
means a trust-boundary defect. `Harden` means a narrower defensive change without
a demonstrated external exploit. `Retain` means preserve behavior and document
its reviewed boundary. The baseline inventory is historical. Implementation
results and residual dispositions are recorded below.

## Open-PR overlap check

`gh pr list --repo kdlbs/kandev --state open --limit 300 --json
number,title,headRefName,baseRefName,body,files` returned 116 PRs. The review
checked titles, bodies, and workflow paths for overlap. The files API was also
paginated for the large SDK PR because its full diff exceeds GitHub's limit.

[PR #4534](https://github.com/kdlbs/kandev/pull/4534) changes `release.yml` for
signed SDK publication and `preview-env.yml` for SDK input paths/cache keys.
It does not fix this set. Preserve its publication dependencies if it lands first.
[PR #3693](https://github.com/kdlbs/kandev/pull/3693) concerns PR-documentation
API reliability and is not a duplicate. PRs #4662, #4644, #4599, #3660, #2308,
and #2140 touch backend test workflow configuration for other capabilities.
None proposes this remediation. Repeat overlap discovery before implementation.

## Complete unsuppressed inventory

Line numbers refer only to the recorded source SHA. Job names and evidence keys
remain usable after edits. Every path in this table is under `.github/workflows/`.

| ID | Rule | Scanner severity | Source and job | Decision and evidence |
| --- | --- | --- | --- | --- |
| F01 | misfeature | Low | `backend-tests.yml:1001` (test-windows) | Retain: N1: fixed Windows PowerShell invocation |
| F02 | artipacked | Medium | `ci-base-image.yml:39` (build-and-push) | Harden: C1: image build needs no Git write |
| F03 | dangerous-triggers | High | `claude-code-review.yml:15` (workflow trigger) | Remediate: R2: PR-owned skill survives upstream config restoration |
| F04 | misfeature | Low | `isolated-windows-tests.yml:62` (isolated-launch) | Retain: N1: fixed Windows PowerShell invocation |
| F05 | misfeature | Low | `isolated-windows-tests.yml:87` (isolated-launch) | Retain: N1: fixed Windows PowerShell invocation |
| F06 | cache-poisoning | High | `notify-docs.yml:82` (preview) | Harden: D1: cache restore in docs deployment job |
| F07 | dangerous-triggers | High | `opencode-code-review.yml:5` (workflow trigger) | Remediate: R1: root OpenCode config can load PR plugin |
| F08 | dangerous-triggers | High | `pr-walkthrough-reconcile.yml:5` (workflow trigger) | Retain: T1: workflow-SHA reconciliation helper, no PR execution |
| F09 | dangerous-triggers | High | `pr-walkthrough.yml:6` (workflow trigger) | Remediate: T2/R3: reserved instruction names in bounded head context |
| F10 | dangerous-triggers | High | `preview-env.yml:7` (workflow trigger) | Retain: T3: separate fork build, package, and trusted deploy jobs |
| F11 | artipacked | Medium | `release.yml:120` (prepare) | Harden: C2: replace prepare-job persistence with push-scoped authentication |
| F12 | artipacked | Medium | `release.yml:2096` (docker-amd64) | Harden: C1: build needs no Git write |
| F13 | artipacked | Medium | `release.yml:2573` (docker-universal-amd64) | Harden: C1: build needs no Git write |
| F14 | artipacked | Medium | `release.yml:2811` (publish-release) | Harden: C1: release API does not require persisted Git auth |
| F15 | artipacked | Medium | `release.yml:3026` (publish-npm) | Harden: C1: npm uses OIDC and explicit API token |
| F16 | artipacked | Medium | `release.yml:3121` (update-homebrew-tap) | Harden: C1: tap helper uses its separate SSH deploy key |
| F17 | template-injection | High | `release.yml:1518` (build-desktop) | Harden: I1: GitHub repository identity and workflow SHA |
| F18 | template-injection | High | `release.yml:1536` (build-desktop) | Harden: I1: GitHub repository identity and workflow SHA |
| F19 | template-injection | High | `release.yml:1733` (build-desktop) | Harden: I1: GitHub repository identity and workflow SHA |
| F20 | template-injection | High | `release.yml:2287` (verify-release-assets) | Harden: I1: GitHub repository identity and workflow SHA |
| F21 | template-injection | Informational | `release.yml:2612` (docker-universal-amd64) | Harden: I2: validated/generated version or tag output |
| F22 | template-injection | Informational | `release.yml:2687` (docker-universal-arm64) | Harden: I2: validated/generated version or tag output |
| F23 | template-injection | Informational | `release.yml:2776` (docker-universal-manifest) | Harden: I2: validated/generated version or tag output |
| F24 | template-injection | Informational | `release.yml:2777` (docker-universal-manifest) | Harden: I2: validated/generated version or tag output |
| F25 | template-injection | Informational | `release.yml:2932` (publish-release) | Harden: I2: validated/generated version or tag output |
| F26 | template-injection | Informational | `release.yml:2936` (publish-release) | Harden: I2: validated/generated version or tag output |
| F27 | template-injection | Informational | `release.yml:2937` (publish-release) | Harden: I2: validated/generated version or tag output |
| F28 | template-injection | Informational | `release.yml:2946` (publish-release) | Harden: I2: validated/generated version or tag output |
| F29 | template-injection | Informational | `release.yml:2947` (publish-release) | Harden: I2: validated/generated version or tag output |
| F30 | template-injection | Informational | `release.yml:3049` (publish-npm) | Harden: I2: validated/generated version or tag output |
| F31 | template-injection | Informational | `release.yml:3051` (publish-npm) | Harden: I2: validated/generated version or tag output |
| F32 | template-injection | Informational | `release.yml:3104` (publish-npm-nightly) | Harden: I2: validated/generated version or tag output |
| F33 | template-injection | Informational | `release.yml:3131` (update-homebrew-tap) | Harden: I2: validated/generated version or tag output |
| F34 | template-injection | Informational | `release.yml:3132` (update-homebrew-tap) | Harden: I2: validated/generated version or tag output |
| F35 | template-injection | Informational | `release.yml:3173` (update-scoop-bucket) | Harden: I2: validated/generated version or tag output |
| F36 | template-injection | Informational | `release.yml:3174` (update-scoop-bucket) | Harden: I2: validated/generated version or tag output |
| F37 | superfluous-actions | Informational | `release.yml:2952` (publish-release) | Retain: N2: pinned release action replacement suggestion |
| F38 | dangerous-triggers | High | `revoke-ready-to-merge.yml:3` (workflow trigger) | Retain: T4: fixed API cleanup with no checkout |
| F39 | artipacked | Medium | `universal-rebuild.yml:49` (rebuild) | Harden: C1: image build needs no Git write |
| F40 | template-injection | Informational | `universal-rebuild.yml:116` (rebuild) | Harden: I3: trusted date command output |

## R1: OpenCode project configuration is an executable input

Both jobs in `opencode-code-review.yml` check out the event head, then start the
CLI from that directory with `OPENCODE_API_KEY`. The workflow deletes `.opencode`
but leaves root `opencode.json` and `opencode.jsonc`. A PR can declare a local
plugin there, such as `{"plugin":["./review-plugin.ts"]}`. The pinned v1.19.3
configuration loader discovers that root file. The plugin loader imports and
invokes configured plugins independently of model tool permissions.

This is a source-confirmed route from PR content to code loading in a process
with a provider credential. Preconditions are an enabled workflow plus its
existing same-repository or approved/allowlisted fork gate. An arbitrary
unapproved fork cannot start the privileged job. Same-repository posting also
uses a later dedicated App token on the same runner. A process retained by an
executed plugin could cross that later step boundary. No credential theft,
external payload, or live model execution was attempted.

Evidence: `.github/workflows/opencode-code-review.yml`,
`.github/actions/setup-opencode/action.yml`, and the pinned upstream
[configuration loader](https://github.com/anomalyco/opencode/blob/v1.19.3/packages/opencode/src/config/config.ts)
and [plugin initialization](https://github.com/anomalyco/opencode/blob/v1.19.3/packages/opencode/src/plugin/index.ts).
The current [OpenCode configuration documentation](https://opencode.ai/docs/config/)
also describes root project configuration. Existing tests verify Bash denial
and `.opencode` deletion, but neither proves root config isolation.

Required change: trusted workflow checkout, immutable head objects, bounded
context, and trusted configuration. Keep both target triggers and producer identities.

## R2: Claude has upstream mitigation but retains PR-owned instructions

The fork job checks out `refs/pull/<number>/head` before its credential-bearing
action. Its explicit prompt asks the reviewer to follow
`.agents/skills/code-review/SKILL.md` in that checkout. The PR controls this file.
The fetched ref can also advance beyond the event head. Approval is required:
the job runs only for the existing label event or allowlisted opening author.

The pinned action `ae68417a745a5df4b2124873632bd73a2255e6f6` restores `.claude`,
`.mcp.json`, `.claude.json`, `.gitmodules`, `.ripgreprc`, root Claude instructions,
and `.husky` from the trusted base before CLI startup. Therefore a simple
PR-owned Claude settings hook is not established as an exploit of this pin.
The restoration does not include `.agents/skills/code-review`. Its explicit
agent mode also does not check out the head itself, so a trusted initial checkout
is compatible with the chosen action mode.

Evidence: pinned action [entrypoint](https://github.com/anthropics/claude-code-action/blob/ae68417a745a5df4b2124873632bd73a2255e6f6/src/entrypoints/run.ts),
[restoration implementation](https://github.com/anthropics/claude-code-action/blob/ae68417a745a5df4b2124873632bd73a2255e6f6/src/github/operations/restore-config.ts),
and [agent mode](https://github.com/anthropics/claude-code-action/blob/ae68417a745a5df4b2124873632bd73a2255e6f6/src/modes/agent/index.ts).
Local evidence is the fork prompt plus `.github/AGENTS.md` and the existing
manual-review isolation implementation in `claude.yml`.

Required change: reuse the trusted-checkout/manual-helper pattern with event-head
binding. This corrects the repository boundary and instruction provenance.
It is not a claim that a Claude credential-exfiltration exploit was reproduced.

## R3: Prepared filenames can become agent instructions

The adversarial review found an additional path in the initial implementation.
The shared context helper preserved original directories and filenames beneath
its data directory. The real pinned OpenCode v1.19.3 `read` tool automatically
loaded nearby PR-owned `AGENTS.md`, `CLAUDE.md`, and `CONTEXT.md` files as
instructions. All six root/nested cases reproduced this behavior. This affects
automatic reviews and the existing walkthrough caller. The original T2
disposition missed this discovery path.

Evidence: pinned upstream [instruction discovery](https://github.com/anomalyco/opencode/blob/v1.19.3/packages/opencode/src/session/instruction.ts)
and local CLI output that included the PR marker in a `system-reminder` with
the reserved file listed in `metadata.loaded`. This proves instruction loading,
not that a model followed the text. No provider request was made.

The shared helper now stores every blob under a flat neutral object-ID filename.
Manifest version 2 maps each original `path` to a `context_path`. All consumers
use that mapping. Exact bytes, original review paths, size limits, omission
reasons, and immutable head binding remain intact. Neutral names also avoid
collisions with contributor files that already end in a proposed suffix.

## I1: High-severity release interpolation

The four findings interpolate `github.repository` and `github.workflow_sha`
into helper-download API paths. Repository identity and a workflow commit SHA
are GitHub-produced values with constrained formats. No PR title, body, branch
name, or unconstrained dispatch input reaches these interpolation sites.
Normal release requires a main dispatch. Backfill/validation remain maintainer
operations with their existing environment and mode gates.

The scanner's high severity does not establish external shell injection at
these four sites. Move the values to environment data anyway. Preserve the
workflow-SHA helper source and each helper's failure behavior. Do not substitute
application checkout SHA or moving `main` for workflow identity.

## I2 and I3: Generated release and date values

The 16 informational release expressions consume `prepare` version/tag outputs
or `nightly-prepare` version output. Backfill accepts strict `vX.Y.Z`, verifies
an existing highest release tag, and checks manifest versions. Normal Stable
versions come from the existing version computation. Nightly versions come from
`scripts/release/nightly-release.sh`. The weekly image date is produced by the
fixed `date -u +%Y%m%d` command. These are not direct arbitrary contributor strings.

Move these values to quoted environment arguments. Preserve annotated-tag
`^{commit}` dereferencing, generated image tag names, npm dist-tags, and release
helper arguments. Validate with stubbed commands and hostile synthetic data.
This is defensive data separation, not 17 demonstrated injection exploits.

## C1 and C2: Checkout credential lifetime

Eight checkouts omit `persist-credentials: false`. Seven jobs do not need
Git writes through the source checkout. GHCR login, release API calls, npm OIDC,
and the Homebrew helper's separate SSH credential are independent mechanisms.
The exception is release `prepare`, which pushes a branch and a signed tag.

The pinned [checkout v6 documentation](https://github.com/actions/checkout/blob/df4cb1c069e1874edd31b4311f1884172cec0e10/README.md)
states that credentials are stored under `RUNNER_TEMP`, rather than directly in
`.git/config`. No credential-containing artifact was demonstrated. The finding
still identifies unnecessary authentication available to later job steps.
Disable all eight defaults and replace only the necessary prepare pushes with
command-local authentication. Preserve all checkout refs and release gates.

## D1: Documentation deployment cache

`notify-docs.yml` restores a pnpm cache before dependency installation and a
Cloudflare preview deployment. The preview requires a same-repository PR,
checks out the landing publisher from its own main branch, and installs from
its lockfile. Fork preview publication is excluded. The comment writer is a
separate job without checkout.

This scan does not prove an unapproved fork can poison the publication cache.
[GitHub cache scoping](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching)
limits restoration across refs. Nevertheless, deployment does not require a
cache. Remove the explicit cache and disable setup-node automatic package caching.
Retain frozen installation and every publication/credential gate.

## T1 through T4: Reviewed target triggers

**T1, reconciliation:** the workflow checks out `github.workflow_sha`, asserts
that identity, validates numeric PR and full head SHA, and invokes the trusted
body helper. It checks the canonical public HTML response, refreshes PR state,
rejects head changes, and verifies body readback. PR text remains API data.
The existing walkthrough contract suite covers this reconciliation path.

**T2, walkthrough:** generation keeps the workflow SHA at the worktree root.
The history helper verifies the event head as a Git object. The bounded context
helper omits symlinks/gitlinks and limits text bytes. The agent configuration
comes from trusted workflow text. Publication and PR linking run separately.
The adversarial review found R3 in its prepared filenames. Shared neutral
filenames and manifest mapping now prevent reserved-name instruction discovery.
Walkthrough contract and context tests protect these boundaries.

**T3, preview:** an authorized fork runs source hooks only in its input-build
job. Packaging uses a fresh job with a trusted executable and restricted
environment. The credential-bearing deploy job checks out the workflow SHA and
uses the completed archive. Required credentials are absent from fork build
commands. Run-scoped artifact transfer is an intentional boundary, not a claim
that an untrusted job has no Actions artifact capability. Same-repository
preview retains its separate existing contributor trust contract.

**T4, revocation:** fixed `github-script` code reads event/API metadata and
performs only the existing approval cleanup. It has no checkout or execution
of PR code. It uses PR write permission, checks sender permissions, and handles
lookup failures through its fail-closed cleanup policy. The 10-test contract
suite protects label scope, queue/auto-merge behavior, and idempotence.

Retain all four triggers. Their scanner warnings remain visible with this
scope-specific evidence. Reassess if checkout provenance, permissions,
artifact handling, or script sources change. This review is not an audit of
all provider internals, preview infrastructure, or hosting code.

## N1 and N2: Non-security scanner observations

**N1:** the three `misfeature` findings identify CMD analysis limitations.
Each launcher invokes a fixed `powershell.exe -NoProfile ... -File` test path.
No GitHub expression is interpolated into those commands. The Windows launcher
and encoding semantics are deliberate. Retain them. The parent's description
called backend findings informational, but this scanner reports the CMD item low.

**N2:** `softprops/action-gh-release` is pinned. `superfluous-actions` suggests
using `gh release` instead. Replacement is not a security fix and would alter
publication/error behavior. Retain the action and its artifact/metadata contract.

## Residual accounting

The proposed edits address eight credential findings, 21 interpolation findings,
and the docs cache finding. The review fixes retain their required triggers,
so all six trigger warnings remain visible after remediation. The expected
residual set is F01, F03, F04, F05, F07, F08, F09, F10, F37, and F38.
F03, F07, and F09 require passing trust-boundary tests before being marked remediated.
Do not label current R1/R2 code safe merely because other target triggers are intentional.

No new suppression is planned. Existing 229 suppressed and 2 ignored findings
are outside this scoped investigation. Compare with the same scanner version
and explain new findings before declaring implementation complete.


## Implementation verification, 2026-10-26

The implementation rechecked main and all 116 open PRs before editing workflows.
The source baseline and overlap conclusions were unchanged. R1 and R2 are now
addressed by workflow-SHA execution checkouts and immutable PR context. Six
fixture tests exercise hostile root configuration, instruction provenance,
new files, omitted symlinks/gitlinks/binary/oversized files, head mismatches,
invalid preparation inputs, and the model invocation from a trusted root.
The Claude reader adds pinned-head regressions while preserving manual use.

All eight checkout defaults now disable persistence. Release pushes use
command-local authentication, and a stubbed push test verifies branch creation
and both signed-tag attempts. All 21 reported interpolation findings are removed.
Docs publication disables dependency caching. Its install and publication gates
remain covered. The normal, validation, dry-run, backfill, Nightly, downstream
notification, and image promotion contracts pass.

Final scan: `zizmor .github/workflows`, Zizmor 1.25.2, exit 14. It reports 235
findings: 223 suppressed, 2 ignored, and 10 unsuppressed. The latter are 6 high
trigger warnings, 3 low CMD limitations, and 1 informational action suggestion.
No suppression directive or scanner configuration was added or changed. The six
fewer suppressed findings result from removed flagged code, not new exemptions.

Residual IDs are exactly F01, F03, F04, F05, F07, F08, F09, F10, F37, and F38.
F03, F07, and F09 retain their required triggers, with the evidenced input
defects fixed. T2 additionally uses neutral context filenames after the
adversarial review. T1, T3, and T4 retain their reviewed boundaries. N1/N2 retain the
intentional shell/action behavior. The release action moved to line 2964 in the
implementation. The trigger and CMD finding locations are unchanged.
