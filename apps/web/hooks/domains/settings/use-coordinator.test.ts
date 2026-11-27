import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const {
  mockGetCoordinator,
  mockPatchCoordinator,
  mockDeleteCoordinator,
  storeState,
  MockApiError,
} = vi.hoisted(() => {
  class MockApiError extends Error {
    status: number;
    constructor(status: number, message = "error") {
      super(message);
      this.status = status;
    }
  }
  return {
    mockGetCoordinator: vi.fn(),
    mockPatchCoordinator: vi.fn(),
    mockDeleteCoordinator: vi.fn(),
    storeState: {
      updateCoordinator: vi.fn(),
      removeCoordinator: vi.fn(),
    },
    MockApiError,
  };
});

vi.mock("@/lib/api/client", () => ({ ApiError: MockApiError }));
vi.mock("@/lib/api/domains/coordinator-api", () => ({
  getCoordinator: mockGetCoordinator,
  patchCoordinator: mockPatchCoordinator,
  deleteCoordinator: mockDeleteCoordinator,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { useCoordinator } from "./use-coordinator";

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "c1",
    workspace_id: "w1",
    name: "Coordinator One",
    agent_profile_id: "agent-1",
    executor_profile_id: "executor-1",
    task_agent_profile_id: "ap-1",
    task_executor_profile_id: "ep-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    agent_profile_status: "ok",
    executor_profile_status: "ok",
    ...overrides,
  };
}

describe("useCoordinator", () => {
  beforeEach(() => {
    mockGetCoordinator.mockReset();
    mockPatchCoordinator.mockReset();
    mockDeleteCoordinator.mockReset();
    storeState.updateCoordinator.mockReset();
    storeState.removeCoordinator.mockReset();
  });

  it("starts loading and transitions to ready on a successful fetch", async () => {
    mockGetCoordinator.mockResolvedValue(coordinator());

    const { result } = renderHook(() => useCoordinator("w1", "c1"));
    expect(result.current.status).toBe("loading");

    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.coordinator).toEqual(coordinator());
    expect(storeState.updateCoordinator).toHaveBeenCalledWith(coordinator());
  });

  it("reports not-found on a 404 instead of throwing", async () => {
    mockGetCoordinator.mockRejectedValue(new MockApiError(404));

    const { result } = renderHook(() => useCoordinator("w1", "unknown"));

    await waitFor(() => expect(result.current.status).toBe("not-found"));
    expect(result.current.coordinator).toBeNull();
  });

  it("reports error on a non-404 failure", async () => {
    mockGetCoordinator.mockRejectedValue(new MockApiError(500));

    const { result } = renderHook(() => useCoordinator("w1", "c1"));

    await waitFor(() => expect(result.current.status).toBe("error"));
  });

  it("patches the coordinator through the API and updates local + store state", async () => {
    mockGetCoordinator.mockResolvedValue(coordinator());
    const patched = coordinator({ name: "Renamed" });
    mockPatchCoordinator.mockResolvedValue(patched);

    const { result } = renderHook(() => useCoordinator("w1", "c1"));
    await waitFor(() => expect(result.current.status).toBe("ready"));

    await act(async () => {
      await result.current.patch({ name: "Renamed" });
    });

    expect(mockPatchCoordinator).toHaveBeenCalledWith("w1", "c1", { name: "Renamed" });
    expect(result.current.coordinator?.name).toBe("Renamed");
    expect(storeState.updateCoordinator).toHaveBeenCalledWith(patched);
  });

  it("deletes the coordinator through the API and removes it from the store", async () => {
    mockGetCoordinator.mockResolvedValue(coordinator());
    mockDeleteCoordinator.mockResolvedValue(undefined);

    const { result } = renderHook(() => useCoordinator("w1", "c1"));
    await waitFor(() => expect(result.current.status).toBe("ready"));

    await act(async () => {
      await result.current.remove();
    });

    expect(mockDeleteCoordinator).toHaveBeenCalledWith("w1", "c1");
    expect(storeState.removeCoordinator).toHaveBeenCalledWith("c1");
  });

  it("treats a 404 on delete as success (already gone) rather than throwing", async () => {
    mockGetCoordinator.mockResolvedValue(coordinator());
    mockDeleteCoordinator.mockRejectedValue(new MockApiError(404));

    const { result } = renderHook(() => useCoordinator("w1", "c1"));
    await waitFor(() => expect(result.current.status).toBe("ready"));

    await act(async () => {
      await result.current.remove();
    });

    expect(storeState.removeCoordinator).toHaveBeenCalledWith("c1");
  });
});
