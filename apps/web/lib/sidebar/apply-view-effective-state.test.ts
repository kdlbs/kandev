import { describe, it, expect } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { applySort, applyGroup, applyView } from "./apply-view";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";

function task(overrides: Partial<TaskSwitcherItem>): TaskSwitcherItem {
  return {
    id: overrides.id ?? "t",
    title: overrides.title ?? "Task",
    ...overrides,
  };
}

// @covers AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.1, .4, .5
describe("applySort — effective state bubbling (subtasks)", () => {
  const stateAsc = { key: "state" as const, direction: "asc" as const };

  it("parent with running subtask sorts above a backlog parent", () => {
    const backlogParent = task({ id: "bk", state: "TODO" });
    const runningParent = task({ id: "run", state: "TODO" });
    const runningSub = task({
      id: "sub",
      parentTaskId: "run",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([["run", [runningSub]]]);
    const out = applySort([backlogParent, runningParent, runningSub], stateAsc, [], subMap);
    // "run" should bubble above "bk" because its subtask is in_progress (1 vs backlog 2)
    expect(out.map((t) => t.id)).toEqual(["run", "sub", "bk"]);
  });

  it("parent with review subtask sorts above an in_progress parent", () => {
    const inProgressParent = task({ id: "ip", state: "IN_PROGRESS", sessionState: "RUNNING" });
    const reviewParent = task({ id: "rv", state: "TODO" });
    const reviewSub = task({
      id: "sub",
      parentTaskId: "rv",
      state: "REVIEW",
      sessionState: "WAITING_FOR_INPUT",
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([["rv", [reviewSub]]]);
    const out = applySort([inProgressParent, reviewParent, reviewSub], stateAsc, [], subMap);
    // review bucket (0) < in_progress bucket (1)
    expect(out.map((t) => t.id)).toEqual(["rv", "sub", "ip"]);
  });

  it("parent with multiple subtasks uses the best (lowest-order) bucket", () => {
    const parent = task({ id: "p", state: "TODO" });
    const reviewSub = task({
      id: "sr",
      parentTaskId: "p",
      state: "REVIEW",
      sessionState: "WAITING_FOR_INPUT",
    });
    const backlogSub = task({ id: "sb", parentTaskId: "p", state: "TODO", sessionState: "IDLE" });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [reviewSub, backlogSub]]]);
    const out = applySort([parent, reviewSub, backlogSub], stateAsc, [], subMap);
    // review (0) wins over backlog (2)
    expect(out.map((t) => t.id)).toEqual(["p", "sr", "sb"]);
  });

  it("completed/failed subtask still bubbles parent to the review section", () => {
    const parent = task({ id: "p", state: "TODO" });
    const failedSub = task({
      id: "sub",
      parentTaskId: "p",
      state: "FAILED",
      sessionState: "FAILED",
    });
    const inProgressPeer = task({ id: "peer", state: "IN_PROGRESS", sessionState: "RUNNING" });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [failedSub]]]);
    const out = applySort([parent, failedSub, inProgressPeer], stateAsc, [], subMap);
    // review bucket (0) < in_progress bucket (1), so parent with failed sub
    // sorts above the genuinely-running peer
    expect(out.map((t) => t.id)).toEqual(["p", "sub", "peer"]);
  });

  it("orphan subtask (parent filtered out) does not bubble any unrelated parent", () => {
    const orphanSub = task({
      id: "orphan",
      parentTaskId: "ghost",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const otherParent = task({ id: "other", state: "TODO" });
    const subMap = new Map<string, TaskSwitcherItem[]>();
    const out = applySort([orphanSub, otherParent], stateAsc, [], subMap);
    // orphan is promoted to root; its state only affects itself
    expect(out.map((t) => t.id)).toEqual(["orphan", "other"]);
  });

  it("falls back to static comparator when no subMap is provided", () => {
    const running = task({ id: "run", sessionState: "RUNNING" });
    const waiting = task({ id: "wait", sessionState: "WAITING_FOR_INPUT" });
    const out = applySort([running, waiting], stateAsc);
    expect(out.map((t) => t.id)).toEqual(["wait", "run"]);
  });

  it("title sort is unaffected by subMap", () => {
    const a = task({ id: "a", title: "Zebra" });
    const b = task({ id: "b", title: "Apple" });
    const subMap = new Map<string, TaskSwitcherItem[]>([
      ["a", [task({ id: "sub", parentTaskId: "a", sessionState: "RUNNING" })]],
    ]);
    const out = applySort([a, b], { key: "title", direction: "asc" }, [], subMap);
    expect(out.map((t) => t.id)).toEqual(["b", "a"]);
  });

  it("createdAt sort is unaffected by subMap", () => {
    const a = task({ id: "a", createdAt: "2026-02-01" });
    const b = task({ id: "b", createdAt: "2026-01-01" });
    const subMap = new Map<string, TaskSwitcherItem[]>([
      ["a", [task({ id: "sub", parentTaskId: "a", sessionState: "RUNNING" })]],
    ]);
    const out = applySort([a, b], { key: "createdAt", direction: "asc" }, [], subMap);
    expect(out.map((t) => t.id)).toEqual(["b", "a"]);
  });
});

