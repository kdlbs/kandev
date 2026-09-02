"use client";

import { useEffect, useMemo } from "react";
import {
  useMCPSelectionEditor,
  type MCPSelectionScopeRef,
} from "@/hooks/domains/workspace/use-mcp-selection-editor";
import { useMCPWorkspaceDefinitions } from "@/hooks/domains/workspace/use-mcp-workspace-settings";
import type { MCPInheritedSelection } from "@/lib/types/http-mcp";

type TaskCreateMCPSetupArgs = {
  open: boolean;
  workspaceId: string | null | undefined;
  isSessionMode: boolean;
  taskId?: string | null;
  effectiveAgentProfileId: string;
  repositories: { repositoryId?: string | null }[];
  mcpServerIdsDirty: boolean;
  setMcpServerIds: (ids: string[]) => void;
};

export function useTaskCreateDialogMCPSetup({
  open,
  workspaceId,
  isSessionMode,
  taskId,
  effectiveAgentProfileId,
  repositories,
  mcpServerIdsDirty,
  setMcpServerIds,
}: TaskCreateMCPSetupArgs) {
  const mcpWorkspaceId = open ? workspaceId : null;
  const definitions = useMCPWorkspaceDefinitions(mcpWorkspaceId);
  const inheritedScopes = useMemo<MCPSelectionScopeRef[]>(() => {
    const refs: MCPSelectionScopeRef[] = [];
    if (effectiveAgentProfileId) {
      refs.push({ scope: "profile", ownerId: effectiveAgentProfileId });
    }
    for (const row of repositories) {
      if (row.repositoryId) refs.push({ scope: "repository", ownerId: row.repositoryId });
    }
    if (isSessionMode && taskId) refs.push({ scope: "task", ownerId: taskId });
    return refs;
  }, [effectiveAgentProfileId, isSessionMode, repositories, taskId]);
  const editor = useMCPSelectionEditor(
    isSessionMode ? "task_session" : "task",
    isSessionMode ? null : taskId,
    mcpWorkspaceId,
    inheritedScopes,
  );
  const inheritedSelections = useMemo<MCPInheritedSelection[]>(() => {
    return Object.entries(editor.inheritedOrigins)
      .map(([definitionId, origins]) => {
        const definition = definitions.definitions.find((item) => item.id === definitionId);
        return definition ? { definition, origins } : null;
      })
      .filter((item): item is MCPInheritedSelection => item !== null);
  }, [definitions.definitions, editor.inheritedOrigins]);

  useEffect(() => {
    if (mcpServerIdsDirty || !editor.selection) return;
    setMcpServerIds(editor.selection.definition_ids);
  }, [editor.selection, mcpServerIdsDirty, setMcpServerIds]);

  return {
    definitions: definitions.definitions,
    definitionsLoading: definitions.loading || editor.loading,
    inheritedSelections,
    editor,
  };
}
