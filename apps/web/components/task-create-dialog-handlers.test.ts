import { beforeEach, describe, expect, it } from "vitest";
import {
  readPendingTaskCreateLastUsedState,
  readQueuedTaskCreateLastUsedState,
  resetTaskCreateLastUsedSync,
  syncTaskCreateLastUsed,
} from "./task-create-dialog-handlers";

const PENDING_LAST_USED_SYNC_KEY = "kandev.taskCreateLastUsed.pendingSync";

describe("syncTaskCreateLastUsed", () => {
  beforeEach(() => {
    window.localStorage.clear();
    resetTaskCreateLastUsedSync({ clearQueued: true });
  });

  it("queues selector changes locally without writing a backend settings patch", () => {
    syncTaskCreateLastUsed({ branch: "feature" });

    expect(readQueuedTaskCreateLastUsedState()).toMatchObject({
      branch: "feature",
    });
    expect(readPendingTaskCreateLastUsedState()).toEqual({});
    expect(window.localStorage.getItem(PENDING_LAST_USED_SYNC_KEY)).toBeNull();
  });

  it("retains prior queued fields after a later selector change", () => {
    syncTaskCreateLastUsed({ branch: "feature" });
    syncTaskCreateLastUsed({ agent_profile_id: "agent-2" });

    expect(readQueuedTaskCreateLastUsedState()).toMatchObject({
      branch: "feature",
      agentProfileId: "agent-2",
    });
  });

  it("keeps queued fields when dialog close resets transient state", () => {
    syncTaskCreateLastUsed({ branch: "feature" });

    resetTaskCreateLastUsedSync();

    expect(readQueuedTaskCreateLastUsedState()).toMatchObject({
      branch: "feature",
    });
    expect(readQueuedTaskCreateLastUsedState().agentProfileId).toBeUndefined();
  });
});
