"use client";

import { useCallback, useState } from "react";
import { IconLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toast } from "sonner";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { JIRA_KEY_RE } from "./jira-ticket-common";
import { useJiraAvailable } from "./my-jira/use-jira-availability";

type JiraLinkButtonProps = {
  taskId: string | null | undefined;
  workspaceId: string | null | undefined;
  taskTitle: string | undefined | null;
};

// JiraLinkButton lets the user attach a Jira ticket to an existing task by
// prepending the ticket key to its title ("PROJ-123: ..."). The existing
// JiraTicketButton picks up the key automatically once the title is updated.
export function JiraLinkButton({ taskId, workspaceId, taskTitle }: JiraLinkButtonProps) {
  const available = useJiraAvailable(workspaceId);
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = useCallback(async () => {
    if (!taskId || !workspaceId) return;
    const match = value.toUpperCase().match(JIRA_KEY_RE);
    if (!match) {
      setError("Paste a Jira ticket URL or key (PROJ-123)");
      return;
    }
    const key = match[0];
    setLoading(true);
    setError(null);
    try {
      await getJiraTicket(workspaceId, key);
      // Strip an existing leading "PROJ-123: " so re-linking a task to a
      // different ticket replaces the prefix instead of stacking
      // ("PROJ-456: PROJ-123: ...").
      const stripped = (taskTitle ?? "").trim().replace(/^[A-Z][A-Z0-9]+-\d+:\s*/, "");
      const newTitle = stripped ? `${key}: ${stripped}` : key;
      await updateTask(taskId, { title: newTitle });
      toast.success(`Linked to ${key}`);
      setOpen(false);
      setValue("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [taskId, workspaceId, value, taskTitle]);

  if (!available || !taskId || !workspaceId) return null;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button size="sm" variant="outline" className="cursor-pointer px-2 gap-1">
              <IconLink className="h-4 w-4" />
              <span className="text-xs font-medium">Link Jira</span>
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>Link this task to a Jira ticket</TooltipContent>
      </Tooltip>
      <PopoverContent align="end" className="w-80 p-3">
        <div className="space-y-2">
          <div className="text-xs font-medium">Link to Jira ticket</div>
          <Input
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="PROJ-123 or paste ticket URL"
            className="h-8 text-xs"
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void submit();
              }
            }}
          />
          {error && (
            <p className="text-[11px] text-destructive" role="alert">
              {error}
            </p>
          )}
          <div className="flex justify-end">
            <Button
              type="button"
              size="sm"
              onClick={() => void submit()}
              disabled={loading || !value.trim()}
              className="h-7 cursor-pointer"
            >
              {loading ? "Linking..." : "Link"}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
