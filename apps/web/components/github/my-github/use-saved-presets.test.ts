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
const SETTINGS_DOWN = "settings down";

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

const legacyValid = {
  id: "p_1",
  kind: "pr" as const,
  label: "My PRs",
  customQuery: "author:@me",
  repoFilter: "",
  createdAt: SETTINGS_TIMESTAMP,
};

const valid: SavedPreset = { ...legacyValid, isDefault: false };

type DefaultSetter = (kind: SavedPreset["kind"], id: string | null) => Promise<void>;

function defaultSetter(current: ReturnType<typeof useSavedPresets>): DefaultSetter {
  const setDefault = (current as unknown as { setDefault?: DefaultSetter }).setDefault;
  expect(setDefault, "setDefault hook action").toBeTypeOf("function");
  return setDefault as DefaultSetter;
}

function resetTestState() {
  window.localStorage.clear();
  __resetSnapshotForTests();
  vi.mocked(fetchUserSettings).mockReset();
  vi.mocked(updateUserSettings).mockReset();
  vi.mocked(fetchGitHubWorkspaceSettings).mockReset();
  vi.mocked(updateGitHubWorkspaceSettings).mockReset();
}

function workspaceSettings(
  savedPresets: unknown = [],
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
    vi.mocked(fetchGitHubWorkspaceSettings).mockRejectedValue(new Error(SETTINGS_DOWN));

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
    vi.mocked(fetchGitHubWorkspaceSettings).mockRejectedValue(new Error(SETTINGS_DOWN));

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() => expect(fetchGitHubWorkspaceSettings).toHaveBeenCalled());

    act(() => {
      result.current.remove("p_1");
    });

    expect(updateGitHubWorkspaceSettings).not.toHaveBeenCalled();
  });
});

describe("useSavedPresets repository defaults", () => {
  beforeEach(() => {
    resetTestState();
  });

  it("preserves the chosen repository in the workspace settings update", async () => {
    const server = { ...valid, id: "p_server", label: "Server" };
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings([server]));
    vi.mocked(updateGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings());

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() => expect(result.current.presets).toEqual([server]));
    let created: SavedPreset | null = null;
    act(() => {
      created = result.current.save({
        kind: "pr",
        label: "Kandev PRs",
        customQuery: "is:open",
        repoFilter: "kdlbs/kandev",
      });
    });

    expect((created as SavedPreset | null)?.isDefault).toBe(false);

    await waitFor(() =>
      expect(updateGitHubWorkspaceSettings).toHaveBeenCalledWith({
        workspace_id: WORKSPACE_ID,
        saved_presets: [
          server,
          expect.objectContaining({
            kind: "pr",
            label: "Kandev PRs",
            customQuery: "is:open",
            repoFilter: "kdlbs/kandev",
          }),
        ],
      }),
    );
  });

  it.each([
    {
      caseName: "missing",
      serverPreset: {
        id: "p_legacy_missing",
        kind: "pr",
        label: "Legacy missing repo",
        customQuery: "author:@me",
        createdAt: SETTINGS_TIMESTAMP,
      },
    },
    {
      caseName: "non-string",
      serverPreset: {
        id: "p_legacy_invalid",
        kind: "pr",
        label: "Legacy invalid repo",
        customQuery: "author:@me",
        repoFilter: 42,
        createdAt: SETTINGS_TIMESTAMP,
      },
    },
  ])("normalizes a $caseName server repoFilter to All repos", async ({ serverPreset }) => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings([serverPreset]));

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() =>
      expect(result.current.presets).toEqual([
        { ...serverPreset, repoFilter: "", isDefault: false },
      ]),
    );
  });

  it("normalizes a legacy saved query to a non-default view", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings([legacyValid]));

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() =>
      expect(result.current.presets).toEqual([{ ...legacyValid, isDefault: false }]),
    );
  });

  it("keeps only the first persisted default for each result kind", async () => {
    const issue = { ...valid, id: "issue-1", kind: "issue", isDefault: true };
    const duplicates = [
      { ...valid, id: "pr-1", isDefault: true },
      { ...valid, id: "pr-2", isDefault: true },
      issue,
    ];
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings(duplicates));

    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));

    await waitFor(() =>
      expect(result.current.presets.map(({ id, isDefault }) => ({ id, isDefault }))).toEqual([
        { id: "pr-1", isDefault: true },
        { id: "pr-2", isDefault: false },
        { id: "issue-1", isDefault: true },
      ]),
    );
  });
});

