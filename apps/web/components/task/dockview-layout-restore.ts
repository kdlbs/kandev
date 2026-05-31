import type { DockviewReadyEvent, SerializedDockview } from "dockview-react";
import type { StoreApi } from "zustand";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { applyLayoutFixups } from "@/lib/state/dockview-layout-builders";
import { savedRightColumnWidth } from "@/lib/state/dockview-env-switch";
import { isLayoutShapeHealthy } from "@/lib/state/dockview-layout-health";
import { measureDockviewContainer } from "@/lib/state/dockview-measure";
import { isEnvScopedDockviewComponent } from "@/lib/state/dockview-env-scoped-components";
import type { LayoutState } from "@/lib/state/layout-manager";
import { setPinnedTarget } from "@/lib/state/layout-manager";
import type { AppState } from "@/lib/state/store";
import { getEnvLayout, getEnvMaximizeState, removeEnvMaximizeState } from "@/lib/local-storage";
import { createDebugLogger, IS_DEBUG } from "@/lib/debug/log";

const debug = createDebugLogger("dockview:restore");

/* eslint-disable @typescript-eslint/no-explicit-any */
type SanitizeLayoutOptions =
  | { stripSessionPanels: true; stripEnvScopedPanels?: boolean; excludeSessionIds?: never }
  | {
      stripSessionPanels?: false | undefined;
      stripEnvScopedPanels?: boolean;
      excludeSessionIds?: Set<string>;
    };

function describeSanitizeMode(options: {
  stripSessionPanels?: boolean;
  stripEnvScopedPanels?: boolean;
  excludeSessionIds?: Set<string>;
}): string {
  if (options.stripSessionPanels && options.stripEnvScopedPanels)
    return "stripSessionsAndEnvScoped";
  if (options.stripSessionPanels) return "stripAllSessions";
  if (options.stripEnvScopedPanels) return "stripEnvScoped";
  if (options.excludeSessionIds) return "excludeSpecificSessions";
  return "keepAll";
}

function logSanitizeOutcome(
  options: {
    stripSessionPanels?: boolean;
    stripEnvScopedPanels?: boolean;
    excludeSessionIds?: Set<string>;
  },
  totalPanels: Record<string, any>,
  validPanels: Record<string, any>,
  invalidIds: Set<string>,
): void {
  if (!IS_DEBUG) return;
  debug("sanitizeLayout", {
    mode: describeSanitizeMode(options),
    excludeSessionCount: options.excludeSessionIds?.size ?? 0,
    excludeSessionIds: options.excludeSessionIds
      ? Array.from(options.excludeSessionIds)
      : undefined,
    totalPanels: Object.keys(totalPanels).length,
    keptPanels: Object.keys(validPanels).length,
    strippedPanels: Array.from(invalidIds),
  });
}

function shouldKeepSessionPanel(id: string, options: SanitizeLayoutOptions): boolean {
  if (options.stripSessionPanels) return false;
  if (!options.excludeSessionIds) return true;

  // Per-env restore: drop session panels that we know belong to a
  // different env (a phantom from a previously-deleted task). Sessions
  // we have no mapping for are kept — they may be a still-loading WS
  // arrival, and useAutoSessionTab's reconcile will clean them up if
  // they turn out to be stale.
  const sid = id.slice("session:".length);
  return !options.excludeSessionIds.has(sid);
}

function shouldKeepPanel(
  id: string,
  panel: any,
  validComponents: Set<string>,
  options: SanitizeLayoutOptions,
): boolean {
  const comp = panel.contentComponent;

  // Session panels are scoped to a specific environment; when restoring the
  // global fallback (no envId yet), they belong to the previous task and
  // would leak in as duplicate tabs. Strip them in that case. The session
  // check must happen before component-validity, since session panels are
  // serialized with contentComponent: "chat" (a valid component) and would
  // otherwise short-circuit the strip guard.
  if (id.startsWith("session:")) return shouldKeepSessionPanel(id, options);

  if (options.stripEnvScopedPanels && isEnvScopedDockviewComponent(comp)) return false;
  return !!(comp && validComponents.has(comp));
}
/* eslint-enable @typescript-eslint/no-explicit-any */

