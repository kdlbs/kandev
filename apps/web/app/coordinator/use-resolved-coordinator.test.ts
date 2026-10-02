import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const mockUseCoordinatorList = vi.fn();

vi.mock("./use-coordinator-list", () => ({
  useCoordinatorList: (...args: unknown[]) => mockUseCoordinatorList(...args),
}));

import { useResolvedCoordinator } from "./use-resolved-coordinator";

const WORKSPACE_ID = "workspace-1";
const retryMock = vi.fn();

function coordinator(id: string): Coordinator {
  return {
    id,
    workspace_id: WORKSPACE_ID,
    name: `Coordinator ${id}`,
    agent_profile_id: "agent-1",
    executor_profile_id: "exec-1",
    task_agent_profile_id: "ap-1",
    task_executor_profile_id: "ep-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useResolvedCoordinator", () => {
  it("returns loading while the coordinator list has not loaded", () => {
    mockUseCoordinatorList.mockReturnValue({
      coordinators: undefined,
      error: false,
      retry: retryMock,
    });

    const { result } = renderHook(() => useResolvedCoordinator(WORKSPACE_ID, "c-1"));

    expect(result.current).toEqual({ status: "loading" });
  });

  it("returns list-error when the list read failed", () => {
    mockUseCoordinatorList.mockReturnValue({
      coordinators: undefined,
      error: true,
      retry: retryMock,
    });

    const { result } = renderHook(() => useResolvedCoordinator(WORKSPACE_ID, "c-1"));

    expect(result.current).toEqual({ status: "list-error", retry: retryMock });
  });

  it("returns no-coordinator when the workspace has none, regardless of the requested id", () => {
    mockUseCoordinatorList.mockReturnValue({ coordinators: [], error: false, retry: retryMock });

    expect(renderHook(() => useResolvedCoordinator(WORKSPACE_ID, "c-1")).result.current).toEqual({
      status: "no-coordinator",
    });
    expect(renderHook(() => useResolvedCoordinator(WORKSPACE_ID, null)).result.current).toEqual({
      status: "no-coordinator",
    });
  });

  it("returns redirect to the first coordinator for the generic route", () => {
    mockUseCoordinatorList.mockReturnValue({
      coordinators: [coordinator("c-1"), coordinator("c-2")],
      error: false,
      retry: retryMock,
    });

    const { result } = renderHook(() => useResolvedCoordinator(WORKSPACE_ID, null));

    expect(result.current).toEqual({ status: "redirect", target: coordinator("c-1") });
  });

  it("returns unknown-coordinator when the id is not in the list", () => {
    mockUseCoordinatorList.mockReturnValue({
      coordinators: [coordinator("c-1")],
      error: false,
      retry: retryMock,
    });

    const { result } = renderHook(() => useResolvedCoordinator(WORKSPACE_ID, "c-missing"));

    expect(result.current).toEqual({ status: "unknown-coordinator", retry: retryMock });
  });

  it("returns ready with the matching coordinator", () => {
    const coordinators = [coordinator("c-1"), coordinator("c-2")];
    mockUseCoordinatorList.mockReturnValue({ coordinators, error: false, retry: retryMock });

    const { result } = renderHook(() => useResolvedCoordinator(WORKSPACE_ID, "c-2"));

    expect(result.current).toEqual({
      status: "ready",
      coordinator: coordinator("c-2"),
      coordinators,
    });
  });
});
