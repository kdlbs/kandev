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

  it("parent with null-state running subtask stays in its own group", () => {
    const parent = task({ id: "p", state: "TODO" });
    const nullStateSub = task({
      id: "sub",
      parentTaskId: "p",
      sessionState: "RUNNING",
      // state intentionally omitted — subtask has a live session but no persisted state
    });
    const subMap = new Map<string, TaskSwitcherItem[]>([["p", [nullStateSub]]]);
    const out = applyGroup([parent, nullStateSub], "state", subMap);
    // Null-state subtask must not displace the parent from its own TODO group
    const todoGroup = out.groups.find((g) => g.key === "TODO");
    expect(todoGroup?.tasks.map((t) => t.id)).toContain("p");
    // Should NOT appear under NOT_STARTED (the old bug) or IN_PROGRESS
    expect(out.groups.find((g) => g.key === "__not_started__")).toBeUndefined();
  });
});

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
