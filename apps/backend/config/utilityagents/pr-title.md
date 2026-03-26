Generate a concise Pull Request title for the following changes.

## Commits:
{{CommitLog}}

## Changed Files:
{{ChangedFiles}}

## Diff Summary:
{{DiffSummary}}

## Instructions:
1. Follow the Conventional Commits format: <type>(<scope>): <description>
2. Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore
3. Scope is optional but recommended (e.g., api, ui, db)
4. Description should be imperative mood ("add" not "added")
5. Keep it under 72 characters
6. Focus on the overall purpose of the PR, not individual commits

## Output Format:
Return ONLY the PR title, no explanations or markdown.