"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useMCPWorkspaceDefinitions } from "@/hooks/domains/workspace/use-mcp-workspace-settings";
import {
  useMCPSelectionEditor,
  type MCPSelectionScopeRef,
} from "@/hooks/domains/workspace/use-mcp-selection-editor";
import type { MCPInheritedSelection } from "@/lib/types/http-mcp";
import { MCPSessionSelector } from "./mcp-session-selector";

function selectionKey(
  sessionId: string,
  selection: ReturnType<typeof useMCPSelectionEditor>["selection"],
) {
  if (!selection) return `${sessionId}:empty`;
  return `${selection.workspace_id}:${selection.owner_id}:${selection.definition_ids.join(",")}:${selection.mcp_state?.desired_revision ?? 0}:${selection.mcp_state?.apply_state ?? ""}`;
}

export function TaskSessionMCPSettings({
  sessionId,
  taskId,
  workspaceId,
  profileId,
}: {
  sessionId: string;
  taskId: string;
  workspaceId: string | null | undefined;
  profileId?: string;
}) {
  const task = useAppStore((state) => state.kanban.tasks.find((item) => item.id === taskId));
  const repositoryIds = useMemo(
    () => [...new Set((task?.repositories ?? []).map((repository) => repository.repository_id))],
    [task?.repositories],
  );
  const inheritedScopes = useMemo<MCPSelectionScopeRef[]>(() => {
    const refs: MCPSelectionScopeRef[] = [];
    if (profileId) refs.push({ scope: "profile", ownerId: profileId });
    for (const repositoryId of repositoryIds) {
      refs.push({ scope: "repository", ownerId: repositoryId });
    }
    refs.push({ scope: "task", ownerId: taskId });
    return refs;
  }, [profileId, repositoryIds, taskId]);
  const definitions = useMCPWorkspaceDefinitions(workspaceId);
  const editor = useMCPSelectionEditor("task_session", sessionId, workspaceId, inheritedScopes);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const currentSelectionKey = selectionKey(sessionId, editor.selection);
  const applyState = editor.selection?.mcp_state?.apply_state;
  const desiredRevision = editor.selection?.mcp_state?.desired_revision;

  useEffect(() => {
    if (editor.selection) setSelectedIds(editor.selection.definition_ids);
  }, [currentSelectionKey]);

  // Provider reconfiguration runs asynchronously after the selection write.
  // Refresh a pending state until the lifecycle manager records the applied,
  // deferred, or failed result so the notice reflects the authoritative state.
  useEffect(() => {
    if (applyState !== "pending_idle") return;
    let cancelled = false;
    let timeoutId: number | undefined;
    const refresh = async () => {
      await editor.reload();
      if (!cancelled) timeoutId = window.setTimeout(() => void refresh(), 500);
    };
    void refresh();
    return () => {
      cancelled = true;
      if (timeoutId !== undefined) window.clearTimeout(timeoutId);
    };
  }, [applyState, desiredRevision, editor.reload]);

  const inherited = useMemo<MCPInheritedSelection[]>(() => {
    return Object.entries(editor.inheritedOrigins)
      .map(([definitionId, origins]) => {
        const definition = definitions.definitions.find((item) => item.id === definitionId);
        return definition ? { definition, origins } : null;
      })
      .filter((item): item is MCPInheritedSelection => item !== null);
  }, [definitions.definitions, editor.inheritedOrigins]);

  const save = useCallback(
    async (ids: string[]) => {
      setSelectedIds(ids);
      try {
        await editor.save(ids);
      } catch {
        // The selector exposes the failed state after the next authoritative read.
      }
    },
    [editor.save],
  );
  const retry = useCallback(() => {
    void editor.save(selectedIds).catch(() => undefined);
  }, [editor.save, selectedIds]);

  if (!workspaceId) return null;
  return (
    <div className="shrink-0 border-b border-border px-3 py-2 sm:px-4">
      <MCPSessionSelector
        definitions={definitions.definitions}
        definitionsLoading={definitions.loading || editor.loading}
        selectedIds={selectedIds}
        onSelectedIdsChange={(ids) => void save(ids)}
        inherited={inherited}
        state={editor.selection?.mcp_state}
        disabled={editor.saving}
        retry={retry}
        retrying={editor.saving}
        error={editor.saveError ?? editor.loadError ?? definitions.error}
        testId="task-session-mcp-settings"
      />
    </div>
  );
}
