import { describe, it, expect } from "vitest";
import type { StatsResponse } from "@/lib/types/http";
import {
  buildStatsSummary,
  formatDuration,
  getRangeLabel,
  getSubtitle,
  isRangeKey,
  statsReducer,
  toPanelState,
} from "./stats-utils";

const emptyGlobal: StatsResponse["global"] = {
  total_tasks: 0,
  completed_tasks: 0,
  in_progress_tasks: 0,
  total_sessions: 0,
  total_turns: 0,
  total_messages: 0,
  total_user_messages: 0,
  total_tool_calls: 0,
  total_duration_ms: 0,
  avg_turns_per_task: 0,
  avg_messages_per_task: 0,
  avg_duration_ms_per_task: 0,
  avg_turn_duration_ms: 0,
  avg_messages_per_turn: 0,
};

const sampleStats: StatsResponse = {
  global: {
    ...emptyGlobal,
    total_tasks: 10,
    completed_tasks: 4,
    in_progress_tasks: 2,
    total_sessions: 7,
    total_duration_ms: 3_725_000,
    avg_duration_ms_per_task: 372_500,
  },
  task_stats: [],
  daily_activity: [],
  completed_activity: [],
  agent_usage: [],
  repository_stats: [
    {
      repository_id: "r1",
      repository_name: "kandev",
      total_tasks: 6,
      completed_tasks: 3,
      in_progress_tasks: 1,
      session_count: 5,
      turn_count: 30,
      message_count: 60,
      user_message_count: 25,
      tool_call_count: 40,
      total_duration_ms: 1_000_000,
      total_commits: 12,
      total_files_changed: 30,
      total_insertions: 1200,
      total_deletions: 800,
    },
  ],
  git_stats: {
    total_commits: 12,
    total_files_changed: 30,
    total_insertions: 1200,
    total_deletions: 800,
  },
};

describe("isRangeKey", () => {
  it("accepts the three valid keys", () => {
    expect(isRangeKey("week")).toBe(true);
    expect(isRangeKey("month")).toBe(true);
    expect(isRangeKey("all")).toBe(true);
  });

  it("rejects null, undefined, and unknown strings", () => {
    expect(isRangeKey(null)).toBe(false);
    expect(isRangeKey(undefined)).toBe(false);
    expect(isRangeKey("")).toBe(false);
    expect(isRangeKey("day")).toBe(false);
    expect(isRangeKey("MONTH")).toBe(false);
  });
});

describe("getRangeLabel", () => {
  it("returns the friendly label for each range", () => {
    expect(getRangeLabel("week")).toBe("Last Week");
    expect(getRangeLabel("month")).toBe("Last Month");
    expect(getRangeLabel("all")).toBe("All Time");
  });
});

describe("formatDuration", () => {
  it("renders an em-dash for zero", () => {
    expect(formatDuration(0)).toBe("—");
  });

  it("formats seconds, minutes, and hours", () => {
    expect(formatDuration(45_000)).toBe("45s");
    expect(formatDuration(125_000)).toBe("2m 5s");
    expect(formatDuration(3_725_000)).toBe("1h 2m");
  });
});

describe("getSubtitle", () => {
  it("renders the summary when stats are available", () => {
    expect(getSubtitle(sampleStats.global, false)).toBe("10 tasks · 7 sessions · 1h 2m");
  });

  it("shows the loading message while the fetch is in flight", () => {
    expect(getSubtitle(null, false)).toBe("Loading stats…");
  });

  it("shows the error message when the fetch has failed", () => {
    expect(getSubtitle(null, true)).toBe("Failed to load stats");
  });

  it("prefers stats over the error flag once data arrives", () => {
    expect(getSubtitle(sampleStats.global, true)).toBe("10 tasks · 7 sessions · 1h 2m");
  });
});

describe("statsReducer", () => {
  const initial = { stats: null, error: null };

  it("fetch action clears stats and error", () => {
    const stale = { stats: sampleStats, error: "previous" };
    expect(statsReducer(stale, { type: "fetch" })).toEqual({ stats: null, error: null });
  });

  it("success stores stats and clears any prior error", () => {
    const errored = { stats: null, error: "boom" };
    expect(statsReducer(errored, { type: "success", stats: sampleStats })).toEqual({
      stats: sampleStats,
      error: null,
    });
  });

  it("failure stores the error and clears stats", () => {
    const loaded = { stats: sampleStats, error: null };
    expect(statsReducer(loaded, { type: "failure", error: "nope" })).toEqual({
      stats: null,
      error: "nope",
    });
  });

  it("does not return the same reference as the input", () => {
    const next = statsReducer(initial, { type: "fetch" });
    expect(next).not.toBe(initial);
  });
});

describe("toPanelState", () => {
  it("returns loading when neither stats nor error is set", () => {
    expect(toPanelState(null, null)).toEqual({ kind: "loading" });
  });

  it("returns ready when stats are available", () => {
    expect(toPanelState(sampleStats, null)).toEqual({ kind: "ready", stats: sampleStats });
  });

  it("returns error when an error is present", () => {
    expect(toPanelState(null, "boom")).toEqual({ kind: "error", message: "boom" });
  });

  it("prefers error over stale stats so panels never linger in a ready+error mix", () => {
    expect(toPanelState(sampleStats, "boom")).toEqual({ kind: "error", message: "boom" });
  });
});

describe("buildStatsSummary", () => {
  it("includes completion %, top repo, and git line", () => {
    const summary = buildStatsSummary(sampleStats, "Last Month", 4);
    expect(summary).toContain("*Kandev Stats — Last Month*");
    expect(summary).toContain("10 total (4 done, 2 in progress) · 40% completion");
    expect(summary).toContain("Completed (Last Month): 4");
    expect(summary).toContain("Top repo: kandev (6 tasks)");
    expect(summary).toContain("12 commits, +1,200/-800");
  });

  it("falls back to em-dash and no-git when there is no activity", () => {
    const empty: StatsResponse = {
      ...sampleStats,
      global: emptyGlobal,
      repository_stats: [],
      git_stats: {
        total_commits: 0,
        total_files_changed: 0,
        total_insertions: 0,
        total_deletions: 0,
      },
    };
    const summary = buildStatsSummary(empty, "All Time", 0);
    expect(summary).toContain("Top repo: —");
    expect(summary).toContain("Git: no git activity");
    expect(summary).toContain("0 total (0 done, 0 in progress) · — completion");
  });
});
