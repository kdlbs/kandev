"use client";

import { IconTicket } from "@tabler/icons-react";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import type { JiraTicket } from "@/lib/types/jira";
import { JIRA_KEY_RE } from "./jira-ticket-common";
import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { ValidatedPopover } from "@/components/integrations/validated-popover";
import { useTranslation } from "react-i18next";

type JiraImportBarProps = {
  workspaceId: string | null;
  disabled?: boolean;
  onImport: (ticket: JiraTicket) => void;
};

export function JiraImportBar({ workspaceId, disabled, onImport }: JiraImportBarProps) {
  const { t } = useTranslation();
  const available = useJiraAvailable(workspaceId);
  if (!available || !workspaceId) return null;

  return (
    <ValidatedPopover
      triggerStyle="ghost-icon"
      triggerIcon={<IconTicket className="h-4 w-4" />}
      triggerAriaLabel={t("jira:importFromJira")}
      triggerDisabled={disabled}
      testIdPrefix="jira-import"
      tooltip={t("jira:importFromJiraTicketUrlOr")}
      align="start"
      headline={t("jira:importJiraTicket")}
      placeholder={t("jira:proj123OrPasteTicketUrl")}
      extractKey={(raw) => raw.toUpperCase().match(JIRA_KEY_RE)?.[0] ?? null}
      validationHint={t("jira:pasteAJiraTicketUrlOr")}
      fetch={(key) => getJiraTicket(key, { workspaceId })}
      onSuccess={(_key, ticket) => onImport(ticket)}
      submitLabel={t("jira:import")}
      submittingLabel={t("jira:loading")}
    />
  );
}
