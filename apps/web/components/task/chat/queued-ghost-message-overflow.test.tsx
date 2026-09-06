import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import { QueuedGhostMessage } from "./queued-ghost-message";

const PREVIEW_TEST_ID = "queue-entry-text";
const EXPAND_TEST_ID = "queue-entry-expand";

type Geometry = { scrollHeight: number; clientHeight: number };
type ResizeListener = EventListenerOrEventListenerObject;

let geometry: (element: HTMLElement) => Geometry;
let geometryReads: string[];
let throwOnScrollHeight = false;
let originalScrollHeight: PropertyDescriptor | undefined;
let originalClientHeight: PropertyDescriptor | undefined;
let originalResizeObserver: typeof globalThis.ResizeObserver | undefined;
let originalVisualViewport: VisualViewport | null;
let windowResizeListeners: ResizeListener[];
let originalWindowAddEventListener: typeof window.addEventListener;
let originalDocumentFonts: PropertyDescriptor | undefined;

class FontFaceSetStub extends EventTarget {
  readonly ready: Promise<FontFaceSet> = Promise.resolve(this as unknown as FontFaceSet);
}

class VisualViewportStub {
  readonly resizeListeners = new Set<ResizeListener>();
  invokeDuringRemove = false;

  addEventListener(type: string, listener: ResizeListener) {
    if (type === "resize") this.resizeListeners.add(listener);
  }

  removeEventListener(type: string, listener: ResizeListener) {
    if (type !== "resize") return;
    if (this.invokeDuringRemove) invokeListener(listener);
    this.resizeListeners.delete(listener);
  }

  emitResize() {
    for (const listener of [...this.resizeListeners]) invokeListener(listener);
  }
}

class ResizeObserverStub {
  static instances: ResizeObserverStub[] = [];
  readonly observed = new Set<Element>();
  disconnected = false;
  invokeDuringDisconnect = false;

  constructor(readonly callback: ResizeObserverCallback) {
    ResizeObserverStub.instances.push(this);
  }

  observe(target: Element) {
    this.observed.add(target);
  }

  unobserve(target: Element) {
    this.observed.delete(target);
  }

  disconnect() {
    if (this.invokeDuringDisconnect) this.emit([...this.observed]);
    this.disconnected = true;
    this.observed.clear();
  }

  emit(targets: Element[] = [...this.observed]) {
    const entries = targets.map((target) => ({ target }) as ResizeObserverEntry);
    this.callback(entries, this as unknown as ResizeObserver);
  }
}

let visualViewport: VisualViewportStub;

function invokeListener(listener: ResizeListener): void {
  const event = new Event("resize");
  if (typeof listener === "function") listener(event);
  else listener.handleEvent(event);
}

function disclosureFor(element: HTMLElement): HTMLElement | null {
  return (
    element
      .closest('[data-testid="queue-entry"]')
      ?.querySelector<HTMLElement>('[data-testid="queue-entry-expand"]') ?? null
  );
}

