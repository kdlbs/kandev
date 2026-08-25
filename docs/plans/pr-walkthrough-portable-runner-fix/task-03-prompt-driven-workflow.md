---
id: "03-prompt-driven-workflow"
title: "Prompt-driven workflow"
status: done
wave: 2
depends_on: ["01-trusted-pr-context", "02-fixed-renderer-contract"]
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-walkthrough.md"
---

# Task 03: Prompt-Driven Workflow

Integrate the trusted filesystem context and fixed renderer with the existing
local OpenCode setup action. Remove the OpenCode TypeScript tools.

- **Acceptance:** The workflow fetches the exact PR head without a depth limit.
  It proves that the base and head have a merge base before context creation.
- **Acceptance:** The workflow regenerates the current walkthrough for a new
  same-repository head received through the `synchronize` event.
- **Acceptance:** The workflow materializes every executable helper and agent
  instruction by copying the complete `pr-walkthrough` skill directory from
  the exact base SHA.
- **Acceptance:** The agent receives one fixed prompt. It can read and search
  context, edit only the draft JSON, and run only the fixed renderer command.
- **Acceptance:** The workflow does not create `.opencode/tools`, grant generic
  Bash, load PR-owned configuration, or provide GitHub write and R2 credentials
  to the agent.
- **Acceptance:** The two OpenCode TypeScript tools are removed. The local
  digest-validated setup action remains the installation adapter.
- **Acceptance:** No walkthrough context or renderer helper remains in the
  repository-root `scripts/` directory. GitHub-specific setup, publication,
  and PR-link helpers remain outside the skill.
- **Acceptance:** A contract test covers the merge-base regression, context
  helper, fixed prompt, permission rules, removed tools, outputs, and failure
  artifacts.
- **Verification:**

  ```text
  python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py
  python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py
  python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
  python3 .github/scripts/lint-action-pinning_test.py
  python3 .github/scripts/lint-action-pinning.py
  actionlint .github/workflows/pr-walkthrough.yml
  git diff --check
  ```

- **Files likely touched:**
  `.github/workflows/pr-walkthrough.yml`,
  `.github/workflows/lint-action-pinning.yml`,
  `.github/scripts/pr-walkthrough-workflow-contract_test.py`,
  `.github/scripts/opencode-pr-file-tool.ts`,
  `.github/scripts/opencode-pr-walkthrough-tool.ts`,
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context`,
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py`,
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render`,
  `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py`,
  `scripts/pr-walkthrough-read-file`,
  `scripts/pr-walkthrough-read-file.test.py`.
- **Dependencies:** Tasks 01 and 02.
- **Parallelism:** sequential. This task consumes both helper contracts and
  owns the shared workflow test.
- **Inputs:** Tasks 01 and 02, the spec permissions and failure modes, the
  filesystem runner ADR, and the existing setup action.
- **Output contract:** Report files changed, exact workflow and security test
  results, diagnostics, blockers, and task or plan status changes.

## Results

- Review remediation explains the trusted pre-agent context contract, removes
  provider-specific skill wording, and enables `synchronize` retriggers.
- The workflow fetches the complete PR-head history, proves the merge base,
  archives the complete skill bundle from the base commit, and prepares the
  bounded filesystem context before the agent starts.
- The managed OpenCode agent can read and search the trusted checkout, edit
  only `.pr-walkthrough/draft.json`, and run only the exact skill-local
  renderer command. It receives no GitHub write or R2 credentials.
- Removed both OpenCode TypeScript tools and all root-level walkthrough
  generation helpers. The GitHub-specific setup, publication, and PR-body
  helpers remain outside the portable skill.
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py`
  passed (4 tests).
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py`
  passed (4 tests).
- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` passed
  (20 tests).
- `python3 .github/scripts/lint-action-pinning_test.py` passed (9 tests).
- `python3 .github/scripts/lint-action-pinning.py` passed (19 workflow files).
- `actionlint .github/workflows/pr-walkthrough.yml` passed with actionlint
  v1.7.12 installed at `/root/go/bin/actionlint`.
- `git diff --check` passed.
