"use client";

import { useCallback } from "react";
import { IconLink } from "@tabler/icons-react";
import { toast } from "sonner";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { LINEAR_KEY_RE } from "./linear-issue-common";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { ValidatedPopover } from "@/components/integrations/validated-popover";

type LinearLinkButtonProps = {
  taskId: string | null | undefined;
  workspaceId: string | null | undefined;
  taskTitle: string | undefined | null;
};

// LinearLinkButton attaches a Linear issue to an existing task by prepending
// the identifier to its title ("ENG-123: ...").
export function LinearLinkButton({ taskId, workspaceId, taskTitle }: LinearLinkButtonProps) {
  const available = useLinearAvailable();

  const buildLinkedTitle = useCallback(
    (key: string) => {
      const stripped = (taskTitle ?? "").trim().replace(/^[A-Z][A-Z0-9]*-\d+:\s*/, "");
      return stripped ? `${key}: ${stripped}` : key;
    },
    [taskTitle],
  );

  if (!available || !taskId || !workspaceId) return null;

  return (
    <ValidatedPopover
      triggerStyle="outline-with-label"
      triggerIcon={<IconLink className="h-4 w-4" />}
      triggerLabel="Link Linear"
      tooltip="Link this task to a Linear issue"
      headline="Link to Linear issue"
      placeholder="ENG-123 or paste issue URL"
      extractKey={(raw) => raw.toUpperCase().match(LINEAR_KEY_RE)?.[0] ?? null}
      validationHint="Paste a Linear issue URL or identifier (ENG-123)"
      // Run getLinearIssue *and* the title update inside the awaited fetch so
      // a failure on either surfaces as the popover's inline error instead of
      // an unhandled rejection.
      fetch={async (key) => {
        const issue = await getLinearIssue(key);
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
