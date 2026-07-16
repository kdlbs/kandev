import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { dispatchToPluginWsHandlers } from "./plugin-bridge";

const ACTION = "task.created";

function cleanupPlugins(...pluginIds: string[]) {
  pluginIds.forEach((id) => pluginRegistry.unregisterPlugin(id));
}

describe("dispatchToPluginWsHandlers", () => {
  afterEach(() => cleanupPlugins("plugin-a", "plugin-b"));

  it("does nothing when no plugin registered a handler for the action", () => {
    expect(() => dispatchToPluginWsHandlers(ACTION, { foo: 1 })).not.toThrow();
  });

  it("forwards the payload to every handler registered for the action", () => {
    const handlerA = vi.fn();
    const handlerB = vi.fn();
    pluginRegistry.forPlugin("plugin-a").registerWsHandler(ACTION, handlerA);
    pluginRegistry.forPlugin("plugin-b").registerWsHandler(ACTION, handlerB);

    dispatchToPluginWsHandlers(ACTION, { taskId: "t-1" });

    expect(handlerA).toHaveBeenCalledWith({ taskId: "t-1" });
    expect(handlerB).toHaveBeenCalledWith({ taskId: "t-1" });
  });

  it("does not forward to a handler registered for a different action", () => {
    const handler = vi.fn();
    pluginRegistry.forPlugin("plugin-a").registerWsHandler("task.deleted", handler);

    dispatchToPluginWsHandlers(ACTION, { taskId: "t-1" });

    expect(handler).not.toHaveBeenCalled();
  });

  it("isolates a throwing handler so a sibling handler still runs", () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    const handlerB = vi.fn();
    pluginRegistry.forPlugin("plugin-a").registerWsHandler(ACTION, () => {
      throw new Error("boom");
    });
    pluginRegistry.forPlugin("plugin-b").registerWsHandler(ACTION, handlerB);

    expect(() => dispatchToPluginWsHandlers(ACTION, {})).not.toThrow();
    expect(handlerB).toHaveBeenCalled();

    consoleError.mockRestore();
  });
});