describe("useSavedPresets default persistence", () => {
  beforeEach(() => {
    resetTestState();
  });

  it("publishes a workspace default only after persistence succeeds", async () => {
    const prA = { ...valid, id: "pr-a", isDefault: true };
    const prB = { ...valid, id: "pr-b" };
    const issue = { ...valid, id: "issue-a", kind: "issue" as const, isDefault: true };
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings([prA, prB, issue]));
    let resolveUpdate!: (value: Awaited<ReturnType<typeof updateGitHubWorkspaceSettings>>) => void;
    vi.mocked(updateGitHubWorkspaceSettings).mockReturnValue(
      new Promise((resolve) => {
        resolveUpdate = resolve;
      }),
    );
    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));
    await waitFor(() => expect(result.current.presets).toHaveLength(3));

    let mutation!: Promise<void>;
    act(() => {
      mutation = defaultSetter(result.current)("pr", "pr-b");
    });
    expect(result.current.presets.map((preset) => preset.isDefault)).toEqual([true, false, true]);

    await act(async () => {
      resolveUpdate(workspaceSettings());
      await mutation;
    });

    expect(updateGitHubWorkspaceSettings).toHaveBeenCalledWith({
      workspace_id: WORKSPACE_ID,
      saved_presets: [{ ...prA, isDefault: false }, { ...prB, isDefault: true }, issue],
    });
    expect(result.current.presets.map((preset) => preset.isDefault)).toEqual([false, true, true]);
  });

  it("retains the prior workspace default when persistence fails", async () => {
    const prA = { ...valid, id: "pr-a", isDefault: true };
    const prB = { ...valid, id: "pr-b" };
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings([prA, prB]));
    vi.mocked(updateGitHubWorkspaceSettings).mockRejectedValue(new Error(SETTINGS_DOWN));
    const { result } = renderHook(() => useSavedPresets(WORKSPACE_ID));
    await waitFor(() => expect(result.current.presets).toHaveLength(2));

    await expect(defaultSetter(result.current)("pr", "pr-b")).rejects.toThrow(SETTINGS_DOWN);

    expect(result.current.presets.map((preset) => preset.isDefault)).toEqual([true, false]);
  });

  it("persists and publishes a portable user default", async () => {
    const prA = { ...valid, id: "pr-a" };
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { github_saved_presets: [prA] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockResolvedValue(
      undefined as unknown as Awaited<ReturnType<typeof updateUserSettings>>,
    );
    const { result } = renderHook(() => useSavedPresets());
    await waitFor(() => expect(result.current.presets).toEqual([prA]));

    await act(async () => {
      await defaultSetter(result.current)("pr", "pr-a");
    });

    expect(updateUserSettings).toHaveBeenCalledWith({
      github_saved_presets: [{ ...prA, isDefault: true }],
    });
    expect(result.current.presets).toEqual([{ ...prA, isDefault: true }]);
  });

  it("retains the prior portable user default when persistence fails", async () => {
    const prA = { ...valid, id: "pr-a", isDefault: true };
    const prB = { ...valid, id: "pr-b" };
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { github_saved_presets: [prA, prB] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockRejectedValue(new Error(SETTINGS_DOWN));
    const { result } = renderHook(() => useSavedPresets());
    await waitFor(() => expect(result.current.presets).toHaveLength(2));

    await expect(defaultSetter(result.current)("pr", "pr-b")).rejects.toThrow(SETTINGS_DOWN);

    expect(result.current.presets.map((preset) => preset.isDefault)).toEqual([true, false]);
  });
});
