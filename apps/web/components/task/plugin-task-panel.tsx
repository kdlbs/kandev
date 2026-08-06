"use client";

import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { PluginErrorBoundary } from "@/components/plugins/plugin-error-boundary";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginPresentation } from "@/lib/plugins/types";

export interface PluginTaskPanelContainerProps {
  pluginId: string;
  panelKey: string;
  /** Full dockview/mobile panel id, e.g. `plugin:<pluginId>:<panelKey>`. */
  panelId: string;
  presentation: PluginPresentation;
}

function PluginTaskPanelUnavailable() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full items-center justify-center p-8 text-center text-sm text-muted-foreground">
      {t("common:pluginPanelUnavailable")}
    </div>
  );
}

function PluginTaskPanelFailed() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full items-center justify-center p-8 text-center text-sm text-muted-foreground">
      {t("common:pluginPanelFailedToLoad")}
    </div>
  );
}

/**
 * Resolves `{ pluginId, panelKey }` to its current `TaskPanelRegistration`
 * and renders the plugin's `Component` with `PluginTaskPanelProps`, wrapped
 * in a `PluginErrorBoundary` (AC6) so a throw inside the plugin's render
 * can't take down the surrounding dockview/mobile layout. Renders a
 * "no longer available" fallback — not a throw — when the plugin was
 * disabled/uninstalled after the panel was opened, or the layout it was
 * restored from references a panel of a plugin that is no longer installed
 * (AC5).
 */
export function PluginTaskPanel({
  pluginId,
  panelKey,
  panelId,
  presentation,
}: PluginTaskPanelContainerProps) {
  // Re-render when the registry changes (plugin disable/uninstall/reload) so
  // this panel picks up the fallback the instant its registration disappears.
  usePluginRegistry();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const registration = pluginRegistry.getTaskPanel(pluginId, panelKey);

  if (!registration || !taskId) {
    return <PluginTaskPanelUnavailable />;
  }

  const { Component } = registration;
  return (
    <PluginErrorBoundary context={`task panel "${panelId}"`} fallback={<PluginTaskPanelFailed />}>
      <Component
        panelId={panelId}
        taskId={taskId}
        sessionId={sessionId}
        presentation={presentation}
      />
    </PluginErrorBoundary>
  );
}
