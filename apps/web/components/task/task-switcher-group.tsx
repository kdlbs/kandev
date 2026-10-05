import { IconChevronDown } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";
import type { TaskState } from "@/lib/types/http";
import {
  STATE_GROUP_ORDER,
  NOT_STARTED_STATE_GROUP_KEY,
} from "@/lib/sidebar/effective-task-tree-state";
import { TaskStateIcon } from "./task-state-icon";
import { getTaskStateIcon } from "@/lib/ui/state-icons";

function GroupStateIcon({ state }: { state?: TaskState }) {
  if (state === "BLOCKED" || state === "FAILED" || state === "CANCELLED") {
    return getTaskStateIcon(state, "h-3.5 w-3.5");
  }
  return <TaskStateIcon state={state} isOnLastWorkflowStep={state === "COMPLETED"} />;
}

export function TaskSwitcherSkeleton() {
  return (
    <div className="animate-pulse">
      <div className="h-10 bg-foreground/5" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
    </div>
  );
}

export function GroupHeader({
  id,
  controlsId,
  label,
  groupKey,
  grouping,
  count,
  isCollapsed,
  isContinuation = false,
  onToggle,
}: {
  id?: string;
  controlsId?: string;
  label: string;
  groupKey: string;
  grouping?: GroupKey;
  count: number;
  isCollapsed: boolean;
  isContinuation?: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      id={id}
      type="button"
      onClick={onToggle}
      data-testid="sidebar-group-header"
      data-group-key={groupKey}
      data-group-label={label}
      aria-expanded={!isCollapsed}
      aria-controls={controlsId}
      className="mx-2 flex min-h-7 w-[calc(100%-1rem)] items-center gap-2 rounded-md px-1 py-1 cursor-pointer hover:bg-foreground/[0.03] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [@media(max-width:767px)]:min-h-11 [@media(pointer:coarse)]:min-h-11"
    >
      <IconChevronDown
        aria-hidden="true"
        className={cn(
          "h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform",
          isCollapsed && "-rotate-90",
        )}
      />
      {grouping === "state" && Object.hasOwn(STATE_GROUP_ORDER, groupKey) && (
        <span data-testid="sidebar-group-state" aria-hidden="true" className="flex shrink-0">
          <GroupStateIcon
            state={groupKey === NOT_STARTED_STATE_GROUP_KEY ? undefined : (groupKey as TaskState)}
          />
        </span>
      )}
      <span className="flex-1 truncate text-left text-[12px] font-semibold text-foreground/90">
        {label}
      </span>
      {isContinuation && (
        <span className="shrink-0 text-[10px] text-muted-foreground/70">
          {t("sidebar:continuationLabel")}
        </span>
      )}
      <span className="min-w-5 rounded-full bg-muted px-1.5 text-[11px] tabular-nums text-muted-foreground">
        {count}
      </span>
    </button>
  );
}
