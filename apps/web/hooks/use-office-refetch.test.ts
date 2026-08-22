import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";

let storeState: { office: { refetchTriggers: Record<string, number> } } = {
  office: { refetchTriggers: {} },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { useOfficeRefetch } from "./use-office-refetch";

function bump(type: string) {
  const triggers = storeState.office.refetchTriggers;
  storeState = {
    office: { refetchTriggers: { ...triggers, [type]: (triggers[type] ?? 0) + 1 } },
  };
}

describe("useOfficeRefetch", () => {
  beforeEach(() => {
    storeState = { office: { refetchTriggers: {} } };
  });

  afterEach(() => {
    cleanup();
  });

  it("does not fire on mount when no trigger has fired for this type", () => {
    const onRefetch = vi.fn();
    renderHook(() => useOfficeRefetch("dashboard", onRefetch));
    expect(onRefetch).not.toHaveBeenCalled();
  });

  it("fires when its exact type is bumped", () => {
    const onRefetch = vi.fn();
    const { rerender } = renderHook(() => useOfficeRefetch("dashboard", onRefetch));

    bump("dashboard");
    rerender();

    expect(onRefetch).toHaveBeenCalledTimes(1);
  });

  it("fires on a prefix match (comments: matches comments:task-123)", () => {
    const onRefetch = vi.fn();
    const { rerender } = renderHook(() => useOfficeRefetch("comments", onRefetch));

    bump("comments:task-123");
    rerender();

    expect(onRefetch).toHaveBeenCalledTimes(1);
  });

  it("does not fire for an unrelated type", () => {
    const onRefetch = vi.fn();
    const { rerender } = renderHook(() => useOfficeRefetch("dashboard", onRefetch));

    bump("tasks");
    rerender();

    expect(onRefetch).not.toHaveBeenCalled();
  });

  it("does not re-fire on a render that doesn't bump its type", () => {
    const onRefetch = vi.fn();
    const { rerender } = renderHook(() => useOfficeRefetch("dashboard", onRefetch));

    bump("dashboard");
    rerender();
    rerender();
    rerender();

    expect(onRefetch).toHaveBeenCalledTimes(1);
  });

  // Regression test: a single WS handler often bumps several distinct trigger
  // types in one synchronous call (e.g. `task:${id}` then `dashboard`), which
  // React coalesces into a single render. A shared "last trigger" store field
  // would overwrite itself on every bump, so only the final type's subscriber
  // would ever observe a change — every other type silently never refetches.
  // Per-type counters must let every matching subscriber see its own bump
  // even when several types were bumped before React ever renders.
  it("lets every subscriber observe its own bump when multiple types are bumped in the same tick", () => {
    const onTaskRefetch = vi.fn();
    const onDashboardRefetch = vi.fn();
    const { rerender: rerenderTask } = renderHook(() =>
      useOfficeRefetch("task:t-1", onTaskRefetch),
    );
    const { rerender: rerenderDashboard } = renderHook(() =>
      useOfficeRefetch("dashboard", onDashboardRefetch),
    );

    // Simulate a WS handler firing both triggers before either hook re-renders.
    bump("task:t-1");
    bump("dashboard");
    rerenderTask();
    rerenderDashboard();

    expect(onTaskRefetch).toHaveBeenCalledTimes(1);
    expect(onDashboardRefetch).toHaveBeenCalledTimes(1);
  });

  // Regression test: a prefix subscriber watches several sub-keys under one
  // triggerType (e.g. "comments" matches "comments:task-A" and
  // "comments:task-B"). A single scalar "last seen" value would let a bump
  // on one sub-key mask a later bump on a sibling sub-key whenever the
  // sibling's counter is lower than the max already observed. Per-key
  // tracking must let each sub-key's bump fire independently.
  it("still fires for a sub-key bump after a higher-numbered sibling sub-key already fired", () => {
    const onRefetch = vi.fn();
    const { rerender } = renderHook(() => useOfficeRefetch("comments", onRefetch));

    // task-B races ahead first, bumped three times.
    bump("comments:task-B");
    bump("comments:task-B");
    bump("comments:task-B");
    rerender();
    expect(onRefetch).toHaveBeenCalledTimes(1);

    // task-A's first bump (counter 1) is lower than task-B's counter (3), so
    // a scalar "max seen" comparison would swallow it.
    bump("comments:task-A");
    rerender();

    expect(onRefetch).toHaveBeenCalledTimes(2);
  });

  it("uses the latest callback without re-firing when only the callback changes", () => {
    const first = vi.fn();
    const second = vi.fn();
    const { rerender } = renderHook(({ cb }) => useOfficeRefetch("dashboard", cb), {
      initialProps: { cb: first },
    });

    rerender({ cb: second });
    bump("dashboard");
    rerender({ cb: second });

    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
  });
});
