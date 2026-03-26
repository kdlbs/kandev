Review Pull Request #{{pr.number}}: {{pr.title}}
Repository: {{pr.repo}}
PR: {{pr.link}}
Author: {{pr.author}}
Branch: {{pr.branch}} → {{pr.base_branch}}

To see ONLY the PR changes, use:
- git diff origin/{{pr.base_branch}}...HEAD (three-dot = only changes on the PR branch)
- git log --oneline origin/{{pr.base_branch}}..HEAD (list PR commits)
Do NOT review files outside this diff.