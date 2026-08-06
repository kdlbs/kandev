"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useAzureDevOpsEnabled } from "@/hooks/domains/azure-devops/use-azure-devops-enabled";

export function AzureDevOpsEnabledControl() {
  const { enabled, setEnabled } = useAzureDevOpsEnabled();
  return (
    <DraftedIntegrationEnabledControl id="azure-devops" enabled={enabled} persist={setEnabled} />
  );
}
