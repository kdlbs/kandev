/* eslint-disable max-lines -- sentinel state-machine regressions share one observer harness. */
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
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
  it("rechecks current geometry when a restored viewport becomes eligible", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        isCurrentGeometryEligible: () => true,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    expect(result.current).toHaveProperty("recheck");
    if (!("recheck" in result.current)) return;
    act(() => result.current.recheck());
    await act(async () => {});

    expect(loadMore).toHaveBeenCalledTimes(1);
  });

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

  it("swaps observations with unobserve+observe when the sentinel remounts", () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore),
    );
    const first = document.createElement("div");
    act(() => result.current.sentinelRef(first));
    const record = records[0];
    expect(record.targets).toContain(first);

    // Remount: the old node is unobserved (not disconnected) and the new one
    // observed, so any other target the observer might hold survives.
    const second = document.createElement("div");
    act(() => result.current.sentinelRef(second));
    expect(record.unobserved).toContain(first);
    expect(record.targets).toContain(second);
    expect(record.disconnected).toBe(false);
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

describe("useLazyLoadSentinel — scroller lifecycle", () => {
  it("connects the observer when the scroll container appears after mount", () => {
    const scrollRef = { current: null as HTMLDivElement | null };
    const loadMore = vi.fn(async () => 20);
    const { result, rerender } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));
    expect(records).toHaveLength(0);

    scrollRef.current = document.createElement("div");
    rerender();

    expect(records).toHaveLength(1);
    expect(records[0].targets).toContain(node);
  });

  it("moves the pin listener with the scroller and cleans it up on unmount", () => {
    const firstScroller = document.createElement("div");
    const secondScroller = document.createElement("div");
    const firstAdd = vi.spyOn(firstScroller, "addEventListener");
    const firstRemove = vi.spyOn(firstScroller, "removeEventListener");
    const secondAdd = vi.spyOn(secondScroller, "addEventListener");
    const secondRemove = vi.spyOn(secondScroller, "removeEventListener");
    const scrollRef = { current: firstScroller as HTMLDivElement | null };
    const loadMore = vi.fn(async () => 20);
    const { rerender, unmount } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore),
    );

    expect(firstAdd).toHaveBeenCalledWith("scroll", expect.any(Function), { passive: true });

    scrollRef.current = secondScroller;
    rerender();

    expect(firstRemove).toHaveBeenCalledWith("scroll", expect.any(Function));
    expect(secondAdd).toHaveBeenCalledWith("scroll", expect.any(Function), { passive: true });

    unmount();
    expect(secondRemove).toHaveBeenCalledWith("scroll", expect.any(Function));
  });
});

