"use client";

import { useMemo } from "react";
import { Badge } from "@kandev/ui/badge";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { IconGitBranch } from "@tabler/icons-react";

export function EnvironmentBadges({
  executorLabel,
  worktreeBranch,
  description,
}: {
  executorLabel: string | null;
  worktreeBranch: string | null;
  description?: string;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      {executorLabel && (
        <Badge variant="secondary" className="text-xs font-normal">
          {executorLabel}
        </Badge>
      )}
      {worktreeBranch && (
        <Badge variant="outline" className="text-xs font-normal gap-1">
          <IconGitBranch className="h-3 w-3" />
          {worktreeBranch}
        </Badge>
      )}
      <span>{description ?? "Same environment as current session"}</span>
    </div>
  );
}

export type SessionOption = { id: string; label: string };

/** Unified context selector: Blank, Copy prompt, and per-session summarize options. */
export function ContextSelect({
  value,
  onValueChange,
  hasInitialPrompt,
  sessionOptions,
  isSummarizing,
}: {
  value: string;
  onValueChange: (v: string) => void;
  hasInitialPrompt: boolean;
  sessionOptions: SessionOption[];
  isSummarizing: boolean;
}) {
  const displayLabel = useMemo(() => {
    if (value === "blank") return "Blank";
    if (value === "copy_prompt") return "Copy initial prompt";
    if (value.startsWith("summarize:")) {
      const sid = value.slice("summarize:".length);
      const opt = sessionOptions.find((o) => o.id === sid);
      return opt ? `Summarize ${opt.label}` : "Summarize";
    }
    return "Blank";
  }, [value, sessionOptions]);

  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-muted-foreground">Context</label>
      <div className="flex items-center gap-2">
        <Select value={value} onValueChange={onValueChange} disabled={isSummarizing}>
          <SelectTrigger className="w-full text-xs">
            <SelectValue>{isSummarizing ? "Summarizing..." : displayLabel}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="blank" className="text-xs cursor-pointer">
              Blank
            </SelectItem>
            <SelectItem
              value="copy_prompt"
              disabled={!hasInitialPrompt}
              className="text-xs cursor-pointer"
            >
              Copy initial prompt
            </SelectItem>
            {sessionOptions.length > 0 && (
              <SelectGroup>
                <SelectLabel className="text-[11px] text-muted-foreground/70">
                  Summarize session
                </SelectLabel>
                {sessionOptions.map((opt) => (
                  <SelectItem
                    key={opt.id}
                    value={`summarize:${opt.id}`}
                    className="text-xs cursor-pointer"
                  >
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            )}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
