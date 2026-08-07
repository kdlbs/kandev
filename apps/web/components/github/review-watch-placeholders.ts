import type { TFunction } from "i18next";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

/**
 * Placeholder tokens available in the review-watch prompt.
 *
 * A function rather than a module-scope const because `description` is copy: a
 * `t()` call at module scope would freeze at the boot locale (see docs/i18n.md).
 * `key` is the substitution token the backend expands and `example` is sample
 * GitHub data — neither is translated.
 */
export function reviewWatchPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "pr.link",
      description: t("github:placeholderPrUrl"),
      example: "https://github.com/org/repo/pull/123",
      executor_types: [],
    },
    {
      key: "pr.number",
      description: t("github:placeholderPrNumber"),
      example: "123",
      executor_types: [],
    },
    {
      key: "pr.title",
      description: t("github:placeholderPrTitle"),
      example: "Add user authentication",
      executor_types: [],
    },
    {
      key: "pr.author",
      description: t("github:placeholderPrAuthor"),
      example: "octocat",
      executor_types: [],
    },
    {
      key: "pr.repo",
      description: t("github:placeholderRepository"),
      example: "org/repo",
      executor_types: [],
    },
    {
      key: "pr.branch",
      description: t("github:placeholderSourceBranch"),
      example: "feature/auth",
      executor_types: [],
    },
    {
      key: "pr.base_branch",
      description: t("github:placeholderTargetBranch"),
      example: "main",
      executor_types: [],
    },
  ];
}

/**
 * Seeded into the prompt field, then persisted and sent to the agent verbatim.
 * Deliberately NOT translated: it is agent instructions and stored data, so
 * translating it would write locale-dependent text into a user's saved watch.
 */
export const DEFAULT_REVIEW_WATCH_PROMPT = `Review Pull Request #{{pr.number}}: {{pr.title}}
Repository: {{pr.repo}}
PR: {{pr.link}}
Author: {{pr.author}}
Branch: {{pr.branch}} → {{pr.base_branch}}

To see ONLY the PR changes, use:
- git diff origin/{{pr.base_branch}}...HEAD (three-dot = only changes on the PR branch)
- git log --oneline origin/{{pr.base_branch}}..HEAD (list PR commits)
Do NOT review files outside this diff.`;
