import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ActiveThread } from "./active-threads";
import { applyStableThreadOrder, useStableThreadOrder } from "./stable-order";

function thread(taskId: string): ActiveThread {
  return {
    taskId,
    title: `Task ${taskId}`,
    workflowId: "wf-1",
    workflowName: "Delivery",
    stepTitle: "Build",
    sessionId: `session-${taskId}`,
    sessionState: "RUNNING",
    pendingAction: null,
    activeSubagentCount: 0,
    queuedPromptCount: 0,
    lastActivityAt: "2026-08-27T10:00:00Z",
  };
}

function ids(threads: readonly ActiveThread[]): string[] {
  return threads.map((entry) => entry.taskId);
}

describe("applyStableThreadOrder", () => {
  it("takes the incoming order the first time, when there is nothing to preserve", () => {
    expect(ids(applyStableThreadOrder([], [thread("b"), thread("a")]))).toEqual(["b", "a"]);
  });

  it("keeps a column in its slot when the incoming order changes under it", () => {
    // Replying to "a" flips it from needing a human to working, which the
    // selector sorts last. The column must not move out from under the reader.
    const reordered = applyStableThreadOrder(["a", "b"], [thread("b"), thread("a")]);

    expect(ids(reordered)).toEqual(["a", "b"]);
  });

  it("appends a newly active thread instead of sorting it into the deck", () => {
    const withNew = applyStableThreadOrder(["a", "b"], [thread("new"), thread("a"), thread("b")]);

    expect(ids(withNew)).toEqual(["a", "b", "new"]);
  });

  it("keeps several new threads in the order the selector ranked them", () => {
    const withNew = applyStableThreadOrder(["a"], [thread("y"), thread("a"), thread("x")]);

    expect(ids(withNew)).toEqual(["a", "y", "x"]);
  });

  it("keeps committed matching columns when a live rank change reaches a cap", () => {
    const retained = applyStableThreadOrder(["a", "b"], [thread("c"), thread("a"), thread("b")], 2);

    expect(ids(retained)).toEqual(["a", "b"]);
  });

  it("fills a released capped slot from the current sorted matches", () => {
    const filled = applyStableThreadOrder(["a", "b"], [thread("c"), thread("a")], 2);

    expect(ids(filled)).toEqual(["a", "c"]);
  });

  it("drops a settled thread without disturbing the columns around it", () => {
    const remaining = applyStableThreadOrder(["a", "b", "c"], [thread("c"), thread("a")]);

    expect(ids(remaining)).toEqual(["a", "c"]);
  });

  it("is idempotent, so re-deriving during a re-render cannot shuffle the deck", () => {
    const once = applyStableThreadOrder(["a", "b"], [thread("b"), thread("a")]);
    const twice = applyStableThreadOrder(ids(once), once);

    expect(ids(twice)).toEqual(ids(once));
  });

  it("forgets a thread that left, so its old slot is not reserved on return", () => {
    const withoutB = applyStableThreadOrder(["a", "b", "c"], [thread("a"), thread("c")]);
    const bReturns = applyStableThreadOrder(ids(withoutB), [thread("b"), thread("a"), thread("c")]);

    expect(ids(bReturns)).toEqual(["a", "c", "b"]);
  });
});

describe("useStableThreadOrder", () => {
  it("holds the deck's slots across re-renders", () => {
    const { result, rerender } = renderHook(({ threads }) => useStableThreadOrder(threads), {
      initialProps: { threads: [thread("a"), thread("b")] },
    });
    expect(ids(result.current)).toEqual(["a", "b"]);

    // The selector now ranks "a" last, and adds a thread ahead of both.
    rerender({ threads: [thread("new"), thread("b"), thread("a")] });

    expect(ids(result.current)).toEqual(["a", "b", "new"]);
  });

  it("restarts from the incoming sorted order when the query reset key changes", () => {
    const { result, rerender } = renderHook(
      ({ threads, resetKey }) => useStableThreadOrder(threads, resetKey),
      {
        initialProps: { threads: [thread("a"), thread("b")], resetKey: "first" },
      },
    );

    rerender({ threads: [thread("b"), thread("a")], resetKey: "second" });

    expect(ids(result.current)).toEqual(["b", "a"]);
  });
});
