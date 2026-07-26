import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { TaskReviewFinding, TaskReviewRun } from "@/lib/types/review";
import { createReviewSlice } from "./review-slice";
import type { ReviewSlice } from "./types";

function newStore() {
  return create<ReviewSlice>()(immer((set) => createReviewSlice(set)));
}

function run(overrides: Partial<TaskReviewRun> = {}): TaskReviewRun {
  return {
    id: "run-1",
    task_id: "t1",
    session_id: "s1",
    trigger: "manual",
    workflow_step_id: "",
    agent_id: "claude-acp",
    model: "haiku",
    status: "pending",
    error_code: "",
    error_message: "",
    summary: "",
    finding_count: 0,
    file_count: 0,
    repository_count: 0,
    prompt_tokens: 0,
    response_tokens: 0,
    duration_ms: 0,
    created_at: "2026-07-24T10:00:00Z",
    ...overrides,
  };
}

function finding(overrides: Partial<TaskReviewFinding> = {}): TaskReviewFinding {
  return {
    id: "f1",
    run_id: "run-1",
    task_id: "t1",
    repository_id: "",
    repository_name: "",
    file_path: "a.go",
    start_line: 1,
    end_line: 1,
    side: "additions",
    severity: "major",
    category: "correctness",
    title: "Bug",
    body: "details",
    suggestion: "",
    anchor_text: "",
    file_diff_hash: "h",
    status: "open",
    created_at: "2026-07-24T10:00:00Z",
    updated_at: "2026-07-24T10:00:00Z",
    ...overrides,
  };
}

describe("setTaskReview", () => {
  it("replaces the task's review state and marks it loaded", () => {
    const store = newStore();
    store.getState().setTaskReview("t1", { runs: [run()], findings: [finding()] });

    const state = store.getState().taskReview;
    expect(state.runsByTaskId.t1).toHaveLength(1);
    expect(state.findingsByTaskId.t1).toHaveLength(1);
    expect(state.loadedTaskIds.t1).toBe(true);
  });

  it("orders runs newest first", () => {
    const store = newStore();
    store.getState().setTaskReview("t1", {
      runs: [
        run({ id: "old", created_at: "2026-07-24T09:00:00Z" }),
        run({ id: "new", created_at: "2026-07-24T11:00:00Z" }),
      ],
      findings: [],
    });
    expect(store.getState().taskReview.runsByTaskId.t1.map((r) => r.id)).toEqual(["new", "old"]);
  });

  it("caps run history", () => {
    const store = newStore();
    const runs = Array.from({ length: 30 }, (_, i) =>
      run({ id: `run-${i}`, created_at: `2026-07-24T${String(i).padStart(2, "0")}:00:00Z` }),
    );
    store.getState().setTaskReview("t1", { runs, findings: [] });
    expect(store.getState().taskReview.runsByTaskId.t1).toHaveLength(20);
  });
});

describe("upsertReviewRun", () => {
  it("replaces a run by id rather than appending", () => {
    const store = newStore();
    store.getState().upsertReviewRun("t1", run({ status: "pending" }));
    store.getState().upsertReviewRun("t1", run({ status: "running" }));

    const runs = store.getState().taskReview.runsByTaskId.t1;
    expect(runs).toHaveLength(1);
    expect(runs[0].status).toBe("running");
  });

  it("adds a run for a task with no prior state", () => {
    const store = newStore();
    store.getState().upsertReviewRun("fresh", run());
    expect(store.getState().taskReview.runsByTaskId.fresh).toHaveLength(1);
  });
});

