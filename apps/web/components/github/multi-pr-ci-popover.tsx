"use client";

import { useState } from "react";
import { IconGitPullRequest } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { PRCIPopover } from "@/components/github/pr-ci-popover";
import { getPRStatusColor, pickDefaultPR } from "@/components/github/pr-task-icon";
import { usePRFeedbackBackgroundSync } from "@/hooks/domains/github/use-pr-ci-popover";
import type { TaskPR } from "@/lib/types/github";

/**
 * Renders nothing — just keeps one PR's feedback cache warm while the popover
 * is mounted, so switching tabs shows fresh data immediately. One instance per
 * PR (keyed by id) so the hook count stays stable as the list changes.
 */
function PRFeedbackWarmer({ pr }: { pr: TaskPR }) {
  usePRFeedbackBackgroundSync(pr);
  return null;
}

function PRTab({
  pr,
  active,
  onSelect,
}: {
  pr: TaskPR;
  active: boolean;
  onSelect: (pr: TaskPR) => void;
}) {
  return (
    <button
      type="button"
      data-testid={`pr-popover-tab-${pr.pr_number}`}
      data-active={active ? "true" : "false"}
      onClick={() => onSelect(pr)}
      className={cn(
        "flex shrink-0 cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs whitespace-nowrap transition-colors",
        active ? "bg-accent text-foreground" : "text-muted-foreground hover:bg-accent/50",
      )}
    >
      <IconGitPullRequest className={cn("h-3.5 w-3.5", getPRStatusColor(pr))} />
      <span className="font-medium">
        {pr.repo} #{pr.pr_number}
      </span>
    </button>
  );
}

/**
 * Multi-PR variant of the CI popover. A segmented header lists every PR linked
 * to the task (one chip per PR, coloured by its status); the body reuses the
 * single-PR PRCIPopover for whichever PR is selected. Defaults to the
 * worst-status open PR so problems surface first.
 */
export function MultiPRCIPopover({
  prs,
  enabled,
  onOpenDetailPanel,
}: {
  prs: TaskPR[];
  enabled: boolean;
  onOpenDetailPanel?: (pr: TaskPR) => void;
}) {
  // `overrideId` is only set when the user clicks a tab. The displayed PR is
  // derived: honour the override while it still exists, otherwise fall back to
  // the worst-status PR. This keeps the selection valid as the list changes
  // (PR closed, new PR opened, task switch) without a setState-in-effect.
  const [overrideId, setOverrideId] = useState<string | null>(null);
  const selected = prs.find((p) => p.id === overrideId) ?? pickDefaultPR(prs);
  if (!selected) return null;

  return (
    <div data-testid="pr-multi-popover" className="flex flex-col gap-2">
      {prs.map((pr) => (
        <PRFeedbackWarmer key={pr.id} pr={pr} />
      ))}
      <div
        data-testid="pr-multi-popover-tabs"
        className="flex gap-1 overflow-x-auto border-b border-border/50 pb-2"
      >
        {prs.map((pr) => (
          <PRTab
            key={pr.id}
            pr={pr}
            active={pr.id === selected.id}
            onSelect={(p) => setOverrideId(p.id)}
          />
        ))}
      </div>
      <PRCIPopover
        pr={selected}
        enabled={enabled}
        onOpenDetailPanel={onOpenDetailPanel ? () => onOpenDetailPanel(selected) : undefined}
      />
    </div>
  );
}
