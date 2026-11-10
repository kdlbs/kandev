# Plugin author notification inventory

Checked: 2026-10-16. This is a planning snapshot for SDK distribution, not a
record of sent notifications. No author was contacted.

Current migration target: `@kdlbs/plugin-sdk`, selected on 2026-10-26.
The dependency names below record the original inspection. Authors must update
old `@kandev/plugin-sdk` declarations and imports during migration.

## Scope and evidence

The [official catalog source](https://github.com/kdlbs/kandev/blob/main/plugin-registry/plugins.yaml)
contains 16 entries: eight first-party plugins and eight community plugins.
Public repository searches found 11 additional plugin projects and two templates.
After deduplicating the Forgejo repository transfer, this inventory contains
29 repositories: 27 plugin projects and two templates.

Evidence comes from the live catalog, GitHub repository metadata, default-branch
manifests and dependency files, contributor lists, and public release endpoints.
Searches covered `kandev-plugin`, broader `kandev` repositories, and old Go/npm
SDK references. These searches can miss private repositories, differently named
projects, and unindexed code. The inventory is a discovered set, not a complete
census. Release tags below are GitHub releases, not SDK versions or compatibility
certifications. Source files can be newer than the latest release.

All 29 inspected default-branch `go.mod` files use a local replacement for
`github.com/kandev/kandev`. Twelve also declare `@kandev/plugin-sdk` with a local
`file:` dependency. Every listed repository therefore needs an SDK migration
assessment. A missing npm dependency does not mean a plugin has no frontend.

Contacts below use repository ownership and manifest authorship. Contributor
contacts are additional routing suggestions, not verified ownership assignments.
No inspected root or `.github/CODEOWNERS` file supplied a different owner.

## Community plugins in the official catalog

Notify these authors when the migration guide and public SDKs are ready. Send an
earlier design notice if feedback is needed before implementation.

| Plugin repository | Suggested contact | Latest public release | Dependency evidence |
| --- | --- | --- | --- |
| [Fclem/kandev-plugin-deepseek-balance](https://github.com/Fclem/kandev-plugin-deepseek-balance) | [@Fclem](https://github.com/Fclem) | [v0.1.3](https://github.com/Fclem/kandev-plugin-deepseek-balance/releases/tag/v0.1.3) | [Go replacement](https://github.com/Fclem/kandev-plugin-deepseek-balance/blob/main/go.mod) |
| [Fclem/kandev-plugin-prompt-history](https://github.com/Fclem/kandev-plugin-prompt-history) | [@Fclem](https://github.com/Fclem) | [v1.4.2](https://github.com/Fclem/kandev-plugin-prompt-history/releases/tag/v1.4.2) | [Go replacement](https://github.com/Fclem/kandev-plugin-prompt-history/blob/main/go.mod) + [npm file dependency](https://github.com/Fclem/kandev-plugin-prompt-history/blob/main/ui/package.json) |
| [ahmedbally/kandev-plugin-youtrack](https://github.com/ahmedbally/kandev-plugin-youtrack) | [@ahmedbally](https://github.com/ahmedbally) | [v0.2.0](https://github.com/ahmedbally/kandev-plugin-youtrack/releases/tag/v0.2.0) | [Go replacement](https://github.com/ahmedbally/kandev-plugin-youtrack/blob/main/go.mod) |
| [khuongdo/kandev-plugin-nulab-backlog](https://github.com/khuongdo/kandev-plugin-nulab-backlog) | [@khuongdo](https://github.com/khuongdo) | [v0.6.1](https://github.com/khuongdo/kandev-plugin-nulab-backlog/releases/tag/v0.6.1) | [Go replacement](https://github.com/khuongdo/kandev-plugin-nulab-backlog/blob/main/go.mod) |
| [runoverlabs/kandev-plugin-forgejo](https://github.com/runoverlabs/kandev-plugin-forgejo) | [@naerymdan](https://github.com/naerymdan) | [v0.4.0](https://github.com/runoverlabs/kandev-plugin-forgejo/releases/tag/v0.4.0) | [Go replacement](https://github.com/runoverlabs/kandev-plugin-forgejo/blob/main/go.mod) + [npm file dependency](https://github.com/runoverlabs/kandev-plugin-forgejo/blob/main/package.json) |
| [yattdev/kandev-plugin-notes](https://github.com/yattdev/kandev-plugin-notes) | [@yattdev](https://github.com/yattdev), contributor [@ayattara-sfl](https://github.com/ayattara-sfl) | [v0.6.1](https://github.com/yattdev/kandev-plugin-notes/releases/tag/v0.6.1) | [Go replacement](https://github.com/yattdev/kandev-plugin-notes/blob/main/go.mod) |
| [yattdev/kandev-plugin-redmine](https://github.com/yattdev/kandev-plugin-redmine) | [@yattdev](https://github.com/yattdev), contributor [@ayattara-sfl](https://github.com/ayattara-sfl) | [v0.3.2](https://github.com/yattdev/kandev-plugin-redmine/releases/tag/v0.3.2) | [Go replacement](https://github.com/yattdev/kandev-plugin-redmine/blob/main/go.mod) |
| [yattdev/kandev-plugin-tags](https://github.com/yattdev/kandev-plugin-tags) | [@yattdev](https://github.com/yattdev), contributor [@ayattara-sfl](https://github.com/ayattara-sfl) | [v0.16.0](https://github.com/yattdev/kandev-plugin-tags/releases/tag/v0.16.0) | [Go replacement](https://github.com/yattdev/kandev-plugin-tags/blob/main/go.mod) |

Forgejo's catalog entry still uses `naerymdan/kandev-plugin-forgejo`. GitHub
redirects it to `runoverlabs/kandev-plugin-forgejo`. The current manifest names
`naerymdan`, and the contributor endpoint identifies that account. Treat these
URLs as one plugin and route the notice to `@naerymdan`.

For Tags, Notes, Redmine, and Coordinator, manifests name `yattdev`.
`ayattara-sfl` is a substantial contributor across these repositories. Do not
assume the accounts are the same person. Tags also has contributions from
`Fclem`, who already owns two other plugins in the catalog.

## Community projects outside the official catalog

Catalog absence does not prove inactivity. Include these authors in outreach,
while distinguishing released projects from repositories without public releases.

| Plugin repository | Suggested contact | Latest public release | Dependency evidence |
| --- | --- | --- | --- |
| [Prithpal-Sooriya/kandev-plugin-github-lite](https://github.com/Prithpal-Sooriya/kandev-plugin-github-lite) | [@Prithpal-Sooriya](https://github.com/Prithpal-Sooriya) | [v0.1.9](https://github.com/Prithpal-Sooriya/kandev-plugin-github-lite/releases/tag/v0.1.9) | [Go replacement](https://github.com/Prithpal-Sooriya/kandev-plugin-github-lite/blob/main/go.mod) + [npm file dependency](https://github.com/Prithpal-Sooriya/kandev-plugin-github-lite/blob/main/package.json) |
| [abhishekbiyala/kandev-plugin-lambda-microvm](https://github.com/abhishekbiyala/kandev-plugin-lambda-microvm) | [@abhishekbiyala](https://github.com/abhishekbiyala) | [None found](https://github.com/abhishekbiyala/kandev-plugin-lambda-microvm/releases) | [Go replacement](https://github.com/abhishekbiyala/kandev-plugin-lambda-microvm/blob/main/go.mod) |
| [jumprope-jesse/kandev-plugin-jira-transition-comments](https://github.com/jumprope-jesse/kandev-plugin-jira-transition-comments) | [@jumprope-jesse](https://github.com/jumprope-jesse) | [None found](https://github.com/jumprope-jesse/kandev-plugin-jira-transition-comments/releases) | [Go replacement](https://github.com/jumprope-jesse/kandev-plugin-jira-transition-comments/blob/main/go.mod) |
| [nova28/kandev-plugin-ops-intel](https://github.com/nova28/kandev-plugin-ops-intel) | [@nova28](https://github.com/nova28) | [v0.33.0](https://github.com/nova28/kandev-plugin-ops-intel/releases/tag/v0.33.0) | [Go replacement](https://github.com/nova28/kandev-plugin-ops-intel/blob/main/go.mod) |
| [tito/kandev-plugin-gitea](https://github.com/tito/kandev-plugin-gitea) | [@tito](https://github.com/tito) | [v0.1.4](https://github.com/tito/kandev-plugin-gitea/releases/tag/v0.1.4) | [Go replacement](https://github.com/tito/kandev-plugin-gitea/blob/main/go.mod) + [npm file dependency](https://github.com/tito/kandev-plugin-gitea/blob/main/package.json) |
| [vivekgupta-memcode/kandev-plugin-memcode](https://github.com/vivekgupta-memcode/kandev-plugin-memcode) | [@vivekgupta-memcode](https://github.com/vivekgupta-memcode) | [None found](https://github.com/vivekgupta-memcode/kandev-plugin-memcode/releases) | [Go replacement](https://github.com/vivekgupta-memcode/kandev-plugin-memcode/blob/main/go.mod) + [npm file dependency](https://github.com/vivekgupta-memcode/kandev-plugin-memcode/blob/main/package.json) |
| [yattdev/kandev-plugin-coordinator](https://github.com/yattdev/kandev-plugin-coordinator) | [@yattdev](https://github.com/yattdev), contributor [@ayattara-sfl](https://github.com/ayattara-sfl) | [None found](https://github.com/yattdev/kandev-plugin-coordinator/releases) | [Go replacement](https://github.com/yattdev/kandev-plugin-coordinator/blob/main/go.mod) + [npm file dependency](https://github.com/yattdev/kandev-plugin-coordinator/blob/main/package.json) |
| [yustme/kandev-telegram](https://github.com/yustme/kandev-telegram) | [@yustme](https://github.com/yustme) | [None found](https://github.com/yustme/kandev-telegram/releases) | [Go replacement](https://github.com/yustme/kandev-telegram/blob/main/go.mod) + [npm file dependency](https://github.com/yustme/kandev-telegram/blob/main/package.json) |

## First-party plugins and templates

Coordinate these migrations with the Kandev maintainers. The contributor
lists identify `jcfs`, `carlosflorencio`, and `zeval` across this group. These
are routing candidates, not a per-repository ownership declaration.

| Repository | Catalog or template status | Latest public release | Dependency evidence |
| --- | --- | --- | --- |
| [kdlbs/kandev-plugin-augpool](https://github.com/kdlbs/kandev-plugin-augpool) | Outside official catalog | [v0.2.0](https://github.com/kdlbs/kandev-plugin-augpool/releases/tag/v0.2.0) | [Go replacement](https://github.com/kdlbs/kandev-plugin-augpool/blob/main/go.mod) |
| [kdlbs/kandev-plugin-bitbucket](https://github.com/kdlbs/kandev-plugin-bitbucket) | Official catalog | [v0.4.0](https://github.com/kdlbs/kandev-plugin-bitbucket/releases/tag/v0.4.0) | [Go replacement](https://github.com/kdlbs/kandev-plugin-bitbucket/blob/main/go.mod) + [npm file dependency](https://github.com/kdlbs/kandev-plugin-bitbucket/blob/main/package.json) |
| [kdlbs/kandev-plugin-cloudflare](https://github.com/kdlbs/kandev-plugin-cloudflare) | Outside official catalog | [None found](https://github.com/kdlbs/kandev-plugin-cloudflare/releases) | [Go replacement](https://github.com/kdlbs/kandev-plugin-cloudflare/blob/main/go.mod) + [npm file dependency](https://github.com/kdlbs/kandev-plugin-cloudflare/blob/main/package.json) |
| [kdlbs/kandev-plugin-coordinator-template](https://github.com/kdlbs/kandev-plugin-coordinator-template) | Template | [v0.2.1](https://github.com/kdlbs/kandev-plugin-coordinator-template/releases/tag/v0.2.1) | [Go replacement](https://github.com/kdlbs/kandev-plugin-coordinator-template/blob/main/go.mod) + [npm file dependency](https://github.com/kdlbs/kandev-plugin-coordinator-template/blob/main/package.json) |
| [kdlbs/kandev-plugin-github-status](https://github.com/kdlbs/kandev-plugin-github-status) | Official catalog | [v0.4.0](https://github.com/kdlbs/kandev-plugin-github-status/releases/tag/v0.4.0) | [Go replacement](https://github.com/kdlbs/kandev-plugin-github-status/blob/main/go.mod) |
| [kdlbs/kandev-plugin-kandy](https://github.com/kdlbs/kandev-plugin-kandy) | Official catalog | [v0.15.0](https://github.com/kdlbs/kandev-plugin-kandy/releases/tag/v0.15.0) | [Go replacement](https://github.com/kdlbs/kandev-plugin-kandy/blob/master/go.mod) |
| [kdlbs/kandev-plugin-petdex](https://github.com/kdlbs/kandev-plugin-petdex) | Outside official catalog | [None found](https://github.com/kdlbs/kandev-plugin-petdex/releases) | [Go replacement](https://github.com/kdlbs/kandev-plugin-petdex/blob/main/go.mod) + [npm file dependency](https://github.com/kdlbs/kandev-plugin-petdex/blob/main/package.json) |
| [kdlbs/kandev-plugin-provider-usage](https://github.com/kdlbs/kandev-plugin-provider-usage) | Official catalog | [v0.10.0](https://github.com/kdlbs/kandev-plugin-provider-usage/releases/tag/v0.10.0) | [Go replacement](https://github.com/kdlbs/kandev-plugin-provider-usage/blob/main/go.mod) |
| [kdlbs/kandev-plugin-session-cost](https://github.com/kdlbs/kandev-plugin-session-cost) | Official catalog | [v0.4.1](https://github.com/kdlbs/kandev-plugin-session-cost/releases/tag/v0.4.1) | [Go replacement](https://github.com/kdlbs/kandev-plugin-session-cost/blob/main/go.mod) |
| [kdlbs/kandev-plugin-slack](https://github.com/kdlbs/kandev-plugin-slack) | Official catalog | [v0.3.0](https://github.com/kdlbs/kandev-plugin-slack/releases/tag/v0.3.0) | [Go replacement](https://github.com/kdlbs/kandev-plugin-slack/blob/master/go.mod) |
| [kdlbs/kandev-plugin-task-manager](https://github.com/kdlbs/kandev-plugin-task-manager) | Official catalog | [v0.2.0](https://github.com/kdlbs/kandev-plugin-task-manager/releases/tag/v0.2.0) | [Go replacement](https://github.com/kdlbs/kandev-plugin-task-manager/blob/main/go.mod) |
| [kdlbs/kandev-plugin-template](https://github.com/kdlbs/kandev-plugin-template) | Template | [v0.2.0](https://github.com/kdlbs/kandev-plugin-template/releases/tag/v0.2.0) | [Go replacement](https://github.com/kdlbs/kandev-plugin-template/blob/main/go.mod) + [npm file dependency](https://github.com/kdlbs/kandev-plugin-template/blob/main/package.json) |
| [kdlbs/kandev-plugin-voice](https://github.com/kdlbs/kandev-plugin-voice) | Official catalog | [v0.2.0](https://github.com/kdlbs/kandev-plugin-voice/releases/tag/v0.2.0) | [Go replacement](https://github.com/kdlbs/kandev-plugin-voice/blob/main/go.mod) |

Both `kandev-plugin-template` and `kandev-plugin-coordinator-template` need a
migration handoff. Updating only the general template leaves new coordinator
plugins using the old dependency layout. These remain separate repository work.

## Notification order and content

1. Coordinate the first-party repositories and both templates with Kandev maintainers.
2. Contact the five catalog author groups: `naerymdan`, `yattdev`, `Fclem`,
   `ahmedbally`, and `khuongdo`. Include `ayattara-sfl` as a contributor contact.
3. Include released unlisted projects: `Prithpal-Sooriya`, `tito`, and `nova28`.
4. Include the remaining project owners: `abhishekbiyala`, `jumprope-jesse`,
   `vivekgupta-memcode`, and `yustme`. `yattdev` also owns the unlisted Coordinator.
5. Publish a general migration notice for undiscovered consumers and fork owners.

The private contact list is not known. The public list contains 12 primary
community contact accounts plus `ayattara-sfl` as a contributor contact.
A separate fork search found 21 repositories. They are excluded from the counts
above because a fork alone does not establish an independently maintained plugin.
Reassess actively diverged forks before sending direct notices.

An initial notice should identify the proposed module path, independent SDK
versioning, host-baseline policy, and draft PR. It must state that publication is
pending. The actionable release notice should link verified Go/npm versions,
the host/API mapping, public migration guide, checker command, and both updated
templates. Existing installed binaries are not automatically incompatible merely
because source imports change. Runtime compatibility remains a test obligation.

## Packaging-tool follow-through

Six inspected `go.mod` files also declare the backend's `cmd/plugin-pack` as a
Go tool: Augpool, Kandy, Session Cost, Task Manager, Voice, and Notes. Their Go
replacement links above show this dependency. Other packaging scripts were not
exhaustively audited.

Before claiming a plugin no longer needs a Kandev checkout, inspect its packaging
commands and CI as well as SDK imports. SDK extraction alone does not establish
independent distribution of backend-owned tools. WO05 must record any remaining
checkout requirement and route unresolved tooling work separately. This inventory
does not select a new tool-distribution contract or authorize external changes.

## Refresh before outreach

Recheck repository redirects, contacts, releases, and catalog membership before
sending messages. Confirm the SDK release passed public consumer verification.
Record notification status only after an explicitly authorized message is sent.
Do not create issues or notify contributors as part of inventory maintenance.

[Return to author migration work order](task-05-author-migration.md).
