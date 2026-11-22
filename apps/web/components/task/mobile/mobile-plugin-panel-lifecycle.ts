"use client";

import { useEffect } from "react";
import type { PluginLifecycleSnapshot } from "@/lib/plugins/registry";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import { parsePluginPanelId } from "@/lib/state/layout-manager/plugin-panels";

export type MobileReviewSource = string | null;

/** Preserve a plugin panel through loading/recovery; only a definitive
 * removal or a ready generation missing the panel selects the core Chat view. */
export function resolveMobilePluginPanel(
  panel: MobileSessionPanel,
  lifecycle: PluginLifecycleSnapshot | undefined,
  registrationPresent: boolean,
): MobileSessionPanel {
  if (!parsePluginPanelId(panel)) return panel;
  if (!lifecycle || lifecycle.status === "loading" || lifecycle.status === "failed") return panel;
  if (lifecycle.status === "removed") return "chat";
  return registrationPresent ? panel : "chat";
}

/** Core mobile panels can lose their backing capability while restored state
 *  still names them. Review falls back without a review source; Background Work
 *  falls back for passthrough sessions or a disabled feature. */
export function resolveMobileCorePanel(
  panel: MobileSessionPanel,
  reviewSource: MobileReviewSource,
  backgroundWorkAvailable: boolean,
): MobileSessionPanel {
  if (panel === "review" && !reviewSource) return "chat";
  if (panel === "background-work" && !backgroundWorkAvailable) return "chat";
  return panel;
}

export function useEffectiveMobilePanel(
  currentMobilePanel: MobileSessionPanel,
  reviewSource: MobileReviewSource,
  handlePanelChange: (panel: MobileSessionPanel) => void,
  backgroundWorkAvailable = false,
): MobileSessionPanel {
  usePluginRegistry();
  const parsedPluginPanel = parsePluginPanelId(currentMobilePanel);
  const panel = resolveMobilePluginPanel(
    currentMobilePanel,
    parsedPluginPanel ? pluginRegistry.getPluginLifecycle(parsedPluginPanel.pluginId) : undefined,
    parsedPluginPanel
      ? pluginRegistry.getTaskPanel(parsedPluginPanel.pluginId, parsedPluginPanel.panelKey) !==
          undefined
      : false,
  );
  const effectivePanel = resolveMobileCorePanel(panel, reviewSource, backgroundWorkAvailable);
  useEffect(() => {
    if (effectivePanel !== currentMobilePanel) handlePanelChange(effectivePanel);
  }, [currentMobilePanel, effectivePanel, handlePanelChange]);
  return effectivePanel;
}
