/* eslint-disable sonarjs/no-duplicate-string -- Repeated wire fixtures assert identity across freshness states. */
import { describe, expect, it } from "vitest";
import { buildLaunchQueueViewModel, launchQueueCapacityCause } from "./launch-queue-view-model";

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

describe("launchQueueCapacityCause", () => {
  const queue = (capacityReason: string, gradeInUse?: Record<string, number>) =>
    buildLaunchQueueViewModel(
      {
        queued_at: queuedAt,
        reason: "session_capacity",
        retrying: true,
        capacity_reason: capacityReason,
        capacity: {
          policy: "all",
          in_use: 13,
          limit: 14,
          observed_at: queuedAt,
          ...(gradeInUse
            ? { grade_in_use: gradeInUse, grade_limits: { control: 4, dev: 10 } }
            : {}),
        },
      },
      { now: Date.parse(queuedAt) },
    )!;

  it("names the one full grade and its usage for a grade refusal", () => {
    expect(launchQueueCapacityCause(queue("ceiling_grade_limit", { control: 4, dev: 9 }))).toEqual({
      key: "task:launchQueueCauseGrade",
      values: { grade: "control", inUse: 4, limit: 4 },
    });
  });

  it("prefers the narrower grade limit when both limits refused", () => {
    expect(
      launchQueueCapacityCause(queue("ceiling_both_limits", { control: 4, dev: 9 }))?.key,
    ).toBe("task:launchQueueCauseGrade");
  });

  it("says the grade limit without numbers when the payload has no grade usage", () => {
    expect(launchQueueCapacityCause(queue("ceiling_grade_limit"))).toEqual({
      key: "task:launchQueueCauseGradeUnmeasured",
    });
    expect(launchQueueCapacityCause(queue("ceiling_grade_limit", { control: 4, dev: 10 }))).toEqual(
      { key: "task:launchQueueCauseGradeUnmeasured" },
    );
  });

  it("names the overall limit for a total refusal and nothing for other reasons", () => {
    expect(launchQueueCapacityCause(queue("ceiling_total_limit"))).toEqual({
      key: "task:launchQueueCauseTotal",
      values: { inUse: 13, limit: 14 },
    });
    expect(launchQueueCapacityCause(queue("ceiling_needs_configuration"))).toBeNull();
  });
});
