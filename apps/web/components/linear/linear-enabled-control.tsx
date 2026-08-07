"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useLinearEnabled } from "@/hooks/domains/linear/use-linear-enabled";

/** Enable/disable slider for the Linear integration, wired to `useLinearEnabled`. */
export function LinearEnabledControl() {
  const { enabled, setEnabled } = useLinearEnabled();
  return (
    <DraftedIntegrationEnabledControl
      id="linear"
      name="Linear"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
