/* eslint-disable sonarjs/no-duplicate-string -- Repeated wire fixtures assert identity across freshness states. */
import { describe, expect, it } from "vitest";
import { buildLaunchQueueViewModel } from "./launch-queue-view-model";

const queuedAt = "2026-09-16T20:15:44Z";

describe("buildLaunchQueueViewModel", () => {
  it("keeps the exact destination and capacity sample without adding queue position", () => {
    expect(
      buildLaunchQueueViewModel(
        {
          session_id: "luna-session",
          agent_profile_id: "luna-profile",
          workflow_step_id: "implement",
          queued_at: queuedAt,
          reason: "session_capacity",
          retrying: true,
          capacity: { in_use: 5, limit: 5, observed_at: queuedAt },
        },
        { now: Date.parse(queuedAt) + 1_000 },
      ),
    ).toEqual({
      destinationId: "luna-profile",
      queuedAt,
      reason: "session_capacity",
      retrying: true,
      capacityFreshness: "current",
      capacity: { inUse: 5, limit: 5, observedAt: queuedAt },
    });
  });

  it("marks capacity data stale after two sweep intervals", () => {
    expect(
      buildLaunchQueueViewModel(
        {
          session_id: "luna-session",
          queued_at: queuedAt,
          reason: "session_capacity",
          retrying: true,
          capacity: { in_use: 5, limit: 5, observed_at: queuedAt },
        },
        { now: Date.parse(queuedAt) + 40_001 },
      ),
    ).toMatchObject({ capacityFreshness: "stale" });
  });

  it("treats the exact freshness boundary as stale", () => {
    expect(
      buildLaunchQueueViewModel(
        {
          session_id: "luna-session",
          queued_at: queuedAt,
          reason: "session_capacity",
          retrying: true,
          capacity: { in_use: 5, limit: 5, observed_at: queuedAt },
        },
        { now: Date.parse(queuedAt) + 40_000 },
      ),
    ).toMatchObject({ capacityFreshness: "stale" });
  });

  it("marks known capacity stale while the client is disconnected", () => {
    expect(
      buildLaunchQueueViewModel(
        {
          session_id: "luna-session",
          queued_at: queuedAt,
          reason: "session_capacity",
          retrying: true,
          capacity: { in_use: 5, limit: 5, observed_at: queuedAt },
        },
        { now: Date.parse(queuedAt), isConnected: false },
      ),
    ).toMatchObject({ capacityFreshness: "stale" });
  });

  it("preserves an unknown capacity instead of inventing an estimate", () => {
    expect(
      buildLaunchQueueViewModel({
        session_id: "luna-session",
        queued_at: queuedAt,
        reason: "ownership_unavailable",
        retrying: true,
      }),
    ).toMatchObject({
      destinationId: null,
      capacityFreshness: "unavailable",
      capacity: null,
    });
  });

  it("rejects incomplete or unknown wire values", () => {
    expect(buildLaunchQueueViewModel(undefined)).toBeNull();
    expect(
      buildLaunchQueueViewModel({
        queued_at: queuedAt,
        reason: "not-a-reason" as "replay_error",
        retrying: true,
      }),
    ).toBeNull();
  });
});
