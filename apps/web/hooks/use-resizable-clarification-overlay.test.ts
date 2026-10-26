import { cleanup, renderHook, act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useResizableClarificationOverlay } from "./use-resizable-clarification-overlay";

const MIN_HEIGHT = 120;
let rafCallbacks: FrameRequestCallback[] = [];

class ResizeObserverStub {
  static instances: ResizeObserverStub[] = [];
  readonly observed = new Set<Element>();
  readonly callback: ResizeObserverCallback;
  disconnected = false;

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
    ResizeObserverStub.instances.push(this);
  }

  observe(element: Element) {
    this.observed.add(element);
  }

  unobserve(element: Element) {
    this.observed.delete(element);
  }

  disconnect() {
    this.disconnected = true;
    this.observed.clear();
  }

  trigger() {
    this.callback([], this as unknown as ResizeObserver);
  }
}

function setRect(element: Element, top: number, height: number, width = 800) {
  element.getBoundingClientRect = () =>
    ({
      x: 0,
      y: top,
      top,
      left: 0,
      right: width,
      bottom: top + height,
      width,
      height,
      toJSON: () => ({}),
    }) as DOMRect;
}

function makeRef<T extends HTMLElement>(element: T) {
  return { current: element } as React.RefObject<T | null>;
}

function dispatchMouseMove(clientY: number) {
  document.dispatchEvent(new MouseEvent("mousemove", { clientY, bubbles: true }));
}

function dispatchMouseUp() {
  document.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
}

function fakeReactMouseEvent(clientY: number) {
  return { clientY, preventDefault: () => {} } as unknown as React.MouseEvent;
}

function fixtureRefs() {
  const allocation = document.createElement("div");
  const reserved = document.createElement("div");
  document.body.append(allocation, reserved);
  setRect(allocation, 0, 800);
  setRect(reserved, 700, 100);
  return {
    allocation,
    reserved,
    allocationRef: makeRef(allocation),
    reservedRef: makeRef(reserved),
  };
}

function flushRaf() {
  const callbacks = rafCallbacks;
  rafCallbacks = [];
  act(() => callbacks.forEach((callback) => callback(0)));
}

function renderResizeHook(isPhone = false) {
  const refs = fixtureRefs();
  const view = renderHook(
    ({ phone }) =>
      useResizableClarificationOverlay({
        initialMaxHeightVh: 50,
        allocationRef: refs.allocationRef,
        reservedSiblingRefs: [refs.reservedRef],
        isPhone: phone,
      }),
    { initialProps: { phone: isPhone } },
  );
  return { ...view, ...refs, observer: ResizeObserverStub.instances.at(-1)! };
}

beforeEach(() => {
  ResizeObserverStub.instances = [];
  rafCallbacks = [];
  Object.defineProperty(window, "innerHeight", { configurable: true, writable: true, value: 1000 });
  Object.defineProperty(window, "visualViewport", { configurable: true, value: undefined });
  document.body.style.cursor = "";
  document.body.style.userSelect = "";
  vi.stubGlobal("ResizeObserver", ResizeObserverStub);
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    rafCallbacks.push(callback);
    return rafCallbacks.length;
  });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  document.body.style.cursor = "";
  document.body.style.userSelect = "";
});

describe("useResizableClarificationOverlay", () => {
  it("uses the smaller of the initial cap and the measured host, then expands to the host budget", () => {
    const { result } = renderResizeHook();

    expect(result.current.height).toBeNull();
    expect(result.current.maxHostHeight).toBe(700);
    expect(result.current.maxHeight).toBe(500);
    expect(result.current.canExpand).toBe(true);
    expect(result.current.canReset).toBe(false);

    act(() => result.current.expandHeight());

    expect(result.current.height).toBe(700);
    expect(result.current.maxHeight).toBe(700);
    expect(result.current.canExpand).toBe(false);
    expect(result.current.canReset).toBe(true);
  });
});

