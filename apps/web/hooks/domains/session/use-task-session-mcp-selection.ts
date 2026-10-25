"use client";

import { useCallback, useEffect, useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import { useMCPWorkspaceDefinitions } from "../workspace/use-mcp-workspace-settings";
import {
  useMCPSelectionEditor,
  type MCPSelectionScopeRef,
} from "../workspace/use-mcp-selection-editor";
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
  const error = editor.saveError ?? editor.loadError ?? definitions.error;
  usePendingSelectionRefresh(
    editor.selection?.mcp_state,
    editor.reload,
    editor.saving || isHandledApiError(error),
  );
  const inherited = useMemo<MCPInheritedSelection[]>(
    () =>
      Object.entries(editor.inheritedOrigins).flatMap(([id, origins]) => {
        const definition = definitions.definitions.find((item) => item.id === id);
        return definition ? [{ definition, origins }] : [];
      }),
    [editor.inheritedOrigins, definitions.definitions],
  );
  const save = useCallback(
    (ids: string[]) => {
      void editor.save(ids).catch(() => undefined);
    },
    [editor.save],
  );
  if (!available) return null;
  return {
    contextKey,
    workspaceId: ownerWorkspace,
    definitions: definitions.definitions,
    definitionsLoading: definitions.loading || editor.loading,
    selectedIds: editor.selection?.definition_ids ?? [],
    inherited,
    onSelectedIdsChange: save,
    state: editor.selection?.mcp_state,
    disabled: editor.saving,
    retry: () => save(editor.selection?.definition_ids ?? []),
    retrying: editor.saving,
    error,
  };
}

export type TaskSessionMCPSelection = NonNullable<ReturnType<typeof useTaskSessionMCPSelection>>;
