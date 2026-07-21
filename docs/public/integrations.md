---
title: "Integrations"
description: "Connect Azure DevOps, GitHub, GitLab, Jira, Linear, Sentry, and Slack, then browse external work or create watched tasks."
---

# Integrations

Integrations let Kandev's backend read and update provider data. They power repository and issue browsers, task associations, watches, pull-request review, and provider-specific task launchers.

They do **not** provide every credential a task needs. Keep these paths distinct:

- an integration credential lets the Kandev backend call a provider API;
- Git or SSH credentials in an executor let the task fetch and push a repository;
- an agent login or API key lets the coding CLI call its model provider.

GitHub is the important exception. Kandev first resolves explicit profile remote-auth secrets; a resulting `GITHUB_TOKEN` or `GH_TOKEN` is an unmanaged override. Otherwise, for each repository identified as GitHub, the task receives an opaque, task/repository-scoped credential lease instead of an ambient token. Git resolves the matching lease against the workspace automation connection when it runs, so an App installation token can be renewed during a long task. The App private key and personal user tokens are never sent to an executor. Repository and credential-generation checks are repeated when a lease is resolved, and disconnecting or replacing the connection invalidates old leases.

For GitLab, Kandev resolves only the active task workspace's connection. It provides that connection's token as `GITLAB_TOKEN` and its normalized host as `GITLAB_HOST`/`KANDEV_GITLAB_HOST` to the execution path, and configures HTTP Git authentication only for the matching host. It does not reuse another workspace's GitLab credential or silently fall back from a self-managed host to `gitlab.com`. SSH remotes still require usable SSH credentials in the executor.

A task can therefore display a pull or merge request while its worktree cannot push, or edit a repository while Kandev cannot read provider state. Diagnose the failing credential path separately. An App token redeemed through the broker is minted for that repository. PAT and named-CLI tokens remain bearer credentials with all provider-granted scopes once delivered to the trusted Git or `gh` subprocess; lease matching prevents accidental cross-repository redemption but cannot narrow those tokens at GitHub. GitLab integration tokens are provided only to tasks in their configured workspace and should be treated as credentials that task agents may receive.

## Open integration settings

Select **Settings > Workspaces > _Workspace_ > Integrations**, then choose a provider. The direct routes are:

- `/settings/workspace/{workspaceId}/integrations/github`
- `/settings/workspace/{workspaceId}/integrations/gitlab`
- `/settings/workspace/{workspaceId}/integrations/azure-devops`
- `/settings/workspace/{workspaceId}/integrations/jira`
- `/settings/workspace/{workspaceId}/integrations/linear`
- `/settings/workspace/{workspaceId}/integrations/sentry`
- `/settings/workspace/{workspaceId}/integrations/slack`

Compatibility routes under **Settings > Integrations** use the active workspace where the provider has workspace settings.

GitHub, GitLab, Azure DevOps, Jira, Linear, and Slack configuration are workspace-specific. GitHub supports one automation connection per workspace and, for App-backed workspaces, one personal identity per Kandev user and workspace. The current GitHub integration targets `github.com`; GitLab supports `gitlab.com` and self-managed origins. Sentry supports multiple named instances per workspace. Do not assume that configuring one workspace gives another the same provider scope.

Provider secrets saved by these forms use Kandev's encrypted secret store. The backend must still decrypt them to make API requests. Limit access to settings and the Kandev data directory, and use the narrowest provider scope that works.

### The Enabled switch

Jira, Linear, Sentry, and Slack pages show an **Enabled** switch. It is a browser-local preference, saved per installation in that browser and on by default. It controls some client-side entry points, availability checks, and configuration fetches; settings pages can still poll provider health. It does not delete backend configuration and does not stop a server-side watch or Slack poller. Pause/delete watches or remove the provider configuration when processing must stop.

Health results are cached and periodically refreshed (normally about every 90 seconds in the settings UI). Use **Test connection** after changing a URL or credential rather than waiting for the next probe.

## GitHub

Use GitHub for pull requests, issues, reviews, checks, repository discovery, task associations, and provider-triggered work. Browse it at `/github` after connecting an account.

### Authenticate

Open the workspace GitHub settings. **Workspace automation** offers three connection types:

