"use client";

import { useCallback, useState } from "react";
import { IconTicket } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import type { JiraTicket } from "@/lib/types/jira";
import { JIRA_KEY_RE } from "./jira-ticket-common";
import { useJiraAvailable } from "./my-jira/use-jira-availability";

function extractKey(input: string): string | null {
  const match = input.toUpperCase().match(JIRA_KEY_RE);
  return match ? match[0] : null;
}

type JiraImportBarProps = {
  workspaceId: string | null;
  disabled?: boolean;
  onImport: (ticket: JiraTicket) => void;
};

export function JiraImportBar({ workspaceId, disabled, onImport }: JiraImportBarProps) {
  const available = useJiraAvailable(workspaceId);
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = useCallback(async () => {
    if (!workspaceId) {
      setError("Select a workspace first");
      return;
    }
    const key = extractKey(value.trim());
    if (!key) {
      setError("Paste a Jira ticket URL or key (PROJ-123)");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const ticket = await getJiraTicket(workspaceId, key);
      onImport(ticket);
      setOpen(false);
      setValue("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [workspaceId, value, onImport]);

  if (!available) return null;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              disabled={disabled}
              aria-label="Import from Jira"
              className="h-7 w-7 cursor-pointer hover:bg-muted/40 text-slate-400"
            >
              <IconTicket className="h-4 w-4" />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>Import from Jira ticket URL or key</TooltipContent>
      </Tooltip>
      <PopoverContent align="start" className="w-80 p-3">
        <div className="space-y-2">
          <div className="text-xs font-medium">Import Jira ticket</div>
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
              {loading ? "Loading..." : "Import"}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
