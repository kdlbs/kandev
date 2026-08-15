"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useJiraEnabled } from "@/hooks/domains/jira/use-jira-enabled";
import type { IntegrationEnabledControlProps } from "@/components/integrations/integration-enabled-control-props";

/**
 * Enable/disable slider for the Jira integration in `workspaceId`, wired to
 * `useJiraEnabled`.
 */
export function JiraEnabledControl({ workspaceId }: IntegrationEnabledControlProps) {
  const { enabled, setEnabled } = useJiraEnabled(workspaceId);
  return (
    <DraftedIntegrationEnabledControl
      id="jira"
      name="Jira"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
