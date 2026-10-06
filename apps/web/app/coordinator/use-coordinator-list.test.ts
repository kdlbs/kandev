import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const listCoordinatorsMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", () => ({
  listCoordinators: (...args: unknown[]) => listCoordinatorsMock(...args),
}));

import { useCoordinatorList } from "./use-coordinator-list";

const WORKSPACE_ID = "workspace-1";

function coordinator(id: string): Coordinator {
  return {
    id,
    workspace_id: WORKSPACE_ID,
    name: `Coordinator ${id}`,
    agent_profile_id: "agent-1",
    executor_profile_id: "exec-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    open_proposals: 0,
  };
}

beforeEach(() => {
  listCoordinatorsMock.mockReset();
});

describe("useCoordinatorList - initial load", () => {
  it("loads coordinators on mount", async () => {
    listCoordinatorsMock.mockResolvedValue({ coordinators: [coordinator("c-1")] });

    const { result } = renderHook(() => useCoordinatorList(WORKSPACE_ID));

    await waitFor(() => expect(result.current.coordinators).toBeDefined());
    expect(result.current.coordinators).toEqual([coordinator("c-1")]);
    expect(result.current.error).toBe(false);
    expect(listCoordinatorsMock).toHaveBeenCalledWith(WORKSPACE_ID);
  });

  it("does not fetch when workspaceId is null", () => {
    renderHook(() => useCoordinatorList(null));

    expect(listCoordinatorsMock).not.toHaveBeenCalled();
  });

  it("sets error when the read fails, keeping coordinators undefined", async () => {
    listCoordinatorsMock.mockRejectedValue(new Error("list failed"));

    const { result } = renderHook(() => useCoordinatorList(WORKSPACE_ID));

    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.coordinators).toBeUndefined();
  });
});

describe("useCoordinatorList - retry", () => {
  it("re-reads the list and clears the error on success", async () => {
    listCoordinatorsMock.mockRejectedValueOnce(new Error("list failed"));

    const { result } = renderHook(() => useCoordinatorList(WORKSPACE_ID));

    await waitFor(() => expect(result.current.error).toBe(true));

    listCoordinatorsMock.mockResolvedValueOnce({ coordinators: [coordinator("c-2")] });

    act(() => {
      result.current.retry();
    });

    await waitFor(() => expect(result.current.coordinators).toEqual([coordinator("c-2")]));
    expect(result.current.error).toBe(false);
    expect(listCoordinatorsMock).toHaveBeenCalledTimes(2);
  });
});

describe("useCoordinatorList - stale request protection", () => {
  it("discards a stale in-flight read when a newer read has already resolved", async () => {
    let resolveFirst: ((v: { coordinators: Coordinator[] }) => void) | undefined;
    listCoordinatorsMock
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValueOnce({ coordinators: [coordinator("c-fresh")] });

    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useCoordinatorList(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_ID } },
    );

    rerender({ workspaceId: "workspace-2" });
    await waitFor(() => expect(result.current.coordinators).toEqual([coordinator("c-fresh")]));

    act(() => {
      resolveFirst?.({ coordinators: [coordinator("c-stale")] });
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.coordinators).toEqual([coordinator("c-fresh")]);
  });
});
