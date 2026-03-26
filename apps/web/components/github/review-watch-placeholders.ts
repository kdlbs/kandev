import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export const REVIEW_WATCH_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "pr.link",
    description: "PR URL",
    example: "https://github.com/org/repo/pull/123",
    executor_types: [],
  },
  {
    key: "pr.number",
    description: "PR number",
    example: "123",
    executor_types: [],
  },
  {
    key: "pr.title",
    description: "PR title",
    example: "Add user authentication",
    executor_types: [],
  },
  {
    key: "pr.author",
    description: "PR author username",
    example: "octocat",
    executor_types: [],
  },
  {
    key: "pr.repo",
    description: "Repository (owner/name)",
    example: "org/repo",
    executor_types: [],
  },
  {
    key: "pr.branch",
    description: "Source branch",
    example: "feature/auth",
    executor_types: [],
  },
  {
    key: "pr.base_branch",
    description: "Target branch",
    example: "main",
    executor_types: [],
  },
];

export const DEFAULT_REVIEW_WATCH_PROMPT = `Review Pull Request #{{pr.number}}: {{pr.title}}
Repository: {{pr.repo}}
PR: {{pr.link}}
Author: {{pr.author}}
Branch: {{pr.branch}} → {{pr.base_branch}}

To see ONLY the PR changes, use:
- git diff origin/{{pr.base_branch}}...HEAD (three-dot = only changes on the PR branch)
- git log --oneline origin/{{pr.base_branch}}..HEAD (list PR commits)
Do NOT review files outside this diff.`;