// @covers AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.1, .2, .3, .4
describe("applyGroup — effective state grouping (subtasks)", () => {
  it("backlog parent with running subtask lands in IN_PROGRESS group", () => {
    const parent = task({ id: "p", state: "TODO" });
    const runningSub = task({
      id: "sub",
      parentTaskId: "p",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [runningSub]]]);
    const out = applyGroup([parent, runningSub], "state", subMap);
    const groupKeys = out.groups.map((g) => g.key);
    expect(groupKeys).toContain("IN_PROGRESS");
    expect(groupKeys).not.toContain("TODO");
    const inProgressGroup = out.groups.find((g) => g.key === "IN_PROGRESS");
    expect(inProgressGroup?.tasks.map((t) => t.id)).toEqual(["p"]);
    expect(out.subTasksByParentId.get("p")?.map((t) => t.id)).toEqual(["sub"]);
  });

  it("parent with review subtask lands in REVIEW group", () => {
    const parent = task({ id: "p", state: "TODO" });
    const reviewSub = task({
      id: "sub",
      parentTaskId: "p",
      state: "REVIEW",
      sessionState: "WAITING_FOR_INPUT",
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [reviewSub]]]);
    const out = applyGroup([parent, reviewSub], "state", subMap);
    const reviewGroup = out.groups.find((g) => g.key === "REVIEW");
    expect(reviewGroup?.tasks.map((t) => t.id)).toEqual(["p"]);
  });

  it("falls back to own state group when subMap is not provided", () => {
    const parent = task({ id: "p", state: "COMPLETED" });
    const runningSub = task({
      id: "sub",
      parentTaskId: "p",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const out = applyGroup([parent, runningSub], "state");
    const completedGroup = out.groups.find((g) => g.key === "COMPLETED");
    expect(completedGroup?.tasks.map((t) => t.id)).toEqual(["p"]);
  });

  it("running session without task state bubbles parent to in-progress group", () => {
    const parent = task({ id: "p", state: "TODO" });
    const nullStateSub = task({
      id: "sub",
      parentTaskId: "p",
      sessionState: "RUNNING",
      // state intentionally omitted — subtask has a live session but no persisted state
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [nullStateSub]]]);
    const out = applyGroup([parent, nullStateSub], "state", subMap);
    // A live session is enough to recover an incomplete task-state projection.
    const inProgressGroup = out.groups.find((g) => g.key === "IN_PROGRESS");
    expect(inProgressGroup?.tasks.map((t) => t.id)).toContain("p");
    expect(out.groups.find((g) => g.key === "TODO")).toBeUndefined();
  });
});

