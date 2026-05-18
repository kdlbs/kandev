import type { DockviewReadyEvent, SerializedDockview } from "dockview-react";
import type { StoreApi } from "zustand";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { applyLayoutFixups } from "@/lib/state/dockview-layout-builders";
import { isLayoutShapeHealthy } from "@/lib/state/dockview-layout-health";
import { measureDockviewContainer } from "@/lib/state/dockview-measure";
import type { LayoutState } from "@/lib/state/layout-manager";
import type { AppState } from "@/lib/state/store";
import { getEnvLayout, getEnvMaximizeState, removeEnvMaximizeState } from "@/lib/local-storage";

const LAYOUT_STORAGE_KEY = "dockview-layout-v1";

/* eslint-disable @typescript-eslint/no-explicit-any */
export function sanitizeLayout(
  layout: any,
  validComponents: Set<string>,
  options:
    | { stripSessionPanels: true; excludeSessionIds?: never }
    | { stripSessionPanels?: false | undefined; excludeSessionIds?: Set<string> } = {},
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
      } else if (options.excludeSessionIds) {
        // Per-env restore: drop session panels that we know belong to a
        // different env (a phantom from a previously-deleted task). Sessions
        // we have no mapping for are kept — they may be a still-loading WS
        // arrival, and useAutoSessionTab's reconcile will clean them up if
        // they turn out to be stale.
        const sid = id.slice("session:".length);
        if (options.excludeSessionIds.has(sid)) {
          invalidIds.add(id);
        } else {
          validPanels[id] = panel;
        }
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

/**
 * Restore the per-env saved layout, after sanitizing phantom session panels.
 * Returns true on a successful fromJSON, false when no usable saved layout
 * exists (caller falls through to maximize-only / global / default build).
 */
function tryRestoreEnvLayout(
  api: DockviewReadyEvent["api"],
  envId: string,
  validComponents: Set<string>,
  phantomSessionIds: Set<string> | undefined,
): boolean {
  const envLayout = getEnvLayout(envId);
  if (!envLayout) return false;
  const sanitized = sanitizeLayout(envLayout, validComponents, {
    excludeSessionIds: phantomSessionIds,
  });
  if (!sanitized) return false;
  api.fromJSON(sanitized as SerializedDockview);
  applyFixupsWithMaximize(api, envId);
  return true;
}

export function tryRestoreLayout(
  api: DockviewReadyEvent["api"],
  currentEnvId: string | null,
  validComponents: Set<string>,
  phantomSessionIds?: Set<string>,
): boolean {
  if (currentEnvId) {
    try {
      if (tryRestoreEnvLayout(api, currentEnvId, validComponents, phantomSessionIds)) return true;
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

/**
 * Collect session ids that DEFINITIVELY belong to a different env than `envId`.
 * These are phantoms (typically from a previously-deleted task) that must be
 * stripped on env-layout restore.
 *
 * Sessions absent from `environmentIdBySessionId` are NOT classified as
 * phantoms — they may be a still-loading WS arrival that legitimately belongs
 * to this env. `useAutoSessionTab`'s reconcile cleans up anything that turns
 * out to be stale once the store catches up.
 */
export function collectPhantomSessionIdsForEnv(
  state: { environmentIdBySessionId: Record<string, string> },
  envId: string,
): Set<string> {
  const result = new Set<string>();
  for (const [sessionId, mappedEnv] of Object.entries(state.environmentIdBySessionId)) {
    if (mappedEnv && mappedEnv !== envId) result.add(sessionId);
  }
  return result;
}

/**
 * Restore the env's saved layout, stripping session panels that we KNOW
 * belong to a different env — guards against phantom panels from
 * previously-deleted tasks resurfacing on restore.
 */
export function restoreEnvLayout(
  api: DockviewReadyEvent["api"],
  envId: string | null,
  appStore: StoreApi<AppState>,
  validComponents: Set<string>,
): boolean {
  const phantoms = envId ? collectPhantomSessionIdsForEnv(appStore.getState(), envId) : undefined;
  return tryRestoreLayout(api, envId, validComponents, phantoms);
}
