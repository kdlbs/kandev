---
status: active
system: integrations
created: 2026-07-31
owners:
  - kandev
---
# Bitbucket Connector Plugin Requirements

## Overview

Teams using Bitbucket Cloud or Bitbucket Data Center need repository, pull-request, and task workflows without putting Bitbucket API knowledge or credentials into the Kandev host. The connector must feel native where users already create, link, review, and reference work, while remaining independently releasable as an official plugin.

## Requirements

### REQ-INTEGRATIONS-BITBUCKET-PLUGIN-001: Bitbucket Connector Plugin

**Intent:** Teams using Bitbucket Cloud or Bitbucket Data Center need repository, pull-request, and task workflows without putting Bitbucket API knowledge or credentials into the Kandev host. The connector must feel native where users already create, link, review, and reference work, while remaining independently releasable as an official plugin.

#### Acceptance criteria

- **AC-INTEGRATIONS-BITBUCKET-PLUGIN-001.1:** Kandev ships the official `kandev-plugin-bitbucket` from its dedicated public repository. Its manifest pins `min_kandev_version` to the first released host version containing the required generic contracts; it never guesses an unreleased version.
- **AC-INTEGRATIONS-BITBUCKET-PLUGIN-001.2:** The initial release follows Kandev's current unsigned marketplace trust contract: the generated internal `checksums.txt` is mandatory, the host reports the package as unsigned, and neither repository claims cryptographically verified publisher provenance. Signing is future host-wide work rather than a Bitbucket release gate, as recorded in [ADR-2026-08-01-bitbucket-initial-release-remains-unsigned](../../../decisions/2026-08-01-bitbucket-initial-release-remains-unsigned.md).
- **AC-INTEGRATIONS-BITBUCKET-PLUGIN-001.3:** The plugin supports Bitbucket Cloud and Bitbucket Data Center through separate adapters behind one Bitbucket domain. Cloud and Data Center have full capability parity wherever their APIs provide an equivalent operation. Capability flags hide unavailable controls or explain version-specific limits; the UI never presents a non-working equivalent.
- **AC-INTEGRATIONS-BITBUCKET-PLUGIN-001.4:** A workspace connects with Cloud API token or OAuth 2.0, and with Data Center personal/HTTP access token or OAuth 2.0 when its administrator configured an incoming OAuth application link. Cloud app passwords are not accepted. OAuth client registrations are bring-your-own per workspace; no client secret ships in the public plugin.
- **AC-INTEGRATIONS-BITBUCKET-PLUGIN-001.5:** The plugin owns Bitbucket REST payloads, OAuth and token rules, product/version probes, pagination, rate-limit handling, health polling, secret refresh, connector screens, watches, normalized review/status data, and provider actions. The host owns reusable authenticated action, repository-provider, task-action, review-provider, reference-source, task ownership, credential-broker, and code-host presentation contracts. GitHub, GitLab, and compatible plugins use the same host-rendered task status and review-detail anatomy rather than provider-owned approximations.
- **AC-INTEGRATIONS-BITBUCKET-PLUGIN-001.6:** Users browse/search repositories, select branches, inspect pull-request URLs, launch tasks from pull requests, link/unlink existing tasks, and create pull requests from a task checkout branch. Remote descriptors preserve the exact credential-free clone URL, including Data Center context paths.
- **AC-INTEGRATIONS-BITBUCKET-PLUGIN-001.7:** Native **Create PR** remains conditional on a task checkout containing commits and having no linked change request. For a registered provider, Kandev keeps the shared dialog, pushes the verified checkout branch through its normal Git path, and then invokes the provider's authenticated create callback. Unsupported provider options, such as draft creation, are hidden rather than silently ignored. This boundary is recorded in [ADR-2026-08-10-plugin-change-request-mutations](../../../decisions/2026-08-10-plugin-change-request-mutations.md).
- **AC-INTEGRATIONS-BITBUCKET-PLUGIN-001.8:** Plugin task actions appear in native task surfaces. Inside the existing **Link** submenu, the required entry is named **Bitbucket Pull Request**; it never repeats the parent verb as “Link Bitbucket Pull Request.” Selecting it opens the same host-owned task change-request dialog used by GitHub: title and explanatory description, one labeled input, inline validation, Cancel and Save footer actions, disabled/submitting state, success toast, and close-on-success. Provider code owns reference parsing and the authenticated link mutation, not modal anatomy. Desktop and mobile expose the same capability and host-native dialog behavior.

## System design

The migrated technical source is split into [part 1](../system-design/bitbucket-plugin-01.md), [part 2](../system-design/bitbucket-plugin-02.md).
