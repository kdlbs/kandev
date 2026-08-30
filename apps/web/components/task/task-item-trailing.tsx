import { TaskMenuButton } from "./task-item-menu-button";
import { TaskItemChangeRequestStatus } from "./task-item-change-request-status";
import type { SidebarTaskRowTrailing } from "@/lib/state/slices/ui/sidebar-task-row-presentation";
import { formatRelativeTime, formatSidebarElapsedTime } from "@/lib/i18n/formats";
import { cn } from "@/lib/utils";

type DiffStats = { additions: number; deletions: number };

function hasDiffStats(diffStats?: DiffStats): diffStats is DiffStats {
  return Boolean(diffStats?.additions || diffStats?.deletions);
}

function DiffStatsRight({ diffStats, menuOpen }: { diffStats: DiffStats; menuOpen: boolean }) {
  return (
    <div
      data-testid="sidebar-task-diff-stats"
      className={cn(
        "mobile-task-diff-stats shrink-0 self-center font-mono text-[11px] transition-opacity duration-100",
        menuOpen
          ? "opacity-0"
          : "[@media(hover:hover)]:group-hover:opacity-0 group-focus-within/actions:opacity-0",
      )}
    >
      <span className="text-emerald-500">+{diffStats.additions}</span>{" "}
      <span className="text-rose-500">-{diffStats.deletions}</span>
    </div>
  );
}

export function TaskItemTrailing({
  trailing,
  diffStats,
  menuOpen,
  effectiveMenuOpen,
  relativeTime,
  taskId,
  prInfo,
}: {
  trailing: SidebarTaskRowTrailing;
  diffStats?: DiffStats;
  menuOpen: boolean;
  effectiveMenuOpen: boolean;
  relativeTime?: string;
  taskId?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
}) {
  const compactRelativeTime = formatSidebarElapsedTime(relativeTime ?? "");
  const accessibleRelativeTime = formatRelativeTime(relativeTime ?? "");

  if (trailing === "git_changes" && hasDiffStats(diffStats)) {
    return (
      <div className="mobile-task-actions-with-stats group/actions relative flex shrink-0 items-center self-center">
        <DiffStatsRight diffStats={diffStats} menuOpen={effectiveMenuOpen} />
        <div className="mobile-task-actions-slot absolute inset-0 flex items-center justify-end">
          <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} />
        </div>
      </div>
    );
  }

  if (trailing === "relative_time" && compactRelativeTime) {
    return (
      <div className="group/actions relative flex w-11 shrink-0 items-center self-center [@media(max-width:639px)]:w-auto">
        <span
          data-testid="sidebar-task-trailing-time"
          data-time-value={relativeTime}
          title={accessibleRelativeTime}
          className={cn(
            "flex w-11 shrink-0 justify-end text-right text-[11px] text-muted-foreground/50 tabular-nums transition-opacity duration-100",
            effectiveMenuOpen && "opacity-0",
            !effectiveMenuOpen &&
              "[@media(hover:hover)]:group-hover:opacity-0 group-focus-within/actions:opacity-0",
          )}
        >
          <span aria-hidden="true">{compactRelativeTime}</span>
          <span className="sr-only">{accessibleRelativeTime}</span>
        </span>
        <div className="mobile-task-actions-slot absolute inset-0 flex items-center justify-end">
          <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} />
        </div>
      </div>
    );
  }

  if (trailing === "change_request_status" && (taskId || prInfo)) {
    return (
      <div
        data-testid="sidebar-task-change-request-actions"
        className="group/actions relative flex shrink-0 items-center gap-0 self-center [@media(hover:hover)]:group-hover:gap-1 group-focus-within:gap-1 [@media(max-width:639px)]:gap-1"
      >
        <TaskItemChangeRequestStatus taskId={taskId} prInfo={prInfo} />
        <div
          data-testid="sidebar-task-change-request-menu-slot"
          className={cn(
            "mobile-task-actions-slot flex w-0 min-w-0 shrink-0 items-center justify-end overflow-x-hidden transition-[width] duration-100",
            effectiveMenuOpen && "w-6",
            "[@media(hover:hover)]:group-hover:w-6 group-focus-within:w-6",
            "[@media(max-width:639px)]:w-auto",
          )}
        >
          <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} rowFocus />
        </div>
      </div>
    );
  }

  return <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} rowFocus />;
}
