"use client";

import { useCallback, useState } from "react";
import { IconHexagon } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import type { LinearIssue } from "@/lib/types/linear";
import { LINEAR_KEY_RE } from "./linear-issue-common";
import { useLinearAvailable } from "./use-linear-availability";

function extractKey(input: string): string | null {
  const match = input.toUpperCase().match(LINEAR_KEY_RE);
  return match ? match[0] : null;
}

type LinearImportBarProps = {
  workspaceId: string | null;
  disabled?: boolean;
  onImport: (issue: LinearIssue) => void;
};

export function LinearImportBar({ workspaceId, disabled, onImport }: LinearImportBarProps) {
  const available = useLinearAvailable(workspaceId);
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
      setError("Paste a Linear issue URL or identifier (ENG-123)");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const issue = await getLinearIssue(workspaceId, key);
      onImport(issue);
      setOpen(false);
      setValue("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [workspaceId, value, onImport]);

  if (!available) return null;

  // Closing the popover discards any stale validation/error state so the
  // next open starts clean rather than rehydrating yesterday's failure.
  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) setError(null);
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              disabled={disabled}
              aria-label="Import from Linear"
              className="h-7 w-7 cursor-pointer hover:bg-muted/40 text-slate-400"
            >
              <IconHexagon className="h-4 w-4" />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>Import from Linear issue URL or identifier</TooltipContent>
      </Tooltip>
      <PopoverContent align="start" className="w-80 p-3">
        <div className="space-y-2">
          <div className="text-xs font-medium">Import Linear issue</div>
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
              {loading ? "Loading..." : "Import"}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