- **Personal access token (PAT):** Kandev validates the token before replacing the current connection and stores it in the encrypted secret store. A classic PAT needs `repo` and `read:org` for full behavior. Scope a fine-grained token to only the repositories and operations the workspace needs.
- **GitHub CLI:** first run `gh auth login` as the operating-system user that runs the Kandev backend. Kandev lists every authenticated host/login pair and stores the selected `github.com` login, not its token. It resolves that exact account with `gh auth token --hostname github.com --user <login>` and never changes the host's active account with `gh auth switch`.
- **GitHub App:** recommended for organization-managed or unattended automation. From the
  workspace, select a known registration, add an App you already own, or create one through
  GitHub's App Manifest flow, then install it on the intended account. Kandev keeps root App
  credentials server-side and mints short-lived installation tokens as needed.

A workspace has one active automation connection at a time. Replacing it changes the identity used by repository discovery, watches, background work, and task GitHub access in that workspace only. Disconnecting a CLI connection does not sign the host out of `gh`; disconnecting an App connection removes only the workspace binding and does not uninstall the App from GitHub.

The status panel identifies the selected source, verified actor, connection state, rate limits, and any missing App capabilities. A failed PAT or CLI validation leaves the previous connection intact. An unknown CLI login, revoked PAT, suspended/deleted installation, or missing App permission affects only the bound workspace and displays a reconnect or capability-specific error.

### Automation and personal identity

PAT and CLI connections are human identities. They provide both workspace automation and the fallback identity for **My GitHub** views and user-triggered actions.

A GitHub App installation is an automation identity, not a person. App-backed repository discovery, watches, task Git operations, pull-request creation, reviews, and merges are attributed to the App when the App is the effective actor. To see pull requests or issues assigned to the current user, connect **My GitHub identity** in that workspace. This is a GitHub App user authorization, stored per Kandev user and workspace.

Kandev routes credentials as follows:

| Operation | Credential | GitHub attribution |
|---|---|---|
| Background reads/writes, watches, task Git, and agent `gh` | Workspace automation | PAT/CLI user or App |
| **My GitHub** reads | Personal identity, then human PAT/CLI automation | User |
| User-triggered review, merge, or other mutation | Personal identity, then human PAT/CLI automation, then App | Effective actor shown in the UI |

An App-only workspace continues automation without a personal connection, but **My GitHub** remains unavailable. A personal connection cannot widen access: Kandev intersects the workspace repository scope, the App installation's repositories, and the user's GitHub access. Personal access and refresh tokens are never exposed to agents or executors.

For task processes, Git's credential helper selects among all attached repository leases. The
broker-aware `gh` shim uses the primary repository lease for each invocation. With App automation,
that makes agent-issued `gh` commands primary-repository scoped; use Kandev's workspace-aware
backend actions for another attached repository. PAT/CLI `gh` commands still receive the broader
bearer grant described above. Explicit executor-profile tokens bypass these managed guarantees and
must be scoped and rotated independently.

### Use a GitHub App

Choose a GitHub App when automation should not depend on one person's long-lived credential, when
an organization wants to approve repository access centrally, or when background jobs need
short-lived tokens. PAT and named CLI connections are simpler for a local workspace that should act
as one person. An App adds ownership, public callback, webhook, installation, and credential-
rotation responsibilities, and GitHub attributes its automation to the App rather than a human.

Kandev stores a catalog of App registrations. Adding or creating a registration does not change a
workspace's active connection; the workspace must explicitly select the registration and complete
an installation. You can use the same registration in several related workspaces, or create
separate registrations for stronger isolation:

| Choice | Shared | Isolated per workspace installation |
|---|---|---|
| Reuse one registration | App owner and bot identity, private key, OAuth client secret, webhook secret, permission policy, and root-credential revocation | Installation token, account/repository grants, workspace scope, personal OAuth tokens, and broker leases |
| Use separate registrations | Nothing at the App-credential layer | App owner, bot identity, root credentials, permission policy, revocation, installations, repository grants, and workspace credentials |

Reuse is appropriate for related workspaces that deliberately share one organizational automation
identity. Create separate registrations for work and personal accounts, unrelated organizations,
or any boundary where rotating or deleting one App must not affect the other.

This feature targets `github.com`, not GitHub Enterprise Server. Before creating or importing an
App, give Kandev a stable public HTTPS origin. GitHub must reach the callback and webhook routes
from the public internet. Guided setup rejects private, loopback, split-horizon, and plain HTTP
origins. A local deployment needs a trusted HTTPS tunnel or reverse proxy whose hostname remains
stable for the life of the registration.

#### Create a new App

1. Open the workspace's GitHub integration settings, choose **Change connection**, then choose
   **GitHub App** and **Create new App**.
2. Choose the GitHub user or organization that will own the App, enter a registration label and
   the public Kandev origin, and review the requested permissions.
