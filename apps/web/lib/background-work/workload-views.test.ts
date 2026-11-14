import { describe, expect, it } from "vitest";
import type {
  RunState,
  WorkloadCapabilities,
  WorkloadRunObservation,
} from "@/lib/types/background-work";
import {
  filterWorkloadsByQuery,
  resolveInitialBackgroundWorkTab,
  selectActiveWorkloads,
  selectHistoryWorkloads,
  selectRunningWorkloads,
  selectWorkloadsForTab,
  uniqueWorkloads,
} from "./workload-views";

const CAPABILITIES: WorkloadCapabilities = {
  discovery: "snapshot",
  output: "stream",
  transcript: false,
  parentage: false,
  reasoning_summary: false,
  attributable_usage: false,
};

function workload(
  workId: string,
  state: RunState,
  overrides: Partial<WorkloadRunObservation> = {},
): WorkloadRunObservation {
  return {
    work_id: workId,
    title: overrides.title ?? workId,
    kind: overrides.kind ?? "shell",
    state,
    capabilities: CAPABILITIES,
    revision: 1,
    ...overrides,
  };
}

describe("workload view selectors", () => {
  it.each<RunState>([
    "running",
    "waiting",
    "completed",
    "failed",
    "interrupted",
    "ended",
    "unknown",
  ])("classifies %s into exactly one shared view", (state) => {
    const rows = [workload("work-1", state)];
    const running = selectRunningWorkloads(rows).length;
    const active = selectActiveWorkloads(rows).length;
    const history = selectHistoryWorkloads(rows).length;

    // Every accepted record belongs to exactly one tab, and only running
    // records enter the composer summary.
    expect(active + history).toBe(1);
    expect(active).toBe(state === "running" || state === "waiting" ? 1 : 0);
    expect(running).toBe(state === "running" ? 1 : 0);
    expect(history).toBe(active === 1 ? 0 : 1);
  });

  it("keeps running-only summaries independent from waiting and unknown work", () => {
    const rows = [
      workload("running-shell", "running", { kind: "shell" }),
      workload("running-agent", "running", { kind: "subagent" }),
      workload("waiting-agent", "waiting", { kind: "subagent" }),
      workload("unknown-shell", "unknown", { kind: "shell" }),
      workload("completed-shell", "completed", { kind: "shell" }),
    ];

    expect(selectRunningWorkloads(rows).map((row) => row.work_id)).toEqual([
      "running-shell",
      "running-agent",
    ]);
    expect(selectActiveWorkloads(rows).map((row) => row.work_id)).toEqual([
      "running-shell",
      "running-agent",
      "waiting-agent",
    ]);
    expect(selectHistoryWorkloads(rows).map((row) => row.work_id)).toEqual([
      "unknown-shell",
      "completed-shell",
    ]);
  });

  it("never counts the same accepted row twice and prefers the newest revision", () => {
    const rows = [
      workload("work-1", "running", { revision: 1, title: "old title" }),
      workload("work-1", "completed", { revision: 2, title: "new title" }),
      workload("work-2", "running"),
    ];

    const unique = uniqueWorkloads(rows);
    expect(unique.map((row) => [row.work_id, row.revision, row.title])).toEqual([
      ["work-1", 2, "new title"],
      ["work-2", 1, "work-2"],
    ]);
    expect(selectRunningWorkloads(rows)).toHaveLength(1);
    expect(selectHistoryWorkloads(rows)).toHaveLength(1);
  });

  it("handles empty and unknown-only lists without inventing active work", () => {
    expect(selectRunningWorkloads([])).toEqual([]);
    expect(selectActiveWorkloads([])).toEqual([]);
    expect(selectHistoryWorkloads([])).toEqual([]);
    expect(resolveInitialBackgroundWorkTab([])).toBe("history");

    const unknownOnly = [workload("unknown", "unknown")];
    expect(selectRunningWorkloads(unknownOnly)).toEqual([]);
    expect(selectActiveWorkloads(unknownOnly)).toEqual([]);
    expect(selectHistoryWorkloads(unknownOnly)).toHaveLength(1);
    expect(resolveInitialBackgroundWorkTab(unknownOnly)).toBe("history");
  });

  it("selects Active when running or waiting work exists, History otherwise", () => {
    expect(resolveInitialBackgroundWorkTab([workload("running", "running")])).toBe("active");
    expect(resolveInitialBackgroundWorkTab([workload("waiting", "waiting")])).toBe("active");
    expect(resolveInitialBackgroundWorkTab([workload("completed", "completed")])).toBe("history");
    expect(resolveInitialBackgroundWorkTab([workload("unknown", "unknown")])).toBe("history");
    expect(
      selectWorkloadsForTab(
        [workload("running", "running"), workload("completed", "completed")],
        "history",
      ).map((row) => row.work_id),
    ).toEqual(["completed"]);
  });

  it("filters the selected tab by title or kind without changing order", () => {
    const rows = [
      workload("build", "running", { title: "Build watcher", kind: "shell" }),
      workload("review", "running", { title: "Review auth", kind: "subagent" }),
    ];

    expect(filterWorkloadsByQuery(rows, "  BUILD  ").map((row) => row.work_id)).toEqual(["build"]);
    expect(filterWorkloadsByQuery(rows, "subagent").map((row) => row.work_id)).toEqual(["review"]);
    expect(filterWorkloadsByQuery(rows, "")).toEqual(rows);
    expect(filterWorkloadsByQuery(rows, "missing")).toEqual([]);
  });
});
