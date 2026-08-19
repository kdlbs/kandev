import { describe, expect, it } from "vitest";
import { groupTasksByStatus } from "./task-board";
import type { OfficeTask, OfficeTaskStatus } from "@/lib/state/slices/office/types";

const COLUMN_STATUSES: OfficeTaskStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "in_review",
  "blocked",
  "done",
  "cancelled",
];

function task(id: string, status: string): OfficeTask {
  return {
    id,
    workspaceId: "workspace-1",
    identifier: id.toUpperCase(),
    title: id,
    // Runtime task.status can carry a raw backend enum value (e.g. "CREATED")
    // even though the type says OfficeTaskStatus — see normalize-status.ts.
    status: status as OfficeTaskStatus,
    priority: "none",
    createdAt: "2026-01-01T00:00:00.000Z",
    updatedAt: "2026-01-01T00:00:00.000Z",
  };
}

describe("groupTasksByStatus", () => {
  it("buckets a raw backend CREATED status into the todo column", () => {
    const created = task("created-task", "CREATED");

    const grouped = groupTasksByStatus([created], COLUMN_STATUSES);

    expect(grouped.get("todo")).toEqual([created]);
  });

  it("buckets WAITING_FOR_INPUT into the in_progress column", () => {
    const waiting = task("waiting-task", "WAITING_FOR_INPUT");

    const grouped = groupTasksByStatus([waiting], COLUMN_STATUSES);

    expect(grouped.get("in_progress")).toEqual([waiting]);
  });

  it("buckets FAILED into the blocked column", () => {
    const failed = task("failed-task", "FAILED");

    const grouped = groupTasksByStatus([failed], COLUMN_STATUSES);

    expect(grouped.get("blocked")).toEqual([failed]);
  });

  it("buckets an unknown/garbage status into the backlog column", () => {
    const garbage = task("garbage-task", "SOME_UNKNOWN_STATE");

    const grouped = groupTasksByStatus([garbage], COLUMN_STATUSES);

    expect(grouped.get("backlog")).toEqual([garbage]);
  });

  it("never drops a task: every input task lands in exactly one bucket", () => {
    const tasks = [
      task("a", "CREATED"),
      task("b", "TODO"),
      task("c", "SCHEDULING"),
      task("d", "IN_PROGRESS"),
      task("e", "WAITING_FOR_INPUT"),
      task("f", "REVIEW"),
      task("g", "BLOCKED"),
      task("h", "FAILED"),
      task("i", "COMPLETED"),
      task("j", "CANCELLED"),
      task("k", "garbage"),
    ];

    const grouped = groupTasksByStatus(tasks, COLUMN_STATUSES);

    const totalBucketed = [...grouped.values()].reduce((sum, list) => sum + list.length, 0);
    expect(totalBucketed).toBe(tasks.length);
  });

  it("still buckets already-canonical lowercase statuses correctly", () => {
    const todoTask = task("todo-task", "todo");

    const grouped = groupTasksByStatus([todoTask], COLUMN_STATUSES);

    expect(grouped.get("todo")).toEqual([todoTask]);
  });
});
