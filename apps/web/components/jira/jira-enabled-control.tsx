"use client";

import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { useJiraEnabled } from "@/hooks/domains/jira/use-jira-enabled";

export function JiraEnabledControl() {
  const { enabled, setEnabled } = useJiraEnabled();
  return <DraftedIntegrationEnabledControl id="jira" enabled={enabled} persist={setEnabled} />;
}
