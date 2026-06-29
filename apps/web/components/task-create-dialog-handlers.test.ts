import { beforeEach, describe, expect, it } from "vitest";
import {
  readQueuedTaskCreateLastUsedState,
  resetTaskCreateLastUsedSync,
  syncTaskCreateLastUsed,
} from "./task-create-dialog-handlers";

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
  });

  it("retains prior queued fields after a later selector change", () => {
    syncTaskCreateLastUsed({ branch: "feature" });
    syncTaskCreateLastUsed({ agent_profile_id: "agent-2" });

    expect(readQueuedTaskCreateLastUsedState()).toMatchObject({
      branch: "feature",
      agentProfileId: "agent-2",
    });
  });

  it("clears queued fields when dialog close resets canceled selections", () => {
    syncTaskCreateLastUsed({ branch: "feature" });

    resetTaskCreateLastUsedSync({ clearQueued: true });

    expect(readQueuedTaskCreateLastUsedState()).toEqual({});
  });

  it("keeps queued fields when create-time close preserves pending backend writes", () => {
    syncTaskCreateLastUsed({ branch: "feature" });

    resetTaskCreateLastUsedSync();

    expect(readQueuedTaskCreateLastUsedState()).toMatchObject({
      branch: "feature",
    });
  });

  it("keeps queued fields when preserved settings have not caught up", () => {
    syncTaskCreateLastUsed({ branch: "feature" });

    resetTaskCreateLastUsedSync({
      syncedSettings: {
        repositoryId: null,
        branch: "main",
        agentProfileId: null,
        executorProfileId: null,
      },
    });

    expect(readQueuedTaskCreateLastUsedState()).toMatchObject({
      branch: "feature",
    });
  });

  it("clears queued fields when preserved settings already match", () => {
    syncTaskCreateLastUsed({ branch: "feature" });

    resetTaskCreateLastUsedSync({
      syncedSettings: {
        repositoryId: null,
        branch: "feature",
        agentProfileId: null,
        executorProfileId: null,
      },
    });

    expect(readQueuedTaskCreateLastUsedState()).toEqual({});
  });
});
