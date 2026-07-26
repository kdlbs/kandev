import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskReviewFinding, TaskReviewRun } from "@/lib/types/review";
import { registerReviewHandlers } from "./review";

function newFakeStore() {
  const actions = {
    upsertReviewRun: vi.fn(),
    addReviewFindings: vi.fn(),
    updateReviewFinding: vi.fn(),
    clearTaskReviewState: vi.fn(),
  };
  const store = { getState: () => actions } as unknown as StoreApi<AppState>;
  return { store, actions };
}

const run = { id: "run-1", task_id: "t1" } as TaskReviewRun;
const finding = { id: "f1", task_id: "t1" } as TaskReviewFinding;

// The router passes fully-typed messages; tests only need the payload.
function message<T>(action: string, payload: T) {
  return { action, payload } as never;
}

describe("registerReviewHandlers", () => {
  it("upserts a run on task.review.run_updated", () => {
    const { store, actions } = newFakeStore();
    const handlers = registerReviewHandlers(store);
    handlers["task.review.run_updated"]!(
      message("task.review.run_updated", { task_id: "t1", run }),
    );
    expect(actions.upsertReviewRun).toHaveBeenCalledWith("t1", run);
  });

  it("adds findings on task.review.findings_published", () => {
    const { store, actions } = newFakeStore();
    const handlers = registerReviewHandlers(store);
    handlers["task.review.findings_published"]!(
      message("task.review.findings_published", {
        task_id: "t1",
        run_id: "run-1",
        findings: [finding],
      }),
    );
    expect(actions.addReviewFindings).toHaveBeenCalledWith("t1", [finding], undefined);
  });

  it("forwards superseded ids so the client drops replaced findings", () => {
    const { store, actions } = newFakeStore();
    const handlers = registerReviewHandlers(store);
    handlers["task.review.findings_published"]!(
      message("task.review.findings_published", {
        task_id: "t1",
        run_id: "run-1",
        findings: [finding],
        superseded_ids: ["old-1"],
      }),
    );
    expect(actions.addReviewFindings).toHaveBeenCalledWith("t1", [finding], ["old-1"]);
  });

  it("updates a finding on task.review.finding_updated", () => {
    const { store, actions } = newFakeStore();
    const handlers = registerReviewHandlers(store);
    handlers["task.review.finding_updated"]!(
      message("task.review.finding_updated", { task_id: "t1", finding }),
    );
    expect(actions.updateReviewFinding).toHaveBeenCalledWith("t1", finding);
  });

  it("clears review state on task.review.cleared", () => {
    const { store, actions } = newFakeStore();
    const handlers = registerReviewHandlers(store);
    handlers["task.review.cleared"]!(message("task.review.cleared", { task_id: "t1" }));
    expect(actions.clearTaskReviewState).toHaveBeenCalledWith("t1");
  });

  it("ignores malformed payloads rather than writing partial state", () => {
    const { store, actions } = newFakeStore();
    const handlers = registerReviewHandlers(store);

    handlers["task.review.run_updated"]!(message("task.review.run_updated", { task_id: "", run }));
    handlers["task.review.run_updated"]!(
      message("task.review.run_updated", { task_id: "t1", run: null }),
    );
    handlers["task.review.findings_published"]!(
      message("task.review.findings_published", { task_id: "t1", run_id: "r", findings: [] }),
    );
    handlers["task.review.finding_updated"]!(
      message("task.review.finding_updated", { task_id: "t1", finding: null }),
    );
    handlers["task.review.cleared"]!(message("task.review.cleared", { task_id: "" }));

    expect(actions.upsertReviewRun).not.toHaveBeenCalled();
    expect(actions.addReviewFindings).not.toHaveBeenCalled();
    expect(actions.updateReviewFinding).not.toHaveBeenCalled();
    expect(actions.clearTaskReviewState).not.toHaveBeenCalled();
  });
});
