"use client";

import { IconTicket } from "@tabler/icons-react";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import type { JiraTicket } from "@/lib/types/jira";
import { JIRA_KEY_RE } from "./jira-ticket-common";
import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { ValidatedPopover } from "@/components/integrations/validated-popover";

type JiraImportBarProps = {
  workspaceId: string | null;
  disabled?: boolean;
  onImport: (ticket: JiraTicket) => void;
};

export function JiraImportBar({ workspaceId, disabled, onImport }: JiraImportBarProps) {
  const available = useJiraAvailable(workspaceId);
  if (!available || !workspaceId) return null;

  return (
    <ValidatedPopover
      triggerStyle="ghost-icon"
      triggerIcon={<IconTicket className="h-4 w-4" />}
      triggerAriaLabel="Import from Jira"
      triggerDisabled={disabled}
      tooltip="Import from Jira ticket URL or key"
      align="start"
      headline="Import Jira ticket"
      placeholder="PROJ-123 or paste ticket URL"
      extractKey={(raw) => raw.toUpperCase().match(JIRA_KEY_RE)?.[0] ?? null}
      validationHint="Paste a Jira ticket URL or key (PROJ-123)"
      fetch={(key) => getJiraTicket(workspaceId, key)}
      onSuccess={(_key, ticket) => onImport(ticket)}
      submitLabel="Import"
      submittingLabel="Loading..."
    />
  );
}