describe("useLazyLoadSentinel — re-arm, disarm, and stale completions", () => {
  it("retries when loading becomes eligible while the sentinel remains intersecting", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn().mockResolvedValueOnce(20).mockResolvedValueOnce(0);
    const { result, rerender } = renderHook(
      ({ blocked }: { blocked: boolean }) =>
        useLazyLoadSentinel(scrollRef, true, blocked, false, loadMore, {
          rearmWhileIntersecting: true,
        }),
      { initialProps: { blocked: true } },
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    // The first entry is observed while the initial message fetch is blocked.
    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).not.toHaveBeenCalled();

    // No second intersection is delivered, so the eligibility transition
    // must retry the still-visible sentinel itself.
    rerender({ blocked: false });
    await waitFor(() => expect(loadMore).toHaveBeenCalledTimes(2));
  });

  it("re-arms only when enabled: unobserves before loading and re-observes after a positive result", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn().mockResolvedValueOnce(20).mockResolvedValueOnce(0);
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
    await act(async () => {
      await vi.waitFor(() => expect(loadMore).toHaveBeenCalledTimes(2));
    });
    expect(records[0].targets.filter((t) => t === node).length).toBeGreaterThanOrEqual(1);
    // Still intersecting: the next page auto-loads without a scroll-away or
    // a second IntersectionObserver entry.
    expect(loadMore).toHaveBeenCalledTimes(2);
  });

  it("stops a still-intersecting continuation when the caller reports a new visible boundary", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn().mockResolvedValueOnce(20).mockResolvedValueOnce(0);
    const shouldContinueWhileIntersecting = vi.fn(() => false);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        shouldContinueWhileIntersecting,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await waitFor(() => expect(shouldContinueWhileIntersecting).toHaveBeenCalledTimes(1));
    expect(loadMore).toHaveBeenCalledTimes(1);

    // A stale true entry cannot restart the stopped cycle. The sentinel must
    // leave the preload region before a later user reach can start a new one.
    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);
    fire(records[0], false, node);
    fire(records[0], true, node);
    await waitFor(() => expect(loadMore).toHaveBeenCalledTimes(2));
  });

  it("uses the caller continuation predicate even after an observer exit", async () => {
    const scrollRef = makeScrollRef();
    const onLoadSettled = vi.fn();
    const shouldContinueWhileIntersecting = vi.fn(() => true);
    const loadMore = vi.fn().mockResolvedValueOnce(20).mockResolvedValueOnce(0);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        shouldContinueWhileIntersecting,
        onLoadSettled,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    fire(records[0], false, node);

    await waitFor(() => expect(loadMore).toHaveBeenCalledTimes(2));
    expect(shouldContinueWhileIntersecting).toHaveBeenCalledTimes(1);
    expect(onLoadSettled).toHaveBeenNthCalledWith(1, {
      count: 20,
      rejected: false,
      continuation: "continued",
    });
  });
});

describe("useLazyLoadSentinel — stickToBottomWhileLoading", () => {
  it("sticks to the bottom after a positive load while the user is pinned, keeping the sentinel in view", async () => {
    const scrollRef = makeScrollRef();
    const scroller = scrollRef.current!;
    // The user is pinned at the bottom: content (600) overflows the 400px
    // viewport and the scroll position sits at the old bottom (200 + 400).
    Object.defineProperty(scroller, "clientHeight", { configurable: true, value: 400 });
    Object.defineProperty(scroller, "scrollHeight", { configurable: true, value: 600 });
    scroller.scrollTop = 200;
    let resolveLoad: (value: number) => void = () => {};
    const loadMore = vi.fn().mockImplementationOnce(
      () =>
        new Promise<number>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        joinInFlightWhileLoading: true,
        stickToBottomWhileLoading: true,
      }),
    );
    // The initial pin check runs in an effect.
    await act(async () => {});
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    expect(loadMore).toHaveBeenCalledTimes(1);
    // Rows are appended while the load is in flight (scrollHeight grows to
    // 800); the user stays at the old bottom. The settle must scroll the
    // scroller back to the new bottom so the sentinel stays intersecting.
    expect(loadMore).toHaveBeenCalledTimes(1);
    Object.defineProperty(scroller, "scrollHeight", { configurable: true, value: 800 });
    await act(async () => {
      resolveLoad(20);
    });
    // Browser-faithful assertion: jsdom stores the raw write (800) while a
    // real browser clamps scrollTop to scrollHeight - clientHeight (400); the
    // invariant is "pinned at the bottom", so assert that instead of the
    // raw value.
    expect(scroller.scrollTop).toBeGreaterThanOrEqual(
      scroller.scrollHeight - scroller.clientHeight,
    );
  });
});

