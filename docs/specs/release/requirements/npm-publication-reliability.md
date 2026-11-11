---
status: active
system: release
created: 2026-11-09
owners:
  - kandev
---

# npm Publication Reliability Requirements

## Overview

Release maintainers need publication results that distinguish registry delay from authentication errors.
They also need safe recovery after npm accepts an upload.
The release system owns these delivery checks and their operational reports.

The [SDK distribution contract](../../plugins/requirements/sdk-distribution.md) owns SDK identity, compatibility, bootstrap, and public consumer verification.
This document adds delivery requirements without changing that contract.

## Terminology

- **Accepted upload:** The npm publication command exits successfully.
  Public availability remains unconfirmed until a registry read finds the version.
- **Uncertain upload:** The command fails without proving whether npm accepted its write.
- **Observation window:** The time during which the release checks public registry visibility.
- **Normal Stable release:** A real Stable bump, excluding dry runs, desktop validation, and existing-tag backfills.

## Requirements

### REQ-RELEASE-NPM-PUBLICATION-001: Bounded SDK registry observation

**Intent:** Registry propagation shall not cause an immediate failure after a valid SDK upload.

#### Acceptance criteria

- **AC-RELEASE-NPM-PUBLICATION-001.1:** After an accepted or uncertain SDK upload, the release shall observe an absent version for five minutes before declaring a visibility timeout.
  It shall stop earlier when the version appears or a definitive error occurs.
- **AC-RELEASE-NPM-PUBLICATION-001.2:** After an SDK channel update, the release shall use the same observation window for a stale dist-tag.
  A matching version that appears after 30 seconds shall remain recoverable within that window.
- **AC-RELEASE-NPM-PUBLICATION-001.3:** Each execution shall attempt the SDK upload at most once.
  An uncertain response shall cause registry reads, without repeating the write.
  The same rule shall apply to a dist-tag update.
- **AC-RELEASE-NPM-PUBLICATION-001.4:** Missing visibility shall never authorize conflicting bytes, invalid provenance, a newer-channel downgrade, or an unverified consumer.
  A failed registry read shall remain an error, rather than evidence of absence.
- **AC-RELEASE-NPM-PUBLICATION-001.5:** A later execution shall verify and reuse an already published matching version.
  It shall resume unfinished channel promotion and public verification without another upload.

### REQ-RELEASE-NPM-PUBLICATION-002: Actionable publication diagnostics

**Intent:** A maintainer shall identify the failed phase and safe recovery action from the job output.

#### Acceptance criteria

- **AC-RELEASE-NPM-PUBLICATION-002.1:** SDK logs shall identify the package, version, phase, observation attempt, elapsed time, and next wait.
  They shall distinguish accepted writes from uncertain writes.
- **AC-RELEASE-NPM-PUBLICATION-002.2:** Errors shall distinguish authentication or authorization rejection, registry read failure, visibility timeout, identity conflict, and consumer verification failure.
  A visibility timeout after command success shall state which write npm accepted.
  A dist-tag failure shall not change an already confirmed publication state.
- **AC-RELEASE-NPM-PUBLICATION-002.3:** Success and failure summaries shall show confirmed publication, channel promotion, the channel write outcome, and consumer verification states.
  Failure summaries shall identify the last confirmed phase and the next recovery action.
  Unknown publication state shall remain explicit.
- **AC-RELEASE-NPM-PUBLICATION-002.4:** Command output and summaries shall exclude credentials, bearer and Basic authorization headers, including Proxy-Authorization, signed request URLs, and raw authentication responses.
  Retained output shall have a finite size limit.
- **AC-RELEASE-NPM-PUBLICATION-002.5:** Recovery guidance shall preserve the existing signed SDK candidate and version.
  Authentication guidance shall name the package's publisher configuration.
  Registry-delay guidance shall direct a maintainer to reuse the same release run after checking public visibility.

### REQ-RELEASE-NPM-PUBLICATION-003: Early Stable authentication checks

**Intent:** An expired or invalid npm publisher shall stop a normal Stable release before release mutations and expensive builds.

#### Acceptance criteria

- **AC-RELEASE-NPM-PUBLICATION-003.1:** Before normal Stable preparation, the release shall exercise package-specific OIDC authentication for the application and every runtime npm package.
  It shall also check the SDK publisher when that package exists.
- **AC-RELEASE-NPM-PUBLICATION-003.2:** Every required authentication check shall pass before release branch creation, PR creation, host tagging, or builds.
  One rejected package among healthy packages shall block normal Stable preparation and publication.
- **AC-RELEASE-NPM-PUBLICATION-003.3:** Dry runs, desktop validation, Nightly, and existing-tag backfills shall skip this new authentication gate.
  Their existing validation, publication, and recovery contracts shall remain effective.
- **AC-RELEASE-NPM-PUBLICATION-003.4:** Only a confirmed whole-package SDK absence shall defer its early authentication check to the existing first-publication procedure.
  This deferral shall not claim ownership, approve bootstrap, or complete SDK publication.
  Registry errors and missing runtime packages shall fail explicitly.
- **AC-RELEASE-NPM-PUBLICATION-003.5:** The gate shall use the public npm registry and the protected GitHub Actions release context.
  Tokens shall remain in process memory.
  The gate shall neither publish packages nor change channels, publisher configuration, or signing state.
- **AC-RELEASE-NPM-PUBLICATION-003.6:** The authentication result shall identify each checked or deferred package.
  Authentication errors shall provide publisher-configuration recovery guidance without exposing response credentials.
  A successful exchange shall not claim to prove direct-publish or dist-tag permissions.
- **AC-RELEASE-NPM-PUBLICATION-003.7:** A successful package-scoped exchange with a nonempty token string shall pass without response metadata.
  Missing or unexpected `token_type`, `created`, and `expires` metadata shall not block authentication.
  The result shall not claim to verify the issued token's lifetime.
  GitHub identity-token expiry validation and registry authentication rejection shall remain effective.

## Out of scope

- Changes to SDK versions, contracts, package bytes, provenance policy, or retained evidence.
- Runtime and Nightly publication retry policies.
- New npm credentials, token fallbacks, or automatic publisher renewal.
- New scheduled monitoring or automatic workflow dispatch.
- Changes to contributor notices, Discord recovery, or platform signing policy.
- Product UI, mobile layouts, and browser tests.

## Implementation plan

- [npm publication reliability](../../../plans/npm-publication-reliability/plan.md).
- [Preflight response diagnostics repair](../../../plans/npm-preflight-response-diagnostics/plan.md).
- [Creation metadata compatibility repair](../../../plans/npm-preflight-created-metadata/plan.md).
- [Exchange response contract repair](../../../plans/npm-preflight-exchange-contract/plan.md).
