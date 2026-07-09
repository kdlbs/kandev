"use client";

import { useCallback } from "react";
import { IconLink } from "@tabler/icons-react";
import { toast } from "sonner";
import { getSentryIssue } from "@/lib/api/domains/sentry-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { SENTRY_SHORT_ID_RE } from "./sentry-issue-common";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";
import { ValidatedPopover } from "@/components/integrations/validated-popover";

type SentryLinkButtonProps = {
  taskId: string | null | undefined;
  workspaceId: string | null | undefined;
  taskTitle: string | undefined | null;
};

// SentryLinkButton attaches a Sentry issue to an existing task by prepending
// its short ID to the title ("PROJ-123: ...").
export function SentryLinkButton({ taskId, workspaceId, taskTitle }: SentryLinkButtonProps) {
  const available = useSentryAvailable(workspaceId);

  const buildLinkedTitle = useCallback(
    (key: string) => {
      const stripped = (taskTitle ?? "").trim().replace(/^[A-Z][A-Z0-9_-]*-\d+:\s*/, "");
      return stripped ? `${key}: ${stripped}` : key;
    },
    [taskTitle],
  );

  // Require taskTitle to be loaded (== null catches null + undefined but allows
  // an empty string): otherwise linking would overwrite the real title with
  // just the Sentry key while the title is still in flight.
  if (!available || !taskId || !workspaceId || taskTitle == null) return null;

  return (
    <ValidatedPopover
      triggerStyle="outline-with-label"
      triggerIcon={<IconLink className="h-4 w-4" />}
      triggerLabel="Link Sentry"
      tooltip="Link this task to a Sentry issue"
      headline="Link to Sentry issue"
      placeholder="PROJ-123 or paste issue URL"
      extractKey={(raw) => {
        const upper = raw.toUpperCase().trim();
        return SENTRY_SHORT_ID_RE.test(upper)
          ? upper
          : (upper.match(/[A-Z][A-Z0-9_-]*-\d+/)?.[0] ?? null);
      }}
      validationHint="Paste a Sentry issue URL or short ID (PROJ-123)"
      fetch={async (key) => {
        const issue = await getSentryIssue(key, { workspaceId });
        await updateTask(taskId, { title: buildLinkedTitle(key) });
        return issue;
      }}
      onSuccess={(key) => {
        toast.success(`Linked to ${key}`);
      }}
      submitLabel="Link"
      submittingLabel="Linking..."
    />
  );
}
