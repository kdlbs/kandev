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

const authoritative: QueueMeta = {
  count: 0,
  max: 10,
  mergeEnabled: true,
  autoRun: true,
  sessionIncarnationId: "inc",
  statusEpoch: "boot",
  statusGeneration: 1,
  autoMergeAvailable: true,
  autoMergeEnabled: false,
  autoMergeSource: "session",
  autoMergeRevision: 3,
};

describe("Auto-merge policy ordering", () => {
  it.each([
    { autoMergeSource: "global" as const, autoMergeRevision: 99 },
    { autoMergeSource: "session" as const, autoMergeRevision: 2 },
  ])("keeps the authoritative session value against a stale tuple: %j", (stale) => {
    const store = queueStore();
    store.getState().setQueueEntries("session", [], authoritative);
    store.getState().setQueueEntries("session", [], {
      ...authoritative,
      statusGeneration: 2,
      autoMergeEnabled: true,
      ...stale,
    });
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoMergeEnabled: false,
      autoMergeSource: "session",
      autoMergeRevision: 3,
    });
  });

  it("applies a newer same-source revision", () => {
    const store = queueStore();
    store.getState().setQueueEntries("session", [], authoritative);
    store.getState().setQueueEntries("session", [], {
      ...authoritative,
      statusGeneration: 2,
      autoMergeEnabled: true,
      autoMergeRevision: 4,
    });
    expect(store.getState().queue.metaBySessionId.session).toMatchObject({
      autoMergeEnabled: true,
      autoMergeRevision: 4,
    });
  });
});
