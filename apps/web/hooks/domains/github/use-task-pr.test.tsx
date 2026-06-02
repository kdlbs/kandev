import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";

const requestMock = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

// listWorkspaceTaskPRs is only used by useWorkspacePRs (not under test
// here). Stub it so the module import doesn't fail in jsdom.
vi.mock("@/lib/api/domains/github-api", () => ({
  listWorkspaceTaskPRs: vi.fn().mockResolvedValue(null),
}));

import { useTaskPR } from "./use-task-pr";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

beforeEach(() => {
  vi.useFakeTimers();
  requestMock.mockReset();
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useTaskPR — permanent flag", () => {
  // The dominant production signal in the SyncWatchesBatched storm was the
  // frontend polling `github.task_pr.sync` every 5s for tasks whose repos
  // were deleted/inaccessible. The backend now returns `permanent: true`
  // on those responses; the hook must stop the retry interval cold.
  it("stops the 5s retry interval when the backend reports permanent: true", async () => {
    requestMock.mockResolvedValue({ prs: [], permanent: true });

    renderHook(() => useTaskPR("task-1"), { wrapper });

    // Initial freshness sync fires synchronously from the mount effect.
    // Flush the resolved promise so the permanent flag is applied before
    // the interval would otherwise fire.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(requestMock).toHaveBeenCalledTimes(1);

    // Advance well past several retry windows. If the permanent
    // short-circuit regressed, this would burst 5-6 additional calls
    // into requestMock.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000 * 6);
    });
    expect(requestMock).toHaveBeenCalledTimes(1);
  });

  // Without permanent, the existing retry cadence must still kick in so
  // tasks waiting on a freshly-pushed branch still get their PR detected.
  it("retries every 5s when permanent is absent and no PR is in the store", async () => {
    requestMock.mockResolvedValue({ prs: [] });

    renderHook(() => useTaskPR("task-1"), { wrapper });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(requestMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    expect(requestMock).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    expect(requestMock).toHaveBeenCalledTimes(3);
  });
});
