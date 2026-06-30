import { beforeEach, describe, expect, it } from "vitest";
import {
  queueTaskCreateLastUsedFromPayload,
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

  it("clears dependent queued fields with explicit null values", () => {
    syncTaskCreateLastUsed({ repository_id: "repo-1", branch: "feature" });

    syncTaskCreateLastUsed({ repository_id: "repo-2", branch: null });

    expect(readQueuedTaskCreateLastUsedState()).toMatchObject({
      repositoryId: "repo-2",
      branch: null,
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

describe("queueTaskCreateLastUsedFromPayload", () => {
  beforeEach(() => {
    window.localStorage.clear();
    resetTaskCreateLastUsedSync({ clearQueued: true });
  });

  it("leaves the queued overlay unchanged for null or undefined payloads", () => {
    syncTaskCreateLastUsed({ branch: "feature" });

    queueTaskCreateLastUsedFromPayload(undefined);
    queueTaskCreateLastUsedFromPayload(null);

    expect(readQueuedTaskCreateLastUsedState()).toEqual({ branch: "feature" });
  });

  it("compacts empty repository payloads to profile selections only", () => {
    queueTaskCreateLastUsedFromPayload({
      repositories: [],
      agent_profile_id: "agent-1",
      executor_profile_id: "exec-1",
    });

    expect(readQueuedTaskCreateLastUsedState()).toEqual({
      agentProfileId: "agent-1",
      executorProfileId: "exec-1",
    });
  });

  it("uses the first workspace repository and skips rows without repository ids", () => {
    queueTaskCreateLastUsedFromPayload({
      repositories: [
        { checkout_branch: "ignored-local" },
        { repository_id: "repo-2", checkout_branch: "feature" },
      ],
    });

    expect(readQueuedTaskCreateLastUsedState()).toEqual({
      repositoryId: "repo-2",
      branch: "feature",
    });
  });

  it("prefers the base branch for fresh-branch repository payloads", () => {
    queueTaskCreateLastUsedFromPayload({
      repositories: [
        {
          repository_id: "repo-1",
          base_branch: "main",
          checkout_branch: "feature",
          fresh_branch: true,
        },
      ],
    });

    expect(readQueuedTaskCreateLastUsedState()).toEqual({
      repositoryId: "repo-1",
      branch: "main",
    });
  });

  it("falls back to checkout branch for fresh-branch payloads without a base branch", () => {
    queueTaskCreateLastUsedFromPayload({
      repositories: [
        {
          repository_id: "repo-1",
          base_branch: "",
          checkout_branch: "feature",
          fresh_branch: true,
        },
      ],
    });

    expect(readQueuedTaskCreateLastUsedState()).toEqual({
      repositoryId: "repo-1",
      branch: "feature",
    });
  });

  it("prefers the checkout branch for normal repository payloads", () => {
    queueTaskCreateLastUsedFromPayload({
      repositories: [
        {
          repository_id: "repo-1",
          base_branch: "main",
          checkout_branch: "feature",
        },
      ],
    });

    expect(readQueuedTaskCreateLastUsedState()).toEqual({
      repositoryId: "repo-1",
      branch: "feature",
    });
  });

  it("falls back to base branch for normal payloads without a checkout branch", () => {
    queueTaskCreateLastUsedFromPayload({
      repositories: [
        {
          repository_id: "repo-1",
          base_branch: "main",
        },
      ],
    });

    expect(readQueuedTaskCreateLastUsedState()).toEqual({
      repositoryId: "repo-1",
      branch: "main",
    });
  });
});