3. Keep **Private** unless the same App must be installable outside its owner. **Public** means
   other GitHub accounts can install it. It does not publish the App to Marketplace, reveal its
   credentials, or grant any repository without an approved installation.
4. Continue to GitHub and confirm the generated manifest within one hour. Kandev verifies the
   single-use callback, encrypts the returned credentials, and adds the registration without a
   restart.
5. Back in the workspace, select the new registration and install it. The GitHub account owner
   still chooses the account, organization, and repositories granted to that workspace.

Creating an App does not select it automatically. If GitHub creates the App but Kandev cannot save
the callback result, remove that orphan App in GitHub or restart the registration flow; Kandev
cannot delete the provider-side App for you.

#### Add an existing App

Use **Add existing App** when you own a GitHub App that should follow the same catalog lifecycle as
a Kandev-created App. The preparation step allocates a registration ID and displays exact,
copyable settings for that pending import. It is short-lived and single-use.

1. Start **Add existing App** from the workspace GitHub connection flow. Enter its owner, label,
   visibility, and the public Kandev origin.
2. Open the App under the owning user's or organization's **Settings > Developer settings > GitHub
   Apps**, then apply every URL, permission, and event shown by Kandev. Set the App homepage to the
   public Kandev origin, use JSON webhook delivery with SSL verification, request user
   authorization during installation, and keep expiring user tokens enabled.
3. Return before the preparation expires and provide the App ID, OAuth client ID and secret, App
   slug, webhook secret, and an RSA private key generated for that App. Treat every secret field as
   a root credential and never put it in workspace environment variables or executor profiles.
4. Kandev authenticates as the App, verifies its ID, owner, slug, homepage, permissions, events,
   and webhook settings where GitHub exposes them, then encrypts the credential bundle. You must
   confirm callback/setup settings that GitHub's API does not expose.
5. Select and install the imported registration. Importing alone never replaces the workspace's
   current PAT, CLI account, or App installation.

If the App is already in the catalog, Kandev directs you to the known registration instead of
storing a second copy of the same root credentials.

Each registration uses its own ID in every public route. The UI supplies complete URLs; these path
templates are useful when checking a proxy or GitHub setting:

| Purpose | URL path |
|---|---|
| Manifest creation callback | `/api/v1/github/app/registrations/{registrationId}/manifest/callback` |
| Workspace installation setup | `/api/v1/github/app/registrations/{registrationId}/install/callback` |
| Personal identity OAuth callback | `/api/v1/github/app/registrations/{registrationId}/personal/callback` |
| Signed webhook delivery | `/api/v1/github/app/registrations/{registrationId}/webhook` |

The registration starts with an unverified webhook status. It becomes verified only after Kandev
receives a correctly signed delivery on that registration's route. A failing status indicates a
route, proxy, secret, or post-signature processing problem; a successful browser callback alone
does not prove webhook reachability.

For full Kandev behavior, request the smallest applicable repository/organization permissions from this list:

| GitHub App permission | Access | Used for |
|---|---|---|
| Metadata | Read | Repository discovery and identity. |
| Contents | Read and write | Clone, fetch, push, and repository content changes. |
| Pull requests | Read and write | PR browsing, creation, reviews, and merges. |
| Issues | Read and write | Issue browsing and updates. |
| Checks | Read | Check runs and conclusions. |
| Commit statuses | Read | Commit status reporting. |
| Actions | Read | Workflow-run status. |
| Administration | Read | Branch-protection details. |
| Members | Read | Organization/team membership lookups. |
| Workflows | Write | Changes under `.github/workflows`; omit when agents must not edit workflow files. |

Subscribe only to `installation`, `installation_repositories`, and `github_app_authorization`. Kandev uses these events to track installation suspension/deletion, repository access changes, and revoked personal authorizations. PR, issue, review, and CI watches continue to poll and do not require their corresponding webhooks. GitHub's [registration guide](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app), [App permission reference](https://docs.github.com/en/rest/authentication/permissions-required-for-github-apps), and [webhook guide](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/using-webhooks-with-github-apps) describe the provider-side settings.

To delete a registration, first disconnect every workspace using it and remove personal identities
issued through it. Kandev blocks deletion while any workspace or personal connection is bound,
deletes only the encrypted catalog credential bundle, and does not delete or uninstall the App on
GitHub. Remove the provider-side App separately only after confirming that no other deployment uses
it.

### Upgrade and recovery

Workspaces that existed when workspace authentication was introduced receive a **Legacy shared** connection so upgrades do not immediately lose GitHub access. It preserves the previous installation-wide resolution behavior while the workspace is migrated. New workspaces start disconnected. After a legacy workspace selects a PAT, named CLI account, or App installation, it cannot return to legacy mode. Copying a workspace never copies authentication or App installation bindings.

