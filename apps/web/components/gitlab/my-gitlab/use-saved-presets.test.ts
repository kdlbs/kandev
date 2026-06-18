import { describe, it, expect, beforeEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  __resetSnapshotForTests,
  readStorage,
  useSavedPresets,
  type SavedPreset,
} from "./use-saved-presets";

const STORAGE_KEY = "kandev:gitlab-presets:v1";
const SYNC_FAILED_KEY = "kandev:gitlab-presets:sync-failed:v1";

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

function set(raw: string | null) {
  if (raw === null) localStorageMock.removeItem(STORAGE_KEY);
  else localStorageMock.setItem(STORAGE_KEY, raw);
}

const valid: SavedPreset = {
  id: "g_1",
  kind: "mr",
  label: "My MRs",
  customQuery: "scope=created_by_me",
  projectFilter: "",
  createdAt: "2026-01-01T00:00:00Z",
};

describe("readStorage (gitlab)", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });

  it("returns empty array when no value is stored", () => {
    expect(readStorage()).toEqual([]);
  });

  it("returns empty array for malformed JSON", () => {
    set("not-json{");
    expect(readStorage()).toEqual([]);
  });

  it("returns empty array when parsed value is not an array", () => {
    set(JSON.stringify({ id: "g_1" }));
    expect(readStorage()).toEqual([]);
  });

  it("keeps valid entries", () => {
    set(JSON.stringify([valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops entries missing an id", () => {
    const missingId = { ...valid } as Partial<SavedPreset>;
    delete missingId.id;
    set(JSON.stringify([missingId, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops entries with invalid kind", () => {
    set(JSON.stringify([{ ...valid, kind: "pr" }, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops non-object entries", () => {
    set(JSON.stringify(["string", 42, null, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops entries with non-string label", () => {
    set(JSON.stringify([{ ...valid, label: 123 }, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("accepts issue kind", () => {
    const issue: SavedPreset = { ...valid, kind: "issue" };
    set(JSON.stringify([issue]));
    expect(readStorage()).toEqual([issue]);
  });

  it("drops entries missing customQuery", () => {
    const broken = { ...valid } as Partial<SavedPreset>;
    delete broken.customQuery;
    set(JSON.stringify([broken, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops entries missing projectFilter", () => {
    const broken = { ...valid } as Partial<SavedPreset>;
    delete broken.projectFilter;
    set(JSON.stringify([broken, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops entries missing createdAt", () => {
    const broken = { ...valid } as Partial<SavedPreset>;
    delete broken.createdAt;
    set(JSON.stringify([broken, valid]));
    expect(readStorage()).toEqual([valid]);
  });
});

describe("useSavedPresets (gitlab)", () => {
  beforeEach(() => {
    localStorageMock.clear();
    __resetSnapshotForTests();
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { gitlab_saved_presets: [] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);
  });

  it("save appends a preset and assigns id + createdAt", () => {
    const { result } = renderHook(() => useSavedPresets());
    act(() => {
      result.current.save({
        kind: "mr",
        label: "My MRs",
        customQuery: "scope=created_by_me",
        projectFilter: "",
      });
    });
    expect(result.current.presets).toHaveLength(1);
    const [p] = result.current.presets;
    expect(p.label).toBe("My MRs");
    expect(p.kind).toBe("mr");
    expect(p.id).toMatch(/^g_/);
    expect(p.createdAt).toBeTruthy();
  });

  it("remove deletes the preset", () => {
    const { result } = renderHook(() => useSavedPresets());
    let created!: SavedPreset;
    act(() => {
      created = result.current.save({
        kind: "issue",
        label: "Triage",
        customQuery: "labels=bug",
        projectFilter: "",
      });
    });
    expect(result.current.presets).toHaveLength(1);
    act(() => result.current.remove(created.id));
    expect(result.current.presets).toHaveLength(0);
  });

  it("two hook instances stay in sync via the module-level store", () => {
    const a = renderHook(() => useSavedPresets());
    const b = renderHook(() => useSavedPresets());
    act(() => {
      a.result.current.save({
        kind: "mr",
        label: "Shared",
        customQuery: "q",
        projectFilter: "",
      });
    });
    expect(a.result.current.presets).toHaveLength(1);
    expect(b.result.current.presets).toHaveLength(1);
    expect(b.result.current.presets[0].label).toBe("Shared");
  });

  it("retries local presets after a failed backend sync and clears the marker", async () => {
    set(JSON.stringify([valid]));
    localStorageMock.setItem(SYNC_FAILED_KEY, "1");

    renderHook(() => useSavedPresets());

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({ gitlab_saved_presets: [valid] });
      expect(localStorageMock.getItem(SYNC_FAILED_KEY)).toBeNull();
    });
  });
});
