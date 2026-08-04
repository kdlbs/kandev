"use client";

import type { FileInfo } from "@/lib/state/store";
import type { PRCommitDetail } from "@/lib/types/github";
import { normalizeFileChangeStatus } from "@/lib/utils/file-change-status";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { CommitDetailTarget } from "./changes-diff-target";
import { requestCommitDiff, type CommitDiffResponse } from "./commit-diff-request";

export type CommitDetailRequestResult =
  | ({ source: "local" } & CommitDiffResponse)
  | {
      source: "github";
      success: boolean;
      commit?: PRCommitDetail;
      files?: Record<string, FileInfo>;
    };

export class CommitDetailProtocolError extends Error {
  constructor(reason: "client_unavailable" | "invalid_response") {
    super(reason);
    this.name = "CommitDetailProtocolError";
  }
}

function mapGitHubFile(file: PRCommitDetail["files"][number]): FileInfo {
  return {
    path: file.filename,
    status: normalizeFileChangeStatus(file.status),
    staged: false,
    additions: file.additions,
    deletions: file.deletions,
    old_path: file.old_path,
    diff: file.patch ?? "",
  };
}

export function mapGitHubCommitFiles(detail: PRCommitDetail): Record<string, FileInfo> {
  return Object.fromEntries(detail.files.map((file) => [file.filename, mapGitHubFile(file)]));
}

async function requestGitHubCommitDetail(
  target: Extract<CommitDetailTarget, { source: "github" }>,
): Promise<CommitDetailRequestResult> {
  const ws = getWebSocketClient();
  if (!ws) throw new CommitDetailProtocolError("client_unavailable");

  const response = await ws.request<{ commit?: PRCommitDetail }>(
    "github.pr_commit.get",
    {
      workspace_id: target.workspaceId,
      owner: target.owner,
      repo: target.repo,
      sha: target.sha,
    },
    10000,
  );
  if (!response?.commit) throw new CommitDetailProtocolError("invalid_response");
  return {
    source: "github",
    success: true,
    commit: response.commit,
    files: mapGitHubCommitFiles(response.commit),
  };
}

export async function requestCommitDetail(params: {
  target: CommitDetailTarget;
  local?: {
    sessionId: string;
    taskId: string | null;
    agentctlReady: boolean;
  };
}): Promise<CommitDetailRequestResult> {
  if (params.target.source === "github") {
    return requestGitHubCommitDetail(params.target);
  }

  if (!params.local) throw new CommitDetailProtocolError("client_unavailable");
  const response = await requestCommitDiff({
    ...params.local,
    commitSha: params.target.sha,
    repo: params.target.repo,
  });
  if (!response) throw new CommitDetailProtocolError("client_unavailable");
  return { source: "local", ...response };
}
