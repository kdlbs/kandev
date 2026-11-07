import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { DiffRequest } from "@/lib/question-review/diff.worker";
import { useReviewDiff } from "./use-review-diff";

class FakeWorker {
  listeners = new Map<string, Set<(event: MessageEvent) => void>>();
  posted: DiffRequest[] = [];
  terminated = false;
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
  terminate() {
    this.terminated = true;
  }
  respond(data: unknown) {
    this.listeners.get("message")?.forEach((listener) => listener({ data } as MessageEvent));
  }
}

const mocks = vi.hoisted(() => ({ worker: null as FakeWorker | null }));
vi.mock("@/lib/question-review/diff-worker", () => ({
  createPixelDiffWorker: () => mocks.worker,
}));

const a = new Blob(["a"]);
const b = new Blob(["b"]);

beforeEach(() => {
  vi.useFakeTimers();
  mocks.worker = new FakeWorker();
});
afterEach(() => vi.useRealTimers());

function ready(id: number, changed: number) {
  return { id, ok: true, width: 2, height: 1, changed, total: 2, mask: new ArrayBuffer(8) };
}

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.6
it("computes in the worker, debounces tolerance, and ignores stale results", () => {
  const { result, rerender } = renderHook(({ tolerance }) => useReviewDiff(true, a, b, tolerance), {
    initialProps: { tolerance: 16 },
  });
  expect(result.current.status).toBe("computing");
  act(() => vi.advanceTimersByTime(150));
  const worker = mocks.worker!;
  expect(worker.posted).toHaveLength(1);
  rerender({ tolerance: 20 });
  act(() => vi.advanceTimersByTime(100));
  rerender({ tolerance: 24 });
  act(() => vi.advanceTimersByTime(150));
  expect(worker.posted.map((request) => request.tolerance)).toEqual([16, 24]);
  expect(worker.posted[0].pairKey).toBe(worker.posted[1].pairKey);
  act(() => worker.respond(ready(worker.posted[0].id, 9)));
  expect(result.current.status).toBe("computing");
  act(() => worker.respond(ready(worker.posted[1].id, 1)));
  expect(result.current).toMatchObject({ status: "ready", result: { changed: 1, total: 2 } });
});

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.9
it("reports unavailable when the worker cannot run or fails", () => {
  mocks.worker = null;
  const { result } = renderHook(() => useReviewDiff(true, a, b, 16));
  expect(result.current.status).toBe("unavailable");

  mocks.worker = new FakeWorker();
  const failing = renderHook(() => useReviewDiff(true, a, b, 16));
  act(() => vi.advanceTimersByTime(150));
  act(() => mocks.worker!.respond({ id: mocks.worker!.posted[0].id, ok: false }));
  expect(failing.result.current.status).toBe("unavailable");
});

it("stays idle when disabled and terminates the worker on unmount", () => {
  const { result, rerender, unmount } = renderHook(
    ({ enabled }) => useReviewDiff(enabled, a, b, 16),
    { initialProps: { enabled: false } },
  );
  expect(result.current.status).toBe("idle");
  rerender({ enabled: true });
  const worker = mocks.worker!;
  unmount();
  expect(worker.terminated).toBe(true);
});
