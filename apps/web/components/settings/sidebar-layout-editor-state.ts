"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import {
  defaultSidebarLayout,
  fromApiSidebarLayout,
  type SidebarLayout,
} from "@/lib/sidebar/layout-types";
import { materializeSidebarPluginNodes } from "@/lib/sidebar/layout-projection";
import {
  setShortcutSectionNameDraft,
  type SidebarLayoutOperationError,
} from "@/lib/sidebar/layout-operations";
import type { DraftOperation } from "./sidebar-layout-editor-types";

export function layoutValue(layout: SidebarLayout): string {
  return JSON.stringify(layout.nodes);
}

export function validationCopy(code: string, t: (key: string) => string): string {
  switch (code) {
    case "invalid_group_name":
      return t("settings:sidebarSectionNameError");
    case "duplicate_target":
      return t("settings:sidebarDuplicateShortcutError");
    case "limit_reached":
      return t("settings:sidebarLayoutLimitError");
    default:
      return t("settings:sidebarLayoutInvalidError");
  }
}

export function operationCode(error: unknown): string {
  return error instanceof Error && "code" in error
    ? String((error as SidebarLayoutOperationError).code)
    : "invalid_layout";
}

function sameScopeLayout(
  layouts: Record<string, Parameters<typeof fromApiSidebarLayout>[0]> | undefined,
  workspaceId: string,
): SidebarLayout {
  return fromApiSidebarLayout(layouts?.[workspaceId]);
}

type ScopedDraft = { saved: SidebarLayout; draft: SidebarLayout };

export function useSidebarDraft(catalog: ShortcutCatalogEntry[]) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const layouts = useAppStore((state) => state.userSettings.sidebarLayoutsByWorkspace);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const store = useAppStoreApi();
  const scopeKey = workspaceId ?? "__none__";
  const fallback = useMemo(
    () =>
      materializeSidebarPluginNodes(
        workspaceId ? sameScopeLayout(layouts, workspaceId) : defaultSidebarLayout(),
        catalog,
      ),
    [catalog, layouts, workspaceId],
  );
  const [scopes, setScopes] = useState<Record<string, ScopedDraft>>({});
  const stored = scopes[scopeKey];
  const saved = stored?.saved ?? fallback;
  const draft = stored?.draft ?? fallback;
  const dirty = layoutValue(draft) !== layoutValue(saved);
  const authoritativeRevision = workspaceId ? layouts?.[workspaceId]?.revision : undefined;

  const updateScope = useCallback(
    (next: ScopedDraft) => setScopes((current) => ({ ...current, [scopeKey]: next })),
    [scopeKey],
  );

  const setDraft = useCallback(
    (next: SidebarLayout | ((current: SidebarLayout) => SidebarLayout)) => {
      setScopes((currentScopes) => {
        const current = currentScopes[scopeKey] ?? { saved, draft };
        const nextDraft = typeof next === "function" ? next(current.draft) : next;
        return {
          ...currentScopes,
          [scopeKey]: {
            saved: current.saved,
            draft: materializeSidebarPluginNodes(nextDraft, catalog),
          },
        };
      });
    },
    [catalog, draft, saved, scopeKey],
  );

  const acknowledge = useCallback(
    (ackWorkspaceId: string, nextSaved: SidebarLayout, applyDraft: boolean) => {
      setScopes((currentScopes) => {
        const current = currentScopes[ackWorkspaceId] ?? { saved: nextSaved, draft: nextSaved };
        return {
          ...currentScopes,
          [ackWorkspaceId]: {
            saved: materializeSidebarPluginNodes(nextSaved, catalog),
            draft: applyDraft
              ? materializeSidebarPluginNodes(nextSaved, catalog)
              : materializeSidebarPluginNodes(current.draft, catalog),
          },
        };
      });
    },
    [catalog],
  );

  useEffect(() => {
    if (!workspaceId || !stored || stored.saved.revision === authoritativeRevision) return;
    if (layoutValue(stored.draft) !== layoutValue(stored.saved)) return;
    const next = materializeSidebarPluginNodes(sameScopeLayout(layouts, workspaceId), catalog);
    updateScope({ saved: next, draft: next });
  }, [authoritativeRevision, catalog, layouts, stored, updateScope, workspaceId]);

  return {
    workspaceId,
    scopeKey,
    saved,
    draft,
    dirty,
    setDraft,
    acknowledge,
    setUserSettings,
    store,
  };
}

export function useSidebarLayoutActions({
  draft,
  workspaceId,
  setDraft,
}: {
  draft: SidebarLayout;
  workspaceId: string | null;
  setDraft: (next: SidebarLayout | ((current: SidebarLayout) => SidebarLayout)) => void;
}) {
  const [operationError, setOperationError] = useState<string | null>(null);
  const draftRef = useRef(draft);
  const workspaceRef = useRef(workspaceId);
  const generationsRef = useRef(new Map<string, number>());
  const requestGenerationsRef = useRef(new Map<string, number>());
  const scopeKey = workspaceId ?? "__none__";
  draftRef.current = draft;
  workspaceRef.current = workspaceId;

  const applyDraftOperation = useCallback(
    (operation: DraftOperation): boolean => {
      if (draftRef.current.unsupportedVersion) return false;
      try {
        const next = operation(draftRef.current);
        draftRef.current = next;
        generationsRef.current.set(scopeKey, (generationsRef.current.get(scopeKey) ?? 0) + 1);
        setDraft(next);
        setOperationError(null);
        return true;
      } catch (error) {
        setOperationError(operationCode(error));
        return false;
      }
    },
    [scopeKey, setDraft],
  );

  const setNameDraft = useCallback(
    (nodeId: string, name: string) => {
      applyDraftOperation((current) => setShortcutSectionNameDraft(current, nodeId, name));
    },
    [applyDraftOperation],
  );

  const commitName = useCallback(
    (rename: DraftOperation) => {
      applyDraftOperation(rename);
    },
    [applyDraftOperation],
  );

  return {
    operationError,
    setOperationError,
    draftRef,
    workspaceRef,
    generationsRef,
    requestGenerationsRef,
    applyDraftOperation,
    setNameDraft,
    commitName,
  };
}
