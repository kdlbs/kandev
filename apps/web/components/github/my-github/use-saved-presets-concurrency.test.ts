import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import { __resetSnapshotForTests, useSavedPresets, type SavedPreset } from "./use-saved-presets";

const WORKSPACE_ID = "ws-1";
const SETTINGS_TIMESTAMP = "2026-01-01T00:00:00Z";
const MODES = ["workspace", "portable user"] as const;

type PersistenceMode = (typeof MODES)[number];

const valid: SavedPreset = {
  id: "pr-a",
  kind: "pr",
  label: "PR A",
  customQuery: "author:@me",
  repoFilter: "",
  createdAt: SETTINGS_TIMESTAMP,
  isDefault: false,
};

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubWorkspaceSettings: vi.fn(),
  updateGitHubWorkspaceSettings: vi.fn(),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
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

function requirePreset(value: SavedPreset | null): SavedPreset {
  expect(value).not.toBeNull();
  if (value === null) throw new Error("Expected a saved preset");
  return value;
}

function mockHydration(mode: PersistenceMode, presets: SavedPreset[]) {
  if (mode === "workspace") {
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings(presets));
    return;
  }
  vi.mocked(fetchUserSettings).mockResolvedValue({
    settings: { github_saved_presets: presets },
  } as Awaited<ReturnType<typeof fetchUserSettings>>);
}

function deferFirstPersistence(mode: PersistenceMode): () => void {
  if (mode === "workspace") {
    const first = deferred<Awaited<ReturnType<typeof updateGitHubWorkspaceSettings>>>();
    vi.mocked(updateGitHubWorkspaceSettings)
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce(workspaceSettings());
    return () => first.resolve(workspaceSettings());
  }
  const first = deferred<Awaited<ReturnType<typeof updateUserSettings>>>();
  const success = undefined as unknown as Awaited<ReturnType<typeof updateUserSettings>>;
  vi.mocked(updateUserSettings).mockReturnValueOnce(first.promise).mockResolvedValueOnce(success);
  return () => first.resolve(success);
}

function expectPersistenceCalls(mode: PersistenceMode, count: number) {
  if (mode === "workspace") {
    expect(updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(count);
  } else {
    expect(updateUserSettings).toHaveBeenCalledTimes(count);
  }
}

function expectLastPersisted(mode: PersistenceMode, presets: SavedPreset[]) {
  if (mode === "workspace") {
    expect(updateGitHubWorkspaceSettings).toHaveBeenLastCalledWith({
      workspace_id: WORKSPACE_ID,
      saved_presets: presets,
    });
  } else {
    expect(updateUserSettings).toHaveBeenLastCalledWith({ github_saved_presets: presets });
  }
}

async function renderLoaded(mode: PersistenceMode, presets: SavedPreset[]) {
  mockHydration(mode, presets);
  const hook = renderHook(() => useSavedPresets(mode === "workspace" ? WORKSPACE_ID : null));
  await waitFor(() => expect(hook.result.current.presets).toEqual(presets));
  return hook;
}

describe("useSavedPresets mutation ordering", () => {
  beforeEach(() => {
    __resetSnapshotForTests();
    vi.mocked(fetchUserSettings).mockReset();
    vi.mocked(updateUserSettings).mockReset();
    vi.mocked(fetchGitHubWorkspaceSettings).mockReset();
    vi.mocked(updateGitHubWorkspaceSettings).mockReset();
  });

  it.each(MODES)("preserves a %s save while a default update is pending", async (mode) => {
    const prA = { ...valid, isDefault: true };
    const prB = { ...valid, id: "pr-b", label: "PR B" };
    const resolveDefault = deferFirstPersistence(mode);
    const { result } = await renderLoaded(mode, [prA, prB]);

    let defaultMutation!: Promise<void>;
    act(() => {
      defaultMutation = result.current.setDefault("pr", "pr-b");
    });
    await waitFor(() => expectPersistenceCalls(mode, 1));

    let created: SavedPreset | null = null;
    act(() => {
      created = result.current.save({
        kind: "pr",
        label: "Saved while pending",
        customQuery: "is:open",
        repoFilter: "kdlbs/kandev",
      });
    });
    const saved = requirePreset(created);

    await act(async () => {
      resolveDefault();
      await defaultMutation;
    });
    await waitFor(() => expectPersistenceCalls(mode, 2));

    const expected = [{ ...prA, isDefault: false }, { ...prB, isDefault: true }, saved];
    expect(result.current.presets).toEqual(expected);
    expectLastPersisted(mode, expected);
  });

  it.each(MODES)("preserves a %s delete while a default update is pending", async (mode) => {
    const prA = { ...valid, isDefault: true };
    const prB = { ...valid, id: "pr-b", label: "PR B" };
    const resolveDefault = deferFirstPersistence(mode);
    const { result } = await renderLoaded(mode, [prA, prB]);

    let defaultMutation!: Promise<void>;
    act(() => {
      defaultMutation = result.current.setDefault("pr", "pr-b");
    });
    await waitFor(() => expectPersistenceCalls(mode, 1));

    act(() => {
      result.current.remove("pr-a");
    });

    await act(async () => {
      resolveDefault();
      await defaultMutation;
    });
    await waitFor(() => expectPersistenceCalls(mode, 2));

    const expected = [{ ...prB, isDefault: true }];
    expect(result.current.presets).toEqual(expected);
    expectLastPersisted(mode, expected);
  });
});