describe("addReviewFindings", () => {
  it("merges by id so a re-publish does not duplicate", () => {
    const store = newStore();
    store.getState().addReviewFindings("t1", [finding({ id: "f1", title: "First" })]);
    store.getState().addReviewFindings("t1", [finding({ id: "f1", title: "Refreshed" })]);

    const findings = store.getState().taskReview.findingsByTaskId.t1;
    expect(findings).toHaveLength(1);
    expect(findings[0].title).toBe("Refreshed");
  });

  it("appends distinct findings", () => {
    const store = newStore();
    store.getState().addReviewFindings("t1", [finding({ id: "f1" })]);
    store.getState().addReviewFindings("t1", [finding({ id: "f2" })]);
    expect(store.getState().taskReview.findingsByTaskId.t1).toHaveLength(2);
  });

  it("drops superseded findings so a re-review shows one per anchor", () => {
    // The backend deletes the old finding and inserts a replacement with a new
    // id; merging by id alone would leave both visible at the same line.
    const store = newStore();
    store.getState().addReviewFindings("t1", [finding({ id: "old" })]);
    store.getState().addReviewFindings("t1", [finding({ id: "new" })], ["old"]);

    const findings = store.getState().taskReview.findingsByTaskId.t1;
    expect(findings).toHaveLength(1);
    expect(findings[0].id).toBe("new");
  });

  it("leaves unrelated findings alone when superseding", () => {
    const store = newStore();
    store.getState().addReviewFindings("t1", [finding({ id: "old" }), finding({ id: "keep" })]);
    store.getState().addReviewFindings("t1", [finding({ id: "new" })], ["old"]);

    const ids = store
      .getState()
      .taskReview.findingsByTaskId.t1.map((f) => f.id)
      .sort();
    expect(ids).toEqual(["keep", "new"]);
  });

  it("tolerates an absent or empty superseded list", () => {
    const store = newStore();
    store.getState().addReviewFindings("t1", [finding({ id: "a" })]);
    store.getState().addReviewFindings("t1", [finding({ id: "b" })], []);
    store.getState().addReviewFindings("t1", [finding({ id: "c" })], undefined);
    expect(store.getState().taskReview.findingsByTaskId.t1).toHaveLength(3);
  });

  it("ignores an empty batch", () => {
    const store = newStore();
    store.getState().addReviewFindings("t1", [finding()]);
    store.getState().addReviewFindings("t1", []);
    expect(store.getState().taskReview.findingsByTaskId.t1).toHaveLength(1);
  });
});

describe("updateReviewFinding", () => {
  it("replaces the finding in place", () => {
    const store = newStore();
    store.getState().addReviewFindings("t1", [finding({ id: "f1", status: "open" })]);
    store.getState().updateReviewFinding("t1", finding({ id: "f1", status: "resolved" }));

    const findings = store.getState().taskReview.findingsByTaskId.t1;
    expect(findings).toHaveLength(1);
    expect(findings[0].status).toBe("resolved");
  });

  it("keeps an update for a finding this client never saw", () => {
    // Another browser (or an event that beat the backfill) can resolve a finding
    // we have not loaded; dropping it would lose the state change.
    const store = newStore();
    store.getState().updateReviewFinding("t1", finding({ id: "unseen", status: "dismissed" }));
    expect(store.getState().taskReview.findingsByTaskId.t1[0].id).toBe("unseen");
  });
});

describe("clearTaskReviewState", () => {
  it("drops runs, findings, and the loaded marker", () => {
    const store = newStore();
    store.getState().setTaskReview("t1", { runs: [run()], findings: [finding()] });
    store.getState().clearTaskReviewState("t1");

    const state = store.getState().taskReview;
    expect(state.runsByTaskId.t1).toBeUndefined();
    expect(state.findingsByTaskId.t1).toBeUndefined();
    expect(state.loadedTaskIds.t1).toBeUndefined();
  });

  it("leaves other tasks untouched", () => {
    const store = newStore();
    store.getState().setTaskReview("t1", { runs: [run()], findings: [] });
    store.getState().setTaskReview("t2", { runs: [run({ id: "other" })], findings: [] });
    store.getState().clearTaskReviewState("t1");
    expect(store.getState().taskReview.runsByTaskId.t2).toHaveLength(1);
  });
});