describe("useResizableClarificationOverlay expansion availability", () => {
  it("starts observing a panel mounted after the hook", () => {
    const { result } = renderResizeHook();
    const container = document.createElement("div");
    setRect(container, 0, 700);

    act(() => result.current.containerRef(container));

    expect(ResizeObserverStub.instances.some((observer) => observer.observed.has(container))).toBe(
      true,
    );
  });

  it("keeps Expand available after a partial manual drag", () => {
    const { result, rerender } = renderResizeHook();
    const container = document.createElement("div");
    setRect(container, 0, 300);
    act(() => result.current.containerRef(container));
    rerender({ phone: false });

    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    act(() => dispatchMouseMove(500));
    act(() => dispatchMouseUp());

    expect(result.current.height).toBe(300);
    expect(result.current.canExpand).toBe(true);
  });

  it("ignores negligible headroom when the rendered panel is within two pixels of the host", () => {
    const { result, rerender } = renderResizeHook();
    const container = document.createElement("div");
    setRect(container, 0, 699);
    act(() => result.current.containerRef(container));
    rerender({ phone: false });

    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    act(() => dispatchMouseMove(500));
    act(() => dispatchMouseUp());

    expect(result.current.height).toBe(699);
    expect(result.current.canExpand).toBe(false);

    setRect(container, 0, 697);
    const panelObserver = ResizeObserverStub.instances.find((observer) =>
      observer.observed.has(container),
    )!;
    act(() => panelObserver.trigger());
    expect(result.current.canExpand).toBe(true);
  });

  it("offers Expand for short auto-sized content below the host budget", () => {
    const allocation = document.createElement("div");
    document.body.append(allocation);
    setRect(allocation, 0, 400);
    const { result, rerender } = renderHook(() =>
      useResizableClarificationOverlay({
        initialMaxHeightVh: 50,
        allocationRef: makeRef(allocation),
        reservedHeightPx: 100,
      }),
    );
    const container = document.createElement("div");
    setRect(container, 0, 200);
    act(() => result.current.containerRef(container));
    rerender();

    expect(result.current.maxHostHeight).toBe(300);
    expect(result.current.maxHeight).toBe(300);
    expect(result.current.canExpand).toBe(true);
  });
});

describe("useResizableClarificationOverlay resize bounds", () => {
  it("clamps an upward drag to the measured host and a downward drag to the host-aware minimum", () => {
    const { result } = renderResizeHook();
    const container = document.createElement("div");
    setRect(container, 0, 300);
    act(() => result.current.containerRef(container));

    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(500)));
    act(() => dispatchMouseMove(0));
    expect(result.current.height).toBe(700);
    act(() => dispatchMouseUp());

    setRect(container, 0, 200);
    act(() => result.current.resizeHandleProps.onMouseDown(fakeReactMouseEvent(100)));
    act(() => dispatchMouseMove(500));
    expect(result.current.height).toBe(MIN_HEIGHT);
    act(() => dispatchMouseUp());
  });

  it("retains desired height when the host shrinks, then restores it when space returns", () => {
    const { result, rerender, allocation, reserved, observer } = renderResizeHook();
    const container = document.createElement("div");
    setRect(container, 0, 700);
    act(() => result.current.containerRef(container));
    rerender({ phone: false });
    act(() => result.current.expandHeight());
    expect(result.current.height).toBe(700);
    expect(result.current.canExpand).toBe(false);

    setRect(allocation, 0, 650);
    setRect(reserved, 550, 100);
    setRect(container, 0, 550);
    act(() => observer.trigger());
    flushRaf();
    expect(result.current.maxHostHeight).toBe(550);
    expect(result.current.height).toBe(700);
    expect(result.current.effectiveHeight).toBe(550);
    expect(result.current.maxHeight).toBe(550);

    setRect(allocation, 0, 1000);
    setRect(reserved, 900, 100);
    setRect(container, 0, 700);
    act(() => observer.trigger());
    flushRaf();
    expect(result.current.maxHostHeight).toBe(900);
    expect(result.current.height).toBe(700);
    expect(result.current.effectiveHeight).toBe(700);
    expect(result.current.canExpand).toBe(true);
  });
});

