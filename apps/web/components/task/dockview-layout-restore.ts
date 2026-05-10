import type { DockviewReadyEvent, SerializedDockview } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { applyLayoutFixups } from "@/lib/state/dockview-layout-builders";
import { isLayoutShapeHealthy } from "@/lib/state/dockview-layout-health";
import { measureDockviewContainer } from "@/lib/state/dockview-measure";
import type { LayoutState } from "@/lib/state/layout-manager";
import { getEnvLayout, getEnvMaximizeState, removeEnvMaximizeState } from "@/lib/local-storage";

const LAYOUT_STORAGE_KEY = "dockview-layout-v1";

/* eslint-disable @typescript-eslint/no-explicit-any */
export function sanitizeLayout(
  layout: any,
  validComponents: Set<string>,
  options: { stripSessionPanels?: boolean } = {},
): any {
  if (!isLayoutShapeHealthy(layout)) return null;

  const invalidIds = new Set<string>();
  const validPanels: Record<string, any> = {};
  for (const [id, panel] of Object.entries(layout.panels)) {
    const comp = (panel as any).contentComponent;
    // Session panels are scoped to a specific environment; when restoring the
    // global fallback (no envId yet), they belong to the previous task and
    // would leak in as duplicate tabs. Strip them in that case. The session
    // check must happen before component-validity, since session panels are
    // serialized with contentComponent: "chat" (a valid component) and would
    // otherwise short-circuit the strip guard.
    if (id.startsWith("session:")) {
      if (options.stripSessionPanels) {
        invalidIds.add(id);
      } else {
        validPanels[id] = panel;
      }
    } else if (comp && validComponents.has(comp)) {
      validPanels[id] = panel;
    } else {
      invalidIds.add(id);
    }
  }

  if (invalidIds.size === 0) return layout;

  function cleanNode(node: any): any {
    if (node.type === "leaf") {
      const views = (node.data.views as string[]).filter((v) => !invalidIds.has(v));
      if (views.length === 0) return null;
      const activeView = views.includes(node.data.activeView) ? node.data.activeView : views[0];
      return { ...node, data: { ...node.data, views, activeView } };
    }
    if (node.type === "branch") {
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
/* eslint-enable @typescript-eslint/no-explicit-any */

type SavedMax = ReturnType<typeof getEnvMaximizeState>;

/**
 * Apply a saved maximize blob onto the live dockview api and mirror the full
 * maximize state into the store. Single source of truth for both restore
 * call sites — keeping `preMaximizeLayout` and `maximizedGroupId` in lockstep.
 */
function applySavedMaximize(api: DockviewReadyEvent["api"], savedMax: NonNullable<SavedMax>): void {
  api.fromJSON(savedMax.maximizedDockviewJson as SerializedDockview);
  const { width, height } = measureDockviewContainer(api);
  api.layout(width, height);
  const ids = applyLayoutFixups(api);
  useDockviewStore.setState({
    ...ids,
    preMaximizeLayout: savedMax.preMaximizeLayout as unknown as LayoutState,
    maximizedGroupId: ids.centerGroupId,
  });
}

function applyFixupsWithMaximize(api: DockviewReadyEvent["api"], envId: string | null): void {
  const savedMax = envId ? getEnvMaximizeState(envId) : null;
  if (savedMax) {
    applySavedMaximize(api, savedMax);
  } else {
    const ids = applyLayoutFixups(api);
    useDockviewStore.setState(ids);
  }
}

function tryRestoreMaximizeOnly(api: DockviewReadyEvent["api"], envId: string): boolean {
  const savedMax = getEnvMaximizeState(envId);
  if (!savedMax) return false;
  try {
    applySavedMaximize(api, savedMax);
    return true;
  } catch {
    // Drop the bad blob so subsequent page loads for this env don't keep
    // re-attempting the same failing fromJSON. Mirrors the self-heal in
    // dockview-store's restoreMaximizeFromStorage.
    removeEnvMaximizeState(envId);
    return false;
  }
}

export function tryRestoreLayout(
  api: DockviewReadyEvent["api"],
  currentEnvId: string | null,
  validComponents: Set<string>,
): boolean {
  if (currentEnvId) {
    try {
      const envLayout = getEnvLayout(currentEnvId);
      if (envLayout) {
        const sanitized = sanitizeLayout(envLayout, validComponents);
        if (!sanitized) return false;
        api.fromJSON(sanitized as SerializedDockview);
        applyFixupsWithMaximize(api, currentEnvId);
        return true;
      }
    } catch {
      // fall through to maximize-only / global fallback
    }
    if (tryRestoreMaximizeOnly(api, currentEnvId)) return true;
  }

  if (!currentEnvId) {
    try {
      const saved = localStorage.getItem(LAYOUT_STORAGE_KEY);
      if (saved) {
        const layout = sanitizeLayout(JSON.parse(saved), validComponents, {
          stripSessionPanels: true,
        });
        if (!layout) return false;
        api.fromJSON(layout);
        useDockviewStore.setState(applyLayoutFixups(api));
        return true;
      }
    } catch {
      // fall through to default-build
    }
  }

  return false;
}
