import { describe, it, expect, beforeEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import {
  __resetSnapshotForTests,
  readStorage,
  useSavedPresets,
  type SavedPreset,
} from "./use-saved-presets";

const STORAGE_KEY = "kandev:github-presets:v1";
const SYNC_FAILED_KEY = "kandev:github-presets:sync-failed:v1";
const WORKSPACE_ID = "ws-1";
const SETTINGS_TIMESTAMP = "2026-01-01T00:00:00Z";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubWorkspaceSettings: vi.fn(),
  updateGitHubWorkspaceSettings: vi.fn(),
}));

// Provide a simple in-memory localStorage mock so the tests are not sensitive
// to how the test runner exposes window.localStorage (e.g. Node's
// --localstorage-file flag without a valid path).
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
Object.defineProperty(window, "localStorage", { value: localStorageMock, configurable: true });

function set(raw: string | null) {
  if (raw === null) localStorageMock.removeItem(STORAGE_KEY);
  else localStorageMock.setItem(STORAGE_KEY, raw);
}

const valid: SavedPreset = {
  id: "p_1",
  kind: "pr",
  label: "My PRs",
  customQuery: "author:@me",
  repoFilter: "",
  createdAt: SETTINGS_TIMESTAMP,
};

function workspaceSettings(
  savedPresets: SavedPreset[] = [],
): Awaited<ReturnType<typeof fetchGitHubWorkspaceSettings>> {
  return {
    workspace_id: WORKSPACE_ID,
    repo_scope_mode: "all",
    repo_scope_orgs: [],
    repo_scope_repos: [],
    saved_presets: savedPresets,
    default_query_presets: null,
    created_at: SETTINGS_TIMESTAMP,
    updated_at: SETTINGS_TIMESTAMP,
  } as Awaited<ReturnType<typeof fetchGitHubWorkspaceSettings>>;
}

describe("readStorage", () => {
  beforeEach(() => {
    localStorageMock.clear();
    __resetSnapshotForTests();
    vi.mocked(fetchUserSettings).mockReset();
    vi.mocked(updateUserSettings).mockReset();
    vi.mocked(fetchGitHubWorkspaceSettings).mockReset();
    vi.mocked(updateGitHubWorkspaceSettings).mockReset();
  });

  it("returns empty array when no value is stored", () => {
    expect(readStorage()).toEqual([]);
  });

  it("returns empty array for malformed JSON", () => {
    set("not-json{");
    expect(readStorage()).toEqual([]);
  });

  it("returns empty array when parsed value is not an array", () => {
    set(JSON.stringify({ id: "p_1" }));
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
    set(JSON.stringify([{ ...valid, kind: "commit" }, valid]));
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
});

describe("useSavedPresets", () => {
  beforeEach(() => {
    localStorageMock.clear();
    __resetSnapshotForTests();
    vi.mocked(fetchUserSettings).mockReset();
    vi.mocked(updateUserSettings).mockReset();
    vi.mocked(fetchGitHubWorkspaceSettings).mockReset();
    vi.mocked(updateGitHubWorkspaceSettings).mockReset();
  });

  it("retries local presets after a failed backend sync and clears the marker", async () => {
    set(JSON.stringify([valid]));
    localStorageMock.setItem(SYNC_FAILED_KEY, "1");
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { github_saved_presets: [] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);

    renderHook(() => useSavedPresets());

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({ github_saved_presets: [valid] });
      expect(localStorageMock.getItem(SYNC_FAILED_KEY)).toBeNull();
    });
  });

  it("retries empty local presets after a failed backend sync", async () => {
    set(JSON.stringify([]));
    localStorageMock.setItem(SYNC_FAILED_KEY, "1");
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { github_saved_presets: [valid] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);

    renderHook(() => useSavedPresets());

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({ github_saved_presets: [] });
      expect(localStorageMock.getItem(SYNC_FAILED_KEY)).toBeNull();
    });
  });

  it("migrates local presets into a fresh workspace", async () => {
    set(JSON.stringify([valid]));
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings());
    vi.mocked(updateGitHubWorkspaceSettings).mockResolvedValue(
      {} as Awaited<ReturnType<typeof updateGitHubWorkspaceSettings>>,
    );

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() => expect(result.current.presets).toEqual([valid]));
    expect(updateGitHubWorkspaceSettings).toHaveBeenCalledWith({
      workspace_id: WORKSPACE_ID,
      saved_presets: [valid],
    });
  });

  it("does not migrate local presets over existing workspace presets", async () => {
    const server = { ...valid, id: "p_server", label: "Server" };
    set(JSON.stringify([valid]));
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings([server]));

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() => expect(result.current.presets).toEqual([server]));
    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });

  it("does not save while workspace presets are still loading", () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    let created: SavedPreset | null = null;
    act(() => {
      created = result.current.save({
        kind: "pr",
        label: "Loading",
        customQuery: "is:open",
        repoFilter: "",
      });
    });

    expect(created).toBeNull();
    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });
});