Legacy shared resolution checks an authenticated host `gh` CLI first, then backend `GITHUB_TOKEN`, backend `GH_TOKEN`, and finally the old stored `GITHUB_TOKEN`/`github_token` secret. Those ambient sources are migration compatibility only; configure an explicit workspace connection to make identity and access deterministic.

For recovery:

- Replace an invalid PAT or select the exact CLI account again; validation must succeed before Kandev swaps the connection.
- Run `gh auth status --hostname github.com` as the Kandev service user when a selected CLI login disappears, then sign in that account again if necessary.
- Reconnect **My GitHub identity** after authorization expiry/revocation. App automation remains available while the personal connection is invalid.
- Ask an organization owner to unsuspend or reinstall an App, restore its repository selection, or grant a reported missing permission. Refresh the workspace status afterward.
- Disconnect and repeat **Install GitHub App** when the workspace is bound to the wrong installation. Removing the binding does not uninstall the provider-side App.
- To replace compromised App root credentials, disconnect every binding, delete the catalog
  registration, rotate the credentials in GitHub, and add the App again. Kandev does not rotate App
  private keys, OAuth client secrets, or webhook secrets automatically.

### Configure and use the workspace

Workspace GitHub settings control repository scope, default/saved searches, quick-action prompts, pull-request analytics, review watches, and issue watches. At `/github`, search or browse pull requests and issues, save queries, apply prompt presets, and launch a Kandev task. A saved query can default to one repository; choose **All repos** for no repository default, and change the repository filter without rewriting the saved query. An associated pull request also appears in task review surfaces for feedback, checks, reviews, and merge actions.

A **Review Watch** polls a GitHub search and creates review work. It requires a workflow, starting step, prompt, and workspace. The default query is `type:pr state:open review-requested:@me -is:draft`; add repository filters or replace the query as needed. An optional agent or executor profile overrides the selected step's defaults. The poll interval defaults to 300 seconds and accepts 60–3,600 seconds.

When a review watch is created, Kandev saves its verified target GitHub login. App-backed polling replaces `review-requested:@me` with that explicit login because an installation is not a user. Creating a user-targeted review watch therefore requires a connected personal identity or human PAT/CLI automation identity. A migrated watch with no verified target is disabled until an identity is reconnected.

An **Issue Watch** behaves similarly for issues. Its default search is `type:issue state:open`. Choose labels or provide a custom GitHub query; the custom query takes precedence over label selection.

Both watch types default to the **Auto** cleanup policy: delete merged/closed tasks only when the user has not typed a message. **Always** deletes even after user engagement; **Never** retains every task. You can pause a watch, poll immediately, or clean completed work. Deleting a GitHub review or issue watch best-effort cascade-deletes the tasks it owns. **Reset** is also destructive: after its preview, it permanently cascade-deletes every watch-created task, including archived tasks, and clears cursor/deduplication state so current matches become eligible again. Review-watch reset schedules a re-import; issue-watch reset re-imports on its next poll. Reset is not a way to keep old tasks and rerun a query.

Repository scope, authentication, and watch filters are workspace-specific. Repository scope constrains Kandev operations in addition to the repositories allowed by the selected credential; it cannot grant access the credential lacks. Explicit executor profile tokens remain a separate override and should be scoped independently. GitHub workspace configuration can be copied, but credentials, App installation bindings, personal identities, and watches are deliberately not copied.

## GitLab

GitLab supports workspace-scoped connections, issue and merge-request browsing, task launch and durable MR links, automation watches, linked-MR review actions, and merge-request creation. GitHub and GitLab can be connected at the same time; each provider uses its own credentials and records.

### Connect a workspace

Open **Settings > Workspaces > _Workspace_ > Integrations > GitLab**. Each workspace owns one connection: a normalized HTTP(S) origin and one authentication method. `https://gitlab.com` is the default. For a self-managed instance, enter the exact HTTP or HTTPS origin that the Kandev backend and task executors can reach. The configured scheme is significant for HTTP remotes, API requests, links, and MR creation.

The normal settings path saves an encrypted personal access token and validates it against GitLab before replacing the current connection. The UI calls out `api` and `read_user`; write actions need the corresponding upstream project permissions, and GitLab tier or project policy can further restrict approvals and merges. Use a dedicated account with only the access Kandev needs. Connection health refreshes periodically, normally about every 90 seconds, and distinguishes rejected authentication from an unreachable host.

