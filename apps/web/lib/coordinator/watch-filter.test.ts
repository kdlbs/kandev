import { describe, expect, it } from "vitest";
import type { AttentionStall, AttentionTask } from "@/lib/coordinator/attention";
import { filterWatched, isTaskWatched, type WatchSet } from "./watch-filter";

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

describe("isTaskWatched with a Projects scope", () => {
  const withRepos = (id: string, repositoryIds: string[] | null | undefined): AttentionTask => ({
    id,
    title: id,
    workflowId: "wf-a",
    repositoryIds,
  });
  const projects = (
    repositoryIds: string[] | null,
    includeNoRepository = false,
  ): WatchSet["projects"] => ({ scope: "selected", repositoryIds, includeNoRepository });
  const watch = (p: WatchSet["projects"]): WatchSet => ({
    scope: "all",
    workflowIds: [],
    projects: p,
  });

  it("watches a task with any repository in scope", () => {
    const set = watch(projects(["r1"]));
    expect(isTaskWatched(withRepos("t", ["r1"]), set)).toBe(true);
    expect(isTaskWatched(withRepos("t", ["r2", "r1"]), set)).toBe(true);
    expect(isTaskWatched(withRepos("t", ["r2"]), set)).toBe(false);
  });

  it("watches a task with no repository only when the toggle is on", () => {
    expect(isTaskWatched(withRepos("t", []), watch(projects(["r1"], true)))).toBe(true);
    expect(isTaskWatched(withRepos("t", []), watch(projects(["r1"], false)))).toBe(false);
  });

  it("fails closed on unknown repositories or an unknown scope", () => {
    expect(isTaskWatched(withRepos("t", null), watch(projects(["r1"], true)))).toBe(false);
    expect(isTaskWatched(withRepos("t", undefined), watch(projects(["r1"], true)))).toBe(false);
    expect(isTaskWatched(withRepos("t", ["r1"]), watch(projects(null, true)))).toBe(false);
  });

  it("combines with the boards scope", () => {
    const set: WatchSet = {
      scope: "selected",
      workflowIds: ["wf-b"],
      projects: projects(["r1"]),
    };
    expect(isTaskWatched(withRepos("t", ["r1"]), set)).toBe(false);
  });

  it("ignores repositories when no Projects scope is set", () => {
    expect(isTaskWatched(withRepos("t", undefined), { scope: "all", workflowIds: [] })).toBe(true);
  });
});