function installGeometryGetters(): void {
  Object.defineProperty(HTMLElement.prototype, "scrollHeight", {
    configurable: true,
    get(this: HTMLElement) {
      if (this.dataset.testid !== PREVIEW_TEST_ID) {
        return originalScrollHeight?.get?.call(this) ?? 0;
      }
      geometryReads.push(disclosureFor(this)?.style.display ?? "absent");
      if (throwOnScrollHeight) throw new Error("geometry failed");
      return geometry(this).scrollHeight;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get(this: HTMLElement) {
      if (this.dataset.testid !== PREVIEW_TEST_ID) {
        return originalClientHeight?.get?.call(this) ?? 0;
      }
      geometryReads.push(disclosureFor(this)?.style.display ?? "absent");
      return geometry(this).clientHeight;
    },
  });
}

function queuedEntry(content: string, id = "queue-1"): QueuedMessage {
  return {
    id,
    session_id: "session-1",
    task_id: "task-1",
    content,
    plan_mode: false,
    queued_at: "2026-09-04T00:00:00Z",
    queued_by: "user",
  };
}

function renderMessage(content: string, key = "queue-1") {
  return render(
    <QueuedGhostMessage
      key={key}
      entry={queuedEntry(content, key)}
      canEdit={false}
      canRemove
      onSave={async () => undefined}
      onRemove={() => undefined}
    />,
  );
}

function latestObserver(): ResizeObserverStub {
  const observer = ResizeObserverStub.instances.at(-1);
  expect(observer).toBeDefined();
  return observer!;
}

function emitWindowResize(): void {
  act(() => window.dispatchEvent(new Event("resize")));
}

beforeEach(() => {
  originalScrollHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "scrollHeight");
  originalClientHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientHeight");
  originalResizeObserver = globalThis.ResizeObserver;
  originalVisualViewport = window.visualViewport;
  originalWindowAddEventListener = window.addEventListener.bind(window);
  originalDocumentFonts = Object.getOwnPropertyDescriptor(document, "fonts");
  geometry = () => ({ scrollHeight: 44, clientHeight: 44 });
  geometryReads = [];
  throwOnScrollHeight = false;
  windowResizeListeners = [];
  ResizeObserverStub.instances = [];
  visualViewport = new VisualViewportStub();
  vi.stubGlobal("ResizeObserver", ResizeObserverStub);
  Object.defineProperty(window, "visualViewport", {
    configurable: true,
    value: visualViewport,
  });
  vi.spyOn(window, "addEventListener").mockImplementation((type, listener, options) => {
    if (type === "resize") windowResizeListeners.push(listener);
    originalWindowAddEventListener(type, listener, options);
  });
  installGeometryGetters();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (originalScrollHeight) {
    Object.defineProperty(HTMLElement.prototype, "scrollHeight", originalScrollHeight);
  } else {
    delete (HTMLElement.prototype as unknown as Record<string, unknown>).scrollHeight;
  }
  if (originalClientHeight) {
    Object.defineProperty(HTMLElement.prototype, "clientHeight", originalClientHeight);
  } else {
    delete (HTMLElement.prototype as unknown as Record<string, unknown>).clientHeight;
  }
  Object.defineProperty(globalThis, "ResizeObserver", {
    configurable: true,
    writable: true,
    value: originalResizeObserver,
  });
  Object.defineProperty(window, "visualViewport", {
    configurable: true,
    value: originalVisualViewport,
  });
  if (originalDocumentFonts) {
    Object.defineProperty(document, "fonts", originalDocumentFonts);
  } else {
    delete (document as unknown as Record<string, unknown>).fonts;
  }
});

describe("queued message rendered overflow measurement semantics", () => {
  // @covers AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.9
  it("offers disclosure for short content that renders beyond the collapsed cap", () => {
    geometry = () => ({ scrollHeight: 45, clientHeight: 44 });
    renderMessage("short rendered overflow");

    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();
  });

  // @covers AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.9
  it("omits disclosure for long content that exactly fits", () => {
    geometry = () => ({ scrollHeight: 44, clientHeight: 44 });
    renderMessage("x".repeat(120));

    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();
  });

  it("escapes self-induced overflow at the width available without disclosure", () => {
    geometry = () => ({ scrollHeight: 80, clientHeight: 44 });
    renderMessage("x".repeat(120));
    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();

    geometryReads = [];
    geometry = (element) => {
      const disclosure = disclosureFor(element);
      return {
        scrollHeight: !disclosure || disclosure.style.display === "none" ? 44 : 45,
        clientHeight: 44,
      };
    };
    emitWindowResize();

    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();
    expect(geometryReads).toEqual(["none", "none"]);
    emitWindowResize();
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();
  });

  it("hides disclosure before geometry reads and restores its exact inline display", () => {
    geometry = () => ({ scrollHeight: 80, clientHeight: 44 });
    renderMessage("x".repeat(120));
    const disclosure = screen.getByTestId(EXPAND_TEST_ID);
    disclosure.style.display = "inline-grid";

    geometryReads = [];
    emitWindowResize();

    expect(geometryReads).toEqual(["none", "none"]);
    expect(disclosure.style.display).toBe("inline-grid");
  });

  it("restores disclosure display when a geometry getter throws", () => {
    geometry = () => ({ scrollHeight: 80, clientHeight: 44 });
    renderMessage("x".repeat(120));
    const disclosure = screen.getByTestId(EXPAND_TEST_ID);
    disclosure.style.display = "inline-flex";
    throwOnScrollHeight = true;

    expect(() => latestObserver().emit()).toThrow("geometry failed");
    expect(disclosure.style.display).toBe("inline-flex");
  });

  it("retains the collapsed cap while expanded and collapses when natural content fits it", () => {
    geometry = () => ({ scrollHeight: 80, clientHeight: 44 });
    renderMessage("x".repeat(120));
    fireEvent.click(screen.getByTestId(EXPAND_TEST_ID));
    expect(screen.getByTestId(PREVIEW_TEST_ID).getAttribute("data-expanded")).toBe("true");

    geometry = () => ({ scrollHeight: 40, clientHeight: 40 });
    emitWindowResize();

    expect(screen.getByTestId(PREVIEW_TEST_ID).getAttribute("data-expanded")).toBe("false");
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();
  });
});

