---
status: draft
system: ci
created: 2026-11-16
requirements:
  - REQ-CI-UI-SCREENSHOTS-001
  - REQ-CI-UI-SCREENSHOTS-002
  - REQ-CI-UI-SCREENSHOTS-003
owners:
  - kandev
---

# UI screenshot presence check

## Ownership and mapping

CI owns the merge check. The existing PR skill owns capture and publication.
Reviewers assess whether images show the changed UI.

| Requirement | Design section |
| --- | --- |
| REQ-CI-UI-SCREENSHOTS-001 | Presence rule |
| REQ-CI-UI-SCREENSHOTS-002 | Exception |
| REQ-CI-UI-SCREENSHOTS-003 | Trusted workflow and rollout |

## Presence rule

Use one small Node script, `.github/scripts/pr-ui-evidence.cjs`, and one GitHub workflow.
Reuse the existing GitHub API client from `pr-docs.cjs`.
Do not add a general policy framework or dependency.

Classify runtime source, styles, and assets under `apps/web/`, `apps/packages/ui/`, and `apps/desktop/src/` as UI candidates.
Include `apps/desktop/index.html`.
Use explicit exclusions for tests, E2E, documentation, scripts, lockfiles, translation-only catalogs, and tool configuration.
Check both paths of a rename.
False positives use the maintainer exception. Review catches UI changes outside these roots.

For a candidate UI PR, inspect only its `## Screenshots` section, ending at the next second-level heading.
Require at least one inline Markdown image with nonempty alt text and an HTTPS URL.
Ignore fenced and indented code blocks, matched inline code spans, and HTML comments. Existing owned section markers remain supported but are not required.
A bot badge elsewhere in the body does not qualify.

```markdown
## Screenshots

![Phone comment action after text selection](https://github.com/user-attachments/assets/<asset-id>)
```

Keep the current PR skill's affected-viewport capture and public upload guidance.
The check does not require a commit declaration, fixed viewport schema, or image manifest.
It does not fetch URLs or inspect image bytes.
The author and reviewer assess freshness, public visibility, and visual relevance.

## Exception

Follow the existing documentation-check pattern.
The exact `no-screenshots-allow` label bypasses this check and appears explicitly in its result.
Use it for nonvisual changes inside a candidate directory or an explained capture limitation.
GitHub already controls who can apply repository labels. No extra permission query or comment command is needed.

The label remains effective across pushes until removed.
Maintainers remove it if the PR scope changes.
Adding or removing it reevaluates the current PR.
A body sentence such as "screenshots not needed" cannot waive the check.

## Trusted workflow and rollout

Use `pull_request_target` for opened, reopened, synchronize, edited, labeled, and unlabeled events.
Admit edited events for body or base changes, and label events for the exact exception label.
Support default-branch manual retry for an existing PR.
Serialize runs per PR without cancelling an active evaluation.

Checkout the trusted workflow revision with SHA-pinned actions and `persist-credentials: false`.
Use `contents: read`, `pull-requests: read`, and `statuses: write`.
Read the current PR, body, labels, and complete changed-file list through GitHub's API.
Never check out or execute the fork head or install its dependencies.

Publish `UI screenshots` on the evaluated PR head.
Reread relevant metadata before success and retry boundedly if it changed.
Missing or incomplete data produces an error, not an exemption.
Report one of: no UI changes, screenshots present, maintainer exception, screenshots missing, or evaluation error.

Update the PR skill, template, and existing contributor guide in the same implementation.
After merge and authorized smoke checks, add the context to the main ruleset without changing other rules.
Local workflow success alone does not establish merge enforcement.
Keep the hosted rollout as its own work order.

Current main has no merge-queue rule.
Queue support is outside this correction and must precede any later queue activation.

## Tests

Use the Node built-in runner for UI classification, image presence, unrelated badges, exception add/remove, forks, complete pagination, and stale events.
Use the existing workflow-contract pattern for trusted checkout, events, permissions, and no fork execution.
No browser suite or image-download tests are required.

## References

- [Policy decision](../../../decisions/2026-11-16-ui-screenshot-evidence.md)
- [Existing documentation-check pattern](pull-request-documentation-coverage.md)
- [Implementation plan](../../../plans/ui-screenshot-enforcement/plan.md)
