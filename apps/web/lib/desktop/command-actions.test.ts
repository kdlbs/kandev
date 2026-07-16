import { describe, expect, it, vi } from "vitest";
import { createDesktopCommandActions, subscribeDesktopCommandActions } from "./command-actions";
import type { DesktopV1Adapter } from "./adapter";

describe("desktop command actions", () => {
  it("routes Settings to the existing general settings page", () => {
    const navigate = vi.fn();
    const actions = createDesktopCommandActions({
      closeContext: vi.fn(),
      navigate,
      requestNewTask: vi.fn(),
    });

    actions["open-settings"]();

    expect(navigate).toHaveBeenCalledWith("/settings/general");
  });

  it("uses the shared New Task request instead of creating another dialog", () => {
    const requestNewTask = vi.fn();
    const actions = createDesktopCommandActions({
      closeContext: vi.fn(),
      navigate: vi.fn(),
      requestNewTask,
    });

    actions["new-task"]();

    expect(requestNewTask).toHaveBeenCalledOnce();
  });

  it("subscribes only the implemented contextual commands and disposes them", async () => {
    const listeners = new Map<string, () => void>();
    const unlisten = vi.fn();
    const adapter: DesktopV1Adapter = {
      isAvailable: () => true,
      listen: vi.fn(async (eventName, listener) => {
        listeners.set(eventName, listener as () => void);
        return unlisten;
      }),
    };
    const closeContext = vi.fn();
    const navigate = vi.fn();
    const requestNewTask = vi.fn();
    const actions = createDesktopCommandActions({
      closeContext,
      navigate,
      requestNewTask,
    });

    const stop = await subscribeDesktopCommandActions(adapter, actions);
    listeners.get("close-context")?.();
    listeners.get("open-settings")?.();
    listeners.get("new-task")?.();
    expect(closeContext).toHaveBeenCalledOnce();
    expect(navigate).toHaveBeenCalledWith("/settings/general");
    expect(requestNewTask).toHaveBeenCalledOnce();
    stop();
    expect(unlisten).toHaveBeenCalledTimes(3);
  });

  it("rolls back successful listeners when one registration fails", async () => {
    const firstUnlisten = vi.fn();
    const secondUnlisten = vi.fn();
    const adapter: DesktopV1Adapter = {
      isAvailable: () => true,
      listen: vi
        .fn()
        .mockResolvedValueOnce(firstUnlisten)
        .mockResolvedValueOnce(secondUnlisten)
        .mockRejectedValueOnce(new Error("listener unavailable")),
    };
    const actions = createDesktopCommandActions({
      closeContext: vi.fn(),
      navigate: vi.fn(),
      requestNewTask: vi.fn(),
    });

    await expect(subscribeDesktopCommandActions(adapter, actions)).rejects.toThrow(
      "listener unavailable",
    );

    expect(firstUnlisten).toHaveBeenCalledOnce();
    expect(secondUnlisten).toHaveBeenCalledOnce();
  });
});
