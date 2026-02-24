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
  // Re-render when the set of registered panels changes.
  const ids = useSyncExternalStore(
    useCallback((cb) => panelPortalManager.subscribe(cb), []),
    useCallback(() => panelPortalManager.ids().join(","), []),
    useCallback(() => panelPortalManager.ids().join(","), []),
  );

  const panelIds = ids ? ids.split(",") : [];

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
 * @param sessionId — when provided, tags the portal as session-scoped so it
 *   gets cleaned up on session switch via `releaseBySession()`.
 */
export function usePortalSlot(
  props: IDockviewPanelProps,
  sessionId?: string,
): React.RefObject<HTMLDivElement | null> {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const panelId = props.api.id;
  const component = props.api.component;
  const params = props.params ?? {};

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const entry = panelPortalManager.acquire(panelId, component, params, props.api, sessionId);

    // Reparent the portal element into this panel's DOM slot.
    container.appendChild(entry.element);

    return () => {
      // Detach only — do NOT release.  The element stays in the manager
      // and will be re-adopted when the panel remounts after fromJSON().
      if (entry.element.parentNode === container) {
        container.removeChild(entry.element);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [panelId]);

  return containerRef;
}
