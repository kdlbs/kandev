import { describe, expect, it } from "vitest";
import {
  getDestinationQueue,
  getWipQueueStatus,
  partitionWipTasks,
  compareWipQueueTasks,
  type WipQueueTask,
} from "./wip-queue";

function task(overrides: Partial<WipQueueTask> = {}): WipQueueTask {
  return {
    id: "task",
    workflowStepId: "review",
    queuedForStepId: "review",
    wipAdmitted: false,
    position: 0,
    priority: "medium",
    queuedAt: "2026-08-12T10:00:00Z",
    createdAt: "2026-08-12T09:00:00Z",
    ...overrides,
  };
}

describe("wip queue helper", () => {
  it("returns only destination-resident queued tasks in backend order", () => {
    const tasks = [
      task({ id: "low", priority: "low" }),
      task({ id: "critical", priority: "critical" }),
      task({ id: "admitted", wipAdmitted: true, queuedForStepId: undefined }),
      task({ id: "feeder", workflowStepId: "intake" }),
    ];

    expect(getDestinationQueue(tasks, "review").map((entry) => entry.task.id)).toEqual([
      "critical",
      "low",
    ]);
  });

  it("uses position, priority, queue time, creation time, then id", () => {
    const tasks = [
      task({ id: "z", position: 2 }),
      task({ id: "a", position: 1, priority: "low" }),
      task({ id: "critical", position: 1, priority: "critical" }),
      task({ id: "early", position: 1, priority: "medium", queuedAt: "2026-08-12T08:00:00Z" }),
    ];

    expect(getDestinationQueue(tasks, "review").map((entry) => entry.task.id)).toEqual([
      "critical",
      "early",
      "a",
      "z",
    ]);
  });

  it("returns one-based queue positions and keeps admitted counts separate", () => {
    const tasks = [
      task({ id: "queued-a" }),
      task({ id: "queued-b", position: 1 }),
      task({ id: "admitted", wipAdmitted: true, queuedForStepId: undefined }),
    ];
    const partition = partitionWipTasks(tasks, "review");

    expect(partition.admitted.map((item) => item.id)).toEqual(["admitted"]);
    expect(partition.queued.map((item) => item.id)).toEqual(["queued-a", "queued-b"]);
    expect(getWipQueueStatus(tasks[1], tasks, "review", "Review")).toEqual({
      position: 2,
      total: 2,
      destinationTitle: "Review",
    });
    expect(getWipQueueStatus(tasks[2], tasks, "review", "Review")).toBeUndefined();
  });

  it("keeps partitioned queued tasks in the same sorted order as their positions", () => {
    const tasks = [
      task({ id: "low", priority: "low", position: 2 }),
      task({ id: "critical", priority: "critical", position: 1 }),
      task({ id: "high", priority: "high", position: 1 }),
    ];

    expect(partitionWipTasks(tasks, "review").queued.map((item) => item.id)).toEqual([
      "critical",
      "high",
      "low",
    ]);
  });

  it("sorts every canonical priority in backend order", () => {
    const tasks = [
      task({ id: "low", priority: "low" }),
      task({ id: "medium", priority: "medium" }),
      task({ id: "high", priority: "high" }),
      task({ id: "critical", priority: "critical" }),
    ];

    expect(getDestinationQueue(tasks, "review").map((entry) => entry.task.id)).toEqual([
      "critical",
      "high",
      "medium",
      "low",
    ]);
  });

  it("orders missing timestamps deterministically without producing NaN", () => {
    const left = task({ id: "left", queuedAt: null, createdAt: null });
    const right = task({ id: "right", queuedAt: "not-a-date", createdAt: "not-a-date" });

    expect(compareWipQueueTasks(left, right)).toBeLessThan(0);
    expect(Number.isNaN(compareWipQueueTasks(left, right))).toBe(false);
  });
});
