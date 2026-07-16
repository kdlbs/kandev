import { describe, expect, it, vi } from "vitest";
import { createTauriEventTransport, type TauriEventInternals } from "./tauri-event-transport";

describe("Tauri desktop event transport", () => {
  it("is unavailable when Tauri internals are absent", () => {
    const transport = createTauriEventTransport(() => undefined);

    expect(transport.isAvailable()).toBe(false);
  });

  it("listens and unlistens through the scoped Tauri event commands", async () => {
    let nativeCallback: ((event: { payload: unknown }) => void) | undefined;
    const invoke = vi.fn(async (command: string) => (command.endsWith("|listen") ? 42 : undefined));
    const internals: TauriEventInternals = {
      invoke,
      transformCallback: vi.fn((callback) => {
        nativeCallback = callback;
        return 7;
      }),
    };
    const listener = vi.fn();
    const transport = createTauriEventTransport(() => internals);

    const unlisten = await transport.listen("kandev-desktop-v1-close-context", listener);
    nativeCallback?.({ payload: { command: "close" } });
    await unlisten();

    expect(invoke).toHaveBeenNthCalledWith(1, "plugin:event|listen", {
      event: "kandev-desktop-v1-close-context",
      handler: 7,
      target: { kind: "Any" },
    });
    expect(listener).toHaveBeenCalledWith({ command: "close" });
    expect(invoke).toHaveBeenNthCalledWith(2, "plugin:event|unlisten", {
      event: "kandev-desktop-v1-close-context",
      eventId: 42,
    });
  });
});
