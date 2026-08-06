"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useSentryEnabled } from "@/hooks/domains/sentry/use-sentry-enabled";

export function SentryEnabledControl() {
  const { enabled, setEnabled } = useSentryEnabled();
  return <DraftedIntegrationEnabledControl id="sentry" enabled={enabled} persist={setEnabled} />;
}
