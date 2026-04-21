import { describe, it, expect } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { applyFilters, applyGroup, applySort, applyView } from "./apply-view";
import type { FilterClause, SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";

function task(overrides: Partial<TaskSwitcherItem>): TaskSwitcherItem {
  return {
    id: overrides.id ?? "t",
    title: overrides.title ?? "Task",
    ...overrides,
  };
}

const C = (c: Omit<FilterClause, "id">): FilterClause => ({ id: "c1", ...c });

describe("applyFilters — basics", () => {
  it("returns all when no clauses", () => {
    const tasks = [task({ id: "a" }), task({ id: "b" })];
    expect(applyFilters(tasks, [])).toHaveLength(2);
  });
});

describe("applyFilters — per-dimension", () => {
  it("filters by isPRReview is true (watcher-created task)", () => {
    const tasks = [task({ id: "a", isPRReview: true }), task({ id: "b" })];
    const out = applyFilters(tasks, [C({ dimension: "isPRReview", op: "is", value: true })]);
    expect(out.map((t) => t.id)).toEqual(["a"]);
  });

  it("filters by isPRReview is false (regular task, even with linked PR)", () => {
    const tasks = [
      task({ id: "a", isPRReview: true }),
      task({ id: "b", prInfo: { number: 1, state: "Open" } }),
    ];
    const out = applyFilters(tasks, [C({ dimension: "isPRReview", op: "is", value: false })]);
    expect(out.map((t) => t.id)).toEqual(["b"]);
  });

  it("supports is_not negation on isPRReview", () => {
    const tasks = [task({ id: "a", isPRReview: true }), task({ id: "b" })];
    const out = applyFilters(tasks, [C({ dimension: "isPRReview", op: "is_not", value: true })]);
    expect(out.map((t) => t.id)).toEqual(["b"]);
  });

  it("filters by isIssueWatch is true (issue-watcher-created task)", () => {
    const tasks = [task({ id: "a", isIssueWatch: true }), task({ id: "b" })];
    const out = applyFilters(tasks, [C({ dimension: "isIssueWatch", op: "is", value: true })]);
    expect(out.map((t) => t.id)).toEqual(["a"]);
  });

  it("filters by isIssueWatch is false (regular task)", () => {
    const tasks = [task({ id: "a", isIssueWatch: true }), task({ id: "b" })];
    const out = applyFilters(tasks, [C({ dimension: "isIssueWatch", op: "is", value: false })]);
    expect(out.map((t) => t.id)).toEqual(["b"]);
  });

  it("supports is_not negation on isIssueWatch", () => {
    const tasks = [task({ id: "a", isIssueWatch: true }), task({ id: "b" })];
    const out = applyFilters(tasks, [C({ dimension: "isIssueWatch", op: "is_not", value: true })]);
    expect(out.map((t) => t.id)).toEqual(["b"]);
  });

  it("filters by archived boolean", () => {
    const tasks = [task({ id: "a", isArchived: true }), task({ id: "b" })];
    const only = applyFilters(tasks, [C({ dimension: "archived", op: "is", value: true })]);
    expect(only.map((t) => t.id)).toEqual(["a"]);
    const not = applyFilters(tasks, [C({ dimension: "archived", op: "is", value: false })]);
    expect(not.map((t) => t.id)).toEqual(["b"]);
  });

  it("filters by state bucket with in / not_in", () => {
    const tasks = [
      task({ id: "a", state: "REVIEW" }),
      task({ id: "b", sessionState: "RUNNING" }),
      task({ id: "c" }),
    ];
    const review = applyFilters(tasks, [C({ dimension: "state", op: "in", value: ["review"] })]);
    expect(review.map((t) => t.id)).toEqual(["a"]);
    const notBacklog = applyFilters(tasks, [
      C({ dimension: "state", op: "not_in", value: ["backlog"] }),
    ]);
    expect(notBacklog.map((t) => t.id).sort()).toEqual(["a", "b"]);
  });

  it("filters by repository", () => {
    const tasks = [
      task({ id: "a", repositoryPath: "org/repo1" }),
      task({ id: "b", repositoryPath: "org/repo2" }),
    ];
    const out = applyFilters(tasks, [C({ dimension: "repository", op: "is", value: "org/repo1" })]);
    expect(out.map((t) => t.id)).toEqual(["a"]);
  });

  it("filters by executorType", () => {
    const tasks = [
      task({ id: "a", remoteExecutorType: "sprites" }),
      task({ id: "b", remoteExecutorType: "local_docker" }),
    ];
    const out = applyFilters(tasks, [C({ dimension: "executorType", op: "is", value: "sprites" })]);
    expect(out.map((t) => t.id)).toEqual(["a"]);
  });

  it("filters by workflow + workflowStep", () => {
    const tasks = [
      task({ id: "a", workflowId: "wf1", workflowStepId: "s1" }),
      task({ id: "b", workflowId: "wf2", workflowStepId: "s1" }),
      task({ id: "c", workflowId: "wf1", workflowStepId: "s2" }),
    ];
    const out = applyFilters(tasks, [
      { id: "c1", dimension: "workflow", op: "is", value: "wf1" },
      { id: "c2", dimension: "workflowStep", op: "is", value: "s1" },
    ]);
    expect(out.map((t) => t.id)).toEqual(["a"]);
  });

  it("filters by hasDiff", () => {
    const tasks = [
      task({ id: "a", diffStats: { additions: 10, deletions: 0 } }),
      task({ id: "b", diffStats: { additions: 0, deletions: 0 } }),
      task({ id: "c" }),
    ];
    const withDiff = applyFilters(tasks, [C({ dimension: "hasDiff", op: "is", value: true })]);
    expect(withDiff.map((t) => t.id)).toEqual(["a"]);
    const noDiff = applyFilters(tasks, [C({ dimension: "hasDiff", op: "is", value: false })]);
    expect(noDiff.map((t) => t.id).sort()).toEqual(["b", "c"]);
  });
});

describe("applyFilters — titleMatch + combos", () => {
  it("filters by titleMatch matches (case-insensitive substring)", () => {
    const tasks = [
      task({ id: "a", title: "Fix auth bug" }),
      task({ id: "b", title: "Update deps" }),
      task({ id: "c", title: "AUTHenticate users" }),
    ];
    const out = applyFilters(tasks, [C({ dimension: "titleMatch", op: "matches", value: "auth" })]);
    expect(out.map((t) => t.id).sort()).toEqual(["a", "c"]);
  });

  it("filters by titleMatch not_matches", () => {
    const tasks = [
      task({ id: "a", title: "Fix auth bug" }),
      task({ id: "b", title: "Update deps" }),
    ];
    const out = applyFilters(tasks, [
      C({ dimension: "titleMatch", op: "not_matches", value: "auth" }),
    ]);
    expect(out.map((t) => t.id)).toEqual(["b"]);
  });

  it("AND combines multiple clauses", () => {
    const tasks = [
      task({ id: "a", sessionState: "RUNNING", isPRReview: true }),
      task({ id: "b", sessionState: "RUNNING" }),
      task({ id: "c", isPRReview: true }),
    ];
    const out = applyFilters(tasks, [
      C({ dimension: "state", op: "is", value: "in_progress" }),
      { id: "c2", dimension: "isPRReview", op: "is", value: true },
    ]);
    expect(out.map((t) => t.id)).toEqual(["a"]);
  });
});

describe("applySort", () => {
  const a = task({ id: "a", state: "REVIEW", updatedAt: "2026-04-01", title: "Zeta" });
  const b = task({ id: "b", sessionState: "RUNNING", updatedAt: "2026-04-05", title: "Alpha" });
  const c = task({ id: "c", updatedAt: "2026-04-03", title: "Mu" });

  it("sorts by state bucket asc (review, in_progress, backlog)", () => {
    const out = applySort([c, b, a], { key: "state", direction: "asc" });
    expect(out.map((t) => t.id)).toEqual(["a", "b", "c"]);
  });

  it("sorts by state bucket desc", () => {
    const out = applySort([a, b, c], { key: "state", direction: "desc" });
    expect(out.map((t) => t.id)).toEqual(["c", "b", "a"]);
  });

  it("sorts by updatedAt desc", () => {
    const out = applySort([a, b, c], { key: "updatedAt", direction: "desc" });
    expect(out.map((t) => t.id)).toEqual(["b", "c", "a"]);
  });

  it("sorts by title asc", () => {
    const out = applySort([a, b, c], { key: "title", direction: "asc" });
    expect(out.map((t) => t.id)).toEqual(["b", "c", "a"]);
  });

  it("is stable for equal keys", () => {
    const x = task({ id: "x", state: "REVIEW" });
    const y = task({ id: "y", state: "REVIEW" });
    const z = task({ id: "z", state: "REVIEW" });
    const out = applySort([x, y, z], { key: "state", direction: "asc" });
    expect(out.map((t) => t.id)).toEqual(["x", "y", "z"]);
  });
});

describe("applyGroup", () => {
  it("wraps all tasks in single pseudo-group when group=none", () => {
    const tasks = [task({ id: "a" }), task({ id: "b" })];
    const out = applyGroup(tasks, "none");
    expect(out.groups).toHaveLength(1);
    expect(out.groups[0].tasks).toHaveLength(2);
  });

  it("groups by repository with multi-repo + per-repo buckets", () => {
    const tasks = [
      task({ id: "a", repositories: ["org/r1", "org/r2"] }),
      task({ id: "b", repositoryPath: "org/r1" }),
      task({ id: "c", repositoryPath: "org/r2" }),
      task({ id: "d" }),
    ];
    const out = applyGroup(tasks, "repository");
    const labels = out.groups.map((g) => g.label);
    expect(labels).toContain("Multi-repo");
    expect(labels).toContain("org/r1");
    expect(labels).toContain("org/r2");
    expect(labels).toContain("Unassigned");
    expect(out.groups[0].label).toBe("Multi-repo");
  });

  it("merges unassigned into single repo group", () => {
    const tasks = [task({ id: "a", repositoryPath: "org/only" }), task({ id: "b" })];
    const out = applyGroup(tasks, "repository");
    expect(out.groups).toHaveLength(1);
    expect(out.groups[0].label).toBe("org/only");
    expect(out.groups[0].tasks).toHaveLength(2);
  });

  it("separates subtasks from root tasks", () => {
    const tasks = [
      task({ id: "parent", repositoryPath: "r" }),
      task({ id: "child", repositoryPath: "r", parentTaskId: "parent" }),
      task({ id: "orphan", repositoryPath: "r", parentTaskId: "ghost" }),
    ];
    const out = applyGroup(tasks, "repository");
    expect(out.groups[0].tasks.map((t) => t.id).sort()).toEqual(["orphan", "parent"]);
    expect(out.subTasksByParentId.get("parent")?.map((t) => t.id)).toEqual(["child"]);
  });

  it("groups by workflow / workflowStep / executorType / state", () => {
    const tasks = [
      task({ id: "a", workflowId: "wf1", workflowStepId: "s1", remoteExecutorType: "sprites" }),
      task({ id: "b", workflowId: "wf2", workflowStepId: "s2", remoteExecutorType: "local_pc" }),
    ];
    expect(applyGroup(tasks, "workflow").groups).toHaveLength(2);
    expect(applyGroup(tasks, "workflowStep").groups).toHaveLength(2);
    expect(applyGroup(tasks, "executorType").groups).toHaveLength(2);
  });

  it("buckets missing group-key value into Unassigned", () => {
    const tasks = [task({ id: "a", workflowId: "wf1" }), task({ id: "b" })];
    const out = applyGroup(tasks, "workflow");
    const labels = out.groups.map((g) => g.label);
    expect(labels).toContain("Unassigned");
  });

  it("labels workflow groups with workflowName when available", () => {
    const tasks = [
      task({ id: "a", workflowId: "wf1", workflowName: "Kanban" }),
      task({ id: "b", workflowId: "wf2", workflowName: "PR Reviews" }),
    ];
    const labels = applyGroup(tasks, "workflow")
      .groups.map((g) => g.label)
      .sort();
    expect(labels).toEqual(["Kanban", "PR Reviews"]);
  });

  it("labels workflowStep groups with workflowStepTitle when available", () => {
    const tasks = [
      task({ id: "a", workflowStepId: "s1", workflowStepTitle: "Backlog" }),
      task({ id: "b", workflowStepId: "s2", workflowStepTitle: "In Progress" }),
    ];
    const labels = applyGroup(tasks, "workflowStep")
      .groups.map((g) => g.label)
      .sort();
    expect(labels).toEqual(["Backlog", "In Progress"]);
  });
});

describe("applyView (integration)", () => {
  it("applies filter + sort + group in sequence", () => {
    const view: SidebarView = {
      id: "v",
      name: "v",
      filters: [{ id: "f1", dimension: "isPRReview", op: "is", value: false }],
      sort: { key: "updatedAt", direction: "desc" },
      group: "none",
      collapsedGroups: [],
    };
    const tasks = [
      task({ id: "a", isPRReview: true, updatedAt: "2026-04-05" }),
      task({ id: "b", updatedAt: "2026-04-01" }),
      task({ id: "c", updatedAt: "2026-04-10" }),
    ];
    const out = applyView(tasks, view);
    expect(out.groups).toHaveLength(1);
    expect(out.groups[0].tasks.map((t) => t.id)).toEqual(["c", "b"]);
  });
});

describe("default view semantics", () => {
  it("default view shows everything, grouped by repository", () => {
    const tasks = [
      task({
        id: "pr-open",
        prInfo: { number: 1, state: "Open" },
        state: "REVIEW",
        repositoryPath: "org/r",
      }),
      task({ id: "plain", state: "IN_PROGRESS", repositoryPath: "org/r" }),
      task({ id: "archived", isArchived: true, repositoryPath: "org/r" }),
    ];
    const out = applyView(tasks, DEFAULT_VIEW);
    const ids = out.groups.flatMap((g) => g.tasks.map((t) => t.id));
    expect(ids.sort()).toEqual(["archived", "plain", "pr-open"]);
  });
});
