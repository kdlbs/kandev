"use client";

import { useEffect } from "react";
import { usePathname } from "@/lib/routing/client-router";
import { normalizePathForTelemetry, trackPageView } from "@/lib/telemetry/track";

/**
 * Emits an allowlisted ui_page_viewed telemetry event on route changes.
 * Pathnames are reduced to route labels (IDs collapse to "id") before
 * leaving the component, and the tracker itself is consent-gated — with
 * telemetry unasked, denied, or env-disabled this renders to nothing
 * observable.
 */
export function TelemetryPageViewBridge() {
  const pathname = usePathname();

  useEffect(() => {
    trackPageView(normalizePathForTelemetry(pathname));
  }, [pathname]);

  return null;
}
