"use client";

import { useEffect, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

/**
 * Renders children into the #office-topbar-slot element defined by OfficeTopbar.
 * Used by detail pages (task, agent) to inject breadcrumbs into the topbar.
 *
 * The target lookup runs in useEffect (not useState initializer) so it sees
 * the slot even when the topbar and this portal commit to the DOM in the same
 * render pass — e.g. detail pages that hydrate with SSR data on first render.
 */
export function OfficeTopbarPortal({ children }: { children: ReactNode }) {
  const [mounted, setMounted] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect -- post-mount DOM discovery
  useEffect(() => setMounted(true), []);

  if (!mounted) return null;
  const target = document.getElementById("office-topbar-slot");
  if (!target) return null;
  return createPortal(children, target);
}
