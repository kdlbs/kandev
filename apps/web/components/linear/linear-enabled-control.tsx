"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useLinearEnabled } from "@/hooks/domains/linear/use-linear-enabled";

export function LinearEnabledControl() {
  const { enabled, setEnabled } = useLinearEnabled();
  return <DraftedIntegrationEnabledControl id="linear" enabled={enabled} persist={setEnabled} />;
}