describe("useResizableClarificationOverlay host geometry", () => {
  it("remeasures against the visible viewport when the virtual keyboard contracts it", () => {
    const { allocationRef } = fixtureRefs();
    const listeners = new Map<string, EventListener>();
    const visualViewport = {
      offsetTop: 100,
      height: 700,
      addEventListener: (type: string, listener: EventListener) => listeners.set(type, listener),
      removeEventListener: (type: string) => listeners.delete(type),
    } as unknown as VisualViewport;
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: visualViewport,
    });
    const { result } = renderHook(() =>
      useResizableClarificationOverlay({
        initialMaxHeightVh: 50,
        allocationRef,
      }),
    );

    expect(result.current.maxHostHeight).toBe(700);
    Object.defineProperty(visualViewport, "height", { configurable: true, value: 300 });
    act(() => listeners.get("resize")?.(new Event("resize")));
    flushRaf();

    expect(result.current.maxHostHeight).toBe(300);
  });

  it("updates the automatic cap when viewport height changes but the host budget stays fixed", () => {
    const { result } = renderResizeHook();
    expect(result.current.maxHostHeight).toBe(700);
    expect(result.current.maxHeight).toBe(500);

    act(() => result.current.expandHeight());
    act(() => result.current.resetHeight());
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      writable: true,
      value: 900,
    });
    act(() => window.dispatchEvent(new Event("resize")));
    flushRaf();

    expect(result.current.maxHostHeight).toBe(700);
    expect(result.current.maxHeight).toBe(450);
  });

  it("measures an anchor from its top and keeps a smaller-than-minimum host usable", () => {
    const { allocation, reserved } = fixtureRefs();
    const anchor = document.createElement("div");
    document.body.append(anchor);
    setRect(allocation, 0, 1000);
    setRect(reserved, 250, 40);
    setRect(anchor, 250, 40);
    const { result } = renderHook(() =>
      useResizableClarificationOverlay({
        initialMaxHeightVh: 50,
        allocationRef: makeRef(allocation),
        anchorRef: makeRef(anchor),
        reservedSiblingRefs: [makeRef(reserved)],
      }),
    );

    expect(result.current.maxHostHeight).toBe(710);
    expect(result.current.minHeight).toBe(MIN_HEIGHT);

    setRect(allocation, 0, 340);
    setRect(anchor, 250, 40);
    setRect(reserved, 250, 40);
    act(() => window.dispatchEvent(new Event("resize")));
    flushRaf();

    expect(result.current.maxHostHeight).toBe(50);
    expect(result.current.minHeight).toBe(50);
    expect(result.current.canExpand).toBe(false);
  });

  it("keeps phone and desktop height preferences separate and resets only the active one", () => {
    const refs = fixtureRefs();
    const { result, rerender } = renderHook(
      ({ isPhone }) =>
        useResizableClarificationOverlay({
          initialMaxHeightVh: 50,
          allocationRef: refs.allocationRef,
          reservedSiblingRefs: [refs.reservedRef],
          isPhone,
        }),
      { initialProps: { isPhone: false } },
    );

    act(() => result.current.expandHeight());
    expect(result.current.height).toBe(700);

    rerender({ isPhone: true });
    expect(result.current.height).toBeNull();
    act(() => result.current.expandHeight());
    expect(result.current.height).toBe(700);
    act(() => result.current.resetHeight());
    expect(result.current.height).toBeNull();

    rerender({ isPhone: false });
    expect(result.current.height).toBe(700);
    act(() => result.current.resetAllHeights());
    expect(result.current.height).toBeNull();
  });

  it("observes explicit host refs and releases observers on unmount", () => {
    const view = renderResizeHook();
    expect(view.observer.observed).toEqual(new Set([view.allocation, view.reserved]));
    view.unmount();
    expect(view.observer.disconnected).toBe(true);
  });
});
