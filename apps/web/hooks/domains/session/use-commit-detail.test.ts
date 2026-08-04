import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { FileInfo } from "@/lib/state/store";

const mocks = vi.hoisted(() => ({
  requestCommitDetail: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      tasks: { activeSessionId: "session-1", activeTaskId: "task-1" },
      taskSessions: { items: { "session-1": { task_id: "task-1" } } },
      sessionAgentctl: { itemsBySessionId: { "session-1": { status: "ready" } } },
    }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/components/task/commit-detail-request", () => ({
  requestCommitDetail: mocks.requestCommitDetail,
}));

import { useCommitDetail } from "./use-commit-detail";

const remoteFiles: Record<string, FileInfo> = {
  "src/remote.ts": {
    path: "src/remote.ts",
    status: "modified",
    staged: false,
    diff: "remote patch",
  },
};

afterEach(() => {
  cleanup();
  mocks.requestCommitDetail.mockReset();
  mocks.toast.mockReset();
});

describe("useCommitDetail", () => {
  it("requests a GitHub target without adding local session routing", async () => {
    mocks.requestCommitDetail.mockResolvedValue({
      source: "github",
      success: true,
      files: remoteFiles,
      commit: {
        sha: "remote123",
        message: "remote commit",
        author_login: "octocat",
        author_name: "Octo Cat",
        author_date: "2026-08-04T12:00:00Z",
        additions: 1,
        deletions: 0,
        files_changed: 1,
        files: [],
      },
    });

    const target = {
      source: "github" as const,
      sha: "remote123",
      workspaceId: "workspace-1",
      owner: "acme",
      repo: "widget",
    };
    const { result } = renderHook(() => useCommitDetail(target));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mocks.requestCommitDetail).toHaveBeenCalledWith({ target });
    expect(result.current.files).toEqual(remoteFiles);
    expect(result.current.commit?.message).toBe("remote commit");
  });

  it("passes local session readiness only for a local target", async () => {
    mocks.requestCommitDetail.mockResolvedValue({
      source: "local",
      success: true,
      files: remoteFiles,
    });

    const target = { source: "local" as const, sha: "local123", repo: "frontend" };
    const { result } = renderHook(() => useCommitDetail(target));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mocks.requestCommitDetail).toHaveBeenCalledWith({
      target,
      local: {
        sessionId: "session-1",
        taskId: "task-1",
        agentctlReady: true,
      },
    });
  });

  it("keeps a GitHub error on the remote path and exposes it to the panel", async () => {
    mocks.requestCommitDetail.mockRejectedValue(new Error("GitHub unavailable"));

    const target = {
      source: "github" as const,
      sha: "remote123",
      workspaceId: "workspace-1",
      owner: "acme",
      repo: "widget",
    };
    const { result } = renderHook(() => useCommitDetail(target));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.files).toBeNull();
    expect(result.current.error).toBe("GitHub unavailable");
    expect(mocks.toast).toHaveBeenCalled();
    expect(mocks.requestCommitDetail).toHaveBeenCalledTimes(1);
  });

  it("refetches the same target on demand", async () => {
    mocks.requestCommitDetail.mockResolvedValue({ source: "github", success: false });
    const target = {
      source: "github" as const,
      sha: "remote123",
      workspaceId: "workspace-1",
      owner: "acme",
      repo: "widget",
    };
    const { result } = renderHook(() => useCommitDetail(target));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.refetch();
    });
    expect(mocks.requestCommitDetail).toHaveBeenCalledTimes(2);
  });
});