describe("queued message rendered overflow signals and fallbacks", () => {
  it("remeasures from additive observer, window, and visual viewport signals", () => {
    geometry = () => ({ scrollHeight: 44, clientHeight: 44 });
    renderMessage("x".repeat(120));
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();

    geometry = () => ({ scrollHeight: 45, clientHeight: 44 });
    emitWindowResize();
    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();

    geometry = () => ({ scrollHeight: 44, clientHeight: 44 });
    act(() => visualViewport.emitResize());
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();

    geometry = () => ({ scrollHeight: 45, clientHeight: 44 });
    act(() => latestObserver().emit());
    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();
  });

  it("remeasures when an asynchronous preview descendant loads", () => {
    geometry = () => ({ scrollHeight: 44, clientHeight: 44 });
    renderMessage("x".repeat(120));
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();

    geometry = () => ({ scrollHeight: 45, clientHeight: 44 });
    const image = document.createElement("img");
    screen.getByTestId(PREVIEW_TEST_ID).append(image);

    act(() => image.dispatchEvent(new Event("load")));

    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();
  });

  it("remeasures when web fonts settle", () => {
    Object.defineProperty(document, "fonts", {
      configurable: true,
      value: new FontFaceSetStub(),
    });
    geometry = () => ({ scrollHeight: 44, clientHeight: 44 });
    renderMessage("x".repeat(120));
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();

    geometry = () => ({ scrollHeight: 45, clientHeight: 44 });
    act(() => document.fonts.dispatchEvent(new Event("loadingdone")));

    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();
  });

  it("uses initial and viewport signals when ResizeObserver is unavailable or non-callable", () => {
    vi.stubGlobal("ResizeObserver", undefined);
    geometry = () => ({ scrollHeight: 44, clientHeight: 44 });
    const view = renderMessage("x".repeat(120));
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();

    geometry = () => ({ scrollHeight: 45, clientHeight: 44 });
    emitWindowResize();
    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();

    view.unmount();
    vi.stubGlobal("ResizeObserver", {});
    geometry = () => ({ scrollHeight: 45, clientHeight: 44 });
    renderMessage("short");
    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();

    geometry = () => ({ scrollHeight: 44, clientHeight: 44 });
    act(() => visualViewport.emitResize());
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();
  });

  it("ignores observer batches that do not contain the captured preview", () => {
    geometry = () => ({ scrollHeight: 80, clientHeight: 44 });
    renderMessage("x".repeat(120));
    geometryReads = [];

    act(() => latestObserver().emit([document.createElement("div")]));

    expect(geometryReads).toEqual([]);
    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();
  });
});

