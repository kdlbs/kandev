export type RemoteExecutorStatus = {
  is_remote_executor?: boolean;
  executor_type?: string | null;
  executor_name?: string | null;
  remote_name?: string | null;
  remote_state?: string | null;
  remote_created_at?: string | null;
  remote_checked_at?: string | null;
  remote_status_error?: string | null;
  remote_repository_id?: string | null;
  remote_branch?: string | null;
  remote_pull_request_url?: string | null;
  remote_agent_url?: string | null;
  remote_history_gap?: boolean;
  capabilities?: {
    embedded_vscode?: boolean;
  };
};

function toNullable(value: string | null | undefined): string | null {
  return value ?? null;
}

export function resolveRemoteExecutor(status?: RemoteExecutorStatus | null) {
  const remoteExecutorName = status?.remote_name ?? status?.executor_name ?? null;
  return {
    isRemoteExecutor: status?.is_remote_executor ?? false,
    remoteExecutorType: toNullable(status?.executor_type),
    remoteExecutorName,
    remoteState: toNullable(status?.remote_state),
    remoteCreatedAt: toNullable(status?.remote_created_at),
    remoteCheckedAt: toNullable(status?.remote_checked_at),
    remoteStatusError: toNullable(status?.remote_status_error),
    remoteHistoryGap: status?.remote_history_gap ?? false,
    ...resolveRemoteExecutorResults(status),
  };
}

export type ResolvedRemoteExecutor = ReturnType<typeof resolveRemoteExecutor>;

function resolveRemoteExecutorResults(status?: RemoteExecutorStatus | null) {
  return {
    remoteRepositoryID: toNullable(status?.remote_repository_id),
    remoteBranch: toNullable(status?.remote_branch),
    remotePullRequestURL: toNullable(status?.remote_pull_request_url),
    remoteAgentURL: toNullable(status?.remote_agent_url),
  };
}

export function isCursorCloudTask(
  primaryExecutorType?: string | null,
  sessionExecutorType?: string | null,
): boolean {
  return primaryExecutorType === "cursor_cloud" || sessionExecutorType === "cursor_cloud";
}
