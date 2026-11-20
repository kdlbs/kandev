import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { PreviewRegionView } from "@/hooks/use-preview-capture";
import { useRegionDrag } from "./use-region-drag";

const initialRect = { x: 20, y: 30, width: 50, height: 40 };

function region(rect = initialRect): PreviewRegionView {
  return {
    id: "region-1",
    index: 0,
    rect,
    pending: false,
    busy: false,
    drifted: false,
    number: 1,
  };
}

function pointer(target: EventTarget, type: string, pointerId: number, x: number, y: number) {
  const event = new Event(type);
  Object.assign(event, { pointerId, clientX: x, clientY: y });
  target.dispatchEvent(event);
}

function drag(
  onHandlePointerDown: ReturnType<typeof useRegionDrag>["onHandlePointerDown"],
  pointerId: number,
  delta: { x: number; y: number },
) {
  const target = new EventTarget();
  act(() =>
    onHandlePointerDown(0, "move", {
      currentTarget: target,
      pointerId,
      clientX: 100,
      clientY: 100,
      preventDefault: vi.fn(),
    } as unknown as React.PointerEvent<HTMLElement>),
  );
  act(() => pointer(target, "pointermove", pointerId, 100 + delta.x, 100 + delta.y));
  act(() => pointer(target, "pointerup", pointerId, 100 + delta.x, 100 + delta.y));
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => (resolve = done));
  return { promise, resolve };
}

function useDrag(
  regions: PreviewRegionView[],
  onRecapture: (id: string, rect: PreviewRegionView["rect"]) => Promise<boolean>,
) {
  return useRegionDrag({
    regions,
    bounds: () => ({ width: 400, height: 400 }),
    onOpen: vi.fn(),
    onRecapture,
  });
}

describe("useRegionDrag", () => {
  it("returns control to the live layout after recapture when the page scrolls", async () => {
    const recapture = deferred<boolean>();
    const { result, rerender } = renderHook(
      ({ regions }) => useDrag(regions, () => recapture.promise),
      { initialProps: { regions: [region()] } },
    );

    drag(result.current.onHandlePointerDown, 1, { x: 20, y: 30 });
    expect(result.current.regions[0].rect).toEqual({ x: 40, y: 60, width: 50, height: 40 });

    const scrolledLayout = region({ x: 42, y: 4, width: 50, height: 40 });
    rerender({ regions: [scrolledLayout] });
    await act(async () => recapture.resolve(true));

    expect(result.current.regions[0].rect).toEqual(scrolledLayout.rect);
  });

  it("does not let an earlier recapture replace a newer drag", async () => {
    const first = deferred<boolean>();
    const second = deferred<boolean>();
    const recapture = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    const { result } = renderHook(() => useDrag([region()], recapture));

    drag(result.current.onHandlePointerDown, 1, { x: 20, y: 30 });
    drag(result.current.onHandlePointerDown, 2, { x: 40, y: 20 });
    expect(result.current.regions[0].rect).toEqual({ x: 60, y: 50, width: 50, height: 40 });

    await act(async () => first.resolve(true));
    expect(result.current.regions[0].rect).toEqual({ x: 60, y: 50, width: 50, height: 40 });

    await act(async () => second.resolve(true));
    expect(result.current.regions[0].rect).toEqual(initialRect);
  });
});
