"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useAzureDevOpsEnabled } from "@/hooks/domains/azure-devops/use-azure-devops-enabled";

/** Enable/disable slider for the Azure DevOps integration, wired to `useAzureDevOpsEnabled`. */
export function AzureDevOpsEnabledControl() {
  const { enabled, setEnabled } = useAzureDevOpsEnabled();
  return (
    <DraftedIntegrationEnabledControl
      id="azure-devops"
      name="Azure DevOps"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
