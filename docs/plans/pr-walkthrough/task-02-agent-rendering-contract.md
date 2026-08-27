---
id: "02-agent-rendering-contract"
title: "Provider-neutral agent rendering contract"
status: superseded
superseded_by: ../pr-walkthrough-portable-runner-fix/task-02-fixed-renderer-contract.md
wave: 1
depends_on: ["01-walkthrough-skill-renderer"]
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-walkthrough.md"
---

# Task 02: Provider-neutral agent rendering contract

Make the walkthrough agent responsible for the complete JSON, rendering, and
repair loop without coupling the skill to OpenCode. Supply a narrow managed
adapter when a CI runner cannot safely receive generic write or shell access.

- **Acceptance:** The provider-neutral skill requires the selected agent to
  render its walkthrough, inspect renderer failures, correct the data, and
  finish only after both JSON and HTML outputs exist.
- **Acceptance:** The managed rendering helper binds PR number, title, URL,
  repository, base branch, and head branch from trusted host metadata. It
  removes model-supplied PR/file URL overrides before invoking the trusted
  renderer.
- **Acceptance:** The OpenCode adapter accepts only complete walkthrough JSON,
  invokes only the fixed trusted helper, and returns renderer errors so the
  agent can retry. OpenCode retains denied generic edit and Bash permissions.
- **Verification:**

  ```text
  python3 scripts/pr-walkthrough-render.test.py
  bun build .github/scripts/opencode-pr-walkthrough-tool.ts --external @opencode-ai/plugin --target=bun --outfile /tmp/kandev-opencode-pr-walkthrough-tool.js
  ```

- **Files likely touched:** `.agents/skills/pr-walkthrough/SKILL.md`,
  `scripts/pr-walkthrough-render`, `scripts/pr-walkthrough-render.test.py`,
  `.github/scripts/opencode-pr-walkthrough-tool.ts`.
- **Dependencies:** Task 01.
- **Parallelism:** sequential because the workflow task consumes this adapter.

## Results

Added the trusted metadata-binding renderer helper and the OpenCode custom tool
adapter. The helper writes only the fixed PR JSON/HTML paths, strips model URL
overrides, and leaves no published outputs after renderer validation fails.
The provider-neutral skill wording remains pending because `.agents/**` is
read-only in the implementing session.
