import type { DockviewReadyEvent, SerializedDockview } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { applyLayoutFixups } from "@/lib/state/dockview-layout-builders";
import { getSessionLayout, getSessionMaximizeState } from "@/lib/local-storage";

const LAYOUT_STORAGE_KEY = "dockview-layout-v1";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function sanitizeLayout(layout: any, validComponents: Set<string>): any {
  if (!layout?.panels || !layout?.grid?.root) return null;

  const invalidIds = new Set<string>();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const validPanels: Record<string, any> = {};
  for (const [id, panel] of Object.entries(layout.panels)) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const comp = (panel as any).contentComponent;
    if (comp && (validComponents.has(comp) || id.startsWith("session:"))) {
      validPanels[id] = panel;
    } else {
      invalidIds.add(id);
    }
  }

  if (invalidIds.size === 0) return layout;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function cleanNode(node: any): any {
    if (node.type === "leaf") {
      const views = (node.data.views as string[]).filter((v) => !invalidIds.has(v));
      if (views.length === 0) return null;
      const activeView = views.includes(node.data.activeView) ? node.data.activeView : views[0];
      return { ...node, data: { ...node.data, views, activeView } };
    }
    if (node.type === "branch") {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const children = (node.data as any[]).map(cleanNode).filter(Boolean);
      if (children.length === 0) return null;
      return { ...node, data: children };
    }
    return node;
  }

  const cleanedRoot = cleanNode(layout.grid.root);
  if (!cleanedRoot) return null;

  return {
    ...layout,
    grid: { ...layout.grid, root: cleanedRoot },
    panels: validPanels,
  };
}

function applyFixupsWithMaximize(api: DockviewReadyEvent["api"], sessionId: string | null): void {
  const savedMax = sessionId ? getSessionMaximizeState(sessionId) : null;
  if (savedMax) {
    api.fromJSON(savedMax.maximizedDockviewJson as SerializedDockview);
    api.layout(api.width, api.height);
    const ids = applyLayoutFixups(api);
    type LM = import("@/lib/state/layout-manager").LayoutState;
    useDockviewStore.setState({
      ...ids,
      preMaximizeLayout: savedMax.preMaximizeLayout as unknown as LM,
    });
  } else {
    const ids = applyLayoutFixups(api);
    useDockviewStore.setState(ids);
  }
}

function tryRestoreMaximizeOnly(api: DockviewReadyEvent["api"], sessionId: string): boolean {
  const savedMax = getSessionMaximizeState(sessionId);
  if (!savedMax) return false;
  try {
    api.fromJSON(savedMax.maximizedDockviewJson as SerializedDockview);
    api.layout(api.width, api.height);
    const ids = applyLayoutFixups(api);
    type LM = import("@/lib/state/layout-manager").LayoutState;
    useDockviewStore.setState({
      ...ids,
      preMaximizeLayout: savedMax.preMaximizeLayout as unknown as LM,
    });
    return true;
  } catch {
    return false;
  }
}

export function tryRestoreLayout(
  api: DockviewReadyEvent["api"],
  currentSessionId: string | null,
  validComponents: Set<string>,
): boolean {
  if (currentSessionId) {
    try {
      const sessionLayout = getSessionLayout(currentSessionId);
      if (sessionLayout) {
        const sanitized = sanitizeLayout(sessionLayout, validComponents);
        if (!sanitized) return false;
        api.fromJSON(sanitized as SerializedDockview);
        applyFixupsWithMaximize(api, currentSessionId);
        return true;
      }
    } catch {
      // Per-session restore failed, try global
    }
    if (tryRestoreMaximizeOnly(api, currentSessionId)) return true;
  }

  if (!currentSessionId) {
    try {
      const saved = localStorage.getItem(LAYOUT_STORAGE_KEY);
      if (saved) {
        const layout = sanitizeLayout(JSON.parse(saved), validComponents);
        if (!layout) return false;
        api.fromJSON(layout);
        useDockviewStore.setState(applyLayoutFixups(api));
        return true;
      }
    } catch {
      // Global restore failed, build default
    }
  }

  return false;
}
