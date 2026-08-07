"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useSentryEnabled } from "@/hooks/domains/sentry/use-sentry-enabled";

/** Enable/disable slider for the Sentry integration, wired to `useSentryEnabled`. */
export function SentryEnabledControl() {
  const { enabled, setEnabled } = useSentryEnabled();
  return (
    <DraftedIntegrationEnabledControl
      id="sentry"
      name="Sentry"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