describe("applyGroup — effective state precedence (subtasks)", () => {
  it.each([
    ["REVIEW", "WAITING_FOR_INPUT"],
    ["WAITING_FOR_INPUT", "WAITING_FOR_INPUT"],
    ["FAILED", "FAILED"],
    ["CANCELLED", "CANCELLED"],
    ["COMPLETED", "COMPLETED"],
  ] as const)("running child overrides a %s parent", (parentState, parentSessionState) => {
    const parent = task({ id: "p", state: parentState, sessionState: parentSessionState });
    const runningSub = task({
      id: "sub",
      parentTaskId: "p",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [runningSub]]]);

    const out = applyGroup([parent, runningSub], "state", subMap);

    expect(out.groups.find((group) => group.key === "IN_PROGRESS")?.tasks).toEqual([parent]);
  });

  it("uses scheduling when no included member is already in progress", () => {
    const parent = task({ id: "p", state: "COMPLETED", sessionState: "COMPLETED" });
    const schedulingSub = task({
      id: "sub",
      parentTaskId: "p",
      state: "SCHEDULING",
      sessionState: "STARTING",
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [schedulingSub]]]);

    const out = applyGroup([parent, schedulingSub], "state", subMap);

    expect(out.groups.find((group) => group.key === "SCHEDULING")?.tasks).toEqual([parent]);
    expect(out.groups.find((group) => group.key === "COMPLETED")).toBeUndefined();
  });

  it("does not use Completed while an included member is incomplete", () => {
    const parent = task({ id: "p", state: "COMPLETED", sessionState: "COMPLETED" });
    const todoSub = task({ id: "sub", parentTaskId: "p", state: "TODO" });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [todoSub]]]);

    const out = applyGroup([parent, todoSub], "state", subMap);

    expect(out.groups.find((group) => group.key === "TODO")?.tasks).toEqual([parent]);
    expect(out.groups.find((group) => group.key === "COMPLETED")).toBeUndefined();
  });

  it("resolves a running nested descendant and survives a malformed cycle", () => {
    const parent = task({ id: "p", state: "COMPLETED", sessionState: "COMPLETED" });
    const child = task({ id: "c", parentTaskId: "p", state: "TODO" });
    const grandchild = task({
      id: "gc",
      parentTaskId: "c",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([
      ["p", [child]],
      ["c", [grandchild]],
      ["gc", [parent]],
    ]);

    const out = applyGroup([parent, child, grandchild], "state", subMap);

    expect(out.groups.find((group) => group.key === "IN_PROGRESS")?.tasks).toEqual([parent]);
  });

  it("preserves each row object while changing only tree placement", () => {
    const parent = task({ id: "p", state: "COMPLETED", sessionState: "COMPLETED" });
    const runningSub = task({
      id: "sub",
      parentTaskId: "p",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [runningSub]]]);

    const out = applyGroup([parent, runningSub], "state", subMap);

    expect(out.groups.find((group) => group.key === "IN_PROGRESS")?.tasks[0]).toBe(parent);
    expect(out.subTasksByParentId.get("p")?.[0]).toBe(runningSub);
    expect(runningSub.state).toBe("IN_PROGRESS");
  });
});

