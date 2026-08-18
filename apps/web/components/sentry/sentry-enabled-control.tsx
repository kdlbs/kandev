"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useSentryEnabled } from "@/hooks/domains/sentry/use-sentry-enabled";
import type { IntegrationEnabledControlProps } from "@/components/integrations/integration-enabled-control-props";

/**
 * Enable/disable slider for the Sentry integration in `workspaceId`, wired to
 * `useSentryEnabled`.
 */
export function SentryEnabledControl({ workspaceId }: IntegrationEnabledControlProps) {
  const { enabled, setEnabled } = useSentryEnabled(workspaceId);
  return (
    <DraftedIntegrationEnabledControl
      id="sentry"
      name="Sentry"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
