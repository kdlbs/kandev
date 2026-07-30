import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import type { GitHubWorkspaceSettings, TaskGitCredentialsMode } from "@/lib/types/github";
import { useTaskGitCredentials } from "./use-task-git-credentials";

const WORKSPACE_A = "workspace-a";
const WORKSPACE_B = "workspace-b";

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubWorkspaceSettings: vi.fn(),
  updateGitHubWorkspaceSettings: vi.fn(),
}));

function workspaceSettings(
  workspaceId: string,
  mode?: TaskGitCredentialsMode,
): GitHubWorkspaceSettings {
  return {
    workspace_id: workspaceId,
    task_git_credentials_mode: mode,
    repo_scope_mode: "all",
    repo_scope_orgs: [],
    repo_scope_repos: [],
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe("useTaskGitCredentials", () => {
  it("loads the workspace task access mode", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(
      workspaceSettings(WORKSPACE_A, "executor"),
    );

    const { result } = renderHook(() => useTaskGitCredentials(WORKSPACE_A));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current).toMatchObject({ mode: "executor", error: false });
  });

  it("reports a task access load failure", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockRejectedValue(new Error("unavailable"));

    const { result } = renderHook(() => useTaskGitCredentials(WORKSPACE_A));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current).toMatchObject({ mode: "managed", error: true });
  });

  it("loads the replacement workspace after a workspace change", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings)
      .mockResolvedValueOnce(workspaceSettings(WORKSPACE_A, "managed"))
      .mockResolvedValueOnce(workspaceSettings(WORKSPACE_B, "executor"));

    const { result, rerender } = renderHook(
      ({ workspaceId }) => useTaskGitCredentials(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_A } },
    );
    await waitFor(() => expect(result.current.loading).toBe(false));

    rerender({ workspaceId: WORKSPACE_B });

    await waitFor(() => expect(result.current.mode).toBe("executor"));
    expect(fetchGitHubWorkspaceSettings).toHaveBeenLastCalledWith(WORKSPACE_B);
  });

  it("updates the loaded mode after a successful save", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(
      workspaceSettings(WORKSPACE_A, "managed"),
    );
    vi.mocked(updateGitHubWorkspaceSettings).mockResolvedValue(
      workspaceSettings(WORKSPACE_A, "executor"),
    );
    const { result } = renderHook(() => useTaskGitCredentials(WORKSPACE_A));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(() => result.current.save("executor"));

    expect(updateGitHubWorkspaceSettings).toHaveBeenCalledWith({
      workspace_id: WORKSPACE_A,
      task_git_credentials_mode: "executor",
    });
    expect(result.current).toMatchObject({ mode: "executor", error: false });
  });

  it("preserves the loaded state when a save fails", async () => {
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(
      workspaceSettings(WORKSPACE_A, "managed"),
    );
    vi.mocked(updateGitHubWorkspaceSettings).mockRejectedValue(new Error("save failed"));
    const { result } = renderHook(() => useTaskGitCredentials(WORKSPACE_A));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await expect(result.current.save("executor")).rejects.toThrow("save failed");

    expect(result.current).toMatchObject({ mode: "managed", error: false });
  });

  it("does not apply a completed save from the previous workspace", async () => {
    const pendingSave = deferred<GitHubWorkspaceSettings>();
    vi.mocked(fetchGitHubWorkspaceSettings).mockImplementation(async (workspaceId) =>
      workspaceSettings(workspaceId, workspaceId === WORKSPACE_A ? "managed" : "executor"),
    );
    vi.mocked(updateGitHubWorkspaceSettings).mockReturnValue(pendingSave.promise);
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useTaskGitCredentials(workspaceId),
      { initialProps: { workspaceId: WORKSPACE_A } },
    );
    await waitFor(() => expect(result.current.mode).toBe("managed"));

    const save = result.current.save("managed");
    rerender({ workspaceId: WORKSPACE_B });
    await waitFor(() => expect(result.current.mode).toBe("executor"));

    pendingSave.resolve(workspaceSettings(WORKSPACE_A, "managed"));
    await act(() => save);

    expect(result.current.mode).toBe("executor");
  });
});
