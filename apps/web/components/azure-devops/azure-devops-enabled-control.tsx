"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useAzureDevOpsEnabled } from "@/hooks/domains/azure-devops/use-azure-devops-enabled";
import type { IntegrationEnabledControlProps } from "@/components/integrations/integration-enabled-control-props";

/**
 * Enable/disable slider for the Azure DevOps integration in `workspaceId`, wired to
 * `useAzureDevOpsEnabled`.
 */
export function AzureDevOpsEnabledControl({ workspaceId }: IntegrationEnabledControlProps) {
  const { enabled, setEnabled } = useAzureDevOpsEnabled(workspaceId);
  return (
    <DraftedIntegrationEnabledControl
      id="azure-devops"
      name="Azure DevOps"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
