import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  __resetSnapshotForTests,
  useDefaultQueryPresets,
  type StoredQueryPreset,
} from "./use-default-query-presets";

const STORAGE_KEY = "kandev:github-default-queries:v1";
const SYNC_FAILED_KEY = "kandev:github-default-queries:sync-failed:v1";

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

const preset: StoredQueryPreset = {
  value: "mine",
  label: "Mine",
  filter: "author:@me is:open",
  group: "created",
};

describe("useDefaultQueryPresets", () => {
  beforeEach(() => {
    localStorageMock.clear();
    __resetSnapshotForTests();
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { github_default_query_presets: null },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);
  });

  it("retries local defaults after a failed backend sync and clears the marker", async () => {
    const local = { pr: [preset], issue: [] };
    localStorageMock.setItem(STORAGE_KEY, JSON.stringify(local));
    localStorageMock.setItem(SYNC_FAILED_KEY, "1");

    renderHook(() => useDefaultQueryPresets());

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        github_default_query_presets: local,
      });
      expect(localStorageMock.getItem(SYNC_FAILED_KEY)).toBeNull();
    });
  });

  it("retries null local defaults after a failed reset sync", async () => {
    localStorageMock.setItem(SYNC_FAILED_KEY, "1");
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { github_default_query_presets: { pr: [preset], issue: [] } },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    renderHook(() => useDefaultQueryPresets());

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        github_default_query_presets: null,
      });
      expect(localStorageMock.getItem(SYNC_FAILED_KEY)).toBeNull();
    });
  });
});
