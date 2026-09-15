import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useRepositoryAutoSelectEffect } from "./task-create-dialog-repository-autopick";
import { useRepositorySelectionState } from "./task-create-dialog-repositories-state";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import type { Repository } from "@/lib/types/http";
const STORAGE_KEYS = { LAST_REPOSITORY_ID: "kandev.dialog.lastRepositoryId" } as const;
import {
  readQueuedTaskCreateLastUsedState,
  resetTaskCreateLastUsedSync,
} from "./task-create-dialog-handlers";

beforeEach(() => {
  localStorage.clear();
  resetTaskCreateLastUsedSync({ clearQueued: true });
});

function makeRepository(id: string): Repository {
  return {
    id,
    workspace_id: "ws-1",
    name: "repo",
    source_type: "local",
    local_path: "/repo",
    default_branch: "main",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  } as Repository;
}

function makeRepoAutoSelectFs(
  rows: TaskRepoRow[],
  setRepositories: DialogFormState["setRepositories"],
): DialogFormState {
  return {
    repositories: rows,
    useRemote: false,
    setRepositories,
  } as unknown as DialogFormState;
}

describe("useRepositoryAutoSelectEffect loading gates", () => {
  it("waits for store-backed settings before falling back to an empty row", async () => {
    const setRepositories = vi.fn();
    const fs = makeRepoAutoSelectFs([], setRepositories);

    const { rerender } = renderHook(
      ({ loaded, lastUsedRepositoryId }) =>
        useRepositoryAutoSelectEffect(
          fs,
          true,
          "ws-1",
          [makeRepository("repo-1"), makeRepository("repo-2")],
          {
            lastUsedRepositoryId,
            userSettingsLoaded: loaded,
          },
        ),
      { initialProps: { loaded: false, lastUsedRepositoryId: null as string | null } },
    );

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(setRepositories).not.toHaveBeenCalled();

    rerender({ loaded: true, lastUsedRepositoryId: "repo-2" });

    await waitFor(() => expect(setRepositories).toHaveBeenCalled());
    const updater = setRepositories.mock.calls[0]![0] as (prev: TaskRepoRow[]) => TaskRepoRow[];
    expect(updater([])).toEqual([{ key: "row-0", repositoryId: "repo-2", branch: "" }]);
  });

  it("ignores a stale cached repository while waiting for backend settings", async () => {
    window.localStorage.setItem(STORAGE_KEYS.LAST_REPOSITORY_ID, JSON.stringify("repo-1"));
    const setRepositories = vi.fn();
    const fs = makeRepoAutoSelectFs([], setRepositories);

    renderHook(() =>
      useRepositoryAutoSelectEffect(
        fs,
        true,
        "ws-1",
        [makeRepository("repo-1"), makeRepository("repo-2")],
        {
          lastUsedRepositoryId: null,
          userSettingsLoaded: false,
        },
      ),
    );

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(setRepositories).not.toHaveBeenCalled();
    expect(readQueuedTaskCreateLastUsedState()).toEqual({});
  });

  it("leaves the selection empty when loaded settings have no repository candidate", async () => {
    window.localStorage.setItem(STORAGE_KEYS.LAST_REPOSITORY_ID, JSON.stringify("repo-1"));
    const setRepositories = vi.fn();
    const setNoRepository = vi.fn();
    const fs = {
      ...makeRepoAutoSelectFs([], setRepositories),
      noRepository: false,
      repositorySelections: [],
      setNoRepository,
    } as unknown as DialogFormState;

    renderHook(() =>
      useRepositoryAutoSelectEffect(fs, true, "ws-1", [], { userSettingsLoaded: true }),
    );

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(setRepositories).not.toHaveBeenCalled();
    expect(setNoRepository).toHaveBeenCalledWith(true);
    expect(readQueuedTaskCreateLastUsedState()).toEqual({});
  });
});

describe("useRepositoryAutoSelectEffect canonical empty drafts", () => {
  it("waits for the repository catalog before selecting scratch", async () => {
    const setRepositories = vi.fn();
    const setNoRepository = vi.fn();
    const fs = {
      ...makeRepoAutoSelectFs([], setRepositories),
      noRepository: false,
      repositorySelections: [],
      setNoRepository,
    } as unknown as DialogFormState;
    const settings = (repositoriesLoaded: boolean) =>
      ({ userSettingsLoaded: true, repositoriesLoaded }) as Parameters<
        typeof useRepositoryAutoSelectEffect
      >[4];

    const { rerender } = renderHook(
      ({ repositoriesLoaded }) =>
        useRepositoryAutoSelectEffect(fs, true, "ws-1", [], settings(repositoriesLoaded)),
      { initialProps: { repositoriesLoaded: false } },
    );

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(setNoRepository).not.toHaveBeenCalled();

    rerender({ repositoriesLoaded: true });
    await waitFor(() => expect(setNoRepository).toHaveBeenCalledWith(true));
  });

  it("preserves an initial folder selection when no repositories are registered", async () => {
    const setRepositories = vi.fn();
    const setNoRepository = vi.fn();
    const fs = {
      ...makeRepoAutoSelectFs([], setRepositories),
      noRepository: false,
      repositorySelections: [{ kind: "folder", key: "folder-0", localPath: "/tmp/assets" }],
      setNoRepository,
    } as unknown as DialogFormState;

    renderHook(() =>
      useRepositoryAutoSelectEffect(fs, true, "ws-1", [], {
        userSettingsLoaded: true,
        repositoriesLoaded: true,
      }),
    );

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(setNoRepository).not.toHaveBeenCalled();
  });
});