describe("queued message rendered overflow stale callback guards", () => {
  it("ignores callbacks from an earlier generation on the same preview element", () => {
    geometry = () => ({ scrollHeight: 80, clientHeight: 44 });
    const view = renderMessage("x".repeat(120));
    const preview = screen.getByTestId(PREVIEW_TEST_ID);
    const oldObserver = latestObserver();

    view.rerender(
      <QueuedGhostMessage
        key="queue-1"
        entry={queuedEntry("y".repeat(120))}
        canEdit={false}
        canRemove
        onSave={async () => undefined}
        onRemove={() => undefined}
      />,
    );
    expect(screen.getByTestId(PREVIEW_TEST_ID)).toBe(preview);
    geometryReads = [];
    act(() => oldObserver.emit([preview]));

    expect(geometryReads).toEqual([]);
    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();
  });

  it("ignores callbacks captured for a replaced preview element", () => {
    geometry = () => ({ scrollHeight: 80, clientHeight: 44 });
    const view = renderMessage("x".repeat(120), "first");
    const oldPreview = screen.getByTestId(PREVIEW_TEST_ID);
    const oldObserver = latestObserver();

    view.rerender(
      <QueuedGhostMessage
        key="second"
        entry={queuedEntry("y".repeat(120), "second")}
        canEdit={false}
        canRemove
        onSave={async () => undefined}
        onRemove={() => undefined}
      />,
    );
    expect(screen.getByTestId(PREVIEW_TEST_ID)).not.toBe(oldPreview);
    geometryReads = [];
    act(() => oldObserver.emit([oldPreview]));

    expect(geometryReads).toEqual([]);
    expect(screen.getByTestId(EXPAND_TEST_ID)).toBeTruthy();
  });

  it("invalidates callbacks before teardown and keeps every callback stale after unmount", () => {
    geometry = () => ({ scrollHeight: 80, clientHeight: 44 });
    const view = renderMessage("x".repeat(120));
    fireEvent.click(screen.getByTestId(EXPAND_TEST_ID));
    const observer = latestObserver();
    const preview = screen.getByTestId(PREVIEW_TEST_ID);
    const capturedWindowListeners = [...windowResizeListeners];
    const capturedViewportListeners = [...visualViewport.resizeListeners];
    observer.invokeDuringDisconnect = true;
    visualViewport.invokeDuringRemove = true;
    const originalRemove = window.removeEventListener.bind(window);
    vi.spyOn(window, "removeEventListener").mockImplementation((type, listener, options) => {
      if (type === "resize") invokeListener(listener);
      originalRemove(type, listener, options);
    });
    geometryReads = [];

    view.unmount();

    expect(geometryReads).toEqual([]);
    act(() => {
      observer.emit([preview]);
      capturedWindowListeners.forEach(invokeListener);
      capturedViewportListeners.forEach(invokeListener);
    });
    expect(geometryReads).toEqual([]);
    expect(screen.queryByTestId(PREVIEW_TEST_ID)).toBeNull();
  });
});

describe("queued message rendered overflow reset and styling", () => {
  it("resets disclosure and expansion when preview content disappears", () => {
    geometry = () => ({ scrollHeight: 80, clientHeight: 44 });
    const view = renderMessage("x".repeat(120));
    fireEvent.click(screen.getByTestId(EXPAND_TEST_ID));

    view.rerender(
      <QueuedGhostMessage
        entry={queuedEntry("")}
        canEdit={false}
        canRemove
        onSave={async () => undefined}
        onRemove={() => undefined}
      />,
    );
    expect(screen.queryByTestId(PREVIEW_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(EXPAND_TEST_ID)).toBeNull();

    geometry = () => ({ scrollHeight: 44, clientHeight: 44 });
    view.rerender(
      <QueuedGhostMessage
        entry={queuedEntry("visible again")}
        canEdit={false}
        canRemove
        onSave={async () => undefined}
        onRemove={() => undefined}
      />,
    );
    expect(screen.getByTestId(PREVIEW_TEST_ID).getAttribute("data-expanded")).toBe("false");
  });

  it("does not animate maximum-height changes", () => {
    geometry = () => ({ scrollHeight: 45, clientHeight: 44 });
    renderMessage("short");

    expect(screen.getByTestId(PREVIEW_TEST_ID).className).not.toContain("transition-[max-height]");
  });
});
