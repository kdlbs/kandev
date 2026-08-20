import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useLazyLoadSentinel } from "./use-lazy-load-sentinel";

type ObserverRecord = {
  callback: IntersectionObserverCallback;
  instance: IntersectionObserver;
  options?: IntersectionObserverInit;
  targets: Element[];
  unobserved: Element[];
  disconnected: boolean;
};

const records: ObserverRecord[] = [];

/** IntersectionObserver stub: records every instance so tests can fire
 * intersections and assert observe/unobserve/disconnect calls. Duplicate
 * observe of the same target is a no-op, like the browser. */
class MockIntersectionObserver implements IntersectionObserver {
  readonly root: Element | Document | null = null;
  readonly rootMargin: string;
  readonly thresholds: readonly number[] = [];
  private readonly record: ObserverRecord;

  constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
    this.rootMargin = options?.rootMargin ?? "0px";
    this.record = {
      callback,
      instance: this,
      options,
      targets: [],
      unobserved: [],
      disconnected: false,
    };
    records.push(this.record);
  }

  disconnect() {
    this.record.disconnected = true;
  }

  observe(target: Element) {
    if (!this.record.targets.includes(target)) this.record.targets.push(target);
  }

  takeRecords(): IntersectionObserverEntry[] {
    return [];
  }

  unobserve(target: Element) {
    this.record.unobserved.push(target);
  }
}

/** Fires an intersection callback for the FIRST recorded observer. */
function fire(record: ObserverRecord, isIntersecting: boolean, target: Element) {
  act(() => {
    record.callback([{ isIntersecting, target } as IntersectionObserverEntry], record.instance);
  });
}

function makeScrollRef() {
  return { current: document.createElement("div") } as React.RefObject<HTMLDivElement | null>;
}

beforeEach(() => {
  records.length = 0;
  vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("useLazyLoadSentinel", () => {
  it("defaults exactly to the transcript margin, no re-arm, and no join", () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    const record = records[0];
    expect(record.options?.rootMargin).toBe("200px 0px 0px 0px");
    expect(record.targets).toContain(node);

    fire(record, true, node);
    expect(loadMore).toHaveBeenCalledTimes(1);
  });

  it("fires loadMore on intersection when eligible", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);
  });

  it("never fires or joins while blocked, even with joinInFlightWhileLoading", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, true, true, loadMore, {
        joinInFlightWhileLoading: true,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).not.toHaveBeenCalled();
  });

  it("fires during an in-flight load only when joinInFlightWhileLoading is enabled", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, true, loadMore),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).not.toHaveBeenCalled();

    const { result: joined } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, true, loadMore, {
        joinInFlightWhileLoading: true,
      }),
    );
    const node2 = document.createElement("div");
    act(() => joined.current.sentinelRef(node2));
    fire(records[1], true, node2);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);
  });
});

describe("useLazyLoadSentinel — re-arm, disarm, and stale completions", () => {
  it("re-arms only when enabled: unobserves before loading and re-observes after a positive result", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        joinInFlightWhileLoading: true,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    // The sentinel was unobserved before the await; the load resolved
    // positively, so it is re-observed (still current).
    expect(records[0].unobserved).toContain(node);
    await act(async () => {});
    expect(records[0].targets.filter((t) => t === node).length).toBeGreaterThanOrEqual(1);

    // Still intersecting: the next page auto-loads without a scroll-away.
    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(2);
  });

  it("does not re-arm after a positive result when rearmWhileIntersecting is false", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));
    const observeCallsBefore = records[0].targets.length;

    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);
    expect(records[0].unobserved).not.toContain(node);
    expect(records[0].targets.length).toBe(observeCallsBefore);
  });

  it("disarms after a zero-result load, arms on an observed exit, and retries on the next re-entry", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 0);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        joinInFlightWhileLoading: true,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);

    // Disarmed: a still-true intersection must not retry.
    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);

    // Observed exit arms; the next re-entry retries.
    fire(records[0], false, node);
    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(2);
  });
});

