"use client";

import { useMemo } from "react";
import { useSessionGitStatusRefresh } from "@/hooks/domains/session/use-session-git-status";
import type {
  GitStatusEntry,
  GitStatusRefreshState,
} from "@/lib/state/slices/session-runtime/types";
import { hasRecoverableGitStatusDetailFailure } from "@/lib/state/slices/session-runtime/git-status-detail-failures";

type GitStatusScope = Pick<
  GitStatusEntry,
  "status_state" | "files_complete" | "detail_state" | "files" | "repository_name"
>;

type RepositoryStatus = { repository_name: string; status: GitStatusScope };

export type ChangesPanelGitStatusInput = {
  gitStatus?: GitStatusScope;
  statusByRepo?: RepositoryStatus[];
  environmentRefresh?: GitStatusRefreshState;
  repositoryRefresh?: Record<string, GitStatusRefreshState>;
};

export type ChangesPanelGitStatus = {
  hasPriorData: boolean;
  membershipReady: boolean;
  refreshPending: boolean;
  loading: boolean;
  unavailable: boolean;
  detailsPending: boolean;
  failedRepositories: string[];
  showEmpty: boolean;
};

function isCompleteMembership(status: GitStatusScope): boolean {
  return (
    status.status_state !== "loading" &&
    status.status_state !== "unavailable" &&
    status.files_complete !== false &&
    status.files !== undefined
  );
}

function hasPendingDetails(status: GitStatusScope): boolean {
  return (
    status.detail_state === "pending" ||
    Object.values(status.files ?? {}).some((file) => file.diff_state === "pending")
  );
}

function hasUnavailableDetails(statuses: GitStatusScope[]): boolean {
  return statuses.some(hasRecoverableGitStatusDetailFailure);
}

function hasUnavailableMembership(statuses: GitStatusScope[]): boolean {
  return statuses.some((status) => status.status_state === "unavailable");
}

function hasAnyUnavailableState(...states: boolean[]): boolean {
  return states.some(Boolean);
}

function repositoryStatusesForFailure(input: ChangesPanelGitStatusInput): RepositoryStatus[] {
  if (input.statusByRepo?.length) return input.statusByRepo;
  if (!input.gitStatus?.repository_name) return [];
  return [{ repository_name: input.gitStatus.repository_name, status: input.gitStatus }];
}

function failedRepositoryNames(input: ChangesPanelGitStatusInput): string[] {
  const names = new Set<string>();
  for (const [repositoryName, refresh] of Object.entries(input.repositoryRefresh ?? {})) {
    if (refresh.state === "unavailable") names.add(repositoryName);
  }
  for (const { repository_name, status } of repositoryStatusesForFailure(input)) {
    if (status.status_state === "unavailable" || hasRecoverableGitStatusDetailFailure(status)) {
      names.add(repository_name);
    }
  }
  return Array.from(names).sort((a, b) => a.localeCompare(b));
}

export function deriveChangesPanelGitStatus(
  input: ChangesPanelGitStatusInput,
): ChangesPanelGitStatus {
  let statuses: GitStatusScope[] = [];
  if (input.statusByRepo && input.statusByRepo.length > 0) {
    statuses = input.statusByRepo.map(({ status }) => status);
  } else if (input.gitStatus) {
    statuses = [input.gitStatus];
  }
  const completeStatuses = statuses.filter(isCompleteMembership);
  const repositoryRefreshStates = Object.values(input.repositoryRefresh ?? {});
  const failedRepositories = failedRepositoryNames(input);
  const refreshUnavailable =
    input.environmentRefresh?.state === "unavailable" ||
    repositoryRefreshStates.some((refresh) => refresh.state === "unavailable");
  const statusUnavailable = hasUnavailableMembership(statuses);
  const detailUnavailable = hasUnavailableDetails(statuses);
  const unavailable = hasAnyUnavailableState(
    refreshUnavailable,
    statusUnavailable,
    detailUnavailable,
  );
  const refreshPending =
    input.environmentRefresh?.state === "pending" ||
    repositoryRefreshStates.some((refresh) => refresh.state === "pending");
  const loading =
    refreshPending ||
    statuses.some(
      (status) =>
        status.status_state === "loading" ||
        (status.status_state !== "unavailable" && !isCompleteMembership(status)),
    );
  const hasPriorData = completeStatuses.length > 0;
  const membershipReady =
    hasPriorData &&
    completeStatuses.length === statuses.length &&
    !loading &&
    !refreshUnavailable &&
    !statusUnavailable;
  const detailsPending = completeStatuses.some(hasPendingDetails);

  return {
    hasPriorData,
    membershipReady,
    refreshPending,
    loading,
    unavailable,
    detailsPending,
    failedRepositories,
    showEmpty: membershipReady,
  };
}

export function useChangesPanelGitStatus(
  sessionId: string | null,
  gitStatus: GitStatusEntry | undefined,
  statusByRepo: Array<{ repository_name: string; status: GitStatusEntry }>,
): ChangesPanelGitStatus {
  const refresh = useSessionGitStatusRefresh(sessionId);
  return useMemo(
    () =>
      deriveChangesPanelGitStatus({
        gitStatus,
        statusByRepo,
        environmentRefresh: refresh.environment,
        repositoryRefresh: refresh.repositories,
      }),
    [gitStatus, statusByRepo, refresh.environment, refresh.repositories],
  );
}

export function deriveChangesPanelToolbarStatus(
  gitStatus: ChangesPanelGitStatus,
  commitDetailsPending: boolean,
): "loading" | "unavailable" | null {
  if (gitStatus.refreshPending) return "loading";
  if (gitStatus.unavailable) return "unavailable";
  if (gitStatus.loading || gitStatus.detailsPending || commitDetailsPending) return "loading";
  return null;
}
