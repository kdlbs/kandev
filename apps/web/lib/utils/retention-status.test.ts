import { describe, expect, it } from "vitest";
import type { RetentionSettings, RetentionStatus } from "@/lib/types/system";
import { buildRetentionStatusViewModel } from "./retention-status";

const settings: RetentionSettings = {
  enabled: true,
  sweep_interval_hours: 6,
  batch_limit: 5000,
  routine_runs: { window_days: 30, floor_per_owner: 50, warn_rows: 10 },
  runs: { window_days: 30, floor_per_owner: 50, warn_rows: 10 },
  run_events: { warn_rows: 10 },
};
const AS_OF = "2026-09-15T10:00:00Z";

function status(overrides: Partial<RetentionStatus> = {}): RetentionStatus {
  return {
    settings,
    last_sweep: null,
    skip_count: 0,
    retained_counts: {
      office_routine_runs: { state: "not_computed", retained_count: 0, as_of: "" },
      runs: { state: "not_computed", retained_count: 0, as_of: "" },
      run_events: { state: "not_computed", retained_count: 0, as_of: "" },
    },
    ...overrides,
  };
}

describe("buildRetentionStatusViewModel", () => {
  it("keeps unavailable counts distinct from zero and compares thresholds strictly", () => {
    const model = buildRetentionStatusViewModel(
      status({
        retained_counts: {
          office_routine_runs: {
            state: "fresh",
            retained_count: 10,
            as_of: AS_OF,
          },
          runs: { state: "fresh", retained_count: 11, as_of: AS_OF },
          run_events: { state: "fresh", retained_count: 0, as_of: AS_OF },
        },
      }),
    );

    expect(model.counts.map((count) => [count.state, count.overThreshold])).toEqual([
      ["measured", false],
      ["measured", true],
      ["zero", false],
    ]);
    expect(model.commonAsOf).toBe(AS_OF);
  });

  it("sums committed records and preserves mixed preview and deletion state", () => {
    const model = buildRetentionStatusViewModel(
      status({
        last_sweep: {
          started_at: "2026-09-15T09:00:00Z",
          finished_at: "2026-09-15T09:00:05Z",
          office_routine_runs: {
            deleted: 4,
            backlog: true,
            error: "",
            previewed: false,
            would_delete: 0,
          },
          runs: { deleted: 0, backlog: false, error: "", previewed: true, would_delete: 8 },
          run_events: { deleted: 2, backlog: false, error: "" },
          route_attempts: { deleted: 1, backlog: false, error: "" },
          run_skills: { deleted: 3, backlog: false, error: "" },
        },
      }),
    );

    expect(model.sweep).toMatchObject({
      outcome: "mixed",
      deletionTotal: 10,
      hasDeletion: true,
      hasPreview: true,
      hasBacklog: true,
      hasError: false,
    });
  });
});
