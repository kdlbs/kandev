"use client";

import { useCallback, useEffect, useRef, useState } from "react";
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

function resetMCPSelectionEditor({
  requestRef,
  saveRequestRef,
  setSelection,
  setInheritedOrigins,
  setLoadError,
  setSaveError,
  setLoading,
  setSaving,
}: {
  requestRef: { current: number };
  saveRequestRef: { current: number };
  setSelection: (value: MCPSelectionResponse | null) => void;
  setInheritedOrigins: (value: MCPSelectionOrigins) => void;
  setLoadError: (value: unknown) => void;
  setSaveError: (value: unknown) => void;
  setLoading: (value: boolean) => void;
  setSaving: (value: boolean) => void;
}) {
  requestRef.current += 1;
  saveRequestRef.current += 1;
  setSelection(null);
  setInheritedOrigins({});
  setLoadError(null);
  setSaveError(null);
  setLoading(false);
  setSaving(false);
}

async function saveMCPSelection({
  definitionIds,
  ownerId,
  workspaceId,
  scope,
  contextKey,
  requestRef,
  saveRequestRef,
  contextRef,
  setSelection,
  setSaveError,
  setSaving,
}: {
  definitionIds: string[];
  ownerId: string;
  workspaceId: string;
  scope: MCPSelectionScope;
  contextKey: string;
  requestRef: { current: number };
  saveRequestRef: { current: number };
  contextRef: { current: string };
  setSelection: (value: MCPSelectionResponse) => void;
  setSaveError: (value: unknown) => void;
  setSaving: (value: boolean) => void;
}) {
  // Invalidate an in-flight reload before writing. A polling response
  // from before this save must not replace the user's newer selection.
  requestRef.current += 1;
  const requestID = ++saveRequestRef.current;
  const requestContext = contextKey;
  const isCurrent = () =>
    saveRequestRef.current === requestID && contextRef.current === requestContext;
  setSaving(true);
  try {
    const next = await replaceMCPSelections(scope, ownerId, workspaceId, definitionIds);
    if (!isCurrent()) return null;
    setSelection(next);
    setSaveError(null);
    return next;
  } catch (cause) {
    if (isCurrent()) setSaveError(cause);
    throw cause;
  } finally {
    requestRef.current += 1;
    if (isCurrent()) setSaving(false);
  }
}

function useMCPSelectionSave({
  ownerId,
  workspaceId,
  scope,
  contextKey,
  requestRef,
  saveRequestRef,
  contextRef,
  setSelection,
  setSaveError,
  setSaving,
}: {
  ownerId: string | null | undefined;
  workspaceId: string | null | undefined;
  scope: MCPSelectionScope;
  contextKey: string;
  requestRef: { current: number };
  saveRequestRef: { current: number };
  contextRef: { current: string };
  setSelection: (value: MCPSelectionResponse) => void;
  setSaveError: (value: unknown) => void;
  setSaving: (value: boolean) => void;
}) {
  return useCallback(
    (definitionIds: string[]) => {
      if (!ownerId || !workspaceId) throw new Error("MCP selection context is required");
      return saveMCPSelection({
        definitionIds,
        ownerId,
        workspaceId,
        scope,
        contextKey,
        requestRef,
        saveRequestRef,
        contextRef,
        setSelection,
        setSaveError,
        setSaving,
      });
    },
    [contextKey, ownerId, scope, workspaceId],
  );
}

async function loadMCPSelectionData(
  scope: MCPSelectionScope,
  ownerId: string | null | undefined,
  workspaceId: string,
  inheritedScopes: MCPSelectionScopeRef[],
) {
  const scopes = inheritedScopes.filter((item) => item.ownerId);
  const [selection, inherited] = await Promise.all([
    ownerId
      ? getMCPSelections(scope, ownerId, workspaceId, { cache: "no-store" })
      : Promise.resolve(null),
    Promise.all(
      scopes.map(async (item) => ({
        ref: item,
        response: await getMCPSelections(item.scope, item.ownerId, workspaceId, {
          cache: "no-store",
        }),
      })),
    ),
  ]);
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
  return { selection, origins };
}

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
  const [loadError, setLoadError] = useState<unknown>(null);
  const [saveError, setSaveError] = useState<unknown>(null);
  const requestRef = useRef(0);
  const saveRequestRef = useRef(0);
  const scopesRef = useRef(inheritedScopes);
  scopesRef.current = inheritedScopes;

  const inheritedKey = inheritedScopes
    .filter((item) => item.ownerId)
    .map((item) => `${item.scope}:${item.ownerId}`)
    .sort()
    .join(",");
  const contextKey = `${scope}:${ownerId ?? ""}:${workspaceId ?? ""}:${inheritedKey}`;
  const contextRef = useRef(contextKey);
  contextRef.current = contextKey;

  useEffect(() => {
    resetMCPSelectionEditor({
      requestRef,
      saveRequestRef,
      setSelection,
      setInheritedOrigins,
      setLoadError,
      setSaveError,
      setLoading,
      setSaving,
    });
  }, [contextKey]);

  const reload = useCallback(async () => {
    const requestID = ++requestRef.current;
    const requestContext = contextKey;
    const isCurrent = () =>
      requestRef.current === requestID && contextRef.current === requestContext;
    if (!workspaceId) {
      if (isCurrent()) {
        setSelection(null);
        setInheritedOrigins({});
      }
      return null;
    }
    if (isCurrent()) setLoading(true);
    try {
      const { selection: next, origins } = await loadMCPSelectionData(
        scope,
        ownerId,
        workspaceId,
        scopesRef.current,
      );
      if (!isCurrent()) return null;
      setSelection(next);
      setInheritedOrigins(origins);
      setLoadError(null);
      return next;
    } catch (cause) {
      if (isCurrent()) setLoadError(cause);
      return null;
    } finally {
      if (isCurrent()) setLoading(false);
    }
  }, [contextKey, ownerId, scope, workspaceId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const save = useMCPSelectionSave({
    ownerId,
    workspaceId,
    scope,
    contextKey,
    requestRef,
    saveRequestRef,
    contextRef,
    setSelection,
    setSaveError,
    setSaving,
  });

  return {
    selection,
    inheritedOrigins,
    loading,
    saving,
    error: loadError ?? saveError,
    loadError,
    saveError,
    reload,
    save,
  };
}
