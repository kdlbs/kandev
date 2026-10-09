import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { ApiError } from "@/lib/api/client";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { useSavedViews, type SavedView } from "./use-saved-views";

const STORAGE_KEY = "kandev:jira:saved-views:v1";
const UNASSIGNED_VIEW_ID = "builtin:unassigned";
const WRITE_ERROR = "write failed";
const NEW_VIEW_NAME = "New view";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}));

function makeLocalStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
    clear: () => store.clear(),
    get length() {
      return store.size;
    },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);

const view: SavedView = {
  id: "custom:one",
  name: "Mine",
  filters: {
    projectKeys: [],
    statuses: [],
    assignee: "me",
    searchText: "",
    sort: "updated",
  },
  customJql: null,
  builtin: false,
};

function resetSettingsMocks() {
  localStorageMock.clear();
  vi.mocked(fetchUserSettings).mockResolvedValue({
    settings: { jira_saved_views: [] },
  } as Awaited<ReturnType<typeof fetchUserSettings>>);
  vi.mocked(updateUserSettings).mockResolvedValue({
    settings: {},
  } as Awaited<ReturnType<typeof updateUserSettings>>);
}

describe("useSavedViews server hydration", () => {
  beforeEach(resetSettingsMocks);

  it("ignores stale local views when backend settings are empty", async () => {
    localStorageMock.setItem(STORAGE_KEY, JSON.stringify([view]));

    const { result } = renderHook(() => useSavedViews());

    await waitFor(() => expect(result.current.custom).toEqual([]));
    expect(updateUserSettings).not.toHaveBeenCalled();
  });

  it("hydrates a legacy statusCategories view to statuses: [] without throwing", async () => {
    // Views persisted before the status-name migration carry `statusCategories`
    // and no `statuses`. They must load, dropping the old category filter.
    const legacy = {
      id: "custom:legacy",
      name: "Legacy",
      filters: {
        projectKeys: ["CLIP"],
        statusCategories: ["indeterminate"],
        assignee: "me",
        searchText: "",
        sort: "updated",
      },
    };
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { jira_saved_views: [legacy] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    const { result } = renderHook(() => useSavedViews());

    await waitFor(() => {
      const loaded = result.current.custom.find((v) => v.id === "custom:legacy");
      expect(loaded).toBeDefined();
      expect(loaded?.filters.statuses).toEqual([]);
      expect(loaded?.filters.projectKeys).toEqual(["CLIP"]);
      expect("statusCategories" in loaded!.filters).toBe(false);
    });
  });
});

describe("useSavedViews mutation queue", () => {
  beforeEach(resetSettingsMocks);

  it("replays a saved view mutation on hydrated server views", async () => {
    const settings = deferred<Awaited<ReturnType<typeof fetchUserSettings>>>();
    vi.mocked(fetchUserSettings).mockReturnValueOnce(settings.promise);
    const serverView = { ...view, id: "custom:server", name: "Server view" };

    const { result } = renderHook(() => useSavedViews());
    const savePromise = result.current.save(NEW_VIEW_NAME, view.filters, null);

    expect(updateUserSettings).not.toHaveBeenCalled();

    settings.resolve({
      settings: { jira_saved_views: [serverView] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    let saved!: SavedView;
    await act(async () => {
      saved = await savePromise;
    });
    await waitFor(() => {
      expect(result.current.custom.map((savedView) => savedView.id)).toEqual([
        "custom:server",
        saved.id,
      ]);
      expect(updateUserSettings).toHaveBeenCalledWith({
        jira_saved_views: expect.arrayContaining([
          expect.objectContaining({ id: "custom:server" }),
          expect.objectContaining({ id: saved.id }),
        ]),
      });
    });
  });

  it("does not sync queued mutations when fetching settings fails", async () => {
    vi.mocked(fetchUserSettings).mockRejectedValueOnce(new Error("network unavailable"));
    vi.mocked(fetchUserSettings).mockRejectedValueOnce(new Error("network still unavailable"));
    const fetchCallsBefore = vi.mocked(fetchUserSettings).mock.calls.length;
    const syncCallsBefore = vi.mocked(updateUserSettings).mock.calls.length;

    const { result } = renderHook(() => useSavedViews());
    await waitFor(() => expect(fetchUserSettings).toHaveBeenCalledTimes(fetchCallsBefore + 1));
    let savePromise!: Promise<SavedView>;
    act(() => {
      savePromise = result.current.save(NEW_VIEW_NAME, view.filters, null);
    });

    await act(async () => {
      await expect(savePromise).rejects.toThrow("network still unavailable");
    });
    expect(fetchUserSettings).toHaveBeenCalledTimes(fetchCallsBefore + 2);
    expect(updateUserSettings).toHaveBeenCalledTimes(syncCallsBefore);
  });

  it("keeps saved views visible and retries hydration after a fetch failure", async () => {
    vi.mocked(fetchUserSettings).mockRejectedValueOnce(new Error("network unavailable"));
    const fetchCallsBefore = vi.mocked(fetchUserSettings).mock.calls.length;

    const { result } = renderHook(() => useSavedViews());
    await waitFor(() => expect(fetchUserSettings).toHaveBeenCalledTimes(fetchCallsBefore + 1));

    let saved!: SavedView;
    let savePromise!: Promise<SavedView>;
    act(() => {
      savePromise = result.current.save(NEW_VIEW_NAME, view.filters, null);
    });

    expect(result.current.custom).toEqual([]);
    await act(async () => {
      saved = await savePromise;
    });
    await waitFor(() => {
      expect(fetchUserSettings).toHaveBeenCalledTimes(fetchCallsBefore + 2);
      expect(updateUserSettings).toHaveBeenCalledWith({
        jira_saved_views: [expect.objectContaining({ id: saved.id })],
      });
    });
  });
});

describe("useSavedViews default persistence", () => {
  beforeEach(resetSettingsMocks);

  it("hydrates the saved views and default ID together", async () => {
    const settings = deferred<Awaited<ReturnType<typeof fetchUserSettings>>>();
    vi.mocked(fetchUserSettings).mockReturnValueOnce(settings.promise);
    const { result } = renderHook(() => useSavedViews());

    expect(result.current.ready).toBe(false);
    expect(result.current.defaultViewId).toBe("");

    settings.resolve({
      settings: { jira_saved_views: [view], jira_default_view_id: view.id },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    await waitFor(() => expect(result.current.ready).toBe(true));
    expect(result.current.custom).toEqual([view]);
    expect(result.current.defaultViewId).toBe(view.id);
  });

  it("publishes a default replacement and clear only after the server acknowledges each write", async () => {
    vi.mocked(fetchUserSettings).mockResolvedValueOnce({
      settings: { jira_saved_views: [view], jira_default_view_id: view.id },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    const { result } = renderHook(() => useSavedViews());
    await waitFor(() => expect(result.current.ready).toBe(true));

    const replacement = deferred<Awaited<ReturnType<typeof updateUserSettings>>>();
    vi.mocked(updateUserSettings).mockReturnValueOnce(replacement.promise);
    let replacePromise!: Promise<boolean>;
    act(() => {
      replacePromise = result.current.setDefaultView(UNASSIGNED_VIEW_ID);
    });
    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        jira_default_view_id: UNASSIGNED_VIEW_ID,
      }),
    );
    expect(result.current.defaultViewId).toBe(view.id);

    await act(async () => {
      replacement.resolve({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);
      await replacePromise;
    });
    expect(result.current.defaultViewId).toBe(UNASSIGNED_VIEW_ID);

    await act(async () => {
      await result.current.setDefaultView("");
    });
    expect(updateUserSettings).toHaveBeenLastCalledWith({ jira_default_view_id: "" });
    expect(result.current.defaultViewId).toBe("");
  });

  it("keeps the previous default after a failed write", async () => {
    vi.mocked(fetchUserSettings).mockResolvedValueOnce({
      settings: { jira_saved_views: [view], jira_default_view_id: view.id },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    const { result } = renderHook(() => useSavedViews());
    await waitFor(() => expect(result.current.ready).toBe(true));
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError(WRITE_ERROR, 400, null));

    await act(async () => {
      await expect(result.current.setDefaultView(UNASSIGNED_VIEW_ID)).rejects.toThrow(WRITE_ERROR);
    });
    expect(result.current.defaultViewId).toBe(view.id);
  });
});

describe("useSavedViews deletion", () => {
  beforeEach(resetSettingsMocks);

  it("deletes a default view and clears its ID in one acknowledged settings patch", async () => {
    vi.mocked(fetchUserSettings).mockResolvedValueOnce({
      settings: { jira_saved_views: [view], jira_default_view_id: view.id },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    const { result } = renderHook(() => useSavedViews());
    await waitFor(() => expect(result.current.ready).toBe(true));
    const deletion = deferred<Awaited<ReturnType<typeof updateUserSettings>>>();
    vi.mocked(updateUserSettings).mockReturnValueOnce(deletion.promise);

    let removePromise!: Promise<boolean>;
    act(() => {
      removePromise = result.current.remove(view.id);
    });
    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        jira_saved_views: [],
        jira_default_view_id: "",
      }),
    );
    expect(result.current.custom).toEqual([view]);
    expect(result.current.defaultViewId).toBe(view.id);

    await act(async () => {
      deletion.resolve({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);
      await removePromise;
    });
    expect(result.current.custom).toEqual([]);
    expect(result.current.defaultViewId).toBe("");
  });

  it("does not save a stale view list while default deletion is pending", async () => {
    vi.mocked(fetchUserSettings).mockResolvedValueOnce({
      settings: { jira_saved_views: [view], jira_default_view_id: view.id },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    const { result } = renderHook(() => useSavedViews());
    await waitFor(() => expect(result.current.ready).toBe(true));
    const deletion = deferred<Awaited<ReturnType<typeof updateUserSettings>>>();
    vi.mocked(updateUserSettings).mockReturnValueOnce(deletion.promise);
    const syncCallsBefore = vi.mocked(updateUserSettings).mock.calls.length;

    let removePromise!: Promise<boolean>;
    act(() => {
      removePromise = result.current.remove(view.id);
    });
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(syncCallsBefore + 1));

    await act(async () => {
      await expect(result.current.save(NEW_VIEW_NAME, view.filters, null)).rejects.toThrow();
    });
    expect(updateUserSettings).toHaveBeenCalledTimes(syncCallsBefore + 1);

    await act(async () => {
      deletion.resolve({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);
      await removePromise;
    });
    let saved!: SavedView;
    await act(async () => {
      saved = await result.current.save(NEW_VIEW_NAME, view.filters, null);
    });

    expect(result.current.custom).toEqual([saved]);
    expect(updateUserSettings).toHaveBeenLastCalledWith({ jira_saved_views: [saved] });
  });
});

describe("useSavedViews rejected writes", () => {
  beforeEach(resetSettingsMocks);

  it("does not offer a rejected saved view as a default", async () => {
    vi.mocked(fetchUserSettings).mockResolvedValueOnce({
      settings: { jira_saved_views: [] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    const { result } = renderHook(() => useSavedViews());
    await waitFor(() => expect(result.current.ready).toBe(true));
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError(WRITE_ERROR, 400, null));
    const syncCallsBefore = vi.mocked(updateUserSettings).mock.calls.length;

    let savePromise!: Promise<SavedView>;
    act(() => {
      savePromise = result.current.save("Unpersisted", view.filters, null);
    });
    await act(async () => {
      await expect(savePromise).rejects.toThrow(WRITE_ERROR);
    });

    const failedViews = vi.mocked(updateUserSettings).mock.calls.at(-1)?.[0].jira_saved_views;
    const failedViewId = (failedViews?.[0] as SavedView | undefined)?.id;
    expect(failedViewId).toBeDefined();
    expect(result.current.custom).toEqual([]);
    await act(async () => {
      await expect(result.current.setDefaultView(failedViewId!)).resolves.toBe(false);
    });
    expect(result.current.defaultViewId).toBe("");
    expect(updateUserSettings).toHaveBeenCalledTimes(syncCallsBefore + 1);
  });

  it("retains the saved view and default when deleting the default fails", async () => {
    vi.mocked(fetchUserSettings).mockResolvedValueOnce({
      settings: { jira_saved_views: [view], jira_default_view_id: view.id },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    const { result } = renderHook(() => useSavedViews());
    await waitFor(() => expect(result.current.ready).toBe(true));
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError("delete failed", 400, null));

    await act(async () => {
      await expect(result.current.remove(view.id)).rejects.toThrow("delete failed");
    });
    expect(result.current.custom).toEqual([view]);
    expect(result.current.defaultViewId).toBe(view.id);
  });
});
