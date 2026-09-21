"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useStaticDestinations, useAppDestinations } from "@/hooks/use-app-destinations";
import { listWorkspaceCanvases, type Canvas } from "@/lib/api/domains/canvas-api";
import { buildShortcutCatalog, type ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import { useWorkspaceAutomations } from "@/components/runs/use-workspace-automations";
import { useLiveRefresh } from "@/components/runs/use-live-refresh";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";

const EMPTY_CANVASES: Canvas[] = [];

type CanvasState = {
  workspaceId: string | undefined;
  items: Canvas[];
  status: "loading" | "ready" | "error";
  error: string | null;
};

function settledState(workspaceId: string | undefined): CanvasState {
  return {
    workspaceId,
    items: EMPTY_CANVASES,
    status: "ready",
    error: null,
  };
}

function useSidebarCanvases(workspaceId: string | undefined, enabled: boolean, active: boolean) {
  const lifecycleRevision = useCanvasLifecycleRevision();
  const [state, setState] = useState<CanvasState>(() =>
    enabled && workspaceId
      ? { workspaceId, items: EMPTY_CANVASES, status: "loading", error: null }
      : settledState(workspaceId),
  );
  const requestRef = useRef(0);

  const refresh = useCallback(() => {
    const requestId = ++requestRef.current;
    if (!workspaceId || !enabled || !active) {
      setState(settledState(workspaceId));
      return;
    }
    setState({ workspaceId, items: EMPTY_CANVASES, status: "loading", error: null });
    listWorkspaceCanvases(workspaceId)
      .then((response) => {
        if (requestRef.current !== requestId) return;
        setState({
          workspaceId,
          items: response.canvases ?? EMPTY_CANVASES,
          status: "ready",
          error: null,
        });
      })
      .catch((error: unknown) => {
        if (requestRef.current !== requestId) return;
        setState({
          workspaceId,
          items: EMPTY_CANVASES,
          status: "error",
          error: error instanceof Error ? error.message : null,
        });
      });
  }, [active, enabled, workspaceId]);

  useEffect(() => {
    refresh();
  }, [lifecycleRevision, refresh]);

  useLiveRefresh(Boolean(workspaceId && enabled && active), refresh);
  useForegroundRefresh(refresh, Boolean(workspaceId && enabled && active), workspaceId);

  const switching = state.workspaceId !== workspaceId;
  return {
    items: switching ? EMPTY_CANVASES : state.items,
    loading: Boolean(workspaceId && active) && (switching || state.status === "loading"),
    error: switching ? null : state.error,
    refresh,
  };
}

export function useSidebarShortcutCatalog({ active = true }: { active?: boolean } = {}) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((state) => state.workspaces.activeId) ?? undefined;
  const automationStoreItems = useAppStore((state) => state.automations.items);
  const canvasesEnabled = useFeature("canvases");
  const staticDestinations = useStaticDestinations("sidebar", ["primary", "plugins", "insights"]);
  const integrationDestinations = useAppDestinations("sidebar", "integrations");
  const automations = useWorkspaceAutomations(workspaceId, { active });
  const canvases = useSidebarCanvases(workspaceId, canvasesEnabled, active);
  const previousAutomationItems = useRef(automationStoreItems);
  useEffect(() => {
    if (previousAutomationItems.current === automationStoreItems) return;
    previousAutomationItems.current = automationStoreItems;
    if (active && workspaceId) automations.refresh();
  }, [active, automationStoreItems, automations.refresh, workspaceId]);
  useLiveRefresh(Boolean(workspaceId && active), automations.refresh);
  useForegroundRefresh(automations.refresh, Boolean(workspaceId && active), workspaceId);
  const catalog = useMemo<ShortcutCatalogEntry[]>(
    () =>
      buildShortcutCatalog({
        workspaceId,
        destinations: [...staticDestinations, ...integrationDestinations],
        canvases: canvases.items,
        automations: automations.automations,
        translate: t,
      }),
    [
      automations.automations,
      canvases.items,
      integrationDestinations,
      staticDestinations,
      t,
      workspaceId,
    ],
  );

  return {
    workspaceId,
    catalog,
    automations: automations.automations,
    // Canvas availability is independent from the automation definitions and
    // from the built-in/plugin destination catalog. Keep those source states
    // separate so a disabled or unavailable canvas API cannot hide choices or
    // turn a pinned automation's activity bubble into an error.
    loading: automations.loading,
    error: automations.error,
    definitionsLoading: automations.loading,
    definitionsError: automations.error,
    canvasLoading: canvases.loading,
    canvasError: canvases.error,
    refresh: () => {
      automations.refresh();
      canvases.refresh();
    },
  };
}
