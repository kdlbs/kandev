"use client";

import { IconCode, IconWorld, IconX } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { formatUserHomePath } from "@/lib/utils";
import type { Repository } from "@/lib/types/http";

/**
 * A single attached repository, rendered as a chip with friendly label
 * + remove button. Falls back to the raw stored string when no
 * workspace row matches (custom URL or unimported local path).
 *
 * Shared by the project detail repos section and the create-project
 * dialog so both surfaces present repositories identically.
 */
export function RepoChip({
  value,
  workspaceRepos,
  onRemove,
}: {
  value: string;
  workspaceRepos: Repository[];
  onRemove: () => void;
}) {
  const matched = workspaceRepos.find((r) => r.local_path === value);
  const isUrl = looksLikeUrl(value);
  const label = matched?.name ?? (isUrl ? value : leafSegment(value));
  const detail = matched?.local_path ?? value;
  const displayDetail = isUrl ? value : formatUserHomePath(detail);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="inline-flex items-center gap-1.5 rounded-md border border-input bg-input/20 dark:bg-input/30 pl-2.5 pr-0.5 h-8 text-xs"
          data-testid="project-repo-chip"
          data-repository-value={value}
        >
          {isUrl ? (
            <IconWorld className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <IconCode className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          )}
          <span className="truncate max-w-[240px]">{label}</span>
          <button
            type="button"
            onClick={onRemove}
            aria-label="Remove repository"
            className="h-6 w-6 inline-flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-muted/60 cursor-pointer"
            data-testid="project-repo-chip-remove"
          >
            <IconX className="h-3 w-3" />
          </button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{displayDetail}</TooltipContent>
    </Tooltip>
  );
}

function looksLikeUrl(value: string): boolean {
  return /^(https?:\/\/|git@|ssh:\/\/|git:\/\/)/i.test(value);
}

function leafSegment(path: string): string {
  const cleaned = path.replace(/\\/g, "/").replace(/\/+$/g, "");
  const idx = cleaned.lastIndexOf("/");
  return idx >= 0 ? cleaned.slice(idx + 1) : cleaned;
}
