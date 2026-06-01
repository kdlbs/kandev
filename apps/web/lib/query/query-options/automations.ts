import { queryOptions } from "@tanstack/react-query";
import { listAutomations, listAutomationRuns } from "@/lib/api/domains/automation-api";
import { qk } from "@/lib/query/keys";

/**
 * Query options factories for the automations domain.
 *
 * Uses staleTime: 60_000 (1 min) — automations are low-frequency config data.
 * Runs use the default staleTime (30s) — they update more often.
 */
export const automationsQueryOptions = {
  /**
   * List all automations for a workspace.
   */
  list: (workspaceId: string) =>
    queryOptions({
      queryKey: qk.automations.list(workspaceId),
      queryFn: () => listAutomations(workspaceId),
      staleTime: 60_000,
      enabled: !!workspaceId,
    }),

  /**
   * List recent runs for a specific automation.
   */
  runs: (automationId: string) =>
    queryOptions({
      queryKey: qk.automations.runs(automationId),
      queryFn: () => listAutomationRuns(automationId),
      enabled: !!automationId,
    }),
};
