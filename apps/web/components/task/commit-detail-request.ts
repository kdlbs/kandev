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

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function isInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value);
}

function isOptionalString(value: unknown): boolean {
  return value === undefined || typeof value === "string";
}

function isCompleteGitHubFile(value: unknown): value is PRCommitDetail["files"][number] {
  if (!value || typeof value !== "object") return false;
  const file = value as Record<string, unknown>;
  return (
    isNonEmptyString(file.filename) &&
    isNonEmptyString(file.status) &&
    isInteger(file.additions) &&
    isInteger(file.deletions) &&
    isOptionalString(file.old_path) &&
    isOptionalString(file.patch)
  );
}

function isCompleteGitHubCommitDetail(value: unknown): value is PRCommitDetail {
  if (!value || typeof value !== "object") return false;
  const commit = value as Record<string, unknown>;
  return (
    isNonEmptyString(commit.sha) &&
    typeof commit.message === "string" &&
    typeof commit.author_login === "string" &&
    isNonEmptyString(commit.author_name) &&
    isNonEmptyString(commit.author_date) &&
    isInteger(commit.additions) &&
    isInteger(commit.deletions) &&
    isInteger(commit.files_changed) &&
    Array.isArray(commit.files) &&
    commit.files.every(isCompleteGitHubFile)
  );
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
  if (!isCompleteGitHubCommitDetail(response?.commit)) {
    throw new CommitDetailProtocolError("invalid_response");
  }
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
