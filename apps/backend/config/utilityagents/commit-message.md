Generate a concise git commit message for the following changes.

## Staged Changes (git diff --staged):
{{GitDiff}}

## Instructions:
1. Follow the Conventional Commits format: <type>(<scope>): <description>
2. Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore
3. Scope is optional but recommended (e.g., api, ui, db)
4. Description should be imperative mood ("add" not "added")
5. Keep the first line under 72 characters
6. If needed, add a blank line and bullet points for details

## Output Format:
Return ONLY the commit message, no explanations or markdown.