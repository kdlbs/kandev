"use client";

import { useSystemHealth } from "@/hooks/domains/settings/use-system-health";
import { useUpdates } from "./use-updates";

/**
 * Number rendered next to the `System` sidebar group and the `Status` child
 * entry. Counts non-info health issues plus 1 when an update is available.
 *
 * Sources:
 *   - useSystemHealth (existing) — drives the health-issue count.
 *   - useUpdates (new)           — drives the +1 update-available bump.
 */
export function useSystemBadgeCount(): number {
  const { issues } = useSystemHealth();
  const { updates } = useUpdates();

  const nonInfoIssues = issues.filter((i) => i.severity !== "info").length;
  const updateBump = updates?.update_available ? 1 : 0;
  return nonInfoIssues + updateBump;
}
