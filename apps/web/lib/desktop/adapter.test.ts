import { describe, expect, it, vi } from "vitest";
import { createDesktopV1Adapter, type DesktopEventTransport } from "./adapter";
import { DESKTOP_NATIVE_EVENTS } from "./protocol";

describe("desktop v1 adapter", () => {
  it("maps adapter event names to the frozen native event literals", async () => {
    const listener = vi.fn();
    const transportListener = vi.fn();
    const unlisten = vi.fn();
    const transport: DesktopEventTransport = {
      isAvailable: () => true,
      listen: vi.fn(async (eventName, handler) => {
        expect(eventName).toBe(DESKTOP_NATIVE_EVENTS["close-context"]);
        transportListener.mockImplementation(handler);
        return unlisten;
      }),
    };

    const adapter = createDesktopV1Adapter(transport);
    const stop = await adapter.listen("close-context", listener);
    transportListener(undefined);

    expect(listener).toHaveBeenCalledOnce();
    stop();
    expect(unlisten).toHaveBeenCalledOnce();
  });

  it("does not attach listeners outside the desktop runtime", async () => {
    const transport: DesktopEventTransport = {
      isAvailable: () => false,
      listen: vi.fn(),
    };
    const adapter = createDesktopV1Adapter(transport);

    const stop = await adapter.listen("open-settings", vi.fn());
    stop();

    expect(adapter.isAvailable()).toBe(false);
    expect(transport.listen).not.toHaveBeenCalled();
  });

  it("preserves the transport receiver when checking availability", () => {
    const transport = {
      available: true,
      isAvailable() {
        return this.available;
      },
      listen: vi.fn(),
    };

    expect(createDesktopV1Adapter(transport).isAvailable()).toBe(true);
  });
});