describe("useLazyLoadSentinel — failure recovery and stale completions", () => {
  it("permits an onUserGesture retry while disarmed and still intersecting", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 0);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        joinInFlightWhileLoading: true,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);

    // Still intersecting (short content cannot scroll away): the gesture
    // retries through the same user-gesture path.
    act(() => result.current.onUserGesture());
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(2);
  });

  it("re-arms after a successful gesture retry so the next page loads without another gesture", async () => {
    const scrollRef = makeScrollRef();
    // First attempt fails (zero rows); the gesture retry succeeds.
    const loadMore = vi.fn().mockResolvedValueOnce(0).mockResolvedValueOnce(20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        joinInFlightWhileLoading: true,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1); // zero result → disarmed

    // Successful gesture retry clears the disarmed state.
    act(() => result.current.onUserGesture());
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(2);

    // The re-observed sentinel is armed: a true intersection loads the next
    // page WITHOUT another gesture.
    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(3);
  });
});

describe("useLazyLoadSentinel — stale observers", () => {
  it("does not disarm the replacement observer from a stale zero completion", async () => {
    const scrollRef = makeScrollRef();
    let resolveOld: (value: number) => void = () => {};
    const firstLoadMore = vi.fn(
      () =>
        new Promise<number>((resolve) => {
          resolveOld = resolve;
        }),
    );
    const { result, rerender } = renderHook(
      ({ loadMore }: { loadMore: () => Promise<number> }) =>
        useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
          rearmWhileIntersecting: true,
          joinInFlightWhileLoading: true,
        }),
      { initialProps: { loadMore: firstLoadMore } },
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    // Re-render replaces loadMore → the observer is recreated while the old
    // load is still in flight.
    const secondLoadMore = vi.fn(async () => 20);
    rerender({ loadMore: secondLoadMore });
    // The OLD load settles with a zero result: it must not disarm the NEW
    // observer (its observer identity is stale).
    await act(async () => {
      resolveOld(0);
    });
    expect(records[1].disconnected).toBe(false);

    // The new observer still fires on intersection (armed, not disarmed).
    fire(records[1], true, node);
    await act(async () => {});
    expect(secondLoadMore).toHaveBeenCalledTimes(1);
  });

  it("disarms after a rejected load without looping", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => {
      throw new Error("boom");
    });
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        joinInFlightWhileLoading: true,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);

    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);

    fire(records[0], false, node);
    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(2);
  });
});

describe("useLazyLoadSentinel — stale completions", () => {
  it("ignores a stale positive completion after unmount (no re-arm)", async () => {
    const scrollRef = makeScrollRef();
    let resolveLoad: (value: number) => void = () => {};
    const loadMore = vi.fn(
      () =>
        new Promise<number>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const { result, unmount } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        joinInFlightWhileLoading: true,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    const observesBefore = records[0].targets.length;
    unmount();
    await act(async () => {
      resolveLoad(20);
    });
    // No re-observe after unmount: the target list is unchanged.
    expect(records[0].targets.length).toBe(observesBefore);
  });

  it("ignores a stale positive completion after observer cleanup (no re-arm)", async () => {
    const scrollRef = makeScrollRef();
    let resolveLoad: (value: number) => void = () => {};
    const firstLoadMore = vi.fn(
      () =>
        new Promise<number>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const { result, rerender } = renderHook(
      ({ loadMore }: { loadMore: () => Promise<number> }) =>
        useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
          rearmWhileIntersecting: true,
          joinInFlightWhileLoading: true,
        }),
      { initialProps: { loadMore: firstLoadMore } },
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    // Re-render with a new loadMore tears the observer down (cleanup) while
    // the first load is still in flight.
    const secondLoadMore = vi.fn(async () => 20);
    rerender({ loadMore: secondLoadMore });
    await act(async () => {
      resolveLoad(20);
    });
    // The old observer was disconnected and must not re-observe the node.
    expect(records[0].disconnected).toBe(true);
    expect(records[0].targets.length).toBe(1);
  });
});
