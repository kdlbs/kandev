import { describe, it, expect, beforeEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import { __resetSnapshotForTests, useSavedPresets, type SavedPreset } from "./use-saved-presets";

const STORAGE_KEY = "kandev:github-presets:v1";
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

function set(raw: string | null) {
  if (raw === null) window.localStorage.removeItem(STORAGE_KEY);
  else window.localStorage.setItem(STORAGE_KEY, raw);
}

const valid: SavedPreset = {
  id: "p_1",
  kind: "pr",
  label: "My PRs",
  customQuery: "author:@me",
  repoFilter: "",
  createdAt: SETTINGS_TIMESTAMP,
};

function resetTestState() {
  window.localStorage.clear();
  __resetSnapshotForTests();
  vi.mocked(fetchUserSettings).mockReset();
  vi.mocked(updateUserSettings).mockReset();
  vi.mocked(fetchGitHubWorkspaceSettings).mockReset();
  vi.mocked(updateGitHubWorkspaceSettings).mockReset();
}

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

describe("useSavedPresets", () => {
  beforeEach(() => {
    resetTestState();
  });

  it("ignores stale local presets when backend settings are empty", async () => {
    set(JSON.stringify([valid]));
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { github_saved_presets: [] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    const { result } = renderHook(() => useSavedPresets());

    await waitFor(() => expect(result.current.presets).toEqual([]));
    expect(updateUserSettings).not.toHaveBeenCalled();
  });
});

describe("useSavedPresets workspace sync", () => {
  beforeEach(() => {
    resetTestState();
  });

  it("ignores stale local presets when workspace settings are empty", async () => {
    set(JSON.stringify([valid]));
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings());

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() => expect(result.current.presets).toEqual([]));
    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
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

  it("does not save after workspace presets fail to load", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockRejectedValue(new Error("settings down"));

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() => expect(fetchGitHubWorkspaceSettings).toHaveBeenCalled());

    let created: SavedPreset | null = valid;
    act(() => {
      created = result.current.save({
        kind: "pr",
        label: "Failed load",
        customQuery: "is:open",
        repoFilter: "",
      });
    });

    expect(created).toBeNull();
    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });

  it("does not remove while workspace presets are still loading", () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    act(() => {
      result.current.remove("p_1");
    });

    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });

  it("does not remove after workspace presets fail to load", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockRejectedValue(new Error("settings down"));

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() => expect(fetchGitHubWorkspaceSettings).toHaveBeenCalled());

    act(() => {
      result.current.remove("p_1");
    });

    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });
});
