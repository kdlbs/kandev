"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
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
  acceptedRef,
  savingRef,
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
  acceptedRef: { current: MCPSelectionResponse | null };
  savingRef: { current: boolean };
  setSelection: (value: MCPSelectionResponse) => void;
  setSaveError: (value: unknown) => void;
  setSaving: (value: boolean) => void;
}) {
  const accepted = acceptedRef.current;
  if (
    savingRef.current ||
    !accepted ||
    contextRef.current !== contextKey ||
    accepted.owner_id !== ownerId ||
    accepted.workspace_id !== workspaceId
  ) {
    // i18n-exempt: Internal rejected operation; callers render localized save feedback.
    throw new Error("MCP selection is not ready to save");
  }
  // Invalidate an in-flight reload before writing. A polling response
  // from before this save must not replace the user's newer selection.
  requestRef.current += 1;
  const requestID = ++saveRequestRef.current;
  const requestContext = contextKey;
  const isCurrent = () =>
    saveRequestRef.current === requestID && contextRef.current === requestContext;
  setSaving(true);
  try {
    const condition =
      scope === "task_session"
        ? {
            expected_revision:
              accepted.selection_revision ?? accepted.mcp_state?.desired_revision ?? 0,
          }
        : { expected_definition_ids: accepted.definition_ids };
    const next = await replaceMCPSelections(scope, ownerId, workspaceId, definitionIds, condition);
    if (!isCurrent()) return null;
    setSelection(next);
    setSaveError(null);
    return next;
  } catch (cause) {
    if (isCurrent()) {
      acceptedRef.current = null;
      setSaveError(cause);
    }
    throw cause;
  } finally {
    if (isCurrent()) {
      requestRef.current += 1;
      setSaving(false);
    }
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
  acceptedRef,
  savingRef,
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
  acceptedRef: { current: MCPSelectionResponse | null };
  savingRef: { current: boolean };
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
        acceptedRef,
        savingRef,
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

function useMCPSelectionReload({
  scope,
  ownerId,
  workspaceId,
  contextKey,
  contextRef,
  requestRef,
  scopesRef,
  loadedContextRef,
  savingRef,
  setLoading,
  setRefreshing,
  setLoadedContext,
  setSelection,
  setInheritedOrigins,
  setLoadError,
}: {
  scope: MCPSelectionScope;
  ownerId: string | null | undefined;
  workspaceId: string | null | undefined;
  contextKey: string;
  contextRef: { current: string };
  requestRef: { current: number };
  scopesRef: { current: MCPSelectionScopeRef[] };
  loadedContextRef: { current: string | null };
  savingRef: { current: boolean };
  setLoading: (value: boolean) => void;
  setRefreshing: (value: boolean) => void;
  setLoadedContext: (value: string) => void;
  setSelection: (value: MCPSelectionResponse | null) => void;
  setInheritedOrigins: (value: MCPSelectionOrigins) => void;
  setLoadError: (value: unknown) => void;
}) {
  const reload = useCallback(async () => {
    if (savingRef.current) return null;
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
    if (isCurrent()) {
      setLoading(loadedContextRef.current !== requestContext);
      setRefreshing(loadedContextRef.current === requestContext);
    }
    try {
      const { selection: next, origins } = await loadMCPSelectionData(
        scope,
        ownerId,
        workspaceId,
        scopesRef.current,
      );
      if (!isCurrent()) return null;
      loadedContextRef.current = requestContext;
      setLoadedContext(requestContext);
      setSelection(next);
      setInheritedOrigins(origins);
      setLoadError(null);
      return next;
    } catch (cause) {
      if (isCurrent()) setLoadError(cause);
      return null;
    } finally {
      if (isCurrent()) {
        setLoading(false);
        setRefreshing(false);
      }
    }
  }, [contextKey, ownerId, scope, workspaceId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return reload;
}

function inheritedScopesKey(inheritedScopes: MCPSelectionScopeRef[]) {
  return inheritedScopes
    .filter((item) => item.ownerId)
    .map((item) => `${item.scope}:${item.ownerId}`)
    .sort()
    .join(",");
}

function useAcceptedSelection() {
  const [selection, setSelectionState] = useState<MCPSelectionResponse | null>(null);
  const acceptedRef = useRef<MCPSelectionResponse | null>(null);
  const setSelection = useCallback((next: MCPSelectionResponse | null) => {
    acceptedRef.current = next;
    setSelectionState(next);
  }, []);
  return { selection, acceptedRef, setSelection };
}

function useSelectionSaving(
  setLoading: (value: boolean) => void,
  setRefreshing: (value: boolean) => void,
) {
  const [saving, setSavingState] = useState(false);
  const savingRef = useRef(false);
  const setSaving = useCallback(
    (value: boolean) => {
      savingRef.current = value;
      if (value) {
        setLoading(false);
        setRefreshing(false);
      }
      setSavingState(value);
    },
    [setLoading, setRefreshing],
  );
  return { saving, savingRef, setSaving };
}

export function useMCPSelectionEditor(
  scope: MCPSelectionScope,
  ownerId: string | null | undefined,
  workspaceId: string | null | undefined,
  inheritedScopes: MCPSelectionScopeRef[] = EMPTY_MCP_SELECTION_SCOPES,
) {
  const { selection, acceptedRef, setSelection } = useAcceptedSelection();
  const [inheritedOrigins, setInheritedOrigins] = useState<MCPSelectionOrigins>({});
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [loadedContext, setLoadedContext] = useState<string | null>(null);
  const loadedContextRef = useRef<string | null>(null);
  const { saving, savingRef, setSaving } = useSelectionSaving(setLoading, setRefreshing);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [saveError, setSaveError] = useState<unknown>(null);
  const requestRef = useRef(0);
  const saveRequestRef = useRef(0);
  const scopesRef = useRef(inheritedScopes);
  const inheritedKey = inheritedScopesKey(inheritedScopes);
  const contextKey = `${scope}:${ownerId ?? ""}:${workspaceId ?? ""}:${inheritedKey}`;
  const contextRef = useRef(contextKey);
  useLayoutEffect(() => {
    contextRef.current = contextKey;
    scopesRef.current = inheritedScopes;
  });

  useEffect(() => {
    loadedContextRef.current = null;
    setLoadedContext(null);
    setRefreshing(false);
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
    return () => {
      requestRef.current += 1;
      saveRequestRef.current += 1;
    };
  }, [contextKey, setSaving]);

  const reload = useMCPSelectionReload({
    scope,
    ownerId,
    workspaceId,
    contextKey,
    contextRef,
    requestRef,
    scopesRef,
    loadedContextRef,
    savingRef,
    setLoading,
    setRefreshing,
    setLoadedContext,
    setSelection: (next) => {
      setSelection(next);
      setSaveError(null);
    },
    setInheritedOrigins,
    setLoadError,
  });

  const save = useMCPSelectionSave({
    ownerId,
    workspaceId,
    scope,
    contextKey,
    requestRef,
    saveRequestRef,
    contextRef,
    acceptedRef,
    savingRef,
    setSelection: (next) => {
      loadedContextRef.current = contextKey;
      setLoadedContext(contextKey);
      setSelection(next);
    },
    setSaveError,
    setSaving,
  });

  const ready = loadedContext === contextKey;
  return {
    selection: ready ? selection : null,
    inheritedOrigins: ready ? inheritedOrigins : {},
    loading,
    refreshing,
    saving,
    error: loadError ?? saveError,
    loadError,
    saveError,
    reload,
    save,
  };
}
