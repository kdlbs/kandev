import { describe, expect, it } from "vitest";
import type { WorkspaceAggregateEntry } from "@/lib/api/domains/office-extended-api";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
import {
  aggregateStatus,
  aggregateTotals,
  buildAggregateAttention,
  groupActivity,
  uncoveredWorkspaceIds,
} from "./control-aggregate-signals";

function entry(overrides: Partial<WorkspaceAggregateEntry>): WorkspaceAggregateEntry {
  return {
    workspace_id: "ws",
    name: "Workspace",
    task_count: 0,
    open_tasks: 0,
    in_progress_tasks: 0,
    blocked_tasks: 0,
    done_tasks: 0,
    pending_approvals: 0,
    agent_count: 0,
    running_agents: 0,
    ...overrides,
  };
}

function activity(id: string, overrides: Partial<ActivityEntry> = {}): ActivityEntry {
  return {
    id,
    workspaceId: "ws-a",
    actorType: "agent",
    actorId: "agent-1",
    action: "task.status_changed",
    targetId: "task-1",
    createdAt: "2026-10-20T00:00:00Z",
    ...overrides,
  } as ActivityEntry;
}

describe("aggregateStatus", () => {
  it("reads in-progress work with no working agent as stalled, even when tasks are blocked", () => {
    expect(aggregateStatus(entry({ in_progress_tasks: 2, blocked_tasks: 1 }))).toBe("stalled");
  });
  it("reads blocked tasks without a stall as blocked", () => {
    expect(aggregateStatus(entry({ blocked_tasks: 1, running_agents: 1 }))).toBe("blocked");
  });
  it("reads a working agent as active", () => {
    expect(aggregateStatus(entry({ in_progress_tasks: 1, running_agents: 1 }))).toBe("active");
    expect(aggregateStatus(entry({ running_agents: 1 }))).toBe("active");
  });
  it("reads no work and no working agent as idle", () => {
    expect(aggregateStatus(entry({ open_tasks: 4, done_tasks: 3 }))).toBe("idle");
  });
});

describe("aggregateTotals", () => {
  it("sums every count and tallies workspaces by status", () => {
    const totals = aggregateTotals([
      entry({ workspace_id: "a", task_count: 3, in_progress_tasks: 3, agent_count: 1 }),
      entry({
        workspace_id: "b",
        task_count: 5,
        open_tasks: 1,
        blocked_tasks: 2,
        done_tasks: 2,
        pending_approvals: 4,
        agent_count: 2,
        running_agents: 1,
      }),
    ]);
    expect(totals).toMatchObject({
      workspaces: 2,
      tasks: 8,
      open: 1,
      inProgress: 3,
      blocked: 2,
      done: 2,
      approvals: 4,
      agents: 3,
      runningAgents: 1,
      byStatus: { stalled: 1, blocked: 1, active: 0, idle: 0 },
    });
  });
});

describe("buildAggregateAttention", () => {
  it("orders stalls, then blocked tasks, then approvals, largest count first", () => {
    const items = buildAggregateAttention([
      entry({ workspace_id: "a", name: "Alpha", pending_approvals: 1, blocked_tasks: 1 }),
      entry({ workspace_id: "b", name: "Beta", in_progress_tasks: 2 }),
      entry({ workspace_id: "c", name: "Gamma", blocked_tasks: 5, pending_approvals: 3 }),
      entry({ workspace_id: "d", name: "Delta", running_agents: 1, in_progress_tasks: 1 }),
    ]);
    expect(items.map((item) => [item.kind, item.workspaceId, item.count])).toEqual([
      ["stalled", "b", 2],
      ["blocked", "c", 5],
      ["blocked", "a", 1],
      ["approvals", "c", 3],
      ["approvals", "a", 1],
    ]);
  });
  it("raises nothing for idle and active workspaces", () => {
    expect(
      buildAggregateAttention([entry({ open_tasks: 2 }), entry({ running_agents: 1 })]),
    ).toEqual([]);
  });
});

describe("groupActivity", () => {
  it("folds consecutive repeats of the same action, actor and target", () => {
    const groups = groupActivity([
      activity("1"),
      activity("2"),
      activity("3", { targetId: "task-2" }),
      activity("4"),
      activity("5", { workspaceId: "ws-b" }),
    ]);
    expect(groups.map((group) => [group.entry.id, group.count])).toEqual([
      ["1", 2],
      ["3", 1],
      ["4", 1],
      ["5", 1],
    ]);
    expect(new Set(groups.map((group) => group.key)).size).toBe(groups.length);
  });
});

describe("uncoveredWorkspaceIds", () => {
  it("lists control workspaces the Office aggregate does not cover", () => {
    expect(uncoveredWorkspaceIds(["a", "b", "c"], [entry({ workspace_id: "b" })])).toEqual([
      "a",
      "c",
    ]);
  });
});
