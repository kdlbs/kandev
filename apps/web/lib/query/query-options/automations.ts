import { queryOptions } from "@tanstack/react-query";
import { listAutomationRuns, listAutomations } from "@/lib/api/domains/automation-api";
import { qk } from "../keys";

export function automationsQueryOptions(workspaceId: string) {
  return queryOptions({
    queryKey: qk.automations.list(workspaceId),
    queryFn: () => listAutomations(workspaceId),
    enabled: Boolean(workspaceId),
  });
}

export function automationRunsQueryOptions(automationId: string) {
  return queryOptions({
    queryKey: qk.automations.runs(automationId),
    queryFn: () => listAutomationRuns(automationId),
    enabled: Boolean(automationId),
  });
}
