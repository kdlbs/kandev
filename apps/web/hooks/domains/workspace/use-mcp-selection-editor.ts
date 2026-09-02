"use client";

import { useCallback, useEffect, useState } from "react";
import { getMCPSelections, replaceMCPSelections } from "@/lib/api/domains/mcp-api";
import type {
  MCPSelectionOrigin,
  MCPSelectionResponse,
  MCPSelectionScope,
} from "@/lib/types/http-mcp";

export type MCPSelectionScopeRef = {
  scope: MCPSelectionScope;
  ownerId: string;
};

export type MCPSelectionOrigins = Record<string, MCPSelectionOrigin[]>;

const EMPTY_MCP_SELECTION_SCOPES: MCPSelectionScopeRef[] = [];

export function useMCPSelectionEditor(
  scope: MCPSelectionScope,
  ownerId: string | null | undefined,
  workspaceId: string | null | undefined,
  inheritedScopes: MCPSelectionScopeRef[] = EMPTY_MCP_SELECTION_SCOPES,
) {
  const [selection, setSelection] = useState<MCPSelectionResponse | null>(null);
  const [inheritedOrigins, setInheritedOrigins] = useState<MCPSelectionOrigins>({});
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const inheritedKey = inheritedScopes
    .filter((item) => item.ownerId)
    .map((item) => `${item.scope}:${item.ownerId}`)
    .sort()
    .join(",");

  const reload = useCallback(async () => {
    if (!workspaceId) {
      setSelection(null);
      setInheritedOrigins({});
      return null;
    }
    setLoading(true);
    try {
      const [next, inherited] = await Promise.all([
        ownerId
          ? getMCPSelections(scope, ownerId, workspaceId, { cache: "no-store" })
          : Promise.resolve(null),
        Promise.all(
          inheritedScopes
            .filter((item) => item.ownerId)
            .map(async (item) => ({
              ref: item,
              response: await getMCPSelections(item.scope, item.ownerId, workspaceId, {
                cache: "no-store",
              }),
            })),
        ),
      ]);
      setSelection(next);
      const origins: MCPSelectionOrigins = {};
      for (const item of inherited) {
        for (const definitionId of item.response.definition_ids) {
          const current = origins[definitionId] ?? [];
          origins[definitionId] = [
            ...current,
            {
              scope: item.ref.scope,
              workspace_id: workspaceId,
              owner_id: item.ref.ownerId,
            },
          ];
        }
      }
      setInheritedOrigins(origins);
      setError(null);
      return next;
    } catch (cause) {
      setError(cause);
      return null;
    } finally {
      setLoading(false);
    }
  }, [inheritedKey, inheritedScopes, ownerId, scope, workspaceId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const save = useCallback(
    async (definitionIds: string[]) => {
      if (!ownerId || !workspaceId) throw new Error("MCP selection context is required");
      setSaving(true);
      try {
        const next = await replaceMCPSelections(scope, ownerId, workspaceId, definitionIds);
        setSelection(next);
        setError(null);
        return next;
      } catch (cause) {
        setError(cause);
        throw cause;
      } finally {
        setSaving(false);
      }
    },
    [ownerId, scope, workspaceId],
  );

  return { selection, inheritedOrigins, loading, saving, error, reload, save };
}