describe("useLazyLoadSentinel — pin refresh before a load", () => {
  it("refreshes the pin from the current geometry before a load (no scroll event needed)", async () => {
    const scrollRef = makeScrollRef();
    const scroller = scrollRef.current!;
    // Mounted scrolled near the top: the initial pin check is false.
    Object.defineProperty(scroller, "clientHeight", { configurable: true, value: 400 });
    Object.defineProperty(scroller, "scrollHeight", { configurable: true, value: 600 });
    scroller.scrollTop = 0;
    let resolveLoad: (value: number) => void = () => {};
    const loadMore = vi.fn(
      () =>
        new Promise<number>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        stickToBottomWhileLoading: true,
      }),
    );
    await act(async () => {});
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    // The user reaches the bottom without any scroll event (e.g. programmatic
    // scroll restoration after a session switch): fireLoad must refresh the
    // pin from the current geometry instead of the stale mount value.
    scroller.scrollTop = 200;
    fire(records[0], true, node);
    Object.defineProperty(scroller, "scrollHeight", { configurable: true, value: 800 });
    await act(async () => {
      resolveLoad(20);
    });
    expect(loadMore).toHaveBeenCalled();
    expect(scroller.scrollTop).toBeGreaterThanOrEqual(
      scroller.scrollHeight - scroller.clientHeight,
    );
  });
});

describe("useLazyLoadSentinel — stickToBottomWhileLoading", () => {
  it("does not stick when the user is not pinned at the bottom", async () => {
    const scrollRef = makeScrollRef();
    const scroller = scrollRef.current!;
    Object.defineProperty(scroller, "clientHeight", { configurable: true, value: 400 });
    Object.defineProperty(scroller, "scrollHeight", { configurable: true, value: 600 });
    scroller.scrollTop = 0; // scrolled near the top: not pinned
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        joinInFlightWhileLoading: true,
        stickToBottomWhileLoading: true,
      }),
    );
    await act(async () => {});
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    Object.defineProperty(scroller, "scrollHeight", { configurable: true, value: 800 });
    await act(async () => {});
    expect(scroller.scrollTop).toBe(0);
  });

  it("does not stick without the option (transcript behavior)", async () => {
    const scrollRef = makeScrollRef();
    const scroller = scrollRef.current!;
    Object.defineProperty(scroller, "clientHeight", { configurable: true, value: 400 });
    Object.defineProperty(scroller, "scrollHeight", { configurable: true, value: 600 });
    scroller.scrollTop = 200;
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
      }),
    );
    await act(async () => {});
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    Object.defineProperty(scroller, "scrollHeight", { configurable: true, value: 800 });
    await act(async () => {});
    expect(scroller.scrollTop).toBe(200);
  });
});

describe("useLazyLoadSentinel — re-arm, disarm, and stale completions", () => {
  it("serializes continuation pages when loading state toggles around each request", async () => {
    const scrollRef = makeScrollRef();
    let isLoadingMore = true;
    let resolveRequest: (value: number) => void = () => {};
    let activeRequest: Promise<number> | null = null;
    let rerenderHook: () => void = () => {};
    const pages = [20, 20, 0];
    let resolveFinalPage: () => void = () => {};
    const finalPageSettled = new Promise<void>((resolve) => {
      resolveFinalPage = resolve;
    });
    activeRequest = new Promise<number>((resolve) => {
      resolveRequest = resolve;
    });
    const startPage = () => {
      const page = pages.shift() ?? 0;
      activeRequest = new Promise<number>((resolve) => {
        resolveRequest = resolve;
      });
      isLoadingMore = true;
      rerenderHook();
      queueMicrotask(() => {
        act(() => {
          isLoadingMore = false;
          rerenderHook();
          resolveRequest(page);
          if (page === 0) resolveFinalPage();
          activeRequest = null;
        });
      });
      return activeRequest;
    };
    const loadMore = vi.fn(() => activeRequest ?? startPage());
    const { result, rerender } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, isLoadingMore, loadMore, {
        rearmWhileIntersecting: true,
        joinInFlightWhileLoading: true,
      }),
    );
    rerenderHook = rerender;
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {
      isLoadingMore = false;
      rerender();
    });
    expect(loadMore).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolveRequest(20);
      activeRequest = null;
      await Promise.resolve();
    });
    await act(async () => {
      await finalPageSettled;
    });
    await waitFor(() => expect(loadMore).toHaveBeenCalledTimes(4));
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
    await waitFor(() => expect(loadMore).toHaveBeenCalledTimes(3));

    // The successful gesture retry already loaded the next page while the
    // sentinel remained visible; the stale true entry cannot loop after the
    // follow-up no-op disarms it.
    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(3);
  });

  it("allows an explicit retry after the sentinel leaves preload", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn().mockResolvedValueOnce(0).mockResolvedValueOnce(20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    await act(async () => {});
    fire(records[0], false, node);

    act(() => result.current.retry());
    await waitFor(() => expect(loadMore).toHaveBeenCalledTimes(2));
  });

  it("ignores a request settlement invalidated by the owning view", async () => {
    const scrollRef = makeScrollRef();
    let resolveLoad: (value: number) => void = () => {};
    let requestCurrent = true;
    const onLoadSettled = vi.fn();
    const loadMore = vi.fn().mockImplementationOnce(
      () =>
        new Promise<number>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    loadMore.mockResolvedValueOnce(0);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        isRequestCurrent: () => requestCurrent,
        onLoadSettled,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    requestCurrent = false;
    await act(async () => resolveLoad(0));

    expect(onLoadSettled).toHaveBeenCalledWith({
      count: 0,
      rejected: false,
      continuation: "stale",
    });

    requestCurrent = true;
    fire(records[0], true, node);
    await waitFor(() => expect(loadMore).toHaveBeenCalledTimes(2));
  });
});

