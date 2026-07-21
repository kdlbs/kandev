"use client";

import { useCallback } from "react";
import { getAzureDevOpsConfig } from "@/lib/api/domains/azure-devops-api";
import { useIntegrationAuthed } from "@/hooks/domains/integrations/use-integration-availability";

export function useAzureDevOpsAvailable(workspaceId?: string | null): boolean {
  const fetchConfig = useCallback(
    () => (workspaceId ? getAzureDevOpsConfig(workspaceId) : Promise.resolve(null)),
    [workspaceId],
  );
  return useIntegrationAuthed(fetchConfig, undefined, !!workspaceId);
}
