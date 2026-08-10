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
const WORKSPACE_MODE = "workspace";
const PORTABLE_USER_MODE = "portable user";
const MODES = [WORKSPACE_MODE, PORTABLE_USER_MODE] as const;

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
  if (mode === WORKSPACE_MODE) {
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(workspaceSettings(presets));
    return;
  }
  vi.mocked(fetchUserSettings).mockResolvedValue({
    settings: { github_saved_presets: presets },
  } as Awaited<ReturnType<typeof fetchUserSettings>>);
}

function deferFirstPersistence(mode: PersistenceMode): () => void {
  if (mode === WORKSPACE_MODE) {
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
  if (mode === WORKSPACE_MODE) {
    expect(updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(count);
  } else {
    expect(updateUserSettings).toHaveBeenCalledTimes(count);
  }
}

function expectLastPersisted(mode: PersistenceMode, presets: SavedPreset[]) {
  if (mode === WORKSPACE_MODE) {
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
  const hook = renderHook(() => useSavedPresets(mode === WORKSPACE_MODE ? WORKSPACE_ID : null));
  await waitFor(() => expect(hook.result.current.presets).toEqual(presets));
  return hook;
}

async function expectPendingDefaultTargetRemoval(mode: PersistenceMode) {
  const prA = { ...valid, isDefault: true };
  const prB = { ...valid, id: "pr-b", label: "PR B" };
  const resolveDefault = deferFirstPersistence(mode);
  const { result } = await renderLoaded(mode, [prA, prB]);

  let defaultMutation!: Promise<boolean>;
  act(() => {
    defaultMutation = result.current.setDefault("pr", "pr-b");
  });
  await waitFor(() => expectPersistenceCalls(mode, 1));

  let removeMutation!: Promise<boolean>;
  act(() => {
    removeMutation = result.current.remove("pr-b");
  });

  await act(async () => {
    resolveDefault();
    await Promise.all([defaultMutation, removeMutation]);
  });
  await waitFor(() => expectPersistenceCalls(mode, 2));

  const expected = [prA];
  expect(result.current.presets).toEqual(expected);
  expectLastPersisted(mode, expected);
}

async function expectIndependentWorkspacePersistence() {
  const firstWorkspaceId = "ws-first";
  const secondWorkspaceId = "ws-second";
  const firstWrite = deferred<Awaited<ReturnType<typeof updateGitHubWorkspaceSettings>>>();
  vi.mocked(fetchGitHubWorkspaceSettings).mockImplementation(async (workspaceId) => ({
    ...workspaceSettings([valid]),
    workspace_id: workspaceId,
  }));
  vi.mocked(updateGitHubWorkspaceSettings).mockImplementation((settings) => {
    const response = { ...workspaceSettings(), workspace_id: settings.workspace_id };
    return settings.workspace_id === firstWorkspaceId
      ? firstWrite.promise
      : Promise.resolve(response);
  });
  const firstHook = renderHook(() => useSavedPresets(firstWorkspaceId));
  const secondHook = renderHook(() => useSavedPresets(secondWorkspaceId));
  await waitFor(() => expect(firstHook.result.current.presets).toEqual([valid]));
  await waitFor(() => expect(secondHook.result.current.presets).toEqual([valid]));

  let firstSave!: Promise<SavedPreset | null>;
  act(() => {
    firstSave = firstHook.result.current.save({
      kind: "pr",
      label: "First workspace",
      customQuery: "is:open",
      repoFilter: "",
    });
  });
  await waitFor(() =>
    expect(updateGitHubWorkspaceSettings).toHaveBeenCalledWith(
      expect.objectContaining({ workspace_id: firstWorkspaceId }),
    ),
  );

  let secondSave!: Promise<SavedPreset | null>;
  act(() => {
    secondSave = secondHook.result.current.save({
      kind: "pr",
      label: "Second workspace",
      customQuery: "is:closed",
      repoFilter: "",
    });
  });

  try {
    await waitFor(
      () =>
        expect(updateGitHubWorkspaceSettings).toHaveBeenCalledWith(
          expect.objectContaining({ workspace_id: secondWorkspaceId }),
        ),
      { timeout: 250 },
    );
  } finally {
    await act(async () => {
      firstWrite.resolve({ ...workspaceSettings(), workspace_id: firstWorkspaceId });
      await Promise.all([firstSave, secondSave]);
    });
  }
}

async function expectQueuedWorkspaceSaveIsolation() {
  const firstWorkspaceId = "ws-first";
  const secondWorkspaceId = "ws-second";
  const firstWorkspacePreset = { ...valid, label: "Workspace A" };
  const secondWorkspacePreset = { ...valid, id: "pr-b", label: "Workspace B" };
  const firstWrite = deferred<Awaited<ReturnType<typeof updateGitHubWorkspaceSettings>>>();
  vi.mocked(fetchGitHubWorkspaceSettings).mockImplementation(async (workspaceId) => ({
    ...workspaceSettings(
      workspaceId === firstWorkspaceId ? [firstWorkspacePreset] : [secondWorkspacePreset],
    ),
    workspace_id: workspaceId,
  }));
  vi.mocked(updateGitHubWorkspaceSettings)
    .mockReturnValueOnce(firstWrite.promise)
    .mockResolvedValueOnce({ ...workspaceSettings(), workspace_id: firstWorkspaceId });
  const { result, rerender } = renderHook(({ workspaceId }) => useSavedPresets(workspaceId), {
    initialProps: { workspaceId: firstWorkspaceId },
  });
  await waitFor(() => expect(result.current.presets).toEqual([firstWorkspacePreset]));

  let firstSave!: Promise<SavedPreset | null>;
  act(() => {
    firstSave = result.current.save({
      kind: "pr",
      label: "First queued",
      customQuery: "is:open",
      repoFilter: "",
    });
  });
  await waitFor(() => expect(updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(1));

  let secondSave!: Promise<SavedPreset | null>;
  act(() => {
    secondSave = result.current.save({
      kind: "pr",
      label: "Second queued",
      customQuery: "is:closed",
      repoFilter: "",
    });
  });
  rerender({ workspaceId: secondWorkspaceId });
  await waitFor(() => expect(result.current.presets).toEqual([secondWorkspacePreset]));

  await act(async () => {
    firstWrite.resolve({ ...workspaceSettings(), workspace_id: firstWorkspaceId });
    await Promise.all([firstSave, secondSave]);
  });
  await waitFor(() => expect(updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(2));

  const secondPayload = vi.mocked(updateGitHubWorkspaceSettings).mock.calls[1]?.[0];
  expect(secondPayload?.workspace_id).toBe(firstWorkspaceId);
  expect((secondPayload?.saved_presets as SavedPreset[]).map((preset) => preset.label)).toEqual([
    "Workspace A",
    "First queued",
    "Second queued",
  ]);
  expect(result.current.presets).toEqual([secondWorkspacePreset]);
}

async function expectSaveThenRemove(mode: PersistenceMode) {
  const resolveSave = deferFirstPersistence(mode);
  const { result } = await renderLoaded(mode, [valid]);

  let saveMutation!: Promise<SavedPreset | null>;
  act(() => {
    saveMutation = result.current.save({
      kind: "pr",
      label: "Saved while deleting",
      customQuery: "is:open",
      repoFilter: "",
    });
  });
  await waitFor(() => expectPersistenceCalls(mode, 1));

  let removeMutation!: Promise<boolean>;
  act(() => {
    removeMutation = result.current.remove(valid.id);
  });

  let created: SavedPreset | null = null;
  let removed = false;
  await act(async () => {
    resolveSave();
    [created, removed] = await Promise.all([saveMutation, removeMutation]);
  });
  await waitFor(() => expectPersistenceCalls(mode, 2));

  const expected = [requirePreset(created)];
  expect(removed).toBe(true);
  expect(result.current.presets).toEqual(expected);
  expectLastPersisted(mode, expected);
}

async function expectPortableDefaultsAcrossHookInstances() {
  const pr = { ...valid, isDefault: false };
  const issue = { ...valid, id: "issue-a", kind: "issue" as const, isDefault: false };
  const resolveFirst = deferFirstPersistence(PORTABLE_USER_MODE);
  mockHydration(PORTABLE_USER_MODE, [pr, issue]);
  const first = renderHook(() => useSavedPresets());
  const second = renderHook(() => useSavedPresets());
  await waitFor(() => expect(first.result.current.presets).toEqual([pr, issue]));
  await waitFor(() => expect(second.result.current.presets).toEqual([pr, issue]));

  let firstMutation!: Promise<boolean>;
  act(() => {
    firstMutation = first.result.current.setDefault("pr", pr.id);
  });
  await waitFor(() => expectPersistenceCalls(PORTABLE_USER_MODE, 1));

  let secondMutation!: Promise<boolean>;
  act(() => {
    secondMutation = second.result.current.setDefault("issue", issue.id);
  });

  await act(async () => {
    resolveFirst();
    await Promise.all([firstMutation, secondMutation]);
  });
  await waitFor(() => expectPersistenceCalls(PORTABLE_USER_MODE, 2));

  const expected = [
    { ...pr, isDefault: true },
    { ...issue, isDefault: true },
  ];
  expectLastPersisted(PORTABLE_USER_MODE, expected);
  expect(first.result.current.presets).toEqual(expected);
  expect(second.result.current.presets).toEqual(expected);
}

function resetMutationTestState() {
  __resetSnapshotForTests();
  vi.mocked(fetchUserSettings).mockReset();
  vi.mocked(updateUserSettings).mockReset();
  vi.mocked(fetchGitHubWorkspaceSettings).mockReset();
  vi.mocked(updateGitHubWorkspaceSettings).mockReset();
}

describe("useSavedPresets mutation ordering", () => {
  beforeEach(resetMutationTestState);

  it.each(MODES)("preserves a %s save while a default update is pending", async (mode) => {
    const prA = { ...valid, isDefault: true };
    const prB = { ...valid, id: "pr-b", label: "PR B" };
    const resolveDefault = deferFirstPersistence(mode);
    const { result } = await renderLoaded(mode, [prA, prB]);

    let defaultMutation!: Promise<boolean>;
    act(() => {
      defaultMutation = result.current.setDefault("pr", "pr-b");
    });
    await waitFor(() => expectPersistenceCalls(mode, 1));

    let saveMutation!: Promise<SavedPreset | null>;
    act(() => {
      saveMutation = result.current.save({
        kind: "pr",
        label: "Saved while pending",
        customQuery: "is:open",
        repoFilter: "kdlbs/kandev",
      });
    });

    let created: SavedPreset | null = null;
    await act(async () => {
      resolveDefault();
      [, created] = await Promise.all([defaultMutation, saveMutation]);
    });
    await waitFor(() => expectPersistenceCalls(mode, 2));

    const saved = requirePreset(created);
    const expected = [{ ...prA, isDefault: false }, { ...prB, isDefault: true }, saved];
    expect(result.current.presets).toEqual(expected);
    expectLastPersisted(mode, expected);
  });

  it.each(MODES)("preserves a %s delete while a default update is pending", async (mode) => {
    const prA = { ...valid, isDefault: true };
    const prB = { ...valid, id: "pr-b", label: "PR B" };
    const resolveDefault = deferFirstPersistence(mode);
    const { result } = await renderLoaded(mode, [prA, prB]);

    let defaultMutation!: Promise<boolean>;
    act(() => {
      defaultMutation = result.current.setDefault("pr", "pr-b");
    });
    await waitFor(() => expectPersistenceCalls(mode, 1));

    let removeMutation!: Promise<boolean>;
    act(() => {
      removeMutation = result.current.remove("pr-a");
    });

    await act(async () => {
      resolveDefault();
      await Promise.all([defaultMutation, removeMutation]);
    });
    await waitFor(() => expectPersistenceCalls(mode, 2));

    const expected = [{ ...prB, isDefault: true }];
    expect(result.current.presets).toEqual(expected);
    expectLastPersisted(mode, expected);
  });

  it.each(MODES)("applies a %s delete while a save is pending", expectSaveThenRemove);

  it(
    "serializes portable defaults across hook instances",
    expectPortableDefaultsAcrossHookInstances,
  );

  it.each(MODES)("removes a pending %s default target", expectPendingDefaultTargetRemoval);

  it("persists different workspaces independently", expectIndependentWorkspacePersistence);

  it("keeps queued saves scoped across workspace navigation", expectQueuedWorkspaceSaveIsolation);

  it("detaches future workspace writes from a stale test queue", async () => {
    const firstWrite = deferred<Awaited<ReturnType<typeof updateGitHubWorkspaceSettings>>>();
    vi.mocked(updateGitHubWorkspaceSettings)
      .mockReturnValueOnce(firstWrite.promise)
      .mockResolvedValue(workspaceSettings());
    const firstHook = await renderLoaded(WORKSPACE_MODE, [valid]);

    let firstSave!: Promise<SavedPreset | null>;
    act(() => {
      firstSave = firstHook.result.current.save({
        kind: "pr",
        label: "First write",
        customQuery: "is:open",
        repoFilter: "",
      });
    });
    await waitFor(() => expect(updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(1));
    firstHook.unmount();

    __resetSnapshotForTests();
    const secondHook = await renderLoaded(WORKSPACE_MODE, [valid]);
    let secondSave!: Promise<SavedPreset | null>;
    act(() => {
      secondSave = secondHook.result.current.save({
        kind: "pr",
        label: "Second write",
        customQuery: "is:closed",
        repoFilter: "",
      });
    });

    await waitFor(() => expect(updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(2));
    await act(async () => {
      firstWrite.resolve(workspaceSettings());
      await Promise.all([firstSave, secondSave]);
    });
  });
});
