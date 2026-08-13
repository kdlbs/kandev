import { describe, expect, it } from "vitest";
import { toKanbanTask } from "./map-task";

describe("toKanbanTask dependency projection", () => {
  it("maps both directions with per-entry detail", () => {
    const task = toKanbanTask({
      id: "b",
      workflow_step_id: "step-1",
      title: "B",
      blocked: true,
      blocked_reason: "pending",
      depends_on: [{ id: "a", title: "A", state: "TODO", status: "pending" }],
      blocks: [{ id: "c", title: "C", state: "TODO" }],
      start_when_unblocked: true,
    });
    expect(task.blocked).toBe(true);
    expect(task.blockedReason).toBe("pending");
    expect(task.dependsOn).toEqual([{ id: "a", title: "A", state: "TODO", status: "pending" }]);
    expect(task.blocks).toEqual([{ id: "c", title: "C", state: "TODO" }]);
    expect(task.startWhenUnblocked).toBe(true);
  });

  it("leaves the projection undefined when the payload mentions no dependency key", () => {
    // Lightweight task.updated events omit these entirely. Inventing empty lists
    // here erased real edges: such an event can insert the task into the board
    // store before boot hydration, and hydration then keeps the "fresher" WS copy.
    const task = toKanbanTask({ id: "a", workflow_step_id: "step-1", title: "A" });
    expect(task.blocked).toBeUndefined();
    expect(task.dependsOn).toBeUndefined();
    expect(task.blocks).toBeUndefined();
    expect(task.startWhenUnblocked).toBeUndefined();
  });

  it("treats an explicit empty list as a real 'no edges'", () => {
    const task = toKanbanTask({
      id: "a",
      workflow_step_id: "step-1",
      title: "A",
      blocked: false,
      depends_on: [],
      blocks: [],
    });
    expect(task.blocked).toBe(false);
    expect(task.dependsOn).toEqual([]);
    expect(task.blocks).toEqual([]);
  });

  it("clears blocked state when the payload reports it resolved", () => {
    const task = toKanbanTask({
      id: "b",
      workflow_step_id: "step-1",
      title: "B",
      blocked: false,
      depends_on: [{ id: "a", title: "A", status: "resolved" }],
    });
    expect(task.blocked).toBe(false);
    expect(task.blockedReason).toBeUndefined();
    // The edge survives resolution — it is history, not a live gate.
    expect(task.dependsOn).toHaveLength(1);
  });

  it("keeps a failed reason distinguishable from pending", () => {
    const task = toKanbanTask({
      id: "b",
      workflow_step_id: "step-1",
      title: "B",
      blocked: true,
      blocked_reason: "failed",
      depends_on: [{ id: "a", title: "A", status: "failed" }],
    });
    expect(task.blockedReason).toBe("failed");
    expect(task.dependsOn?.[0].status).toBe("failed");
  });
});
