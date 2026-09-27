import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const {
  mockListCoordinators,
  mockCreateCoordinator,
  mockPatchCoordinator,
  mockDeleteCoordinator,
  storeState,
} = vi.hoisted(() => ({
  mockListCoordinators: vi.fn(),
  mockCreateCoordinator: vi.fn(),
  mockPatchCoordinator: vi.fn(),
  mockDeleteCoordinator: vi.fn(),
  storeState: {
    coordinators: { items: [] as Coordinator[], loaded: false, loading: false },
    setCoordinators: vi.fn(),
    setCoordinatorsLoading: vi.fn(),
    addCoordinator: vi.fn(),
    updateCoordinator: vi.fn(),
    removeCoordinator: vi.fn(),
  },
}));

vi.mock("@/lib/api/domains/coordinator-api", () => ({
  listCoordinators: mockListCoordinators,
  createCoordinator: mockCreateCoordinator,
  patchCoordinator: mockPatchCoordinator,
  deleteCoordinator: mockDeleteCoordinator,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { useCoordinators } from "./use-coordinators";

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "c1",
    workspace_id: "w1",
    name: "Coordinator One",
    agent_profile_id: "agent-1",
    executor_profile_id: "executor-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

function resetCoordinatorMocks() {
  mockListCoordinators.mockReset();
  mockCreateCoordinator.mockReset();
  mockPatchCoordinator.mockReset();
  mockDeleteCoordinator.mockReset();
  storeState.setCoordinators.mockReset();
  storeState.setCoordinatorsLoading.mockReset();
  storeState.addCoordinator.mockReset();
  storeState.updateCoordinator.mockReset();
  storeState.removeCoordinator.mockReset();
}

describe("useCoordinators fetching", () => {
  beforeEach(resetCoordinatorMocks);

  it("fetches the workspace's coordinators once on mount", async () => {
    mockListCoordinators.mockResolvedValue({ coordinators: [coordinator()] });

    renderHook(() => useCoordinators("w1"));

    await waitFor(() => expect(mockListCoordinators).toHaveBeenCalledWith("w1"));
    await waitFor(() => expect(storeState.setCoordinators).toHaveBeenCalledWith([coordinator()]));
  });

  it("does not refetch for the same workspace on rerender", async () => {
    mockListCoordinators.mockResolvedValue({ coordinators: [] });

    const view = renderHook(({ workspaceId }) => useCoordinators(workspaceId), {
      initialProps: { workspaceId: "w1" as string | null },
    });
    await waitFor(() => expect(mockListCoordinators).toHaveBeenCalledTimes(1));

    view.rerender({ workspaceId: "w1" });
    expect(mockListCoordinators).toHaveBeenCalledTimes(1);
  });

  it("refetches when the workspace id changes", async () => {
    const first = deferred<{ coordinators: Coordinator[] }>();
    const second = deferred<{ coordinators: Coordinator[] }>();
    mockListCoordinators.mockImplementation((workspaceId: string) =>
      workspaceId === "w1" ? first.promise : second.promise,
    );

    const view = renderHook(({ workspaceId }) => useCoordinators(workspaceId), {
      initialProps: { workspaceId: "w1" as string | null },
    });
    await act(async () => {
      first.resolve({ coordinators: [coordinator({ id: "c1" })] });
      await first.promise;
    });

    view.rerender({ workspaceId: "w2" });
    await act(async () => {
      second.resolve({ coordinators: [coordinator({ id: "c2" })] });
      await second.promise;
    });

    expect(mockListCoordinators).toHaveBeenCalledWith("w2");
  });
});

describe("useCoordinators mutations", () => {
  beforeEach(resetCoordinatorMocks);

  it("creates a coordinator through the API and adds it to the store", async () => {
    mockListCoordinators.mockResolvedValue({ coordinators: [] });
    const created = coordinator({ id: "new" });
    mockCreateCoordinator.mockResolvedValue(created);

    const { result } = renderHook(() => useCoordinators("w1"));
    await waitFor(() => expect(mockListCoordinators).toHaveBeenCalled());

    await act(async () => {
      await result.current.create({
        name: "Coordinator One",
        agent_profile_id: "agent-1",
        executor_profile_id: "executor-1",
      });
    });

    expect(mockCreateCoordinator).toHaveBeenCalledWith("w1", {
      name: "Coordinator One",
      agent_profile_id: "agent-1",
      executor_profile_id: "executor-1",
    });
    expect(storeState.addCoordinator).toHaveBeenCalledWith(created);
  });

  it("patches a coordinator through the API and updates the store", async () => {
    mockListCoordinators.mockResolvedValue({ coordinators: [] });
    const patched = coordinator({ name: "Renamed" });
    mockPatchCoordinator.mockResolvedValue(patched);

    const { result } = renderHook(() => useCoordinators("w1"));
    await waitFor(() => expect(mockListCoordinators).toHaveBeenCalled());

    await act(async () => {
      await result.current.patch("c1", { name: "Renamed" });
    });

    expect(mockPatchCoordinator).toHaveBeenCalledWith("w1", "c1", { name: "Renamed" });
    expect(storeState.updateCoordinator).toHaveBeenCalledWith(patched);
  });

  it("removes a coordinator through the API and updates the store", async () => {
    mockListCoordinators.mockResolvedValue({ coordinators: [] });
    mockDeleteCoordinator.mockResolvedValue(undefined);

    const { result } = renderHook(() => useCoordinators("w1"));
    await waitFor(() => expect(mockListCoordinators).toHaveBeenCalled());

    await act(async () => {
      await result.current.remove("c1");
    });

    expect(mockDeleteCoordinator).toHaveBeenCalledWith("w1", "c1");
    expect(storeState.removeCoordinator).toHaveBeenCalledWith("c1");
  });
});

describe("useCoordinators load errors", () => {
  beforeEach(resetCoordinatorMocks);

  // Build decision B3: a list load failure shows an inline error with Retry
  // rather than a silent empty list.
  it("reports a load error on the initial fetch, and clears it once refresh succeeds", async () => {
    mockListCoordinators.mockRejectedValueOnce(new Error("network down"));

    const { result } = renderHook(() => useCoordinators("w1"));
    await waitFor(() => expect(result.current.loadError).toBe(true));
    expect(storeState.setCoordinators).not.toHaveBeenCalled();

    mockListCoordinators.mockResolvedValueOnce({ coordinators: [coordinator()] });
    await act(async () => {
      result.current.refresh();
    });

    await waitFor(() => expect(result.current.loadError).toBe(false));
    expect(storeState.setCoordinators).toHaveBeenCalledWith([coordinator()]);
  });

  it("reports a load error when a retry itself fails again", async () => {
    mockListCoordinators.mockRejectedValue(new Error("still down"));

    const { result } = renderHook(() => useCoordinators("w1"));
    await waitFor(() => expect(result.current.loadError).toBe(true));

    await act(async () => {
      result.current.refresh();
    });

    await waitFor(() => expect(mockListCoordinators).toHaveBeenCalledTimes(2));
    expect(result.current.loadError).toBe(true);
  });
});
