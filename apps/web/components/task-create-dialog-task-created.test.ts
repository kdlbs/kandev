import { describe, expect, it, vi } from "vitest";
import { createTaskCreatedHandlerRegistry } from "./task-create-dialog-task-created";

describe("task-create completion handlers", () => {
  it("logs rejected async handlers and continues notifying other handlers", async () => {
    const registry = createTaskCreatedHandlerRegistry();
    const failure = new Error("plugin rejected");
    const firstHandler = vi.fn().mockRejectedValue(failure);
    const secondHandler = vi.fn();
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    registry.register(firstHandler);
    registry.register(secondHandler);

    registry.notify({ id: "async-task", workspace_id: "async-workspace" });
    await Promise.resolve();

    expect(error).toHaveBeenCalledExactlyOnceWith(
      "[plugins] Task-create completion handler failed",
      failure,
    );
    expect(secondHandler).toHaveBeenCalledExactlyOnceWith({
      id: "async-task",
      workspace_id: "async-workspace",
    });
  });

  it("passes a copied, immutable task identity", () => {
    const registry = createTaskCreatedHandlerRegistry();
    const handler = vi.fn();
    const task = { id: "task-1", workspace_id: "workspace-1" };
    registry.register(handler);

    registry.notify(task);
    task.id = "mutated";

    expect(handler).toHaveBeenCalledExactlyOnceWith({
      id: "task-1",
      workspace_id: "workspace-1",
    });
    expect(Object.isFrozen(handler.mock.calls[0]?.[0])).toBe(true);
  });
});
