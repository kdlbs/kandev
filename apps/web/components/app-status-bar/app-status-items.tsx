"use client";

import { useMemo, type ReactNode } from "react";
import { StatusSurfaceMetrics } from "@/components/system-metrics/status-surface-metrics";
import { useAppStore } from "@/components/state-provider";
import { useLspStatusPlacement } from "@/hooks/use-lsp-status-placement";
import { getMonacoLanguage } from "@/lib/editor/language-map";
import { toLspLanguage } from "@/lib/lsp/lsp-client-manager";
import type { EffectiveLspStatusPlacement } from "@/lib/lsp/lsp-status-placement";
import { usePluginRegistry, type PluginSlotRegistration } from "@/lib/plugins/registry";
import type { AppStatusBarSlotProps } from "@/lib/plugins/types";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useEditorResolverStore, type EditorProvider } from "@/lib/state/editor-resolver-store";
import { ConnectionStatusItem } from "./connection-status-item";
import { LspStatusItem } from "./lsp-status-item";
import { AppStatusBarPluginContribution } from "./app-status-bar-plugin-slots";
import {
  APP_STATUS_CONNECTION_ID,
  APP_STATUS_LSP_ID,
  APP_STATUS_METRICS_ID,
  type AppStatusItemDescriptor,
} from "./app-status-bar-order";

export type AppStatusItemPresentation = {
  presentation: "bar" | "mobile-drawer";
  density: "full" | "compact";
  drawerOpen: boolean;
};

export type AppStatusItem = AppStatusItemDescriptor & {
  render: (presentation: AppStatusItemPresentation) => ReactNode;
};

type AppStatusContext = Pick<
  AppStatusBarSlotProps,
  "pathname" | "activeWorkspaceId" | "activeTaskId" | "activeSessionId"
>;

export function useAppStatusItems(
  context: AppStatusContext,
  { connectionOnly = false }: { connectionOnly?: boolean } = {},
): AppStatusItem[] {
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const lspPlacement = useLspStatusPlacement();
  const activeFilePath = useDockviewStore((state) => state.activeFilePath);
  const codeEditorProvider = useEditorResolverStore((state) => state.providers["code-editor"]);
  const metricsEnabled = useAppStore(
    (state) => state.userSettings.systemMetricsDisplay.showInTopbar,
  );
  const activeLsp = useMemo(
    () =>
      resolveActiveLspStatusItem({
        placement: lspPlacement,
        activeSessionId: context.activeSessionId,
        activeFilePath,
        editorProvider: codeEditorProvider,
      }),
    [activeFilePath, codeEditorProvider, context.activeSessionId, lspPlacement],
  );

  return useMemo(() => {
    const left = registry.getSlotRegistrations("app-status-bar-left");
    const right = registry.getSlotRegistrations("app-status-bar-right");
    if (connectionOnly) return [connectionItem()];
    return [
      connectionItem(),
      ...left.map((registration) => pluginItem(registration, "left", context)),
      ...(activeLsp ? [lspItem(activeLsp)] : []),
      ...(metricsEnabled ? [metricsItem()] : []),
      ...right.map((registration) => pluginItem(registration, "right", context)),
    ];
  }, [activeLsp, connectionOnly, context, metricsEnabled, registry, registryVersion]);
}

type ActiveLspStatusItem = {
  sessionId: string;
  monacoLanguage: string;
};

export function resolveActiveLspStatusItem({
  placement,
  activeSessionId,
  activeFilePath,
  editorProvider,
}: {
  placement: EffectiveLspStatusPlacement;
  activeSessionId: string | null;
  activeFilePath: string | null;
  editorProvider: EditorProvider;
}): ActiveLspStatusItem | null {
  if (
    placement !== "status_bar" ||
    editorProvider !== "monaco" ||
    !activeSessionId ||
    !activeFilePath
  ) {
    return null;
  }
  const monacoLanguage = getMonacoLanguage(activeFilePath);
  if (!toLspLanguage(monacoLanguage)) return null;
  return { sessionId: activeSessionId, monacoLanguage };
}

function connectionItem(): AppStatusItem {
  return {
    id: APP_STATUS_CONNECTION_ID,
    defaultSide: "left",
    render: ({ presentation }) => <ConnectionStatusItem presentation={presentation} />,
  };
}

function lspItem(active: ActiveLspStatusItem): AppStatusItem {
  return {
    id: APP_STATUS_LSP_ID,
    defaultSide: "right",
    render: () => <LspStatusItem {...active} />,
  };
}

function metricsItem(): AppStatusItem {
  return {
    id: APP_STATUS_METRICS_ID,
    defaultSide: "right",
    render: ({ presentation, density, drawerOpen }) => (
      <StatusSurfaceMetrics presentation={presentation} density={density} drawerOpen={drawerOpen} />
    ),
  };
}

function pluginItem(
  registration: PluginSlotRegistration,
  placement: "left" | "right",
  context: AppStatusContext,
): AppStatusItem {
  return {
    id: registration.orderingId,
    defaultSide: placement,
    render: ({ presentation, density }) => (
      <AppStatusBarPluginContribution
        registration={registration}
        {...context}
        placement={placement}
        presentation={presentation}
        density={density}
      />
    ),
  };
}
