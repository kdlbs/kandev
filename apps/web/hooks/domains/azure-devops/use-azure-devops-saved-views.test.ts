import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  getAzureDevOpsSavedViews,
  setAzureDevOpsSavedViews,
} from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsSavedView } from "@/lib/types/azure-devops";
import { useAzureDevOpsSavedViews } from "./use-azure-devops-saved-views";

vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  getAzureDevOpsSavedViews: vi.fn(),
  setAzureDevOpsSavedViews: vi.fn(),
}));

const WORKSPACE_ID = "workspace-1";
const view: AzureDevOpsSavedView = {
  id: "mine",
  kind: "work_item",
  label: "My work",
  projectId: "project-1",
  wiql: "SELECT [System.Id] FROM WorkItems",
  createdAt: "2026-07-20T12:00:00Z",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe("useAzureDevOpsSavedViews", () => {
  beforeEach(() => {
    vi.mocked(getAzureDevOpsSavedViews).mockReset();
    vi.mocked(setAzureDevOpsSavedViews).mockReset();
  });

  it("loads saved views and treats a load failure as empty", async () => {
    vi.mocked(getAzureDevOpsSavedViews)
      .mockResolvedValueOnce({ views: [view] })
      .mockRejectedValueOnce(new Error("unavailable"));

    const loaded = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(loaded.result.current.loading).toBe(false));
    expect(loaded.result.current.views).toEqual([view]);
    loaded.unmount();

    const failed = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(failed.result.current.loading).toBe(false));
    expect(failed.result.current.views).toEqual([]);
  });

  it("optimistically replaces views and commits the server-normalized list", async () => {
    const response = deferred<{ views: AzureDevOpsSavedView[] }>();
    const normalized = { ...view, label: "Normalized" };
    vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: [] });
    vi.mocked(setAzureDevOpsSavedViews).mockReturnValue(response.promise);
    const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(result.current.loading).toBe(false));

    let replacePromise!: Promise<void>;
    act(() => {
      replacePromise = result.current.replace([view]);
    });
    expect(result.current.views).toEqual([view]);

    await act(async () => {
      response.resolve({ views: [normalized] });
      await replacePromise;
    });
    expect(result.current.views).toEqual([normalized]);
  });

  it("rolls back an optimistic replacement when persistence fails", async () => {
    vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: [view] });
    vi.mocked(setAzureDevOpsSavedViews).mockRejectedValue(new Error("save failed"));
    const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(result.current.views).toEqual([view]));

    const replacement = { ...view, id: "replacement" };
    await expect(
      act(async () => {
        await result.current.replace([replacement]);
      }),
    ).rejects.toThrow("save failed");
    expect(result.current.views).toEqual([view]);
  });

  it("removes the target view through replace", async () => {
    const other = { ...view, id: "other" };
    vi.mocked(getAzureDevOpsSavedViews).mockResolvedValue({ views: [view, other] });
    vi.mocked(setAzureDevOpsSavedViews).mockResolvedValue({ views: [other] });
    const { result } = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    await waitFor(() => expect(result.current.views).toEqual([view, other]));

    await act(async () => result.current.remove(view.id));
    expect(setAzureDevOpsSavedViews).toHaveBeenCalledWith(WORKSPACE_ID, [other]);
    expect(result.current.views).toEqual([other]);
  });

  it("ignores an in-flight load after unmount", async () => {
    const response = deferred<{ views: AzureDevOpsSavedView[] }>();
    vi.mocked(getAzureDevOpsSavedViews).mockReturnValue(response.promise);
    const hook = renderHook(() => useAzureDevOpsSavedViews(WORKSPACE_ID));
    hook.unmount();

    await act(async () => {
      response.resolve({ views: [view] });
      await response.promise;
    });
    expect(hook.result.current.views).toEqual([]);
  });
});
