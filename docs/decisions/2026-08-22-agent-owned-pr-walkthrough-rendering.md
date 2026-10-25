# ADR-2026-08-22-agent-owned-pr-walkthrough-rendering: Keep PR walkthrough rendering agent owned and provider neutral

**Status:** superseded by 2026-08-22-pr-walkthrough-filesystem-runner
**Date:** 2026-08-22
**Area:** workflow, infra, security

## Context

PR walkthrough generation starts with OpenCode, but the selected agent runner
may change later. The agent must be able to correct invalid walkthrough data
after seeing renderer errors, while the secret-bearing `pull_request_target`
workflow must not grant a model broad write access or permission to execute
pull-request-owned code.

## Decision

The provider-neutral `.agents/skills/pr-walkthrough/SKILL.md` owns the complete
generation procedure. The selected agent creates the walkthrough data, invokes
the trusted renderer, inspects any validation error, and revises the data until
both the JSON and HTML outputs exist.

Each managed agent runner supplies the narrowest adapters needed to perform
that procedure. The OpenCode runner exposes base-commit-controlled
`render_pr_walkthrough` and `read_pr_file` custom tools. The renderer tool
accepts walkthrough JSON, binds authoritative pull-request identity from the
GitHub event, removes model-owned link overrides, and runs only the trusted
renderer against fixed output paths. The file tool reads only bounded regular
UTF-8 blobs from the immutable pull request head Git object. It never checks
the untrusted head out in the secret-bearing worktree. The runner does not
grant generic edit, shell, or external-directory access. A future Codex or
other runner must preserve these boundaries, follow the same skill, and
produce the same artifacts.

The generated artifact is one HTML file, not a fully offline bundle. Its fixed
shell loads Tailwind, Mermaid, Marked, DOMPurify, Shiki, and fonts from
exact-version or commit-pinned HTTPS URLs. Those providers are an explicit
runtime trust dependency. Pull request data cannot choose or override these
asset URLs.

Provider installation, credentials, and invocation remain workflow adapter
concerns. They must not be embedded in the walkthrough skill or renderer
contract.

## Consequences

- The agent can repair malformed data during its own turn instead of returning
  invalid JSON for a later workflow step to reject.
- Replacing OpenCode changes the runner adapter and workflow wiring, not the
  skill, JSON schema, renderer, object key, or hosting contract.
- The OpenCode adapter adds a small trusted custom tool, but avoids broad model
  access to shell execution or repository writes.
- Reviewers need network access to the trusted external asset hosts when they
  open a walkthrough; R2 stores no separate runtime asset bundle.
- Workflow verification checks the agent-produced files rather than rebuilding
  HTML from the agent's final response.

## Alternatives Considered

- **Render after the agent exits:** simpler workflow plumbing, but the agent
  cannot see renderer failures and repair its data.
- **Allow generic edit and shell tools:** lets the agent run the renderer
  directly, but also permits source mutation and execution of untrusted PR
  content in a secret-bearing workflow.
- **Make the skill OpenCode-specific:** avoids an adapter abstraction, but
  couples the durable walkthrough instructions to the first selected provider.