Workspaces are isolated. Two workspaces can connect to different GitLab hosts or accounts simultaneously, and browse, watch, review, and write requests resolve only the requested workspace's connection. Removing a connection leaves links and watch definitions stored, but provider polling and actions fail until that workspace is connected again.

The settings header's **Copy configuration** action copies the host, authentication method, and stored PAT into another workspace after confirmation. It overwrites the target connection, but does not copy watches, task-launch action presets, or task-to-MR links.

### Browse, launch, and link tasks

Use `/gitlab` with the active workspace to browse built-in or saved merge-request and issue queries. Results are server-paginated in pages of 25. The project picker narrows only the current page's results; it is not a GitLab permission boundary or a provider-side project query, so matching items can still exist on later pages.

Each row has a task-launch menu backed by workspace-specific prompt presets. Launching from an issue adds its URL, title, and prompt context to the task but does not create a durable issue association. Launching from an MR also attempts to record a durable link. If that second step fails, the task still opens and Kandev shows an error with instructions to retry from the task.

Automatic repository selection and MR linking require a repository recorded as provider `gitlab` with the same normalized host and full `group/subgroup/project` path. A same-named project on another host is not eligible. From an existing task, select **Link GitLab merge request**, paste a URL from the workspace's configured host, and choose the task repository when the task has more than one. Unlinking removes only the Kandev association; it does not close, merge, or unsubscribe from the upstream MR.

Linked MRs appear in the GitLab results and task toolbar. One MR can be linked to several tasks, and multi-repository tasks keep each link scoped to its selected repository.

### Review a linked merge request

Open a linked MR from the task review surface. Kandev fetches and displays its overview, source and target branches, mergeability and conflicts, approvals, pipeline rollup, files, commits, reviewers, assignees, labels, and threaded discussions. From the same panel you can:

- reply to or resolve a discussion and add selected feedback to the active task session's prompt context;
- approve or unapprove, merge after confirmation, and update labels or assignees;
- search active project members and replace or clear the reviewer list using GitLab user IDs;
- subscribe or unsubscribe the connected GitLab user from that MR's upstream notifications;
- refresh, open in GitLab, or unlink the association.

These actions use the connected GitLab user's permissions and do not bypass protected branches, approval requirements, merge rules, or reviewer eligibility. The current product UI exposes notification subscription for linked merge requests; issue subscribe/unsubscribe is implemented in the backend integration but has no issue-detail control in `/gitlab`.

### Create review and issue watches

GitLab settings include **Merge request review watches** and **Issue watches**. Each watch selects a workflow and initial step, optional repository/base branch and profile overrides, a task prompt, project paths, raw GitLab query parameters, a 60–3,600 second poll interval, an optional maximum in-flight task count, and an **Auto**, **Always**, or **Never** cleanup policy. Issue watches can also require labels. Leaving project paths empty searches every project visible to the connected user.

With no custom query, a review watch searches open MRs that directly request the connected user as reviewer; an issue watch searches open issues assigned to that user. A custom query replaces those default constraints. GitLab has no team review-request equivalent, so the broader **Direct and group-compatible requests** setting currently behaves like direct user requests.

A new match creates at most one task for that watch and external item. MR-created tasks are linked to the MR; issue-created tasks retain issue metadata but are not durable issue associations. Watch controls let you edit, pause/enable, run now, reset, and delete. Invalid or removed workspace dependencies self-disable the affected watch and leave its error visible.

Both watch types inspect only their first GitLab result page, up to 50 items. Already-seen items still occupy that window, so narrow broad queries enough that important matches can reach the first page. The in-flight cap defers dispatch when too many watch-created tasks are active; it does not expand the provider result window.

**Reset** previews its task count, then permanently best-effort deletes all tasks owned by that watch, including archived tasks, clears deduplication state, and makes current matches eligible again. A review-watch reset schedules an immediate rerun; an issue-watch reset waits for the next poll or **Run now**. Deleting a watch also best-effort deletes all of its owned tasks and cannot trigger another run.

### Create a merge request from a task

For a task repository whose `origin` matches the workspace's GitLab host, the Changes surface labels the provider action **Create Merge Request**. Kandev pushes the current branch, uses an explicit target branch or resolves the GitLab project's default branch, supports draft MRs, and creates through an authenticated `glab` when available or the workspace token REST fallback. HTTPS, SSH, `gitlab.com`, and configured self-managed remotes are supported.

A successful create returns the MR URL and asynchronously records it against the originating task repository. If association fails, use the manual link action. Retrying is idempotent for an existing open MR with the same source and target branches. A push can succeed even when MR creation fails; Kandev reports that partial result and leaves the remote branch in place for retry.

