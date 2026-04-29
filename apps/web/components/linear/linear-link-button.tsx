"use client";

import { useCallback, useState } from "react";
import { IconLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toast } from "sonner";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { LINEAR_KEY_RE } from "./linear-issue-common";
import { useLinearAvailable } from "./use-linear-availability";

type LinearLinkButtonProps = {
  taskId: string | null | undefined;
  workspaceId: string | null | undefined;
  taskTitle: string | undefined | null;
};

// LinearLinkButton lets the user attach a Linear issue to an existing task by
// prepending the issue identifier to its title ("ENG-123: ..."). The
// LinearIssueButton picks up the identifier automatically once the title is
// updated.
export function LinearLinkButton({ taskId, workspaceId, taskTitle }: LinearLinkButtonProps) {
  const available = useLinearAvailable(workspaceId);
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = useCallback(async () => {
    if (!taskId || !workspaceId) return;
    const match = value.toUpperCase().match(LINEAR_KEY_RE);
    if (!match) {
      setError("Paste a Linear issue URL or identifier (ENG-123)");
      return;
    }
    const key = match[0];
    setLoading(true);
    setError(null);
    try {
      await getLinearIssue(workspaceId, key);
      // Strip any existing leading "ENG-123: " so re-linking replaces the
      // prefix instead of stacking it.
      const stripped = (taskTitle ?? "").trim().replace(/^[A-Z][A-Z0-9]*-\d+:\s*/, "");
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

  // Closing the popover discards any stale validation/error so reopening
  // starts clean.
  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) setError(null);
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button size="sm" variant="outline" className="cursor-pointer px-2 gap-1">
              <IconLink className="h-4 w-4" />
              <span className="text-xs font-medium">Link Linear</span>
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>Link this task to a Linear issue</TooltipContent>
      </Tooltip>
      <PopoverContent align="end" className="w-80 p-3">
        <div className="space-y-2">
          <div className="text-xs font-medium">Link to Linear issue</div>
          <Input
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="ENG-123 or paste issue URL"
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
