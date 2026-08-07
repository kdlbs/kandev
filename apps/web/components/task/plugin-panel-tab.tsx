"use client";

import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { resolvePluginIcon } from "@/lib/plugins/icons";

/**
 * Tab chrome for a plugin task panel: the plugin's curated icon (if any)
 * beside the default title/close chrome, mirroring `plan-tab.tsx`'s
 * structure. `params.pluginId`/`params.panelKey` identify which
 * registration to read the icon from — the title itself is owned by
 * dockview's `api.title`, set from the registration when the panel opens.
 */
export function PluginPanelTab(props: IDockviewPanelHeaderProps) {
  usePluginRegistry();
  const pluginId = props.params?.pluginId as string | undefined;
  const panelKey = props.params?.panelKey as string | undefined;
  const registration =
    pluginId && panelKey ? pluginRegistry.getTaskPanel(pluginId, panelKey) : undefined;
  const Icon = resolvePluginIcon(registration?.icon);

  return (
    <div data-testid="plugin-panel-tab" className="relative flex items-center gap-1.5">
      <Icon className="ml-2 size-3.5 shrink-0 text-muted-foreground" />
      <DockviewDefaultTab {...props} />
    </div>
  );
}