describe("useRepositoryAutoSelectEffect defaults", () => {
  it("fills an untouched placeholder row from backend settings", async () => {
    const setRepositories = vi.fn();
    const fs = makeRepoAutoSelectFs([{ key: "row-0", branch: "" }], setRepositories);

    renderHook(() =>
      useRepositoryAutoSelectEffect(
        fs,
        true,
        "ws-1",
        [makeRepository("repo-1"), makeRepository("repo-2")],
        { lastUsedRepositoryId: "repo-2" },
      ),
    );

    await waitFor(() => expect(setRepositories).toHaveBeenCalled());
    const updater = setRepositories.mock.calls[0]![0] as (prev: TaskRepoRow[]) => TaskRepoRow[];

    expect(updater([{ key: "row-0", branch: "" }])).toEqual([
      { key: "row-0", repositoryId: "repo-2", branch: "" },
    ]);
  });

  it("fills an empty repo row list from backend settings", async () => {
    const setRepositories = vi.fn();
    const fs = makeRepoAutoSelectFs([], setRepositories);

    renderHook(() =>
      useRepositoryAutoSelectEffect(
        fs,
        true,
        "ws-1",
        [makeRepository("repo-1"), makeRepository("repo-2")],
        { lastUsedRepositoryId: "repo-1" },
      ),
    );

    await waitFor(() => expect(setRepositories).toHaveBeenCalled());
    const updater = setRepositories.mock.calls[0]![0] as (prev: TaskRepoRow[]) => TaskRepoRow[];

    expect(updater([])).toEqual([{ key: "row-0", repositoryId: "repo-1", branch: "" }]);
  });

  it("leaves an intentional no-repository draft empty", async () => {
    const setRepositories = vi.fn();
    const fs = {
      ...makeRepoAutoSelectFs([], setRepositories),
      noRepository: true,
    } as unknown as DialogFormState;

    renderHook(() => useRepositoryAutoSelectEffect(fs, true, "ws-1", [makeRepository("repo-1")]));

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(setRepositories).not.toHaveBeenCalled();
  });
});

describe("useRepositoryAutoSelectEffect reducer integration", () => {
  it("keeps the selection empty until a repository loads without marking the draft touched", async () => {
    const { result, rerender } = renderHook(
      ({ repositories }: { repositories: Repository[] }) => {
        const selectionState = useRepositorySelectionState();
        useRepositoryAutoSelectEffect(
          {
            ...selectionState,
            noRepository: false,
            useRemote: false,
          } as unknown as DialogFormState,
          true,
          "ws-1",
          repositories,
        );
        return selectionState;
      },
      { initialProps: { repositories: [] as Repository[] } },
    );

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(result.current.repositories).toEqual([]);
    expect(result.current.repositorySelectionsTouched).toBe(false);

    rerender({ repositories: [makeRepository("repo-1")] });

    await waitFor(() =>
      expect(result.current.repositories).toEqual([
        { key: "row-0", repositoryId: "repo-1", branch: "" },
      ]),
    );
    expect(result.current.repositorySelectionsTouched).toBe(false);
    expect(result.current.repositoriesDirty).toBe(false);
  });

  it("keeps an explicitly removed row removed when the auto-picker runs again", async () => {
    const repository = makeRepository("repo-1");
    const { result, rerender } = renderHook(
      ({ repositories }: { repositories: Repository[] }) => {
        const selectionState = useRepositorySelectionState();
        useRepositoryAutoSelectEffect(
          {
            ...selectionState,
            noRepository: false,
            useRemote: false,
          } as unknown as DialogFormState,
          true,
          "ws-1",
          repositories,
        );
        return selectionState;
      },
      { initialProps: { repositories: [repository] } },
    );

    await waitFor(() => expect(result.current.repositories).toHaveLength(1));
    act(() => result.current.removeRepository(result.current.repositories[0].key));
    expect(result.current.repositories).toEqual([]);
    expect(result.current.repositorySelectionsTouched).toBe(true);

    rerender({ repositories: [repository] });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(result.current.repositories).toEqual([]);
  });

  it("does not hydrate a local row after a remote preset is restored", async () => {
    const repository = makeRepository("repo-1");
    const { result } = renderHook(() => {
      const selectionState = useRepositorySelectionState();
      useRepositoryAutoSelectEffect(
        {
          ...selectionState,
          noRepository: false,
          useRemote: false,
        } as unknown as DialogFormState,
        true,
        "ws-1",
        [repository],
      );
      return selectionState;
    });

    act(() => {
      result.current.hydrateRepositorySelections([
        {
          kind: "remote",
          key: "remote-0",
          url: "https://github.com/acme/remote",
          branch: "main",
          source: "paste",
        },
      ]);
    });

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(result.current.repositorySelections).toEqual([
      expect.objectContaining({ kind: "remote", key: "remote-0" }),
    ]);
    expect(result.current.repositories).toEqual([]);
    expect(result.current.repositorySelectionsTouched).toBe(false);
  });
});
