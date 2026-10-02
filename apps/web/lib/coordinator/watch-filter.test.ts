import { describe, expect, it } from "vitest";
import type { AttentionStall, AttentionTask } from "@/lib/coordinator/attention";
import { filterWatched, isTaskWatched } from "./watch-filter";

const task = (id: string, workflowId?: string | null): AttentionTask => ({
  id,
  title: id,
  workflowId,
});
const stall = (taskId: string): AttentionStall => ({
  task_id: taskId,
  stalled_for_ms: 1,
  last_event_at: "2026-01-01T00:00:00Z",
  detected_at: "2026-01-01T00:00:00Z",
});

describe("isTaskWatched", () => {
  it("watches every task with a workflow when the scope is all", () => {
    expect(isTaskWatched(task("t", "wf-a"), { scope: "all", workflowIds: [] })).toBe(true);
  });

  it("never watches a task without a workflow", () => {
    expect(isTaskWatched(task("t", null), { scope: "all", workflowIds: [] })).toBe(false);
    expect(isTaskWatched(task("t"), { scope: "selected", workflowIds: ["wf-a"] })).toBe(false);
  });

  it("watches only the selected workflows", () => {
    const set = { scope: "selected", workflowIds: ["wf-a"] } as const;
    expect(isTaskWatched(task("t", "wf-a"), set)).toBe(true);
    expect(isTaskWatched(task("t", "wf-b"), set)).toBe(false);
  });

  it("watches nothing for an empty selected set", () => {
    expect(isTaskWatched(task("t", "wf-a"), { scope: "selected", workflowIds: [] })).toBe(false);
  });
});

describe("filterWatched", () => {
  const tasks = [task("a", "wf-a"), task("b", "wf-b"), task("c", null)];
  const stalls = [stall("a"), stall("b"), stall("gone")];

  it("keeps watched tasks and the stalls whose task is among them", () => {
    const out = filterWatched({ tasks, stalls }, { scope: "selected", workflowIds: ["wf-a"] });
    expect(out.tasks.map((t) => t.id)).toEqual(["a"]);
    expect(out.stalls.map((s) => s.task_id)).toEqual(["a"]);
  });

  it("drops a stall whose task is absent even when the scope is all", () => {
    const out = filterWatched({ tasks, stalls }, { scope: "all", workflowIds: [] });
    expect(out.tasks.map((t) => t.id)).toEqual(["a", "b"]);
    expect(out.stalls.map((s) => s.task_id)).toEqual(["a", "b"]);
  });

  it("keeps nothing for an empty selected set", () => {
    const out = filterWatched({ tasks, stalls }, { scope: "selected", workflowIds: [] });
    expect(out).toEqual({ tasks: [], stalls: [] });
  });
});
