"use client";

import { useCallback, useEffect, useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useMCPWorkspaceDefinitions } from "../workspace/use-mcp-workspace-settings";
import {
  useMCPSelectionEditor,
  type MCPSelectionScopeRef,
} from "../workspace/use-mcp-selection-editor";
import { useMCPSelectionDraft } from "../workspace/use-mcp-selection-draft";
import { isHandledApiError } from "@/lib/api/client";
import type { MCPInheritedSelection, MCPSessionSelectionState } from "@/lib/types/http-mcp";

function usePendingSelectionRefresh(
  state: MCPSessionSelectionState | undefined,
  reload: () => Promise<unknown>,
  blocked: boolean,
) {
  useEffect(() => {
    if (!state || state.apply_state === "applied" || blocked) return;
    const delay = state.apply_state === "pending_idle" ? 500 : 2000;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      await reload();
      if (!cancelled) timer = setTimeout(() => void refresh(), delay);
    };
    timer = setTimeout(() => void refresh(), delay);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [state?.apply_state, state?.desired_revision, reload, blocked]);
}

type SessionMCPContext = {
  taskId: string | null;
  sessionId: string | null;
  workspaceId?: string | null;
};

function useTaskSessionMCPContext({ taskId, sessionId, workspaceId }: SessionMCPContext) {
  const { task, session, workflowWorkspace } = useAppStore(
    useShallow((state) => {
      const task = state.kanban.tasks.find((item) => item.id === taskId);
      return {
        task,
        session: sessionId ? state.taskSessions.items[sessionId] : undefined,
        workflowWorkspace: state.workflows.items.find((item) => item.id === task?.workflowId)
          ?.workspaceId,
      };
    }),
  );
  const taskWorkspace = task?.workspaceId ?? workflowWorkspace;
  const ownerWorkspace = workspaceId ?? taskWorkspace;
  const available = Boolean(
    task &&
    session &&
    session.task_id === taskId &&
    ownerWorkspace &&
    taskWorkspace === ownerWorkspace,
  );
  const inheritedScopes = useMemo<MCPSelectionScopeRef[]>(() => {
    if (!available || !taskId) return [];
    const refs: MCPSelectionScopeRef[] = [{ scope: "task", ownerId: taskId }];
    if (session?.agent_profile_id)
      refs.push({ scope: "profile", ownerId: session.agent_profile_id });
    for (const id of new Set((task?.repositories ?? []).map((item) => item.repository_id)))
      refs.push({ scope: "repository", ownerId: id });
    return refs;
  }, [available, taskId, session?.agent_profile_id, task?.repositories]);
  return {
    available,
    ownerWorkspace,
    inheritedScopes,
    contextKey: `${ownerWorkspace}:${taskId}:${sessionId}:${session?.agent_profile_id ?? ""}`,
  };
}

export function useTaskSessionMCPSelection(context: SessionMCPContext) {
  const { available, ownerWorkspace, inheritedScopes, contextKey } =
    useTaskSessionMCPContext(context);
  const definitions = useMCPWorkspaceDefinitions(available ? ownerWorkspace : null);
  const editor = useMCPSelectionEditor(
    "task_session",
    available ? context.sessionId : null,
    available ? ownerWorkspace : null,
    inheritedScopes,
  );
  const draft = useMCPSelectionDraft(contextKey, editor.selection);
  const error = editor.saveError ?? editor.loadError ?? definitions.error;
  const {
    mcp_state: state,
    definition_ids: savedSelectedIds = [],
    excluded_definition_ids: excludedIds = [],
  } = editor.selection ?? {};
  const blocked = editor.saving || Boolean(editor.saveError);
  usePendingSelectionRefresh(state, editor.reload, blocked || isHandledApiError(error));
  const inherited = useMemo<MCPInheritedSelection[]>(
    () =>
      Object.entries(editor.inheritedOrigins).flatMap(([id, origins]) => {
        const definition = definitions.definitions.find((item) => item.id === id);
        const ownOrigin =
          editor.selection?.definition_ids.includes(id) && context.sessionId && ownerWorkspace
            ? [
                {
                  scope: "task_session" as const,
                  workspace_id: ownerWorkspace,
                  owner_id: context.sessionId,
                },
              ]
            : [];
        return definition ? [{ definition, origins: [...origins, ...ownOrigin] }] : [];
      }),
    [
      editor.inheritedOrigins,
      editor.selection,
      definitions.definitions,
      context.sessionId,
      ownerWorkspace,
    ],
  );
  const save = useCallback(
    (ids: string[]) => {
      draft.setSelectedIds(ids);
      void editor
        .save(ids)
        .then(draft.acknowledge)
        .catch(() => undefined);
    },
    [editor.save, draft.setSelectedIds, draft.acknowledge],
  );
  const onExcludedIdsChange = useCallback(
    (ids: string[]) => {
      void editor.save(editor.selection?.definition_ids ?? [], ids).catch(() => undefined);
    },
    [editor.save, editor.selection],
  );
  if (!available) return null;
  return {
    contextKey,
    ...(context.sessionId ? { assignmentSessionId: context.sessionId } : {}),
    workspaceId: ownerWorkspace,
    definitions: definitions.definitions,
    definitionsLoading: definitions.loading || editor.loading,
    selectedIds: draft.selectedIds,
    savedSelectedIds,
    excludedIds,
    onExcludedIdsChange,
    inherited,
    onSelectedIdsChange: save,
    state,
    disabled: blocked,
    reload: editor.reload,
    retry: blocked ? undefined : () => void editor.save(savedSelectedIds).catch(() => undefined),
    retrying: editor.saving,
    error,
  };
}

export type TaskSessionMCPSelection = NonNullable<ReturnType<typeof useTaskSessionMCPSelection>>;
