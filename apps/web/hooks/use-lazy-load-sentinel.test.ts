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

// eslint-disable-next-line max-lines-per-function -- observer lifecycle cases share one deterministic harness.
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

  it("does not fire during an in-flight load without the option", async () => {
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
  });

  it("does not stick to the bottom without the option (transcript behavior)", async () => {
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
});

// eslint-disable-next-line max-lines-per-function -- eligibility, disarm, and stale-completion cases share one lifecycle fixture
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

  // @covers AC-UI-TRANSCRIPT-AUTO-SCROLL-001.13
  it("does not retry a blocked intersection that is outside current geometry", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn(async () => 20);
    const { result, rerender } = renderHook(
      ({ blocked }: { blocked: boolean }) =>
        useLazyLoadSentinel(scrollRef, true, blocked, false, loadMore, {
          rearmWhileIntersecting: true,
          isCurrentGeometryEligible: () => false,
        }),
      { initialProps: { blocked: true } },
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    rerender({ blocked: false });
    await act(async () => {});

    expect(loadMore).not.toHaveBeenCalled();
  });

  it("re-arms only when enabled: unobserves before loading and re-observes after a positive result", async () => {
    const scrollRef = makeScrollRef();
    const loadMore = vi.fn().mockResolvedValueOnce(20).mockResolvedValueOnce(0);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
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

describe("useLazyLoadSentinel — re-arm, disarm, and stale completions", () => {
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

describe("useLazyLoadSentinel — gesture geometry", () => {
  it("rejects a gesture retry after prepend moves a stale intersection outside preload", async () => {
    const scrollRef = makeScrollRef();
    let geometryEligible = true;
    const loadMore = vi.fn(async () => 0);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        rearmWhileIntersecting: true,
        isCurrentGeometryEligible: () => geometryEligible,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));
    fire(records[0], true, node);
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);

    geometryEligible = false;
    act(() => result.current.onUserGesture());
    await act(async () => {});
    expect(loadMore).toHaveBeenCalledTimes(1);

    geometryEligible = true;
    act(() => result.current.onUserGesture());
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

  // @covers AC-UI-TRANSCRIPT-AUTO-SCROLL-001.13
  it("does not hand off a stale intersection that left current geometry", async () => {
    const scrollRef = makeScrollRef();
    let activeView = "A";
    let geometryEligible = true;
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
          isCurrentGeometryEligible: () => geometryEligible,
          isRequestCurrent: () => activeView === view,
        }),
      { initialProps: { view: "A" } },
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    fire(records[0], true, node);
    activeView = "B";
    rerender({ view: "B" });
    fire(records[1], true, node);
    geometryEligible = false;

    await act(async () => resolveA(20));
    await act(async () => {});

    expect(loadB).not.toHaveBeenCalled();
  });

  it("rejects a queued positive intersection when current geometry is ineligible", () => {
    const scrollRef = makeScrollRef();
    let geometryEligible = true;
    const loadMore = vi.fn(async () => 20);
    const { result } = renderHook(() =>
      useLazyLoadSentinel(scrollRef, true, false, false, loadMore, {
        isCurrentGeometryEligible: () => geometryEligible,
      }),
    );
    const node = document.createElement("div");
    act(() => result.current.sentinelRef(node));

    geometryEligible = false;
    fire(records[0], true, node);

    expect(loadMore).not.toHaveBeenCalled();
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
