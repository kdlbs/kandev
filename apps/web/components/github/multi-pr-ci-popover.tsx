"use client";

import { useId, useState, type KeyboardEvent } from "react";
import { IconGitPullRequest } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { PRCIPopover } from "@/components/github/pr-ci-popover";
import { getPRStatusColor, pickDefaultPR } from "@/components/github/pr-task-icon";
import { prIdentitySlug } from "@/components/github/pr-utils";
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

/**
 * DOM id for a PR tab's roving-tabindex button.
 *
 * @param uid - The popover instance's `useId()` value, so ids stay unique
 *   across multiple popovers mounted at once.
 * @param pr - The PR the tab renders; identity-scoped so two PRs never
 *   collide within one popover.
 */
function prTabDomId(uid: string, pr: TaskPR): string {
  return `${uid}-pr-tab-${prIdentitySlug(pr)}`;
}

function PRTab({
  pr,
  uid,
  panelId,
  active,
  onSelect,
}: {
  pr: TaskPR;
  uid: string;
  panelId: string;
  active: boolean;
  onSelect: (pr: TaskPR) => void;
}) {
  return (
    <button
      type="button"
      role="tab"
      id={prTabDomId(uid, pr)}
      // Roving tabindex (WAI-ARIA tabs pattern): only the active tab is in the
      // page tab order; ArrowLeft/ArrowRight move between tabs.
      tabIndex={active ? 0 : -1}
      aria-selected={active}
      aria-controls={panelId}
      data-testid={`pr-popover-tab-${prIdentitySlug(pr)}`}
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
  // `overrideId` is only set when the user activates a tab. The displayed PR is
  // derived: honour the override while it still exists, otherwise fall back to
  // the worst-status PR. This keeps the selection valid as the list changes
  // (PR closed, new PR opened, task switch) without a setState-in-effect.
  const [overrideId, setOverrideId] = useState<string | null>(null);
  // useId-scoped DOM ids so two mounted popovers (chip + topbar) can't collide.
  // The panel id is stable across tab switches so every tab's aria-controls
  // always references an element that exists in the DOM.
  const uid = useId();
  const panelId = `${uid}-pr-ci-tabpanel`;
  const selected = prs.find((p) => p.id === overrideId) ?? pickDefaultPR(prs);
  if (!selected) return null;

  // ArrowLeft/ArrowRight move selection (and focus) within the tablist,
  // wrapping at the ends — selection follows focus, per the ARIA tabs pattern.
  const onTablistKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    e.preventDefault();
    const idx = prs.findIndex((p) => p.id === selected.id);
    const delta = e.key === "ArrowRight" ? 1 : -1;
    const next = prs[(idx + delta + prs.length) % prs.length];
    setOverrideId(next.id);
    document.getElementById(prTabDomId(uid, next))?.focus();
  };

  return (
    <div data-testid="pr-multi-popover" className="flex flex-col gap-2">
      {prs
        .filter((p) => p.state === "open")
        .map((pr) => (
          <PRFeedbackWarmer key={pr.id} pr={pr} />
        ))}
      <div
        role="tablist"
        aria-label="Pull requests"
        data-testid="pr-multi-popover-tabs"
        className="flex gap-1 overflow-x-auto border-b border-border/50 pb-2"
        onKeyDown={onTablistKeyDown}
      >
        {prs.map((pr) => (
          <PRTab
            key={pr.id}
            pr={pr}
            uid={uid}
            panelId={panelId}
            active={pr.id === selected.id}
            onSelect={(p) => setOverrideId(p.id)}
          />
        ))}
      </div>
      <div role="tabpanel" id={panelId} aria-labelledby={prTabDomId(uid, selected)}>
        <PRCIPopover
          pr={selected}
          enabled={enabled}
          onOpenDetailPanel={onOpenDetailPanel ? () => onOpenDetailPanel(selected) : undefined}
        />
      </div>
    </div>
  );
}