## Azure DevOps

Azure DevOps configuration is workspace-specific. The current integration supports Azure DevOps Services organizations at `https://dev.azure.com/<organization>`. A trailing slash is accepted and removed when Kandev saves the canonical URL. Azure DevOps Server/TFS and alternate organization URL forms are not supported.

Enter the organization URL on the Azure DevOps settings page, then hover, focus, or tap the info icon beside **Personal Access Token**. Follow its **Create personal access token** link. In Azure DevOps, select **New Token**, choose the organization and an expiration, and select **Custom defined** scopes. Under **Work Items**, check **Read**; under **Code**, check **Read**; leave every other scope unchecked. Create the token, copy it while Azure DevOps still displays it, and paste it into Kandev.

Kandev stores the PAT in its encrypted secret store and calls Azure DevOps REST API 7.1 directly. The connection, work-item, and pull-request paths do not require GitHub, `gh`, `az`, or Azure CLI authentication. When editing a saved connection, a blank PAT preserves that workspace's existing credential. Copy configuration transfers the encrypted credential to the target workspace.

Use `/azure-devops` to browse work items and pull requests with built-in scopes or saved views. Kandev loads the default **Recently updated** work-item query when the page opens. Raw WIQL remains available under **Advanced** for custom work-item searches. Pull-request feedback includes reviewers and votes, comment threads, linked work items, and branch-policy results. Provider content is read-only in this release: Kandev does not edit work items, vote, comment, complete pull requests, or change policies.

You can launch a task from a work item or pull request. When the selected Kandev repository is configured with matching Azure project and repository identifiers, launching from a pull request also stores a durable task association. Task surfaces show its normalized status, review, and policy summary while Azure-native feedback remains in the Azure DevOps browser. Synchronization uses the backend REST client and does not depend on tools installed in the task environment.

The **Remote** picker in **New Task** searches configured GitHub, GitLab, and Azure DevOps repositories and keeps manual supported URLs available. When more than one repository provider is connected, use the provider tabs at the bottom of the picker to switch the visible results; the tabs stay hidden for a single provider. When all three providers are available, the tabs use compact provider icons with hover labels. For a private Azure repository, the backend uses the workspace PAT only while initially cloning or fetching the managed checkout. The PAT is not written into the remote URL, task metadata, command arguments, or agent environment. Configure executor Git credentials independently for pushes and for repository access outside that backend materialization path.

This release has no Entra OAuth flow, webhook, or watch poller.

## Jira

Jira configuration is workspace-specific. Use `/jira` to search with JQL, save views, open issue details, run supported transitions, and launch tasks with Jira prompt presets. Launch copies Jira URL/content into the task title and description; it does not store a durable Jira issue association on the task.

Enter the site URL (a missing scheme is normalized to HTTPS), choose **Cloud** or **Server/Data Center**, and optionally set a default project key. Authentication options are:

| Deployment | Method | Required values |
|---|---|---|
| Jira Cloud | API token (recommended) | Atlassian account email and API token. |
| Jira Cloud | Browser session | Only the value of the `cloud.session.token` or `tenant.session.token` cookie. Do not include the cookie name or `=`. |
| Server/Data Center | Personal access token | Bearer personal access token with the required read/write access. |

Cloud API tokens are not accepted for Server/Data Center, and Server/Data Center PATs are not the Cloud token flow. Browser-session JWTs expire and are less reliable than an API token; Kandev surfaces the decoded expiry and warns as it approaches.

When editing, a blank secret preserves the saved credential only if the URL, account identity, and authentication method still match. Supply a new secret when changing those identity fields. Save, select **Test connection**, and check the background health result.

### Jira issue watches

Create a watch with JQL, test the query, then choose a workflow and starting step. A new watch starts with `project = PROJ AND status = "Open" ORDER BY created DESC`; replace `PROJ` before testing. Repository selection is optional: leaving it blank creates repo-less tasks. When a repository is selected, a blank branch resolves to that repository's default branch. Blank agent and executor profile fields inherit the starting step's defaults. Customize the task prompt and set a poll interval, which defaults to 300 seconds and accepts 60–3,600 seconds.

The maximum in-flight value defaults to 5. Leave it blank for no cap. A cap defers remaining matches rather than importing them all at once. Each poll fetches only the first 50 JQL matches and does not paginate. Already-seen issues still occupy that provider result window, so a stable broad query can leave later matches unseen indefinitely; narrow the JQL enough that every important issue can enter the first page. Pause the watch before changing a broad query. Jira task-preset prompts can use ticket key, URL, title, and description placeholders from the preset editor.

