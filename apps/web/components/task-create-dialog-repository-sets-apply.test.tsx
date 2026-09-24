import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Repository, RepositorySet } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";
import type { TaskRepoRow, TaskRepositorySelection } from "@/components/task-create-dialog-types";

const toast = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { useApplyRepositorySet } from "@/components/task-create-dialog-repository-sets-apply";

const REPOSITORY_ID = "repository-1";

const repositories = [
  {
    id: repositoryId(REPOSITORY_ID),
    name: "Repository 1",
  } as unknown as Repository,
];

const repositorySet: RepositorySet = {
  id: "set-1",
  workspace_id: workspaceId("workspace-1"),
  name: "Set 1",
  description: "",
  repositories: [{ repository_id: repositoryId(REPOSITORY_ID), position: 0 }],
  created_at: "2026-08-17T09:00:00Z",
  updated_at: "2026-08-17T09:00:00Z",
};

const placeholder: TaskRepoRow = { key: "row-0", branch: "" };

describe("useApplyRepositorySet", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("leaves no-repository mode when a set adds a row", () => {
    const setRepositories = vi.fn();
    const setRepositoriesDirty = vi.fn();
    const setNoRepository = vi.fn();
    const { result } = renderHook(() =>
      useApplyRepositorySet({
        rows: [placeholder],
        repositories,
        setRepositories,
        setRepositoriesDirty,
        setNoRepository,
      }),
    );

    act(() => result.current(repositorySet));

    expect(setNoRepository).toHaveBeenCalledWith(false);
    expect(setRepositories).toHaveBeenCalledTimes(1);
    expect(setRepositoriesDirty).toHaveBeenCalledWith(true);
  });

  it("keeps no-repository mode when the set has no applicable members", () => {
    const setNoRepository = vi.fn();
    const { result } = renderHook(() =>
      useApplyRepositorySet({
        rows: [placeholder],
        repositories: [],
        setRepositories: vi.fn(),
        setRepositoriesDirty: vi.fn(),
        setNoRepository,
      }),
    );

    act(() => result.current(repositorySet));

    expect(setNoRepository).not.toHaveBeenCalled();
  });

  it("appends set members without dropping an existing folder selection", () => {
    const folder: TaskRepositorySelection = {
      kind: "folder",
      key: "folder-1",
      localPath: "/work/assets",
    };
    const setRepositorySelections = vi.fn();
    const { result } = renderHook(() =>
      useApplyRepositorySet({
        rows: [],
        repositories,
        setRepositories: vi.fn(),
        setRepositoriesDirty: vi.fn(),
        setNoRepository: vi.fn(),
        selections: [folder],
        setRepositorySelections,
      }),
    );

    act(() => result.current(repositorySet));

    expect(setRepositorySelections).toHaveBeenCalledWith([
      folder,
      expect.objectContaining({ kind: "local", repositoryId: REPOSITORY_ID }),
    ]);
  });
});
