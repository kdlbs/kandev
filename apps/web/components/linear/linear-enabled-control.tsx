"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useLinearEnabled } from "@/hooks/domains/linear/use-linear-enabled";
import type { IntegrationEnabledControlProps } from "@/components/integrations/integration-enabled-control-props";

/**
 * Enable/disable slider for the Linear integration in `workspaceId`, wired to
 * `useLinearEnabled`.
 */
export function LinearEnabledControl({ workspaceId }: IntegrationEnabledControlProps) {
  const { enabled, setEnabled } = useLinearEnabled(workspaceId);
  return (
    <DraftedIntegrationEnabledControl
      id="linear"
      name="Linear"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