Deleting a Jira watch leaves its previously created tasks in place. **Reset** is destructive: after the preview, it permanently deletes every watch-created task, including archived tasks, clears cursor/deduplication state, and makes current matches eligible for the next poll.

## Linear

Linear configuration is workspace-specific. Enter a personal API key and optionally a default team. Kandev calls the fixed Linear GraphQL endpoint at `https://api.linear.app/graphql` and sends the key as its authorization value. Leaving the credential blank during an edit keeps the stored key.

After saving and testing the connection, use `/linear` to search by text, team, or assignee, view issue details, change supported states, and launch tasks. Linear launch uses fixed title/description construction, has no prompt-preset editor, and does not store a durable Linear issue association.

Linear watches can filter by team, states, labels, priorities, assignee, creator, estimate range, and free-text query. At least one of those filters is required. They also define dispatch order, workflow and starting step, optional repository/base branch/profile overrides, prompt, poll interval, and a maximum in-flight count. New watches default to five in-flight tasks and **Priority (high → low)** dispatch. The poll interval defaults to 300 seconds and accepts 60–3,600 seconds; clear the in-flight field for no cap.

Leaving the repository blank creates repo-less tasks. When a repository is selected, a blank branch resolves to its default branch. Test narrow filters before enabling the watch. Deleting a Linear watch retains existing tasks; **Reset** permanently deletes every watch-created task, including archived tasks, clears cursor/deduplication state, and makes current matches eligible for the next poll.

Linear polling is also bounded. **Default (Linear order)** reads one page of 50; an explicit dispatch sort reads at most five pages of 50 before sorting locally. Matches outside that window can remain unseen, and reset does not bypass the bound.

## Sentry

Sentry configuration is workspace-specific and supports multiple named instances. This is useful when one Kandev workspace spans different Sentry organizations or self-hosted installations.

Create an instance with a unique name, base URL, and bearer authentication token. The default URL is `https://sentry.io`; replace it for self-hosted Sentry. A URL with no scheme becomes HTTPS. It must be a bare HTTP(S) host root—paths, queries, and fragments are rejected. The UI lists `org:read`, `project:read`, and `event:read` as the required read scopes.

On any saved edit, a blank token preserves the existing token, including when the URL changes. The pre-save **Test connection** candidate cannot reuse that stored token after a URL change, so paste the token to test the new URL before saving.

A Sentry watch binds to one instance, organization, and project; the selected instance is immutable after creation. It can filter environment, level, one status, and a free-text Sentry query, then select a workflow/step, optional repository/base/profile overrides, prompt, poll interval, and maximum in-flight count. New watches default to `fatal` and `error` levels, `unresolved` status, a 24-hour stats period, five in-flight tasks, and a 300-second poll interval. The interval accepts 60–3,600 seconds; clear the in-flight field for no cap. Although the UI currently permits selecting several statuses, the backend rejects save with more than one because Sentry has no OR form for `is:`. Passthrough agent profiles are not offered to watches.

Leaving the repository blank creates repo-less tasks. With a selected repository, a blank branch resolves to its default branch. Deleting a Sentry watch retains its existing tasks; **Reset** permanently deletes every watch-created task, including archived tasks, clears cursor/deduplication state, and makes current matches eligible for the next poll.

Each Sentry poll reads only the newest first page (up to 100 issues) and does not paginate. Older matches can remain unseen while newer/seen issues occupy that page; reset does not force a complete backlog import.

An instance cannot be deleted while a watch references it. Because the instance binding is immutable, delete those watches first and recreate them against another instance if needed. Sentry issues appear in task issue-selection/current-task surfaces; there is no top-level `/sentry` browser comparable to GitHub, GitLab, Jira, or Linear.

## Slack

Slack support currently uses a browser-session polling connection. It is intended for a controlled personal workspace and is more fragile than OAuth or a bot installation. Kandev does not currently offer a Slack OAuth/bot install flow.

Configure, per workspace:

- an `xoxc-...` browser session token;
- only the value of the Slack `d` cookie;
- a **Utility Agent** from **Settings > Utility agents**;
- a command prefix, default `!kandev`;
- a polling interval, default 30 seconds and allowed range 5–600 seconds.

The workspace owns this configuration record, but it does not hard-pin the destination of a created task. The built-in triage prompt deliberately lists every Kandev workspace and asks the agent to choose one. Separate workspace configurations keep separate polling cursors; reusing the same Slack account and prefix in several configurations can therefore process the same authored message more than once.

