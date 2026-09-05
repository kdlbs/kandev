import { describe, expect, it } from "vitest";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { mapOfficeTaskToTask } from "./map-office-task";

function baseOfficeTask(overrides: Partial<OfficeTask> = {}): OfficeTask {
  return {
    id: "task-1",
    workspaceId: "ws-1",
    identifier: "OFF-1",
    title: "Test task",
    status: "in_progress",
    priority: "medium",
    createdAt: "2026-08-01T09:00:00Z",
    updatedAt: "2026-08-01T09:00:00Z",
    ...overrides,
  };
}

describe("mapOfficeTaskToTask", () => {
  it("maps startedAt and completedAt through when the backend sends them", () => {
    const raw = baseOfficeTask({
      startedAt: "2026-08-01T10:00:00Z",
      completedAt: "2026-08-01T12:00:00Z",
    });

    const task = mapOfficeTaskToTask(raw);

    expect(task.startedAt).toBe("2026-08-01T10:00:00Z");
    expect(task.completedAt).toBe("2026-08-01T12:00:00Z");
  });

  it("leaves startedAt and completedAt undefined when the backend omits them", () => {
    const raw = baseOfficeTask();

    const task = mapOfficeTaskToTask(raw);

    expect(task.startedAt).toBeUndefined();
    expect(task.completedAt).toBeUndefined();
  });

  // The human assignee travels store -> detail Task through this mapper. A
  // dropped hop here is invisible to a component test that builds its own
  // Task, and would surface as a task that reads unassigned no matter what
  // the backend stored.
  it("carries the human assignee through, independently of the agent assignee", () => {
    const task = mapOfficeTaskToTask(
      baseOfficeTask({ assigneeUserId: "user-7", assigneeAgentProfileId: "agent-3" }),
    );

    expect(task.assigneeUserId).toBe("user-7");
    expect(task.assigneeAgentProfileId).toBe("agent-3");
  });

  it("always maps createdBy to an empty string (no source of truth yet)", () => {
    const task = mapOfficeTaskToTask(baseOfficeTask());

    expect(task.createdBy).toBe("");
  });

  // The detail page seeds its Task from the store-held OfficeTask before the
  // API GET resolves. Preserve rawStatus so execution indicators keep their
  // live/ready distinction during that initial render.
  it("carries rawStatus through when the store holds one", () => {
    const mapped = mapOfficeTaskToTask(baseOfficeTask({ status: "todo", rawStatus: "SCHEDULING" }));

    expect(mapped.rawStatus).toBe("SCHEDULING");
  });

  it("falls back to the canonical status when rawStatus is absent", () => {
    const mapped = mapOfficeTaskToTask(baseOfficeTask({ status: "in_progress" }));

    expect(mapped.rawStatus).toBe("in_progress");
  });
});
