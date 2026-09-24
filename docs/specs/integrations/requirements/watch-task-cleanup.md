---
status: draft
system: integrations
created: 2026-09-22
owners:
  - kandev
---

# Watch task cleanup

## Overview

Integration cleanup must preserve provider capacity for active work. Historical
tasks must not generate repeated provider requests. Integrations own this
contract because they select cleanup candidates and request provider state.

## Requirements

### REQ-INTEGRATIONS-WATCH-CLEANUP-001: Cleanup eligibility

- **AC-INTEGRATIONS-WATCH-CLEANUP-001.1:** Every archived or deleted task shall generate zero provider state requests during a cleanup sweep. This applies to GitHub reviews and issues, GitLab reviews and issues, and Azure DevOps reviews and work items.
- **AC-INTEGRATIONS-WATCH-CLEANUP-001.2:** In a collection containing active, archived, deleted, and unassigned records, each existing unarchived task shall remain eligible independently. Eligibility shall not depend on another record's state.
- **AC-INTEGRATIONS-WATCH-CLEANUP-001.3:** GitHub reservations without an assigned task shall remain eligible. Terminal reservations shall be removed under `auto` and `always`, without increasing the deleted-task count. `never` shall preserve them without provider requests.
- **AC-INTEGRATIONS-WATCH-CLEANUP-001.4:** Existing policies shall remain effective for active tasks. `never` shall make no cleanup state requests. `always` shall delete terminal tasks. `auto` shall preserve user-engaged tasks and existing lifecycle-prompt protections.
- **AC-INTEGRATIONS-WATCH-CLEANUP-001.5:** Background and manual cleanup shall use the same task eligibility rules. Watch reset, explicit watch deletion, workspace authorization, and deduplication shall retain their existing behavior.

- **AC-INTEGRATIONS-WATCH-CLEANUP-001.6:** Review cleanup shall request only PR state and, for open PRs, review and viewer identity evidence. It shall not request comments, checks, workflow runs, or workflow jobs.

### REQ-INTEGRATIONS-WATCH-CLEANUP-002: GitHub rate-limit termination

- **AC-INTEGRATIONS-WATCH-CLEANUP-002.1:** After a GitHub cleanup batch receives a recognized rate-limit error, it shall start no further row checks. Completed deletions shall remain counted. Unprocessed tasks and reservations shall remain intact.
- **AC-INTEGRATIONS-WATCH-CLEANUP-002.2:** Rate-limit recognition shall include HTTP 429 and HTTP 403 responses indicating rate limits or abuse detection. An unrelated HTTP 403 shall not be classified as a rate limit.
- **AC-INTEGRATIONS-WATCH-CLEANUP-002.3:** A cleanup sweep shall return its rate-limit error to its caller. That poll cycle shall skip its remaining cleanup calls after this error. Ordinary row errors shall preserve the row and retain existing continuation behavior.
- **AC-INTEGRATIONS-WATCH-CLEANUP-002.4:** A later scheduled sweep shall remain able to retry. Cancellation shall stop remaining batch work. Requests already running within one feedback fetch may finish or be cancelled.

## Exclusions

No cleanup policy, task retention setting, UI control, or provider credential
contract is added. Unarchived tasks on a Done step remain active candidates.
This contract does not guarantee spare quota against unrelated API consumers.
GitLab and Azure DevOps reservation semantics remain unchanged.
