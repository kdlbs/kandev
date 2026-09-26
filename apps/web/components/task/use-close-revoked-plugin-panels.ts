import { useEffect } from "react";
import type { DockviewApi } from "dockview-react";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { parsePluginPanelId } from "@/lib/state/layout-manager/plugin-panels";

/**
 * Closes any open plugin-panel dockview tab whose owning plugin/panel
 * registration has disappeared — a disable, an uninstall, or a plugin
 * reload that dropped the panel (AC4). Re-runs whenever the plugin registry
 * changes; a no-op before dockview is ready.
 */
export function useCloseRevokedPluginPanels(api: DockviewApi | null): void {
  usePluginRegistry();
  const registryVersion = pluginRegistry.getVersion();

  useEffect(() => {
    if (!api) return;
    const missing = api.panels.filter((panel) => {
      const parsed = parsePluginPanelId(panel.id);
      if (!parsed) return false;
      const lifecycle = pluginRegistry.getPluginLifecycle(parsed.pluginId);
      if (!lifecycle || lifecycle.status === "loading" || lifecycle.status === "failed") {
        return false;
      }
      return (
        lifecycle.status === "removed" ||
        !pluginRegistry.getTaskPanel(parsed.pluginId, parsed.panelKey)
      );
    });
    if (missing.length === 0) return;

    for (const panel of missing) api.removePanel(panel);
    // registryVersion drives the re-run; api.panels is read fresh each time.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, registryVersion]);
}
