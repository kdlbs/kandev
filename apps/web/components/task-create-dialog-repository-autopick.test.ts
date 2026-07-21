import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useRepositoryAutoSelectEffect } from "./task-create-dialog-repository-autopick";
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

  it("ignores a stale cached repository after user settings have loaded", async () => {
    window.localStorage.setItem(STORAGE_KEYS.LAST_REPOSITORY_ID, JSON.stringify("repo-1"));
    const setRepositories = vi.fn();
    const fs = makeRepoAutoSelectFs([], setRepositories);

    renderHook(() =>
      useRepositoryAutoSelectEffect(
        fs,
        true,
        "ws-1",
        [makeRepository("repo-1"), makeRepository("repo-2")],
        { userSettingsLoaded: true },
      ),
    );

    await waitFor(() => expect(setRepositories).toHaveBeenCalled());
    const updater = setRepositories.mock.calls[0]![0] as (prev: TaskRepoRow[]) => TaskRepoRow[];
    expect(updater([])).toEqual([{ key: "row-0", branch: "" }]);
    expect(readQueuedTaskCreateLastUsedState()).toEqual({});
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
});
