import { act, renderHook, waitFor, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useWorkspaceSourcePlacement } from "./use-workspace-source-placement";
import type { WorkspaceSourceRow } from "@/components/workspace-source-picker/workspace-source-state";

const { previewTaskWorkspaceSources } = vi.hoisted(() => ({
  previewTaskWorkspaceSources: vi.fn(),
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({ previewTaskWorkspaceSources }));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

const preview = {
  revision: "revision-1",
  workspace_path: "/workspace",
  sources: [],
  supported_placements: [],
};
const folder: WorkspaceSourceRow = {
  key: "notes",
  kind: "folder",
  sourceType: "folder",
  localPath: "/sources/notes",
};
const repositoryInputs: WorkspaceSourceRow[] = [
  { key: "saved", kind: "repository", repositoryId: "repo-1", baseBranch: "main" },
  {
    key: "local",
    kind: "repository",
    sourceType: "local_repository",
    localPath: "/sources/tools",
    baseBranch: "main",
  },
  {
    key: "remote",
    kind: "repository",
    sourceType: "remote_repository",
    remoteUrl: "https://github.com/acme/tools",
    baseBranch: "main",
  },
];

describe("workspace source placement", () => {
  it.each(["local", "worktree"])(
    "previews every supported source kind and mixed batches on %s",
    async (executorType) => {
      previewTaskWorkspaceSources.mockResolvedValue(preview);
      for (const rows of [
        ...repositoryInputs.map((row) => [row]),
        [folder],
        [...repositoryInputs, folder],
      ]) {
        const { result, unmount } = renderHook(() =>
          useWorkspaceSourcePlacement({
            open: true,
            taskId: "task-1",
            executorType,
            rows,
            errors: {},
          }),
        );
        expect(result.current.eligible).toBe(true);
        await waitFor(() => expect(result.current.preview).toEqual(preview));
        act(() => result.current.setPlacement("current_root"));
        await waitFor(() =>
          expect(previewTaskWorkspaceSources).toHaveBeenLastCalledWith(
            "task-1",
            expect.objectContaining({ repository_placement: "current_root" }),
          ),
        );
        unmount();
      }
    },
  );
  it("discards stale previews and retains the selected placement on failure", async () => {
    let finishFirst!: (value: unknown) => void;
    previewTaskWorkspaceSources
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finishFirst = resolve;
          }),
      )
      .mockResolvedValueOnce(preview);
    const rows = [repositoryInputs[1]];
    const { result } = renderHook(() =>
      useWorkspaceSourcePlacement({
        open: true,
        taskId: "task-1",
        executorType: "local",
        rows,
        errors: {},
      }),
    );
    act(() => result.current.setPlacement("current_root"));
    await waitFor(() => expect(result.current.preview).toEqual(preview));
    await act(async () => finishFirst({ ...preview, revision: "stale" }));
    expect(result.current.preview?.revision).toBe("revision-1");
    previewTaskWorkspaceSources.mockRejectedValueOnce(new Error("Destination occupied"));
    act(() => result.current.setPlacement("kandev_directory"));
    await waitFor(() => expect(result.current.previewError).toBe("Destination occupied"));
    expect(result.current.previewLoading).toBe(false);
    expect(result.current.placement).toBe("kandev_directory");
    expect(result.current.preview).toBeNull();
  });
});
