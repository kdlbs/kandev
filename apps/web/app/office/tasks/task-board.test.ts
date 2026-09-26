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

function task(id: string, status: OfficeTaskStatus): OfficeTask {
  return {
    id,
    workspaceId: "workspace-1",
    identifier: id.toUpperCase(),
    title: id,
    status,
    priority: "none",
    createdAt: "2026-01-01T00:00:00.000Z",
    updatedAt: "2026-01-01T00:00:00.000Z",
  };
}

// office.tasks.items is normalized to the canonical OfficeTaskStatus
// vocabulary at ingestion (office-slice.ts setTasks/appendTasks/
// patchTaskInStore), so groupTasksByStatus only ever sees canonical values —
// coverage for raw-backend-status mapping (e.g. "CREATED" -> "todo") lives in
// office-tasks.test.ts and normalize-status.test.ts instead.
describe("groupTasksByStatus", () => {
  it("buckets a task into its matching column", () => {
    const todoTask = task("todo-task", "todo");

    const grouped = groupTasksByStatus([todoTask], COLUMN_STATUSES);

    expect(grouped.get("todo")).toEqual([todoTask]);
  });

  it("buckets an in_progress task into the in_progress column", () => {
    const inProgress = task("in-progress-task", "in_progress");

    const grouped = groupTasksByStatus([inProgress], COLUMN_STATUSES);

    expect(grouped.get("in_progress")).toEqual([inProgress]);
  });

  it("buckets a blocked task into the blocked column", () => {
    const blocked = task("blocked-task", "blocked");

    const grouped = groupTasksByStatus([blocked], COLUMN_STATUSES);

    expect(grouped.get("blocked")).toEqual([blocked]);
  });

  it("drops a task whose status has no matching column", () => {
    const garbage = task("garbage-task", "SOME_UNKNOWN_STATE" as OfficeTaskStatus);

    const grouped = groupTasksByStatus([garbage], COLUMN_STATUSES);

    const totalBucketed = [...grouped.values()].reduce((sum, list) => sum + list.length, 0);
    expect(totalBucketed).toBe(0);
  });

  it("never drops a task with a canonical status: every input task lands in exactly one bucket", () => {
    const tasks: OfficeTask[] = [
      task("a", "backlog"),
      task("b", "todo"),
      task("c", "in_progress"),
      task("d", "in_review"),
      task("e", "blocked"),
      task("f", "done"),
      task("g", "cancelled"),
    ];

    const grouped = groupTasksByStatus(tasks, COLUMN_STATUSES);

    const totalBucketed = [...grouped.values()].reduce((sum, list) => sum + list.length, 0);
    expect(totalBucketed).toBe(tasks.length);
  });
});
