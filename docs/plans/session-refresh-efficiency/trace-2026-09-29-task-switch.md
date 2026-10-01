# Task-switch trace evidence: 2026-09-29

Source: `Trace-20260929T133830.json.gz`, supplied in this task's attachments.
The selected window is 10.702 seconds. Renderer PID 7032, thread 45897423.
Times below are seconds after trace breadcrumb minimum 428192244786 microseconds.
The recording is production-style bundled JavaScript with React DevTools present.
The running build's exact commit was not established. Compare it with the checkout
before treating a finding as a regression in a specific release.

## Observations

| Observation | Measurement | Interpretation |
| --- | --- | --- |
| Task selection | Click at 1.311 s; EventTiming duration 1,213.5 ms | One recorded slow interaction, not a page-wide INP measurement |
| Main-thread stall | RunTask at 1.311 s lasts 1,147.5 ms | Browser cannot service ordinary input while it runs |
| Continuing congestion | 24 RunTask events above 50 ms after excluding startup | Switch cost continues after the click handler |
| Main-thread occupancy | 7,116 ms of RunTask; about 6,841 ms excluding startup | Approximately 64% of recording, still including extensions |
| Editor work | About 213 ms sampled beneath Tiptap createEditor in the click window | Confirmed expensive editor initialization; owning component unresolved |
| Loading view | Present at 6.70 s; destination visible at 7.35 s | Loading persists about 5.4 s after selection; usable-input time not measured |
| PR feedback | Three identical outstanding GETs begin at 2.198, 2.264, 3.227 s | Duplicate same-scope work; each decoded body is 75,517 bytes |
| PR header waits | About 6.60, 6.50, 5.50 s | Real request/server/upstream delay; not solely renderer congestion |
| Agent discovery | Three GETs at 2.189, 2.190, 2.194 s, each 14,898 decoded bytes | Same listAgentsUntilSettled caller across independent hook consumers |
| Task identity | Three destination GETs at 1.957, 1.958, 2.186 s | Route loader and task-surface readers fetch independently |
| Hydration fan-out | At 5.002 s: workflow, agents, repositories, workspaces, workflows, turns, settings, terminals, messages, session | Route enrichment follows required task/session reads |
| Renderer counters | Heap 136 MB initially, 825 MB peak, 396 MB final; nodes 48,583 to 88,201 | Allocation/retention investigation signal, not proof of a leak |

The capture contains 70 renderer ResourceSendRequest events, including legitimate
plugin, avatar, reconciliation, and refresh requests. Do not treat all as duplicates.
A fourth PR read starts at 8.999 s; its trigger may be legitimate invalidation.
The source trace's 275 ms startup task includes 271 ms of CpuProfiler startup.
React DevTools has roughly 640 ms of inclusive sampled work across the capture.
The 554 ms task at 6.744 s includes about 184 ms beneath DevTools callbacks.
These sampled inclusive durations overlap other totals and must not be added.

## Source correlation

- `src/task-detail-route.tsx`: `useTaskDetailRouteFetch` awaits
  `fetchSessionDataForTask`; the loading overlay hides the retained task shell.
- `lib/ssr/session-page-state.ts`: required task and session-list reads precede
  optional enrichment. `fetchSessionDataFromTask` awaits a `Promise.all` including
  convenience resources. A five-second deadline limits optional waiting but
  does not make these resources nonblocking. The 5.002 s burst is not evidence
  that this deadline itself expired.
- `components/task/task-page-content.tsx`: its task-detail effect is a second
  identity reader. Request stacks identify it and the route loader independently.
- `hooks/domains/settings/use-settings-data.ts`: loaded flags settle after
  completion; simultaneous mounts can each enter `refreshAgentList`. Its
  per-hook discovery reconciliation ref also does not provide shared ownership.
- `hooks/domains/github/use-pr-feedback.ts` and `use-pr-ci-popover.ts` each call
  `getPRFeedback`. The latter's request counter rejects stale completions but
  does not join requests across consumers. Background sync uses queueMicrotask,
  which does not itself establish shared ownership.
- `hooks/domains/integrations/use-integration-availability.ts`: each active
  consumer owns its own immediate read, 90-second timer, and invalidation
  listener. Repeated Jira/Linear 204s in ten seconds are mount/refresh work,
  not the normal 90-second cadence. Unscoped and workspace-scoped reads are
  distinct; only identical scopes may join.
- Tiptap `createEditor`, `mount`, `createView`, and `updateStateInner` appear
  in sampled stacks. React effect traversal hides the owning component.
  Composer, plan, read-only plan, and task-reference editors are candidates,
  not individually established causes. Existing `immediatelyRender: false`
  does not prove that creation occurs outside the switch's main-thread task.

The destination session request at 1.718 s received network headers after about
227 ms, but the renderer response event appears at 2.541 s and completion at
2.893 s. Its apparent 1.175 s duration therefore includes browser delay.
Conversely, PR `timing.receiveHeadersEnd` shows genuine multi-second header waits.
Backend versus GitHub attribution requires correlated server timings.

## Method and limits

Parse gzip JSON; select the renderer main thread from metadata. Count complete
RunTask spans rather than summing nested FunctionCall/Layout/React events.
Pair requests using requestId. Compute network header arrival from requestTime
plus receiveHeadersEnd, separately from ResourceReceiveResponse/ResourceFinish.
Reconstruct CPU nodes across ProfileChunk events, accumulating timeDeltas and
counting each ancestor frame once per sample. Inspect embedded script sources
and original Screenshot events. Keep raw task IDs, task content, and URLs out of
committed evidence. Counter values include detached and extension-owned objects.

The trace supports priority changes, not a guaranteed speedup or leak diagnosis.
Repeat on a matched production build without extensions, with recorded build,
fixture, viewport, cache state, and plugin set. Then repeat with the user's plugin
set. Preserve earlier navigation/file-tree regression coverage.