// @covers AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.5, .6
describe("applyView — effective state bubbling (integration)", () => {
  const stateView: SidebarView = {
    id: "v",
    name: "v",
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "none",
    collapsedGroups: [],
  };

  it("bubbles a parent with a running subtask above a backlog peer", () => {
    const backlogParent = task({ id: "bk", state: "TODO" });
    const idleParent = task({ id: "idle", state: "TODO" });
    const runningSub = task({
      id: "sub",
      parentTaskId: "idle",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const out = applyView([backlogParent, idleParent, runningSub], stateView);
    // idleParent bubbles above backlogParent because its subtask is in_progress (1 vs 2)
    expect(out.groups[0].tasks.map((t) => t.id)).toEqual(["idle", "bk"]);
  });

  it("pinned tasks still float above bubbled parents", () => {
    const backlogParent = task({ id: "bk", state: "TODO" });
    const idleParent = task({ id: "idle", state: "TODO" });
    const runningSub = task({
      id: "sub",
      parentTaskId: "idle",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const out = applyView([backlogParent, idleParent, runningSub], stateView, {
      pinnedTaskIds: ["bk"],
      orderedTaskIds: [],
    });
    // pinned first, then bubbled idle parent
    expect(out.groups[0].tasks.map((t) => t.id)).toEqual(["bk", "idle"]);
  });

  it("subtask manual order is preserved while parent bubbles among roots", () => {
    const backlogParent = task({ id: "bk", state: "TODO" });
    const idleParent = task({ id: "idle", state: "TODO" });
    const sub1 = task({
      id: "s1",
      parentTaskId: "idle",
      title: "S1",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const sub2 = task({
      id: "s2",
      parentTaskId: "idle",
      title: "S2",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const out = applyView([backlogParent, idleParent, sub1, sub2], stateView, {
      pinnedTaskIds: [],
      orderedTaskIds: [],
      subtaskOrderByParentId: { idle: ["s2", "s1"] },
    });
    // idle parent bubbles above backlog because its subtasks are running (in_progress bucket)
    expect(out.groups[0].tasks.map((t) => t.id)).toEqual(["idle", "bk"]);
    // manual subtask order is preserved underneath the parent
    expect(out.subTasksByParentId.get("idle")?.map((t) => t.id)).toEqual(["s2", "s1"]);
  });
});

// @covers AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.3, .7
describe("applyView — effective state consistency (integration)", () => {
  it("uses the same effective state for sorting and grouping", () => {
    const parent = task({
      id: "parent",
      state: "COMPLETED",
      sessionState: "COMPLETED",
      createdAt: "2026-01-01",
    });
    const peer = task({
      id: "peer",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
      createdAt: "2026-02-01",
    });
    const runningSub = task({
      id: "sub",
      parentTaskId: "parent",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
      createdAt: "2026-01-02",
    });
    const view: SidebarView = {
      id: "state-view",
      name: "State",
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "state",
      collapsedGroups: [],
    };

    const out = applyView([parent, peer, runningSub], view);
    const activeGroup = out.groups.find((group) => group.key === "IN_PROGRESS");

    expect(activeGroup?.tasks.map((task) => task.id)).toEqual(["peer", "parent"]);
    expect(out.groups.find((group) => group.key === "COMPLETED")).toBeUndefined();
  });

  it("ignores a filtered-out running descendant", () => {
    const parent = task({ id: "parent", title: "Visible parent", state: "COMPLETED" });
    const runningSub = task({
      id: "sub",
      parentTaskId: "parent",
      title: "Hidden running child",
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
    });
    const view: SidebarView = {
      id: "filtered-view",
      name: "Filtered",
      filters: [{ id: "title", dimension: "titleMatch", op: "matches", value: "Visible" }],
      sort: { key: "state", direction: "asc" },
      group: "state",
      collapsedGroups: [],
    };

    const out = applyView([parent, runningSub], view);

    expect(out.groups.find((group) => group.key === "COMPLETED")?.tasks).toEqual([parent]);
    expect(out.subTasksByParentId.get("parent")).toBeUndefined();
  });

  it("resolves each deep-tree edge once", () => {
    const depth = 2_000;
    const tasks = Array.from({ length: depth }, (_, index) =>
      task({
        id: `deep-${index}`,
        parentTaskId: index === 0 ? undefined : `deep-${index - 1}`,
        state: index === depth - 1 ? "IN_PROGRESS" : "COMPLETED",
        sessionState: index === depth - 1 ? "RUNNING" : "COMPLETED",
      }),
    );
    class CountingSubtaskMap extends Map<string, TaskSwitcherItem[]> {
      lookups = 0;

      override get(key: string): TaskSwitcherItem[] | undefined {
        this.lookups += 1;
        return super.get(key);
      }
    }
    const subMap = new CountingSubtaskMap();
    for (let index = 0; index < depth - 1; index += 1) {
      subMap.set(`deep-${index}`, [tasks[index + 1]]);
    }

    applySort(tasks, { key: "state", direction: "asc" }, [], subMap);

    expect(subMap.lookups).toBeLessThan(depth * 4);
  });
});
