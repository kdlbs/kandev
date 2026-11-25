import { describe, it, expect, beforeEach, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { makeLocalStorageMock } from "@/hooks/local-storage-mock.test-helpers";
import { useWorkflowListView, WORKFLOW_LIST_VIEW_STORAGE_KEY } from "./use-workflow-list-view";

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", { value: localStorageMock, configurable: true });

describe("useWorkflowListView", () => {
  beforeEach(() => {
    localStorageMock.clear();
    vi.restoreAllMocks();
  });

  it("defaults to compact when nothing is stored", () => {
    const { result } = renderHook(() => useWorkflowListView());
    expect(result.current.view).toBe("compact");
  });

  it("reads a stored detailed preference", () => {
    localStorageMock.setItem(WORKFLOW_LIST_VIEW_STORAGE_KEY, "detailed");
    const { result } = renderHook(() => useWorkflowListView());
    expect(result.current.view).toBe("detailed");
  });

  it("treats an unknown stored value as compact", () => {
    localStorageMock.setItem(WORKFLOW_LIST_VIEW_STORAGE_KEY, "grid");
    const { result } = renderHook(() => useWorkflowListView());
    expect(result.current.view).toBe("compact");
  });

  it("falls back to compact when storage reads throw", () => {
    vi.spyOn(localStorageMock, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    const { result } = renderHook(() => useWorkflowListView());
    expect(result.current.view).toBe("compact");
  });

  it("persists the selected view", () => {
    const { result } = renderHook(() => useWorkflowListView());
    act(() => result.current.setView("detailed"));
    expect(result.current.view).toBe("detailed");
    expect(localStorageMock.getItem(WORKFLOW_LIST_VIEW_STORAGE_KEY)).toBe("detailed");
  });

  it("still switches for the session when storage writes throw", () => {
    vi.spyOn(localStorageMock, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    const { result } = renderHook(() => useWorkflowListView());
    act(() => result.current.setView("detailed"));
    expect(result.current.view).toBe("detailed");
  });
});
