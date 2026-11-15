import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/http";
import type { QueueMeta } from "./types";

function queueStore() {
  const store = createAppStore();
  store.getState().setTaskSession({
    id: sessionId("session"),
    task_id: taskId("task"),
    state: "RUNNING",
    started_at: "2026-11-05T00:00:00Z",
    updated_at: "2026-11-05T00:00:00Z",
    queue_incarnation_id: "inc",
  });
  return store;
}
const initial: QueueMeta = {
  count: 0,
  max: 10,
  mergeEnabled: true,
  autoRun: true,
  sessionIncarnationId: "inc",
  statusEpoch: "boot",
  statusGeneration: 1,
  autoMergeAvailable: true,
  autoMergeEnabled: true,
  autoMergeSource: "session",
  autoMergeRevision: 3,
  autoSortMergeAvailable: true,
  autoSortMergeEnabled: true,
  autoSortMergeSource: "session",
  autoSortMergeRevision: 4,
};

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.15
describe("independent queue automation policy ordering", () => {
  it.each([
    { autoSortMergeSource: "global" as const, autoSortMergeRevision: 99 },
    { autoSortMergeSource: "session" as const, autoSortMergeRevision: 2 },
  ])("keeps authoritative sort policy while allowing a newer merge update: %j", (stale) => {
    const store = queueStore();
    store.getState().setQueueEntries("session", [], initial);
    store.getState().setQueueEntries("session", [], {
      ...initial,
      statusGeneration: 2,
      autoMergeEnabled: false,
      autoMergeRevision: 5,
      autoSortMergeEnabled: false,
      ...stale,
    });
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoMergeEnabled: false,
      autoMergeRevision: 5,
      autoSortMergeEnabled: true,
      autoSortMergeRevision: 4,
      autoSortMergeSource: "session",
    });
  });

  it("preserves a newer sort update when merge falls back to inherited", () => {
    const store = queueStore();
    store.getState().setQueueEntries("session", [], initial);
    store.getState().setQueueEntries("session", [], {
      ...initial,
      statusGeneration: 2,
      autoMergeEnabled: false,
      autoMergeSource: "global",
      autoSortMergeEnabled: false,
      autoSortMergeRevision: 5,
    });
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoMergeEnabled: true,
      autoMergeSource: "session",
      autoMergeRevision: 3,
      autoSortMergeEnabled: false,
      autoSortMergeRevision: 5,
    });
  });

  it("disables unavailable sort without inventing a preference and rejects its delayed status", () => {
    const store = queueStore();
    store.getState().setQueueEntries("session", [], initial);
    store.getState().setQueueEntries("session", [], {
      count: 0,
      max: 10,
      mergeEnabled: true,
      autoRun: true,
      sessionIncarnationId: "inc",
      statusEpoch: "boot",
      statusGeneration: 2,
      autoSortMergeAvailable: false,
    });
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoSortMergeAvailable: false,
      autoSortMergeEnabled: true,
      autoSortMergeRevision: 4,
    });
    store.getState().setQueueEntries("session", [], initial);
    expect(store.getState().queue.metaBySessionId.session?.autoSortMergeAvailable).toBe(false);
  });
});
