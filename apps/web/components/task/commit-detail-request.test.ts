import { afterEach, describe, expect, it, vi } from "vitest";
import type { PRCommitDetail } from "@/lib/types/github";
import type { FileInfo } from "@/lib/state/store";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  requestCommitDiff: vi.fn(),
}));
let websocketClient: { request: typeof mocks.request } | null = { request: mocks.request };

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => websocketClient,
}));

vi.mock("./commit-diff-request", () => ({
  requestCommitDiff: mocks.requestCommitDiff,
}));

import { mapGitHubCommitFiles, requestCommitDetail } from "./commit-detail-request";

const WORKSPACE_ID = "workspace-1";
const remoteTarget = {
  source: "github" as const,
  sha: "abc1234567890",
  workspaceId: WORKSPACE_ID,
  owner: "acme",
  repo: "widget",
};

const detail: PRCommitDetail = {
  sha: "abc1234567890",
  message: "remote commit message",
  author_login: "octocat",
  author_name: "Octo Cat",
  author_date: "2026-08-04T12:00:00Z",
  additions: 3,
  deletions: 1,
  files_changed: 2,
  files: [
    {
      filename: "src/new.ts",
      status: "added",
      additions: 3,
      deletions: 0,
      patch: "@@ -0,0 +1 @@\n+new",
    },
    {
      filename: "assets/logo.bin",
      status: "modified",
      additions: 0,
      deletions: 1,
      patch: "",
    },
  ],
};

afterEach(() => {
  mocks.request.mockReset();
  mocks.requestCommitDiff.mockReset();
  websocketClient = { request: mocks.request };
});

describe("requestCommitDetail", () => {
  it("uses the workspace GitHub action for a remote target", async () => {
    mocks.request.mockResolvedValue({ commit: detail });

    const result = await requestCommitDetail({
      target: {
        source: "github",
        sha: detail.sha,
        workspaceId: WORKSPACE_ID,
        owner: "acme",
        repo: "widget",
      },
      local: {
        sessionId: "session-1",
        taskId: "task-1",
        agentctlReady: true,
      },
    });

    expect(result).toMatchObject({ source: "github", success: true, commit: detail });
    expect(mocks.request).toHaveBeenCalledWith(
      "github.pr_commit.get",
      {
        workspace_id: WORKSPACE_ID,
        owner: "acme",
        repo: "widget",
        sha: detail.sha,
      },
      10000,
    );
    expect(mocks.requestCommitDiff).not.toHaveBeenCalled();
  });

  it("uses the local session action only for a local target", async () => {
    const files: Record<string, FileInfo> = {
      "src/local.ts": {
        path: "src/local.ts",
        status: "modified",
        staged: false,
        diff: "@@ -1 +1 @@",
      },
    };
    mocks.requestCommitDiff.mockResolvedValue({ success: true, files });

    const result = await requestCommitDetail({
      target: { source: "local", sha: "local123", repo: "frontend" },
      local: {
        sessionId: "session-1",
        taskId: "task-1",
        agentctlReady: true,
      },
    });

    expect(result).toEqual({ source: "local", success: true, files });
    expect(mocks.requestCommitDiff).toHaveBeenCalledWith({
      sessionId: "session-1",
      taskId: "task-1",
      commitSha: "local123",
      agentctlReady: true,
      repo: "frontend",
    });
    expect(mocks.request).not.toHaveBeenCalled();
  });

  it("does not fall back to the local action when GitHub fails", async () => {
    mocks.request.mockRejectedValue(new Error("GitHub unavailable"));

    await expect(
      requestCommitDetail({
        target: remoteTarget,
        local: {
          sessionId: "session-1",
          taskId: "task-1",
          agentctlReady: true,
        },
      }),
    ).rejects.toThrow("GitHub unavailable");
    expect(mocks.requestCommitDiff).not.toHaveBeenCalled();
  });

  it("rejects when the WebSocket client is unavailable", async () => {
    websocketClient = null;

    await expect(
      requestCommitDetail({
        target: remoteTarget,
      }),
    ).rejects.toThrow();
  });

  it("rejects a GitHub response without a commit payload", async () => {
    mocks.request.mockResolvedValue({});

    await expect(
      requestCommitDetail({
        target: remoteTarget,
      }),
    ).rejects.toThrow();
  });

  it("rejects a present but incomplete GitHub commit payload", async () => {
    mocks.request.mockResolvedValue({ commit: { files: [] } });

    await expect(
      requestCommitDetail({
        target: remoteTarget,
      }),
    ).rejects.toThrow();
  });
});

describe("mapGitHubCommitFiles", () => {
  it("maps patch and patchless files without introducing a local base ref", () => {
    expect(mapGitHubCommitFiles(detail)).toEqual({
      "src/new.ts": {
        path: "src/new.ts",
        status: "added",
        staged: false,
        additions: 3,
        deletions: 0,
        diff: "@@ -0,0 +1 @@\n+new",
      },
      "assets/logo.bin": {
        path: "assets/logo.bin",
        status: "modified",
        staged: false,
        additions: 0,
        deletions: 1,
        diff: "",
      },
    });
  });
});