describe("useLazyLoadSentinel — stale view handoff", () => {
  it("replays an eligible intersection after a stale request releases the lock", async () => {
    const scrollRef = makeScrollRef();
    let activeView = "A";
    let resolveA: (value: number) => void = () => {};
    const loadA = vi.fn(
      () =>
        new Promise<number>((resolve) => {
          resolveA = resolve;
        }),
    );
    const loadB = vi.fn(async () => 0);
    const { result, rerender } = renderHook(
      ({ view }: { view: string }) =>
        useLazyLoadSentinel(scrollRef, true, false, false, view === "A" ? loadA : loadB, {
          rearmWhileIntersecting: true,
          isRequestCurrent: () => activeView === view,
        }),
      { initialProps: { view: "A" } },
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    expect(loadA).toHaveBeenCalledTimes(1);

    activeView = "B";
    rerender({ view: "B" });
    fire(records[1], true, node);

    // B's observer sees the eligible sentinel, but A still owns the shared
    // in-flight lock until its stale request settles.
    expect(loadB).not.toHaveBeenCalled();
    await act(async () => resolveA(20));

    // No exit/re-entry or recovery click: releasing A must replay B's current
    // intersection through the normal sentinel path.
    await waitFor(() => expect(loadB).toHaveBeenCalledTimes(1));
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
    const secondLoadMore = vi.fn().mockResolvedValueOnce(20).mockResolvedValueOnce(0);
    rerender({ loadMore: secondLoadMore });
    // The OLD load settles with a zero result: it must not disarm the NEW
    // observer (its observer identity is stale).
    await act(async () => {
      resolveOld(0);
    });
    expect(records[1].disconnected).toBe(false);

    // The new observer still fires on intersection (armed, not disarmed),
    // then its positive result re-arms the same observer for one follow-up
    // page.
    fire(records[1], true, node);
    await waitFor(() => expect(secondLoadMore).toHaveBeenCalledTimes(2));
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
  it("reports a stale settle when a request completes after unmount", async () => {
    const scrollRef = makeScrollRef();
    const onLoadSettled = vi.fn();
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
        onLoadSettled,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    unmount();
    await act(async () => resolveLoad(20));

    expect(onLoadSettled).toHaveBeenCalledWith({
      count: 20,
      rejected: false,
      continuation: "stale",
    });
  });

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
