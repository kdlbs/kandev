import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";
import { usePRCIPopover } from "./use-pr-ci-popover";

const getPRFeedbackMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/github-api", () => ({
  getPRFeedback: getPRFeedbackMock,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function makePR(): TaskPR {
  return {
    id: "id",
    workspace_id: "workspace-1",
    task_id: "task-1",
    owner: "acme",
    repo: "app",
    pr_number: 7,
    pr_url: "https://github.com/acme/app/pull/7",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 1,
    checks_passing: 1,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
  };
}

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function renderPopoverHook(refreshTaskPR?: () => void | Promise<void>) {
  return renderHook(
    ({ enabled }: { enabled: boolean }) =>
      usePRCIPopover("workspace-1", makePR(), enabled, refreshTaskPR),
    { initialProps: { enabled: false }, wrapper },
  );
}

/** Lets the open-transition `queueMicrotask` fire and its state land. */
async function flushOpen() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("usePRCIPopover refresh indicator", () => {
  beforeEach(() => {
    getPRFeedbackMock.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("is idle before the popover opens", () => {
    getPRFeedbackMock.mockResolvedValue(null);
    const { result } = renderPopoverHook();
    expect(result.current.isRefreshing).toBe(false);
    expect(getPRFeedbackMock).not.toHaveBeenCalled();
  });

  it("reports refreshing while the feedback fetch is in flight", async () => {
    const feedback = deferred<null>();
    getPRFeedbackMock.mockReturnValue(feedback.promise);

    const { result, rerender } = renderPopoverHook();
    rerender({ enabled: true });
    await flushOpen();

    expect(result.current.isRefreshing).toBe(true);

    await act(async () => {
      feedback.resolve(null);
      await feedback.promise;
    });

    await waitFor(() => expect(result.current.isRefreshing).toBe(false));
  });

  it("stays refreshing until the TaskPR summary sync settles too", async () => {
    getPRFeedbackMock.mockResolvedValue(null);
    const sync = deferred<void>();
    const refreshTaskPR = vi.fn(() => sync.promise);

    const { result, rerender } = renderPopoverHook(refreshTaskPR);
    rerender({ enabled: true });
    await flushOpen();

    // The feedback fetch has already resolved; the summary sync has not.
    await waitFor(() => expect(result.current.isFetching).toBe(false));
    expect(result.current.isRefreshing).toBe(true);

    await act(async () => {
      sync.resolve();
      await sync.promise;
    });

    await waitFor(() => expect(result.current.isRefreshing).toBe(false));
  });

  it("does not hang when the refresh callback returns void", async () => {
    getPRFeedbackMock.mockResolvedValue(null);
    const refreshTaskPR = vi.fn(() => undefined);

    const { result, rerender } = renderPopoverHook(refreshTaskPR);
    rerender({ enabled: true });
    await flushOpen();

    expect(refreshTaskPR).toHaveBeenCalled();
    await waitFor(() => expect(result.current.isRefreshing).toBe(false));
  });

  it("clears the indicator when the summary sync rejects", async () => {
    getPRFeedbackMock.mockResolvedValue(null);
    const refreshTaskPR = vi.fn(() => Promise.reject(new Error("socket down")));

    const { result, rerender } = renderPopoverHook(refreshTaskPR);
    rerender({ enabled: true });
    await flushOpen();

    await waitFor(() => expect(result.current.isRefreshing).toBe(false));
  });
});

// Split from the block above only to stay under the 100-line function limit.
describe("usePRCIPopover refresh indicator timing", () => {
  beforeEach(() => {
    getPRFeedbackMock.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("ignores a stale sync settling after the popover was reopened", async () => {
    vi.useFakeTimers();
    getPRFeedbackMock.mockResolvedValue(null);
    const first = deferred<void>();
    const second = deferred<void>();
    const refreshTaskPR = vi
      .fn<() => Promise<void>>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        usePRCIPopover("workspace-1", makePR(), enabled, refreshTaskPR),
      { initialProps: { enabled: false }, wrapper },
    );
    rerender({ enabled: true });
    await flushOpen();
    rerender({ enabled: false });
    rerender({ enabled: true });
    await flushOpen();
    expect(refreshTaskPR).toHaveBeenCalledTimes(2);

    // The first open's sync lands late; the second is still in flight.
    await act(async () => {
      first.resolve();
      await first.promise;
    });
    // Drain the minimum-visible window first — it would otherwise hold the
    // indicator up on its own and hide a wrongly-cleared flag.
    await act(async () => {
      vi.advanceTimersByTime(2_000);
    });
    expect(result.current.isRefreshing).toBe(true);

    await act(async () => {
      second.resolve();
      await second.promise;
    });
    await act(async () => {
      vi.advanceTimersByTime(2_000);
    });
    expect(result.current.isRefreshing).toBe(false);
  });

  it("keeps the indicator up long enough to read a cache-fast refresh", async () => {
    vi.useFakeTimers();
    getPRFeedbackMock.mockResolvedValue(null);

    const { result, rerender } = renderPopoverHook();
    rerender({ enabled: true });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.isRefreshing).toBe(true);
    // Both sources have settled, but the minimum visible window has not.
    await act(async () => {
      vi.advanceTimersByTime(100);
    });
    expect(result.current.isRefreshing).toBe(true);

    await act(async () => {
      vi.advanceTimersByTime(400);
    });
    expect(result.current.isRefreshing).toBe(false);
  });
});
