import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useWindowControlsOverlay } from "./use-window-controls-overlay";

class FakeWindowControlsOverlay extends EventTarget implements KandevWindowControlsOverlay {
  visible = false;
  private rect = new DOMRect(0, 0, 0, 0);
  private publishBeforeNextListener: (() => void) | undefined;

  getTitlebarAreaRect(): DOMRectReadOnly {
    return this.rect;
  }

  publish(rect: DOMRect, visible = true): void {
    this.rect = rect;
    this.visible = visible;
    this.dispatchEvent(new Event("geometrychange"));
  }

  publishBeforeNextGeometryListener(rect: DOMRect, visible = true): void {
    this.publishBeforeNextListener = () => this.publish(rect, visible);
  }

  override addEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject | null,
    options?: boolean | AddEventListenerOptions,
  ): void {
    if (type === "geometrychange") {
      const publish = this.publishBeforeNextListener;
      this.publishBeforeNextListener = undefined;
      publish?.();
    }
    super.addEventListener(type, listener, options);
  }
}

function installOverlay(overlay: FakeWindowControlsOverlay): void {
  Object.defineProperty(navigator, "windowControlsOverlay", {
    configurable: true,
    value: overlay,
  });
}

describe("useWindowControlsOverlay", () => {
  afterEach(() => {
    Reflect.deleteProperty(navigator, "windowControlsOverlay");
  });

  it("tracks geometry and visibility changes reported after mount", () => {
    const overlay = new FakeWindowControlsOverlay();
    installOverlay(overlay);
    const { result } = renderHook(() => useWindowControlsOverlay());

    expect(result.current.visible).toBe(false);

    act(() => overlay.publish(new DOMRect(72, 0, 1448, 40)));
    expect(result.current).toEqual({ visible: true, x: 72, y: 0, width: 1448, height: 40 });

    act(() => overlay.publish(new DOMRect(0, 0, 0, 0), false));
    expect(result.current.visible).toBe(false);
  });

  it("removes the geometry listener on unmount", () => {
    const overlay = new FakeWindowControlsOverlay();
    const removeListener = vi.spyOn(overlay, "removeEventListener");
    installOverlay(overlay);

    const { unmount } = renderHook(() => useWindowControlsOverlay());
    unmount();

    expect(removeListener).toHaveBeenCalledWith("geometrychange", expect.any(Function));
  });

  it("re-reads geometry after registering the listener", () => {
    const overlay = new FakeWindowControlsOverlay();
    overlay.publishBeforeNextGeometryListener(new DOMRect(72, 0, 1448, 40));
    installOverlay(overlay);

    const { result } = renderHook(() => useWindowControlsOverlay());

    expect(result.current).toEqual({ visible: true, x: 72, y: 0, width: 1448, height: 40 });
  });
});
