"use client";

import { useCallback } from "react";
import { IconLink } from "@tabler/icons-react";
import { toast } from "sonner";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { JIRA_KEY_RE } from "./jira-ticket-common";
import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { ValidatedPopover } from "@/components/integrations/validated-popover";

type JiraLinkButtonProps = {
  taskId: string | null | undefined;
  workspaceId: string | null | undefined;
  taskTitle: string | undefined | null;
};

// JiraLinkButton lets the user attach a Jira ticket to an existing task by
// prepending the ticket key to its title ("PROJ-123: ..."). The existing
// JiraTicketButton picks up the key automatically once the title is updated.
export function JiraLinkButton({ taskId, workspaceId, taskTitle }: JiraLinkButtonProps) {
  const available = useJiraAvailable();

  const buildLinkedTitle = useCallback(
    (key: string) => {
      // Strip an existing leading "PROJ-123: " so re-linking a task to a
      // different ticket replaces the prefix instead of stacking
      // ("PROJ-456: PROJ-123: ...").
      const stripped = (taskTitle ?? "").trim().replace(/^[A-Z][A-Z0-9]+-\d+:\s*/, "");
      return stripped ? `${key}: ${stripped}` : key;
    },
    [taskTitle],
  );

  if (!available || !taskId || !workspaceId) return null;

  return (
    <ValidatedPopover
      triggerStyle="outline-with-label"
      triggerIcon={<IconLink className="h-4 w-4" />}
      triggerLabel="Link Jira"
      tooltip="Link this task to a Jira ticket"
      headline="Link to Jira ticket"
      placeholder="PROJ-123 or paste ticket URL"
      extractKey={(raw) => raw.toUpperCase().match(JIRA_KEY_RE)?.[0] ?? null}
      validationHint="Paste a Jira ticket URL or key (PROJ-123)"
      // Run getJiraTicket *and* the title update inside the awaited fetch so
      // a failure on either surfaces as the popover's inline error instead of
      // an unhandled rejection that leaves the user thinking the link
      // succeeded.
      fetch={async (key) => {
        const ticket = await getJiraTicket(key);
        await updateTask(taskId, { title: buildLinkedTitle(key) });
        return ticket;
      }}
      onSuccess={(key) => {
        toast.success(`Linked to ${key}`);
      }}
      submitLabel="Link"
      submittingLabel="Linking..."
    />
  );
}
