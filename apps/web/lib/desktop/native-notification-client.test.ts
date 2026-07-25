import { afterEach, describe, expect, it, vi } from "vitest";
import { nativeNotifications } from "./native-notification-client";

type TauriWindow = Window & {
  __TAURI_INTERNALS__?: {
    invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
  };
};

afterEach(() => {
  delete (window as TauriWindow).__TAURI_INTERNALS__;
});

describe("native notification client", () => {
  it("exposes distinct permission query and request commands", () => {
    const invoke = vi.fn(async (command: string) =>
      command === "get_native_notification_permission" ? "prompt" : "granted",
    );
    (window as TauriWindow).__TAURI_INTERNALS__ = { invoke };

    expect("permission" in nativeNotifications).toBe(true);
  });

  it("queries permission separately from requesting it", async () => {
    const invoke = vi.fn(async (command: string) =>
      command === "get_native_notification_permission" ? "denied" : "granted",
    );
    (window as TauriWindow).__TAURI_INTERNALS__ = { invoke };

    await expect(nativeNotifications.permission.get()).resolves.toBe("denied");
    await expect(nativeNotifications.permission.request()).resolves.toBe("granted");

    expect(invoke).toHaveBeenNthCalledWith(1, "get_native_notification_permission", undefined);
    expect(invoke).toHaveBeenNthCalledWith(2, "request_native_notification_permission", undefined);
  });
});
