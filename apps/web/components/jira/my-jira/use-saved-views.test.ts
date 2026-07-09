import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { useSavedViews, type SavedView } from "./use-saved-views";

const STORAGE_KEY = "kandev:jira:saved-views:v1";
const SYNC_FAILED_KEY = "kandev:jira:saved-views:sync-failed:v1";

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

describe("useSavedViews", () => {
  beforeEach(() => {
    localStorageMock.clear();
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { jira_saved_views: [] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);
  });

  it("retries local views after a failed backend sync and clears the marker", async () => {
    localStorageMock.setItem(STORAGE_KEY, JSON.stringify([view]));
    localStorageMock.setItem(SYNC_FAILED_KEY, "1");

    renderHook(() => useSavedViews());

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({ jira_saved_views: [view] });
      expect(localStorageMock.getItem(SYNC_FAILED_KEY)).toBeNull();
    });
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
    localStorageMock.setItem(STORAGE_KEY, JSON.stringify([legacy]));

    const { result } = renderHook(() => useSavedViews());

    await waitFor(() => {
      const loaded = result.current.custom.find((v) => v.id === "custom:legacy");
      expect(loaded).toBeDefined();
      expect(loaded?.filters.statuses).toEqual([]);
      expect(loaded?.filters.projectKeys).toEqual(["CLIP"]);
      expect("statusCategories" in loaded!.filters).toBe(false);
    });
  });

  it("retries empty local views after a failed backend sync", async () => {
    localStorageMock.setItem(STORAGE_KEY, JSON.stringify([]));
    localStorageMock.setItem(SYNC_FAILED_KEY, "1");
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { jira_saved_views: [view] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    renderHook(() => useSavedViews());

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({ jira_saved_views: [] });
      expect(localStorageMock.getItem(SYNC_FAILED_KEY)).toBeNull();
    });
  });
});
