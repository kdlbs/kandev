import { TaskMenuButton } from "./task-item-menu-button";
import { TaskItemChangeRequestStatus } from "./task-item-change-request-status";
import type { SidebarTaskRowTrailing } from "@/lib/state/slices/ui/sidebar-task-row-presentation";
import { cn, formatRelativeTime } from "@/lib/utils";

type DiffStats = { additions: number; deletions: number };

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
  const hasDiffStats = !!diffStats && (diffStats.additions > 0 || diffStats.deletions > 0);

  if (trailing === "git_changes" && hasDiffStats) {
    return (
      <div className="mobile-task-actions-with-stats group/actions relative flex shrink-0 items-center self-center">
        <DiffStatsRight diffStats={diffStats} menuOpen={effectiveMenuOpen} />
        <div className="mobile-task-actions-slot absolute inset-0 flex items-center justify-end">
          <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} />
        </div>
      </div>
    );
  }

  if (trailing === "relative_time" && relativeTime) {
    return (
      <div className="group/actions relative flex shrink-0 items-center self-center">
        <span
          data-testid="sidebar-task-trailing-time"
          data-time-value={relativeTime}
          className={cn(
            "shrink-0 text-[11px] text-muted-foreground/50 transition-opacity duration-100",
            effectiveMenuOpen && "opacity-0",
            !effectiveMenuOpen &&
              "[@media(hover:hover)]:group-hover:opacity-0 group-focus-within/actions:opacity-0",
          )}
        >
          {formatRelativeTime(relativeTime)}
        </span>
        <div className="mobile-task-actions-slot absolute inset-0 flex items-center justify-end">
          <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} />
        </div>
      </div>
    );
  }

  if (trailing === "change_request_status") {
    return (
      <div className="flex shrink-0 items-center gap-1 self-center">
        <TaskItemChangeRequestStatus taskId={taskId} prInfo={prInfo} />
        <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} rowFocus />
      </div>
    );
  }

  return <TaskMenuButton visible={effectiveMenuOpen} expanded={menuOpen} rowFocus />;
}
