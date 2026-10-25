---
status: draft
system: office
created: 2026-05-02
updated: 2026-10-06
owners:
  - cfl
---
# Office Live Updates Requirements

## Overview

Office live updates make concurrent agent work visible through workspace-scoped events and reads. Mounted diagnostic consumers also need to read the newly selected workspace without relying on an unrelated event or a page reload. Provider-health reads must preserve live health observed while those reads are outstanding. This document owns that read and publication lifecycle; execution routing policy belongs to the agent system.

## Requirements

### REQ-OFFICE-LIVE-UPDATES-001: Office Live Updates

**Intent:** Users can observe concurrent Office work, comments, and live presence without manually reloading each surface.

#### Acceptance criteria

- **AC-OFFICE-LIVE-UPDATES-001.1:** A pulsing blue dot (`animate-pulse`).
- **AC-OFFICE-LIVE-UPDATES-001.2:** A small text badge with the active-session count (e.g. `2 live`).
- **AC-OFFICE-LIVE-UPDATES-001.3:** `office.task.created`, `office.task.updated`, `office.task.moved`, `office.task.status_changed` cause refetch / re-render of: `Recent Tasks`, `Tasks In Progress`, the `Run Activity` chart, and the `Recent Activity` feed.
- **AC-OFFICE-LIVE-UPDATES-001.4:** `office.agent.completed` and `office.agent.failed` update the `Agents Enabled` card subtitle (running / paused / errors line).
- **AC-OFFICE-LIVE-UPDATES-001.5:** `session.state_changed`, `office.task.updated`, `office.agent.updated` cause the per-agent cards panel to refetch `GET /api/v1/office/workspaces/:wsId/agent-summaries` and replace its state. No optimistic updates - the server is the source of truth and the response is small (N agents x <=5 sessions each).
- **AC-OFFICE-LIVE-UPDATES-001.6:** The task page header shows a small `<IconLoader2 animate-spin /> Working` indicator next to the task title. Clicking it scrolls the timeline so the active session entry is visible. Hidden when no active session, with no layout reservation.
- **AC-OFFICE-LIVE-UPDATES-001.7:** An **inline session entry** appears at its chronological position in the comments timeline (one entry per session for the task, ordered by `session.startedAt`):
- **AC-OFFICE-LIVE-UPDATES-001.8:** Active session entry is expanded by default. Header reads `RUNNING * Working * for {elapsed} * ran {N} commands`. Body embeds `<AdvancedChatPanel taskId sessionId hideInput />`. `{N}` is derived from `messages.bySession[sessionId]` filtered for `type === "tool_call"`.

### REQ-OFFICE-LIVE-UPDATES-002: Current-workspace diagnostic reads

**Intent:** Existing Office routing-preview and provider-health consumers read the selected workspace and expose only request state belonging to their current selection. Provider-health consumers also retain live updates across outstanding snapshot reads. This applies to the diagnostic surfaces that remain mounted during the routing migration; it does not reinstate the deprecated Office routing policy.

#### Acceptance criteria

- **AC-OFFICE-LIVE-UPDATES-002.1:** When a diagnostic consumer mounts with a non-empty workspace, or its selected workspace changes to another non-empty workspace, it shall automatically read that workspace, including a return to a previously selected workspace. An earlier successful read or an existing stored result shall not suppress the read. Ordinary rerenders shall not cause additional reads.
- **AC-OFFICE-LIVE-UPDATES-002.2:** With no selected workspace, the consumer shall expose empty data, no loading, and no error, and shall make no automatic or explicit read.
- **AC-OFFICE-LIVE-UPDATES-002.3:** On selection change, the consumer shall use only the newly selected workspace's stored diagnostic data and request state. It shall clear the previous selection's error and reflect the new read's loading state. A cached result for the selected workspace may remain visible while refreshing.
- **AC-OFFICE-LIVE-UPDATES-002.4:** A read invalidated by selection change or consumer unmount shall not publish diagnostic data, error, or loading completion. Returning to the same workspace shall not make an earlier invalidated read valid again.
- **AC-OFFICE-LIVE-UPDATES-002.5:** For overlapping reads initiated by one consumer within the same selection, only its newest admitted read shall publish data or settle its error/loading state. A failed newest read shall not allow an older read to overwrite the retained result.
- **AC-OFFICE-LIVE-UPDATES-002.6:** A current read failure shall retain the selected workspace's last accepted data and settle loading with its error. Ordinary rerenders shall not retry automatically; explicit refresh shall read the current workspace and can recover. An empty successful response shall be a completed read.
- **AC-OFFICE-LIVE-UPDATES-002.7:** Each mounted consumer shall independently initiate its required reads and own its request state. Another consumer's completion or unmount shall not suppress its reads or reset its request state. Separate application stores shall remain isolated. This criterion does not impose response ordering between separate consumers writing the same workspace entry.
- **AC-OFFICE-LIVE-UPDATES-002.8:** Desktop and phone consumers shall use the same selection, publication, failure, and refresh semantics through their existing surfaces.
- **AC-OFFICE-LIVE-UPDATES-002.9:** When a provider-health snapshot read is outstanding and the consumer observes a live health update for its workspace, publishing that older snapshot shall retain the currently observed update for the same provider, scope, and scope value. This shall apply to automatic and explicit reads, multiple updates to one key, newly observed keys, and updated keys omitted from the snapshot, including an empty snapshot.
- **AC-OFFICE-LIVE-UPDATES-002.10:** Preserving an update during a provider-health read shall not prevent the same response from hydrating unrelated new or unchanged keys, or removing unchanged keys omitted from that response. A read started after an update shall remain able to replace that earlier health or complete with empty health when no later update intervenes. Ordinary successful reads, explicit refresh, and live events observed after publication shall remain usable under criteria `.1` through `.8`.

## Diagnostic exclusions

- Changes to routing policy, dynamic profiles, provider health classification, launch decisions, or the deprecated Office routing contract.
- Shared request coordination, global cache policy, cross-consumer latest-response ordering, routing-preview HTTP-versus-live-event arbitration, polling, or automatic failure retries. Provider-health publication is bounded by `.9` and `.10`; it does not promise universal server chronology, missed-event replay, or permanent live-row precedence over later reads.
- New routes, controls, layout, copy, or persistence.

## System design

The migrated technical source is split into [part 1](../system-design/live-updates-01.md), [part 2](../system-design/live-updates-02.md).

Current-workspace diagnostic reads are defined in [part 2](../system-design/live-updates-02.md#current-workspace-diagnostic-reads).

Provider-health snapshot publication is defined in [part 2](../system-design/live-updates-02.md#provider-health-snapshot-publication).
