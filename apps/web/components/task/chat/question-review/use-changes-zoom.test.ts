import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DiffRequest } from "@/lib/question-review/diff.worker";
import type { CompareMode } from "@/lib/question-review/compare";
import { CHANGE_FLASH_MS, useChangesZoom } from "./use-changes-zoom";

class FakeWorker {
  listeners = new Map<string, Set<(event: MessageEvent) => void>>();
  posted: DiffRequest[] = [];
  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)!.add(listener);
  }
  removeEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.get(type)?.delete(listener);
  }
  postMessage(request: DiffRequest) {
    this.posted.push(request);
  }
  terminate() {}
  respond(bounds: { x: number; y: number; width: number; height: number } | null) {
    const id = this.posted[this.posted.length - 1].id;
    const data = {
      id,
      ok: true,
      width: 4,
      height: 4,
      changed: bounds ? 2 : 0,
      total: 16,
      mask: new ArrayBuffer(64),
      bounds,
    };
    this.listeners.get("message")?.forEach((listener) => listener({ data } as MessageEvent));
  }
}

const mocks = vi.hoisted(() => ({ worker: null as FakeWorker | null }));
vi.mock("@/lib/question-review/diff-worker", () => ({ createPixelDiffWorker: () => mocks.worker }));

const a = new Blob(["a"]);
const b = new Blob(["b"]);
const bounds = { x: 1, y: 1, width: 2, height: 2 };

function setup(mode: CompareMode) {
  const zoom = vi.fn();
  const hook = renderHook(
    (props: { mode: CompareMode; pairKey: string }) =>
      useChangesZoom({ ...props, a, b, tolerance: 16, zoomToRect: zoom }),
    { initialProps: { mode, pairKey: "a:b:16" } },
  );
  return { zoom, hook };
}

beforeEach(() => {
  vi.useFakeTimers();
  mocks.worker = new FakeWorker();
});
afterEach(() => vi.useRealTimers());

describe("useChangesZoom", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.13
  it("computes on demand outside Difference, zooms, and flashes the outline", () => {
    const { zoom, hook } = setup("toggle");
    act(() => vi.advanceTimersByTime(200));
    expect(mocks.worker!.posted).toHaveLength(0);
    act(() => hook.result.current.trigger());
    expect(hook.result.current.status).toBe("computing");
    act(() => vi.advanceTimersByTime(150));
    act(() => mocks.worker!.respond(bounds));
    expect(zoom).toHaveBeenCalledWith(bounds);
    expect(hook.result.current.outline).toEqual(bounds);
    act(() => vi.advanceTimersByTime(CHANGE_FLASH_MS));
    expect(hook.result.current.outline).toBeNull();
    act(() => hook.result.current.trigger());
    expect(zoom).toHaveBeenCalledTimes(2);
    expect(mocks.worker!.posted).toHaveLength(1);
  });

  it("reports no changes without zooming", () => {
    const { zoom, hook } = setup("toggle");
    act(() => hook.result.current.trigger());
    act(() => vi.advanceTimersByTime(150));
    act(() => mocks.worker!.respond(null));
    expect(zoom).not.toHaveBeenCalled();
    expect(hook.result.current.status).toBe("none");
  });

  it("always outlines in Difference and forgets the request for a new pair", () => {
    const { hook } = setup("diff");
    act(() => vi.advanceTimersByTime(150));
    act(() => mocks.worker!.respond(bounds));
    expect(hook.result.current.outline).toEqual(bounds);
    hook.rerender({ mode: "toggle", pairKey: "a:c:16" });
    expect(hook.result.current.status).toBe("idle");
  });
});
