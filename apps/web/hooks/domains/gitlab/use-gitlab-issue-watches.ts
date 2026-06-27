"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createIssueWatch,
  updateIssueWatch,
  deleteIssueWatch,
  triggerIssueWatch,
  triggerAllIssueWatches,
  type CreateIssueWatchRequest,
  type UpdateIssueWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import { qk } from "@/lib/query/keys";
import { gitlabIssueWatchesQueryOptions } from "@/lib/query/query-options/gitlab";
import type { IssueWatch } from "@/lib/types/gitlab";

export function useGitLabIssueWatches(workspaceId?: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery(gitlabIssueWatchesQueryOptions(workspaceId));
  const items = query.data ?? [];

  const create = useCallback(
    async (req: CreateIssueWatchRequest) => {
      const watch = await createIssueWatch(req);
      patchGitLabIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const update = useCallback(
    async (id: string, req: UpdateIssueWatchRequest) => {
      const watch = await updateIssueWatch(id, req);
      patchGitLabIssueWatchCaches(queryClient, workspaceId, watch);
      return watch;
    },
    [queryClient, workspaceId],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteIssueWatch(id);
      removeGitLabIssueWatchFromCaches(queryClient, workspaceId, id);
    },
    [queryClient, workspaceId],
  );

  const trigger = useCallback((id: string) => triggerIssueWatch(id), []);
  const triggerAll = useCallback(() => triggerAllIssueWatches(), []);

  return {
    items,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
    create,
    update,
    remove,
    trigger,
    triggerAll,
  };
}

function patchGitLabIssueWatchCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  watch: IssueWatch,
) {
  const patch = (prev: IssueWatch[] | undefined) => upsertById(prev ?? [], watch);
  queryClient.setQueryData(qk.integrations.gitlab.issueWatches(workspaceId), patch);
  queryClient.setQueryData(qk.integrations.gitlab.issueWatches(undefined), patch);
  queryClient.setQueryData(qk.integrations.gitlab.issueWatches(watch.workspace_id), patch);
}

function removeGitLabIssueWatchFromCaches(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string | null | undefined,
  id: string,
) {
  const remove = (prev: IssueWatch[] | undefined) =>
    (prev ?? []).filter((watch) => watch.id !== id);
  queryClient.setQueryData(qk.integrations.gitlab.issueWatches(workspaceId), remove);
  queryClient.setQueryData(qk.integrations.gitlab.issueWatches(undefined), remove);
}

function upsertById<T extends { id: string }>(items: T[], next: T): T[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) return [...items, next];
  const copy = [...items];
  copy[index] = next;
  return copy;
}
