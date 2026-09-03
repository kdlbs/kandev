import { describe, expect, it } from "vitest";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskMR } from "@/lib/types/gitlab";
import type { TaskPR } from "@/lib/types/github";
import {
  changeRequestNumbers,
  compareDependencyCandidates,
  dependencyOptionValue,
} from "./task-create-dialog-dependency-search";

type Task = KanbanState["tasks"][number];

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: "task-1",
    workflowId: "workflow-1",
    workflowStepId: "step-1",
    title: "Task title",
    position: 0,
    isArchived: false,
    ...overrides,
  };
}

function mr(mrIid: number, overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    task_id: "task-1",
    mr_iid: mrIid,
    ...overrides,
  } as TaskMR;
}

function pr(prNumber: number, overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: `pr-assoc-${prNumber}`,
    task_id: "task-1",
    pr_number: prNumber,
    ...overrides,
  } as TaskPR;
}

describe("changeRequestNumbers", () => {
  it("returns the GitHub PR number from statusSummary", () => {
    const t = task({ statusSummary: { pull_request: { number: 3295, state: "open" } } as never });
    expect(changeRequestNumbers(t, [])).toEqual([3295]);
  });

  it("returns GitLab MR numbers", () => {
    const t = task();
    expect(changeRequestNumbers(t, [mr(12), mr(7)])).toEqual([7, 12]);
  });

  it("merges and de-duplicates GitHub and GitLab numbers, ascending", () => {
    const t = task({ statusSummary: { pull_request: { number: 12, state: "open" } } as never });
    expect(changeRequestNumbers(t, [mr(12), mr(7)])).toEqual([7, 12]);
  });

  it("returns an empty array when neither source has a number", () => {
    const t = task();
    expect(changeRequestNumbers(t, [])).toEqual([]);
  });

  it("includes every linked GitHub PR number for a multi-repo task, de-duplicated with statusSummary", () => {
    const t = task({ statusSummary: { pull_request: { number: 1001, state: "open" } } as never });
    expect(changeRequestNumbers(t, [], [pr(1001), pr(1002)])).toEqual([1001, 1002]);
  });

  it("merges GitHub PR, GitLab MR, and statusSummary numbers, ascending", () => {
    const t = task({ statusSummary: { pull_request: { number: 12, state: "open" } } as never });
    expect(changeRequestNumbers(t, [mr(7)], [pr(20)])).toEqual([7, 12, 20]);
  });
});

describe("dependencyOptionValue", () => {
  it("includes the title, id, and both '#N' and bare 'N' forms for each number", () => {
    const t = task({ id: "task-1", title: "Fix login" });
    const value = dependencyOptionValue(t, [3295]);
    expect(value).toContain("Fix login");
    expect(value).toContain("task-1");
    expect(value).toContain("#3295");
    expect(value).toContain("3295");
  });

  it("has no change-request tokens when there are no numbers", () => {
    const t = task({ id: "task-1", title: "Fix login" });
    expect(dependencyOptionValue(t, [])).toBe("Fix login task-1");
  });
});

const EARLY = "2026-01-01T00:00:00Z";
const LATE = "2026-02-01T00:00:00Z";

describe("compareDependencyCandidates", () => {
  it("orders by updatedAt descending", () => {
    const a = task({ id: "a", updatedAt: EARLY });
    const b = task({ id: "b", updatedAt: LATE });
    expect(compareDependencyCandidates(a, b)).toBeGreaterThan(0);
    expect(compareDependencyCandidates(b, a)).toBeLessThan(0);
  });

  it("uses createdAt to break equal updatedAt timestamps", () => {
    const a = task({ id: "a", updatedAt: EARLY, createdAt: LATE });
    const b = task({ id: "b", updatedAt: EARLY, createdAt: EARLY });
    expect(compareDependencyCandidates(a, b)).toBeLessThan(0);
  });

  it("falls back to createdAt descending when updatedAt is missing", () => {
    const a = task({ id: "a", createdAt: EARLY });
    const b = task({ id: "b", createdAt: LATE });
    expect(compareDependencyCandidates(a, b)).toBeGreaterThan(0);
  });

  it("falls back to title when both timestamps are missing", () => {
    const a = task({ id: "a", title: "Bravo" });
    const b = task({ id: "b", title: "Alpha" });
    expect(compareDependencyCandidates(a, b)).toBeGreaterThan(0);
  });

  it("treats a task with a timestamp as more recent than one without", () => {
    const a = task({ id: "a", updatedAt: EARLY });
    const b = task({ id: "b" });
    expect(compareDependencyCandidates(a, b)).toBeLessThan(0);
  });

  it("falls back to createdAt when updatedAt is malformed", () => {
    const a = task({ id: "a", updatedAt: "not-a-date", createdAt: EARLY });
    const b = task({ id: "b", updatedAt: LATE });
    expect(compareDependencyCandidates(a, b)).toBeGreaterThan(0);
  });

  it("falls back to title when both timestamps are malformed", () => {
    const a = task({ id: "a", title: "Bravo", updatedAt: "not-a-date" });
    const b = task({ id: "b", title: "Alpha", createdAt: "also-not-a-date" });
    expect(compareDependencyCandidates(a, b)).toBeGreaterThan(0);
  });
});