With a saved configuration whose latest authentication health check succeeded, Kandev polls messages visible to the connected Slack user. The browser-local **Enabled** preference is not part of this backend gate. A message authored by that same Slack user and beginning with the prefix can trigger in a channel or direct message.

On the first successful scan the watermark is empty. Slack search returns the newest 30 matching messages, and Kandev processes those matches oldest-first; enabling the integration can therefore act on up to 30 messages that already existed. Use a unique prefix, remove or edit old matching messages, or be ready to remove unintended tasks before first configuration.

For each match, Kandev best-effort adds an eyes reaction, fetches the surrounding thread, and gives the request, thread, and external Kandev MCP endpoint to the selected utility agent. It then best-effort posts the agent's final response in-thread. Reaction failure does not stop task creation. Reply failure still advances the watermark, so a task can exist without a Slack reply. A thread-fetch or agent-run failure stops that scan's batch and retries the failed and later matches on a future scan.

This is external/configuration MCP, not a task-scoped MCP session: the endpoint also exposes destructive task and configuration tools. Use a constrained utility agent and model, treat matching Slack text as untrusted input, and review the [external MCP security boundary](automation-and-mcp.md#external-mcp-security-boundary).

Slack has no separate prompt editor. It uses the chosen Utility Agent's prompt from **Settings > Utility agents**, which can reference `{{SlackInstruction}}`, `{{SlackThread}}`, `{{SlackPermalink}}`, `{{SlackUser}}`, `{{SlackChannelID}}`, and `{{SlackTS}}`. If the raw utility prompt contains any Slack-specific placeholder, its resolved value is the complete prompt. Otherwise Kandev uses the resolved utility prompt as the system text and appends Slack context; the built-in triage instructions are used only when that resolved prompt is blank.

Slack does not trigger on reactions, expose a slash command/shortcut, mirror task status, or provide a live chat bridge to a running coding agent. It searches matching messages rather than performing a one-time history import, which is why the first scan can process existing matches. Browser session credentials can expire without notice; reconnect when polling starts returning authentication failures. Turning off the browser-local **Enabled** switch does not stop the backend poller—remove the saved Slack configuration to stop it.

## Copy configuration between workspaces

Supported integration pages offer **Copy configuration** with provider-specific behavior:

- GitHub copies repository scope, saved/default searches, and quick-action presets. It does not copy authentication or watches.
- Azure DevOps, Jira, Linear, and Slack copy the workspace configuration and encrypted credential, replacing the target's provider configuration and re-running health checks. They do not copy watches.
- Sentry adds copies of the source instances with new IDs and copied secrets, preserves target instances, and deduplicates conflicting names. It does not copy watches.
- GitLab replaces the target workspace's host, authentication method, and stored PAT. It does not copy watches, task-launch action presets, or task-to-MR links.

Workspace automations are never copied by this action. Review the target workspace's repository and workflow scope before enabling any copied connection.

## Security and troubleshooting

Issue bodies, pull-request comments, commit messages, Slack threads, and incident details are untrusted prompt input. Use read-only credentials for triage, restrict repositories/projects/channels, and keep a human workflow gate before merge, release, deployment, or sensitive transitions.

- **Connection test fails:** verify the base URL, deployment type, token format, expiration, scopes, and network/DNS access from the backend host.
- **Cleared token but connection remains:** for GitHub, a higher-priority CLI or environment credential may still be active. Clearing GitLab from its workspace settings removes that workspace connection.
- **Repository, project, or team is missing:** confirm the connected identity can see it and check workspace filters/defaults.
- **Kandev can read but cannot write:** add only the specific provider write scope needed, then repeat the test.
- **Task cannot fetch or push:** inspect the selected executor's Git/SSH credentials and repository remote. For GitHub, inspect any explicit profile `GITHUB_TOKEN`/`GH_TOKEN`; otherwise verify the workspace automation connection, repository scope, broker reachability, and App Contents permission. For GitLab HTTP remotes, confirm the task workspace connection host exactly matches the remote and its token can access the project. The Azure PAT can authenticate the backend's initial managed clone/fetch but is not exposed to the task for later pushes. Jira, Linear, Sentry, and Slack integration credentials are not task Git credentials.
- **A watch still runs after disabling the provider:** the Enabled switch is browser-local. Pause/delete the watch, or remove the backend configuration.
- **Unexpected work is created:** pause the watch or automation, inspect its query, last-polled/status fields, and created-task list, then narrow provider filters before resetting or polling again. Watch tables do not provide a separate run/import history.

Related: [Tasks and workflows](tasks-and-workflows.md), [Sessions and review](sessions-and-review.md), and [Automation and MCP](automation-and-mcp.md).
