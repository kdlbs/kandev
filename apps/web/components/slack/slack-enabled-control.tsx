"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useSlackEnabled } from "@/hooks/domains/slack/use-slack-enabled";

export function SlackEnabledControl() {
  const { enabled, setEnabled } = useSlackEnabled();
  return <DraftedIntegrationEnabledControl id="slack" enabled={enabled} persist={setEnabled} />;
}
