"use client";

import { useState } from "react";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";
import { RepoFilterSelector } from "@/components/github/repo-filter-selector";
import type { RepoFilter } from "@/lib/types/github";

type GitHubPRConfigProps = {
  config: Record<string, unknown>;
  onUpdate: (config: Record<string, unknown>) => void;
};

const PR_EVENTS = [
  {
    value: "opened",
    label: "Opened",
    tooltip:
      "Fires once when a new open PR is detected. Each PR number is deduped — the same PR won't trigger again.",
  },
] as const;

export function GitHubPRConfig({ config, onUpdate }: GitHubPRConfigProps) {
  const events = (config.events as string[]) ?? [];
  const repos = (config.repos as RepoFilter[]) ?? [];
  const allRepos = (config.all_repos as boolean) ?? true;
  const [branches, setBranches] = useState(((config.branches as string[]) ?? []).join(", "));
  const [authors, setAuthors] = useState(((config.authors as string[]) ?? []).join(", "));
  const excludeDraft = (config.exclude_draft as boolean) ?? false;

  const toggleEvent = (event: string) => {
    const next = events.includes(event) ? events.filter((e) => e !== event) : [...events, event];
    onUpdate({ ...config, events: next });
  };

  const handleBranchesBlur = () => {
    const parsed = branches
      .split(",")
      .map((b) => b.trim())
      .filter(Boolean);
    onUpdate({ ...config, branches: parsed });
  };

  const handleAuthorsBlur = () => {
    const parsed = authors
      .split(",")
      .map((a) => a.trim())
      .filter(Boolean);
    onUpdate({ ...config, authors: parsed });
  };

  return (
    <div className="space-y-3">
      <EventsSection events={events} onToggle={toggleEvent} />
      <RepoFilterSelector
        allRepos={allRepos}
        selectedRepos={repos}
        onAllReposChange={(checked) => {
          onUpdate({ ...config, all_repos: checked, repos: checked ? [] : repos });
        }}
        onSelectedReposChange={(next) => onUpdate({ ...config, repos: next })}
      />
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label className="text-xs">Base branches (comma-separated)</Label>
          <Input
            value={branches}
            onChange={(e) => setBranches(e.target.value)}
            onBlur={handleBranchesBlur}
            placeholder="main, develop"
          />
        </div>
        <div className="space-y-1.5">
          <Label className="text-xs">Authors (comma-separated)</Label>
          <Input
            value={authors}
            onChange={(e) => setAuthors(e.target.value)}
            onBlur={handleAuthorsBlur}
            placeholder="alice, bob"
          />
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Switch
          size="sm"
          checked={excludeDraft}
          onCheckedChange={(checked) => onUpdate({ ...config, exclude_draft: checked })}
          className="cursor-pointer"
        />
        <Label className="text-xs">Exclude draft PRs</Label>
      </div>
    </div>
  );
}

function EventsSection({
  events,
  onToggle,
}: {
  events: string[];
  onToggle: (event: string) => void;
}) {
  return (
    <div className="space-y-2">
      <Label className="text-xs">Events</Label>
      <div className="flex flex-wrap gap-3">
        {PR_EVENTS.map((evt) => (
          <label key={evt.value} className="flex items-center gap-1.5 cursor-pointer">
            <Checkbox
              checked={events.includes(evt.value)}
              onCheckedChange={() => onToggle(evt.value)}
              className="cursor-pointer"
            />
            <span className="text-sm">{evt.label}</span>
            <Tooltip>
              <TooltipTrigger asChild>
                <IconInfoCircle className="h-3 w-3 text-muted-foreground" />
              </TooltipTrigger>
              <TooltipContent className="max-w-[220px]">{evt.tooltip}</TooltipContent>
            </Tooltip>
          </label>
        ))}
      </div>
    </div>
  );
}
