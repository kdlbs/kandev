import { afterEach, describe, expect, it, vi } from "vitest";
import { Virtualizer } from "@tanstack/react-virtual";
import { measureFileTreeElement, observeFileTreeRect } from "./file-tree-measurement";

function createVirtualizer(estimateSize: number): Virtualizer<HTMLDivElement, HTMLDivElement> {
  const instance = new Virtualizer<HTMLDivElement, HTMLDivElement>({
    count: 1,
    getScrollElement: () => null,
    estimateSize: () => estimateSize,
    scrollToFn: () => {},
    observeElementRect: () => {},
    observeElementOffset: () => {},
    getItemKey: () => "row-a",
  });
  instance.targetWindow = window;
  return instance;
}

function resizeEntry(element: HTMLDivElement, blockSize: number): ResizeObserverEntry {
  return {
    target: element,
    borderBoxSize: [{ blockSize, inlineSize: 320 }],
  } as unknown as ResizeObserverEntry;
}

describe("file-tree viewport measurements", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("waits for browser geometry, handles resize/hide, and disconnects on disposal", () => {
    const element = document.createElement("div");
    const readHeight = vi.fn(() => 400);
    Object.defineProperty(element, "offsetHeight", { get: readHeight });
    let publish: ResizeObserverCallback = () => {};
    const observe = vi.fn();
    const disconnect = vi.fn();
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(callback: ResizeObserverCallback) {
          publish = callback;
        }
        observe = observe;
        disconnect = disconnect;
        unobserve = disconnect;
      },
    );
    const instance = createVirtualizer(28);
    instance.scrollElement = element;
    instance.targetWindow = window;
    const onRect = vi.fn();
    const dispose = observeFileTreeRect(instance, onRect);
    expect(readHeight).not.toHaveBeenCalled();
    expect(onRect).not.toHaveBeenCalled();
    expect(observe).toHaveBeenCalledWith(element, { box: "border-box" });
    for (const height of [400, 240, 0, 600]) {
      publish([resizeEntry(element, height)], {} as ResizeObserver);
      expect(onRect).toHaveBeenLastCalledWith({ width: 320, height });
    }
    expect(readHeight).not.toHaveBeenCalled();
    dispose?.();
    expect(disconnect).toHaveBeenCalledOnce();
    onRect.mockClear();
    publish([resizeEntry(element, 900)], {} as ResizeObserver);
    expect(onRect).not.toHaveBeenCalled();
  });

  it("uses the library's synchronous fallback when ResizeObserver is unavailable", () => {
    vi.stubGlobal("ResizeObserver", undefined);
    const element = document.createElement("div");
    Object.defineProperties(element, {
      offsetWidth: { value: 320 },
      offsetHeight: { value: 400 },
    });
    const instance = createVirtualizer(28);
    instance.scrollElement = element;
    instance.targetWindow = window;
    const onRect = vi.fn();
    const dispose = observeFileTreeRect(instance, onRect);
    expect(onRect).toHaveBeenCalledWith({ width: 320, height: 400 });
    dispose?.();
  });
});

describe("file-tree row measurements", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("measures rows synchronously without ResizeObserver and protects hidden geometry", () => {
    vi.stubGlobal("ResizeObserver", undefined);
    const element = document.createElement("div");
    element.dataset.index = "0";
    let height = 52;
    const readHeight = vi.fn(() => height);
    Object.defineProperty(element, "offsetHeight", { get: readHeight });
    const virtualizer = createVirtualizer(28);
    virtualizer.itemSizeCache.set("row-a", 44);

    expect(measureFileTreeElement(element, undefined, virtualizer)).toBe(52);
    expect(readHeight).toHaveBeenCalledOnce();
    height = 0;
    expect(measureFileTreeElement(element, undefined, virtualizer)).toBe(44);
  });

  it.each([28, 44, 48])("mounts with positive %dpx geometry without forcing layout", (estimate) => {
    const element = document.createElement("div");
    element.dataset.index = "0";
    const readHeight = vi.fn(() => 0);
    Object.defineProperty(element, "offsetHeight", { get: readHeight });
    const virtualizer = createVirtualizer(estimate);

    expect(measureFileTreeElement(element, undefined, virtualizer)).toBe(estimate);
    expect(readHeight).not.toHaveBeenCalled();
    expect(measureFileTreeElement(element, resizeEntry(element, 52), virtualizer)).toBe(52);
  });

  it("retains a positive cached row size when a hidden row reports zero height", () => {
    const element = document.createElement("div");
    element.dataset.index = "0";
    const virtualizer = createVirtualizer(28);
    virtualizer.itemSizeCache.set("row-a", 32);

    expect(measureFileTreeElement(element, resizeEntry(element, 0), virtualizer)).toBe(32);
  });

  it.each([28, 44])(
    "uses the current estimate when no positive size exists (%dpx)",
    (estimateSize) => {
      const element = document.createElement("div");
      element.dataset.index = "0";
      const virtualizer = createVirtualizer(estimateSize);

      expect(measureFileTreeElement(element, resizeEntry(element, 0), virtualizer)).toBe(
        estimateSize,
      );
    },
  );

  it("accepts a later positive measurement after a cached fallback", () => {
    const element = document.createElement("div");
    element.dataset.index = "0";
    const virtualizer = createVirtualizer(28);
    virtualizer.itemSizeCache.set("row-a", 28);

    expect(measureFileTreeElement(element, resizeEntry(element, 48), virtualizer)).toBe(48);
  });
});
