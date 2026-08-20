import { describe, expect, it } from "vitest";
import {
  canonicalStatusesToBackend,
  normalizeOfficeTask,
  normalizeTaskStatus,
} from "./office-task-normalize";
import type { OfficeTask, OfficeTaskStatus } from "@/lib/state/slices/office/types";

function makeTask(status: string): OfficeTask {
  return {
    id: "task-1",
    workspaceId: "ws-1",
    identifier: "OFC-1",
    title: "Test task",
    status: status as OfficeTask["status"],
    priority: "medium",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

describe("normalizeTaskStatus", () => {
  it("returns backlog for missing input", () => {
    expect(normalizeTaskStatus(undefined)).toBe("backlog");
    expect(normalizeTaskStatus(null)).toBe("backlog");
    expect(normalizeTaskStatus("")).toBe("backlog");
  });

  it("maps backend aliases to canonical Office statuses", () => {
    const aliases: Record<string, OfficeTaskStatus> = {
      TODO: "todo",
      CREATED: "todo",
      SCHEDULING: "todo",
      IN_PROGRESS: "in_progress",
      WAITING_FOR_INPUT: "in_progress",
      REVIEW: "in_review",
      IN_REVIEW: "in_review",
      BLOCKED: "blocked",
      FAILED: "blocked",
      COMPLETED: "done",
      DONE: "done",
      CANCELLED: "cancelled",
      CANCELED: "cancelled",
      BACKLOG: "backlog",
    };
    for (const [raw, canonical] of Object.entries(aliases)) {
      expect(normalizeTaskStatus(raw)).toBe(canonical);
    }
  });

  it("falls back to backlog for unknown statuses", () => {
    expect(normalizeTaskStatus("UNKNOWN_STATE")).toBe("backlog");
  });
});

describe("normalizeOfficeTask", () => {
  it("normalizes a wire task and preserves its raw status", () => {
    const task = normalizeOfficeTask(makeTask("CREATED"));
    expect(task.status).toBe("todo");
    expect(task.rawStatus).toBe("CREATED");
  });

  it("normalizes nested children", () => {
    const task = normalizeOfficeTask({
      ...makeTask("TODO"),
      children: [makeTask("REVIEW")],
    });
    expect(task.children?.[0].status).toBe("in_review");
    expect(task.children?.[0].rawStatus).toBe("REVIEW");
  });
});

describe("canonicalStatusesToBackend", () => {
  it("expands canonical statuses to backend aliases", () => {
    expect(canonicalStatusesToBackend(["todo", "done"])).toEqual([
      "TODO",
      "CREATED",
      "SCHEDULING",
      "COMPLETED",
      "DONE",
    ]);
  });
});
