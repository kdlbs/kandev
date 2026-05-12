"use client";

import React, { useCallback, useEffect, useRef, useSyncExternalStore, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { panelPortalManager } from "./panel-portal-manager";
import type { IDockviewPanelProps } from "dockview-react";

// ---------------------------------------------------------------------------
// PanelPortalHost — renders all persistent panel content via React portals
// ---------------------------------------------------------------------------

/**
 * A render function that receives the panel ID and dockview-compatible props.
 * The host calls this to decide *what* to render inside each portal element.
 */
export type PortalRenderer = (
  panelId: string,
  component: string,
  params: Record<string, unknown>,
) => ReactNode;

type PanelPortalHostProps = {
  /** Render function called for each registered panel. */
  renderPanel: PortalRenderer;
};

/**
 * Renders panel content into persistent portal elements that live outside the
 * dockview tree.  Mount this as a sibling to `<DockviewReact>`.
 */
export function PanelPortalHost({ renderPanel }: PanelPortalHostProps) {
  // Re-render when panels are added/removed OR when any panel's params change.
  // The version counter bumps on all three — we read ids() at render time
  // rather than encoding them in the snapshot so panel ids can contain any
  // character (a path like `file:path/with,comma.txt` would break a delimiter).
  useSyncExternalStore(
    useCallback((cb) => panelPortalManager.subscribe(cb), []),
    useCallback(() => panelPortalManager.getVersion(), []),
    useCallback(() => panelPortalManager.getVersion(), []),
  );

  const panelIds = panelPortalManager.ids();

  return (
    <>
      {panelIds.map((panelId) => {
        const entry = panelPortalManager.get(panelId);
        if (!entry) return null;
        return createPortal(
          renderPanel(panelId, entry.component, entry.params),
          entry.element,
          panelId,
        );
      })}
    </>
  );
}

// ---------------------------------------------------------------------------
// usePortalSlot — hook for dockview panel wrappers (slot components)
// ---------------------------------------------------------------------------

/**
 * Attaches the persistent portal element for `panelId` into the dockview
 * panel's container on mount, and detaches it on unmount (without destroying).
 *
 * Also updates the stored `api` and `params` on every mount, so the portal
 * content can read the latest dockview state.
 *
 * @param envId — when provided, tags the portal as env-scoped so it
 *   gets cleaned up on env switch via `releaseByEnv()`.
 */
export function usePortalSlot(
  props: IDockviewPanelProps,
  envId?: string,
): React.RefObject<HTMLDivElement | null> {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const panelId = props.api.id;
  const component = props.api.component;
  const params = props.params ?? {};

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const entry = panelPortalManager.acquire(panelId, component, params, props.api, envId);

    // Reparent the portal element into this panel's DOM slot, then restore any
    // scroll positions captured before the previous detach. Browsers reset
    // scrollTop/scrollLeft when an element is removed from the DOM, so without
    // this the sidebar (and any other scrollable descendant of a portal) jumps
    // back to the top after every dockview layout switch.
    container.appendChild(entry.element);
    const cancelRestore = restorePortalScroll(entry.element);
    // Track every scroll inside this portal so we always have an up-to-date
    // snapshot to restore after the next reattach. We do this via a capturing
    // listener because dockview can rip the DOM apart before React runs the
    // cleanup below (removeChild fires too late to read scrollTop reliably).
    const onScroll = (e: Event) => {
      const target = e.target;
      if (target instanceof Element && entry.element.contains(target)) {
        const html = target as HTMLElement;
        scrollSnapshots.set(target, { top: html.scrollTop, left: html.scrollLeft });
      }
    };
    entry.element.addEventListener("scroll", onScroll, true);

    return () => {
      cancelRestore();
      entry.element.removeEventListener("scroll", onScroll, true);
      // Detach only — do NOT release.  The element stays in the manager
      // and will be re-adopted when the panel remounts after fromJSON().
      if (entry.element.parentNode === container) {
        container.removeChild(entry.element);
      }
    };
    // envId in deps: when env changes, env-scoped panels re-acquire fresh
    // portals (old ones were released by releaseByEnv in the store action).
    // Global panels pass envId=undefined so this is a no-op for them.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [panelId, envId]);

  // Forward dockview's param updates into the portal manager so preview-tab
  // content (file-editor, diff-viewer, commit-detail) re-renders when the
  // single preview panel switches to a different file/diff/commit.
  useEffect(() => {
    const disposable = props.api.onDidParametersChange((next) => {
      panelPortalManager.updateParams(panelId, next as Record<string, unknown>);
    });
    return () => disposable.dispose();
  }, [panelId, props.api]);

  return containerRef;
}

// ---------------------------------------------------------------------------
// Scroll preservation across portal detach/reattach
// ---------------------------------------------------------------------------

/**
 * Per-element scroll snapshots, kept up-to-date by a capturing scroll
 * listener attached to each portal element. Stored in a WeakMap so detached
 * subtrees can be garbage collected if the portal is ever released.
 */
const scrollSnapshots = new WeakMap<Element, { top: number; left: number }>();

/** Window during which we keep re-applying snapshots after attach. */
const RESTORE_WINDOW_MS = 1500;

/**
 * Restore captured scroll positions on every scrollable descendant of `portal`,
 * then keep re-applying for a short window so that a snapshot which gets
 * clamped by a not-yet-laid-out container (`scrollTop` capped to
 * `scrollHeight - clientHeight`) is corrected once layout settles.
 *
 * Returns a function that cancels any pending retries (call from cleanup so
 * we never write to a detached subtree).
 */
function restorePortalScroll(portal: Element): () => void {
  // Collect every descendant that has a snapshot up front so we don't re-walk
  // the tree on each retry.
  const targets: Array<{ el: HTMLElement; snap: { top: number; left: number } }> = [];
  const walker = document.createTreeWalker(portal, NodeFilter.SHOW_ELEMENT);
  let node: Node | null = walker.currentNode;
  while (node) {
    if (node instanceof Element) {
      const snap = scrollSnapshots.get(node);
      if (snap && (snap.top > 0 || snap.left > 0)) {
        targets.push({ el: node as HTMLElement, snap });
      }
    }
    node = walker.nextNode();
  }
  if (targets.length === 0) return () => {};

  // Track which elements still need retrying. An element is removed from
  // `pending` once the layout has grown enough to accommodate the snapshot,
  // so subsequent ResizeObserver/rAF callbacks won't fight a deliberate user
  // scroll (including scrolling upward, away from the snapshot position).
  const pending = new Set(targets.map((t) => t.el));
  let cancelled = false;
  let stopId = 0;
  let ro: ResizeObserver | null = null;

  const stop = () => {
    cancelled = true;
    ro?.disconnect();
    window.clearTimeout(stopId);
  };

  const apply = () => {
    for (const { el, snap } of targets) {
      if (!pending.has(el)) continue;
      if (el.scrollTop < snap.top) el.scrollTop = snap.top;
      if (el.scrollLeft < snap.left) el.scrollLeft = snap.left;
      // Once layout can fully accommodate the snapshot, the clamping bug is
      // resolved — drop the element from pending so future ticks stay out of
      // the way of any user-initiated scroll.
      const canReachTop = el.scrollHeight - el.clientHeight >= snap.top;
      const canReachLeft = el.scrollWidth - el.clientWidth >= snap.left;
      if (canReachTop && canReachLeft) pending.delete(el);
    }
    if (pending.size === 0) stop();
  };
  apply();

  if (!cancelled) {
    ro = new ResizeObserver(() => {
      if (cancelled) return;
      apply();
    });
    for (const { el } of targets) ro.observe(el);
    // A short rAF chain catches the common case where layout settles within
    // the next two frames without a measurable size change.
    requestAnimationFrame(() => {
      if (cancelled) return;
      apply();
      requestAnimationFrame(() => {
        if (cancelled) return;
        apply();
      });
    });
    stopId = window.setTimeout(stop, RESTORE_WINDOW_MS);
  }

  return stop;
}
