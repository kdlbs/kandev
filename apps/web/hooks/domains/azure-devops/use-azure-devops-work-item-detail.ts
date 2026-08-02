"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getAzureDevOpsWorkItemDetail,
  listAzureDevOpsWorkItemComments,
  listWorkspaceAzureDevOpsTaskWorkItems,
  updateAzureDevOpsWorkItemAssignment,
} from "@/lib/api/domains/azure-devops-api";
import type {
  AzureDevOpsTaskWorkItem,
  AzureDevOpsWorkItem,
  AzureDevOpsWorkItemComment,
  AzureDevOpsWorkItemDetail,
} from "@/lib/types/azure-devops";

type DetailState = {
  item: AzureDevOpsWorkItemDetail | null;
  comments: AzureDevOpsWorkItemComment[];
  continuationToken?: string;
  failedContinuationToken?: string;
  linkedTasks: AzureDevOpsTaskWorkItem[];
  loading: boolean;
  commentsLoading: boolean;
  linkedTasksLoading: boolean;
  error: string | null;
  commentsError: string | null;
};

const EMPTY_STATE: DetailState = {
  item: null,
  comments: [],
  continuationToken: undefined,
  failedContinuationToken: undefined,
  linkedTasks: [],
  loading: false,
  commentsLoading: false,
  linkedTasksLoading: false,
  error: null,
  commentsError: null,
};

export function mergeAzureDevOpsComments(
  current: AzureDevOpsWorkItemComment[],
  next: AzureDevOpsWorkItemComment[],
): AzureDevOpsWorkItemComment[] {
  const merged = [...current];
  const seen = new Set(current.map((comment) => comment.id));
  for (const comment of next) {
    if (!seen.has(comment.id)) {
      seen.add(comment.id);
      merged.push(comment);
    }
  }
  return merged;
}

// eslint-disable-next-line max-lines-per-function -- this hook owns detail, comments, linked-task, and assignment lifecycles.
export function useAzureDevOpsWorkItemDetail(
  workspaceId: string | undefined,
  projectId: string,
  initialItem: AzureDevOpsWorkItem | null,
  enabled: boolean,
) {
  const [state, setState] = useState<DetailState>(EMPTY_STATE);
  const generation = useRef(0);
  const key = `${workspaceId ?? ""}:${projectId}:${initialItem?.id ?? ""}`;

  const loadDetail = useCallback(async () => {
    if (!workspaceId || !projectId || !initialItem) return;
    const current = ++generation.current;
    setState((previous) => ({ ...previous, loading: true, error: null }));
    try {
      const item = await getAzureDevOpsWorkItemDetail(workspaceId, projectId, initialItem.id, {
        cache: "no-store",
      });
      if (current === generation.current)
        setState((previous) => ({ ...previous, item, loading: false }));
    } catch (error) {
      if (current === generation.current) {
        setState((previous) => ({ ...previous, loading: false, error: String(error) }));
      }
    }
  }, [initialItem, projectId, workspaceId]);

  const loadComments = useCallback(
    async (continuationToken?: string) => {
      if (!workspaceId || !projectId || !initialItem) return;
      const current = generation.current;
      setState((previous) => ({ ...previous, commentsLoading: true, commentsError: null }));
      try {
        const page = await listAzureDevOpsWorkItemComments(
          workspaceId,
          projectId,
          initialItem.id,
          continuationToken,
          { cache: "no-store" },
        );
        if (current === generation.current) {
          setState((previous) => ({
            ...previous,
            comments: continuationToken
              ? mergeAzureDevOpsComments(previous.comments, page.comments ?? [])
              : (page.comments ?? []),
            continuationToken: page.continuationToken,
            failedContinuationToken: undefined,
            commentsLoading: false,
          }));
        }
      } catch (error) {
        if (current === generation.current) {
          setState((previous) => ({
            ...previous,
            commentsLoading: false,
            commentsError: String(error),
            failedContinuationToken: continuationToken,
          }));
        }
      }
    },
    [initialItem, projectId, workspaceId],
  );

  const loadLinkedTasks = useCallback(async () => {
    if (!workspaceId || !initialItem) return;
    const current = generation.current;
    setState((previous) => ({ ...previous, linkedTasksLoading: true }));
    try {
      const response = await listWorkspaceAzureDevOpsTaskWorkItems(workspaceId, {
        cache: "no-store",
      });
      const all = Object.values(response.taskWorkItems ?? {}).flat();
      if (current === generation.current) {
        setState((previous) => ({
          ...previous,
          linkedTasks: all.filter(
            (task) => task.projectId === projectId && task.workItemId === initialItem.id,
          ),
          linkedTasksLoading: false,
        }));
      }
    } catch {
      if (current === generation.current)
        setState((previous) => ({ ...previous, linkedTasksLoading: false }));
    }
  }, [initialItem, projectId, workspaceId]);

  useEffect(() => {
    generation.current += 1;
    if (!enabled || !workspaceId || !projectId || !initialItem) {
      setState(EMPTY_STATE);
      return;
    }
    setState({ ...EMPTY_STATE, item: { ...initialItem, planningFields: [] } });
    void loadDetail();
    void loadComments();
    void loadLinkedTasks();
  }, [
    enabled,
    key,
    initialItem,
    loadComments,
    loadDetail,
    loadLinkedTasks,
    projectId,
    workspaceId,
  ]);

  const updateAssignee = useCallback(
    async (assigneeAction: "assign_current_user" | "unassign") => {
      if (!workspaceId || !projectId || !state.item) return null;
      const updated = await updateAzureDevOpsWorkItemAssignment(
        workspaceId,
        projectId,
        state.item.id,
        { revision: state.item.revision, assigneeAction },
        { cache: "no-store" },
      );
      setState((previous) => ({
        ...previous,
        item: previous.item ? { ...previous.item, ...updated } : previous.item,
      }));
      return updated;
    },
    [projectId, state.item, workspaceId],
  );

  const mergeItem = useCallback((item: AzureDevOpsWorkItem) => {
    setState((previous) => ({
      ...previous,
      item: previous.item
        ? { ...previous.item, ...item, planningFields: previous.item.planningFields ?? [] }
        : { ...item, planningFields: [] },
    }));
  }, []);

  return {
    ...state,
    refresh: loadDetail,
    retryComments: () => void loadComments(state.failedContinuationToken),
    loadOlderComments: () =>
      state.continuationToken ? void loadComments(state.continuationToken) : undefined,
    updateAssignee,
    mergeItem,
  };
}
