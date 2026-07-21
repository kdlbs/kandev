import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import {
  __resetSnapshotForTests,
  useDefaultQueryPresets,
  type StoredQueryPreset,
} from "./use-default-query-presets";

const STORAGE_KEY = "kandev:github-default-queries:v1";
const WORKSPACE_ID = "ws-1";
const SETTINGS_TIMESTAMP = "2026-01-01T00:00:00Z";
const DEFAULT_QUERIES_LOADING_ERROR = "Default queries are still loading";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubWorkspaceSettings: vi.fn(),
  updateGitHubWorkspaceSettings: vi.fn(),
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
Object.defineProperty(window, "localStorage", { value: localStorageMock, configurable: true });

const preset: StoredQueryPreset = {
  value: "mine",
  label: "Mine",
  filter: "author:@me is:open",
  group: "created",
};

function workspaceSettings(
  defaultQueryPresets: unknown = null,
): Awaited<ReturnType<typeof fetchGitHubWorkspaceSettings>> {
  return {
    workspace_id: WORKSPACE_ID,
    repo_scope_mode: "all",
    repo_scope_orgs: [],
    repo_scope_repos: [],
    saved_presets: [],
    default_query_presets: defaultQueryPresets,
    created_at: SETTINGS_TIMESTAMP,
    updated_at: SETTINGS_TIMESTAMP,
  } as Awaited<ReturnType<typeof fetchGitHubWorkspaceSettings>>;
}

function resetMocks() {
  vi.clearAllMocks();
  localStorageMock.clear();
  __resetSnapshotForTests();
  vi.mocked(fetchUserSettings).mockResolvedValue({
    settings: { github_default_query_presets: null },
  } as Awaited<ReturnType<typeof fetchUserSettings>>);
  vi.mocked(updateUserSettings).mockResolvedValue({
    settings: {},
  } as Awaited<ReturnType<typeof updateUserSettings>>);
  vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings());
  vi.mocked(updateGitHubWorkspaceSettings).mockResolvedValue(
    {} as Awaited<ReturnType<typeof updateGitHubWorkspaceSettings>>,
  );
}

describe("useDefaultQueryPresets workspace sync", () => {
  beforeEach(resetMocks);

  it("ignores stale local defaults when workspace settings are empty", async () => {
    const local = { pr: [preset], issue: [] };
    localStorageMock.setItem(STORAGE_KEY, JSON.stringify(local));
    __resetSnapshotForTests();

    const { result } = renderHook(() => useDefaultQueryPresets(WORKSPACE_ID));

    await waitFor(() => expect(result.current.prPresets[0]?.value).not.toBe("mine"));
    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });

  it("does not migrate local defaults over existing workspace defaults", async () => {
    const server = { pr: [{ ...preset, value: "server", label: "Server" }], issue: [] };
    localStorageMock.setItem(STORAGE_KEY, JSON.stringify({ pr: [preset], issue: [] }));
    __resetSnapshotForTests();
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings(server));

    const { result } = renderHook(() => useDefaultQueryPresets(WORKSPACE_ID));

    await waitFor(() => expect(result.current.prPresets[0]?.value).toBe("server"));
    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });

  it("does not save or reset while workspace defaults are still loading", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useDefaultQueryPresets(WORKSPACE_ID));

    await expect(result.current.save({ pr: [preset], issue: [] })).rejects.toThrow(
      DEFAULT_QUERIES_LOADING_ERROR,
    );
    await expect(result.current.reset()).rejects.toThrow(DEFAULT_QUERIES_LOADING_ERROR);

    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });

  it("does not save or reset after workspace defaults fail to load", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockRejectedValue(new Error("settings down"));
    const { result } = renderHook(() => useDefaultQueryPresets(WORKSPACE_ID));

    await waitFor(() => expect(fetchGitHubWorkspaceSettings).toHaveBeenCalled());

    await expect(result.current.save({ pr: [preset], issue: [] })).rejects.toThrow(
      DEFAULT_QUERIES_LOADING_ERROR,
    );
    await expect(result.current.reset()).rejects.toThrow(DEFAULT_QUERIES_LOADING_ERROR);

    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });
});