/* eslint-disable @typescript-eslint/no-explicit-any */
export function sanitizeLayout(
  layout: any,
  validComponents: Set<string>,
  options: SanitizeLayoutOptions = {},
): any {
  if (!isLayoutShapeHealthy(layout)) {
    debug("sanitizeLayout: layout shape unhealthy, returning null");
    return null;
  }

  const invalidIds = new Set<string>();
  const validPanels: Record<string, any> = {};
  for (const [id, panel] of Object.entries(layout.panels)) {
    if (shouldKeepPanel(id, panel, validComponents, options)) {
      validPanels[id] = panel;
    } else {
      invalidIds.add(id);
    }
  }

  logSanitizeOutcome(options, layout.panels, validPanels, invalidIds);

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
  if (!cleanedRoot) {
    debug("sanitizeLayout: cleanedRoot is null after stripping, returning null");
    return null;
  }

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
function applySavedMaximize(
  api: DockviewReadyEvent["api"],
  savedMax: NonNullable<SavedMax>,
  savedRightWidth?: number,
): void {
  api.fromJSON(savedMax.maximizedDockviewJson as SerializedDockview);
  const { width, height } = measureDockviewContainer(api);
  api.layout(width, height);
  const ids = applyLayoutFixups(api);
  // The maximize JSON is 2-column — captureRightTarget skips it (sv.length < 3).
  // Seed the right target directly so enforcePinnedTargets can snap the column
  // back to the saved width when the user exits maximize mode.
  if (savedRightWidth !== undefined && savedRightWidth > 0) {
    setPinnedTarget("right", savedRightWidth);
  }
  useDockviewStore.setState({
    ...ids,
    preMaximizeLayout: savedMax.preMaximizeLayout as unknown as LayoutState,
    maximizedGroupId: ids.centerGroupId,
  });
}

function applyFixupsWithMaximize(
  api: DockviewReadyEvent["api"],
  envId: string | null,
  savedRightWidth?: number,
): void {
  const savedMax = envId ? getEnvMaximizeState(envId) : null;
  if (savedMax) {
    applySavedMaximize(api, savedMax, savedRightWidth);
  } else {
    // Anchor the right column to its per-env saved width (see
    // `captureRightTarget`) so a page reload restores the task's remembered
    // width instead of resetting it to the default.
    const ids = applyLayoutFixups(api, savedRightWidth);
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
  if (!envLayout) {
    debug("tryRestoreEnvLayout: no saved layout for env", { envId });
    return false;
  }
  if (IS_DEBUG) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const rawPanelIds = Object.keys((envLayout as any).panels ?? {});
    debug("tryRestoreEnvLayout: loaded saved layout", {
      envId,
      rawPanelCount: rawPanelIds.length,
      rawPanelIds,
      phantomSessionIds: phantomSessionIds ? Array.from(phantomSessionIds) : [],
    });
  }
  const sanitized = sanitizeLayout(envLayout, validComponents, {
    excludeSessionIds: phantomSessionIds,
  });
  if (!sanitized) {
    debug("tryRestoreEnvLayout: sanitize returned null", { envId });
    return false;
  }
  if (IS_DEBUG) {
    debug("tryRestoreEnvLayout: calling api.fromJSON", {
      envId,
      sanitizedPanelIds: Object.keys(sanitized.panels),
    });
  }
  api.fromJSON(sanitized as SerializedDockview);
  applyFixupsWithMaximize(api, envId, savedRightColumnWidth(sanitized as SerializedDockview));
  return true;
}

export function tryRestoreLayout(
  api: DockviewReadyEvent["api"],
  currentEnvId: string | null,
  validComponents: Set<string>,
  phantomSessionIds?: Set<string>,
): boolean {
  // No env yet — the task is still preparing or its session→env mapping hasn't
  // hydrated. Return false so `onReady` builds the DEFAULT layout instead of
  // restoring a cross-env "last layout": the global layout key is shared across
  // tasks, so restoring it here would flash the *previous* task's proportions
  // while a fresh task prepares. The env's own saved layout is applied later by
  // `switchEnvLayout` once the env hydrates.
  if (!currentEnvId) return false;
  try {
    if (tryRestoreEnvLayout(api, currentEnvId, validComponents, phantomSessionIds)) return true;
  } catch {
    // fall through to maximize-only
  }
  return tryRestoreMaximizeOnly(api, currentEnvId);
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
  if (IS_DEBUG) {
    debug("restoreEnvLayout: entry", {
      envId,
      phantomCount: phantoms?.size ?? 0,
      phantomSessionIds: phantoms ? Array.from(phantoms) : [],
      livePanelIdsBefore: api.panels.map((p) => p.id),
    });
  }
  const result = tryRestoreLayout(api, envId, validComponents, phantoms);
  if (IS_DEBUG) {
    debug("restoreEnvLayout: result", {
      envId,
      restored: result,
      livePanelIdsAfter: api.panels.map((p) => p.id),
    });
  }
  return result;
}
