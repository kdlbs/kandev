# ADR-2026-08-22-pr-walkthrough-filesystem-runner: Use a Filesystem Contract for PR Walkthrough Runners

**Status:** accepted (amended 2026-08-23)
**Date:** 2026-08-22
**Area:** workflow, infra, security

## Context

The first PR walkthrough runner gives OpenCode two custom TypeScript tools.
These tools restrict access to PR files and the trusted renderer. However,
their interface belongs to OpenCode and does not transfer to another agent.

The runner must remain safe inside a secret-bearing `pull_request_target` job.
It must also keep the prompt, context, and output contract independent of the
selected agent.

The portability unit is the complete `.agents/skills/pr-walkthrough/`
directory. A user who copies that directory must not also need repository-root
generation scripts or tests.

## Decision

The walkthrough workflow uses a trusted filesystem contract for every
agent runner. The provider-neutral skill is a self-contained bundle. It owns
its instructions, renderer assets, deterministic generation helpers, and
their focused tests:

- `.agents/skills/pr-walkthrough/SKILL.md`
- `.agents/skills/pr-walkthrough/references/`
- `.agents/skills/pr-walkthrough/scripts/`

The context and managed-renderer entry points live under the skill's
`scripts/` directory. They resolve bundled assets relative to the skill
directory. They do not depend on copies in the repository-root `scripts/`
directory.

The workflow materializes the complete skill directory from the exact workflow
commit before it starts the agent. It then prepares the following inputs:

- Repository guidance from the exact workflow commit.
- A base-to-head patch and changed-file manifest.
- Bounded regular UTF-8 files from the exact PR head commit.
- A trusted renderer command that invokes the workflow-controlled skill script.

The workflow fetches enough PR-head history to resolve the merge base. It does
not check out the PR head in the secret-bearing worktree. A trusted context
helper rejects symlinks, binary files, oversized files, unsafe paths, and data
that exceeds the total context budget.

The workflow passes a short fixed prompt to the selected agent. The prompt
tells the agent to read the skill and prepared context. It also tells the
agent to treat PR content as untrusted data.

The agent can read and search the trusted worktree and prepared context. It can
edit only `.pr-walkthrough/draft.json`. It can run only the fixed renderer
command supplied by the host. The Kandev workflow command invokes
`.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render`. The agent cannot
run arbitrary shell commands, use network tools, invoke subagents, change
source files, or use GitHub write operations.

The trusted renderer reads the fixed draft file. It binds PR identity from the
GitHub event, removes model-owned URL overrides, and writes the final JSON and
HTML files to fixed paths. The agent corrects the draft and runs the renderer
again when validation reports an error.

The local OpenCode setup action remains the initial provider adapter. It
installs a pinned OpenCode archive after SHA-256 validation. The workflow uses
`opencode run` and does not use a third-party GitHub integration action. A
future Claude or Codex adapter must preserve the same prompt, filesystem
inputs, output paths, and permission boundary.

GitHub-specific orchestration stays outside the skill. This includes provider
installation, workflow triggers, artifact publication, R2 upload, and pull
request description updates. Those adapters consume the portable skill; they
do not own its generation logic.

`2026-08-23-pr-walkthrough-workflow-provenance` refines the trusted commit in
this decision. The trusted workflow SHA replaces the event base SHA for all
workflow-controlled inputs.

This decision supersedes
`2026-08-22-agent-owned-pr-walkthrough-rendering`. It keeps agent-owned
rendering but replaces OpenCode custom tools with a portable filesystem
contract.

## Consequences

- The prompt and skill remain unchanged when the selected agent changes.
- Copying `.agents/skills/pr-walkthrough/` carries the instructions, scripts,
  renderer assets, and focused tests together.
- Runner adapters contain installation, invocation, and permission syntax only.
- The repository no longer needs OpenCode-specific TypeScript tools for this
  workflow.
- The repository no longer keeps PR walkthrough generation helpers in its
  top-level `scripts/` directory.
- The trusted context helper adds bounded filesystem preparation before agent
  invocation.
- The permission configuration must permit one draft path and one exact
  renderer command.
- The workflow keeps model credentials away from PR-owned executable content.

## Alternatives Considered

- **Keep the OpenCode custom tools:** This keeps the current narrow interface,
  but each future agent needs a new tool implementation.
- **Use `dceoy/opencode-action`:** This action accepts a prompt and model. It
  uses a remote installer and the broader `opencode github run` integration.
- **Use `Barmore-Genc/opencode-pr-reviewer`:** This action shows useful
  permission rules, but it is coupled to review comments and helper scripts.
- **Allow a generic shell:** This removes adapter code, but PR prompt injection
  can run unrelated commands in a job that contains a model credential.
- **Render after the agent exits:** This removes the renderer permission, but
  the agent cannot correct invalid data in the same turn.
