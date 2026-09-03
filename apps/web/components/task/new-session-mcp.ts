"use client";

import { useMemo, useState } from "react";
import {
  useMCPSelectionEditor,
  type MCPSelectionScopeRef,
} from "@/hooks/domains/workspace/use-mcp-selection-editor";
import { useMCPWorkspaceDefinitions } from "@/hooks/domains/workspace/use-mcp-workspace-settings";
import type { MCPInheritedSelection } from "@/lib/types/http-mcp";

export function useNewSessionMCPSelection({
  workspaceId,
  taskId,
  profileId,
  repositoryIds,
}: {
  workspaceId?: string | null;
  taskId: string;
  profileId: string;
  repositoryIds: string[];
}) {
  const repositoryIdsKey = repositoryIds.join("\u0000");
  const stableRepositoryIds = useMemo(() => repositoryIds, [repositoryIdsKey]);
  const definitions = useMCPWorkspaceDefinitions(workspaceId);
  const inheritedScopes = useMemo<MCPSelectionScopeRef[]>(() => {
    const refs: MCPSelectionScopeRef[] = [];
    if (profileId) refs.push({ scope: "profile", ownerId: profileId });
    for (const repositoryId of stableRepositoryIds) {
      refs.push({ scope: "repository", ownerId: repositoryId });
    }
    refs.push({ scope: "task", ownerId: taskId });
    return refs;
  }, [profileId, stableRepositoryIds, taskId]);
  const editor = useMCPSelectionEditor("task_session", null, workspaceId, inheritedScopes);
  const inherited = useMemo<MCPInheritedSelection[]>(() => {
    return Object.entries(editor.inheritedOrigins)
      .map(([definitionId, origins]) => {
        const definition = definitions.definitions.find((item) => item.id === definitionId);
        return definition ? { definition, origins } : null;
      })
      .filter((item): item is MCPInheritedSelection => item !== null);
  }, [definitions.definitions, editor.inheritedOrigins]);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  return {
    definitions: definitions.definitions,
    definitionsLoading: definitions.loading || editor.loading,
    inherited,
    selectedIds,
    setSelectedIds,
  };
}
