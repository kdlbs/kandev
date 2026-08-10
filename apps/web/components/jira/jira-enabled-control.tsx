"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useJiraEnabled } from "@/hooks/domains/jira/use-jira-enabled";

/** Enable/disable slider for the Jira integration, wired to `useJiraEnabled`. */
export function JiraEnabledControl() {
  const { enabled, setEnabled } = useJiraEnabled();
  return (
    <DraftedIntegrationEnabledControl
      id="jira"
      name="Jira"
      enabled={enabled}
      persist={setEnabled}
    />
  );
}
