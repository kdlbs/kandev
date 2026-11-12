"use client";

/**
 * TaskDependencyChip — the status-row chip that shows what a task is waiting on
 * and what is waiting on it.
 *
 * Mounted next to `<PRStatusChip>` in the row above the composer. Both
 * directions live on the task payload already (`dependsOn` / `blocks`, each
 * entry carrying id/title/state), so opening the chip costs no fetch.
 *
 * Follows PRStatusChip's two conventions: return null when the task has no
 * edges, and use a drawer on touch rather than a hover card.
 */

import { useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import {
  IconLink,
  IconLock,
  IconAlertTriangle,
  IconCheck,
  IconClock,
  IconHierarchy3,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { cn } from "@kandev/ui/lib/utils";
import TaskLink from "@/components/routing/task-link";
import { useOpenTaskGraph } from "@/hooks/domains/task/use-open-task-graph";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  useTaskDependencies,
  type DependencyChipData,
} from "@/hooks/domains/task/use-task-dependencies";
import type { TaskDependencyRef } from "@/lib/state/slices/kanban/types";

function statusIcon(status?: string) {
  if (status === "resolved")
    return <IconCheck className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />;
  if (status === "failed")
    return <IconAlertTriangle className="h-3.5 w-3.5 text-red-600 dark:text-red-400" />;
  return <IconClock className="h-3.5 w-3.5 text-muted-foreground" />;
}

function DependencyRow({
  ref: entry,
  showStatus,
  touchTarget,
  onNavigated,
}: {
  ref: TaskDependencyRef;
  showStatus: boolean;
  touchTarget: boolean;
  onNavigated: () => void;
}) {
  const { t } = useTranslation();
  return (
    <TaskLink
      taskId={entry.id}
      onNavigated={onNavigated}
      className={cn(
        "flex min-w-0 cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-xs hover:bg-accent",
        touchTarget && "[@media(pointer:coarse)]:min-h-11",
      )}
      data-testid="task-dependency-entry"
    >
      {showStatus ? (
        statusIcon(entry.status)
      ) : (
        <IconLink className="h-3.5 w-3.5 text-muted-foreground" />
      )}
      <span className="min-w-0 flex-1 truncate">{entry.title || entry.id}</span>
      {entry.state && (
        <span className="shrink-0 text-[10px] uppercase text-muted-foreground">
          {t(`task:state.${entry.state}`, { defaultValue: entry.state })}
        </span>
      )}
    </TaskLink>
  );
}

function ViewGraphAction({ touchTarget, onClick }: { touchTarget: boolean; onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={onClick}
      data-testid="task-dependency-view-graph"
      className={cn("cursor-pointer self-end", touchTarget && "[@media(pointer:coarse)]:min-h-11")}
    >
      <IconHierarchy3 className="size-3.5" />
      {t("task:taskGraphViewGraph")}
    </Button>
  );
}

function DependencyLists({
  data,
  touchTarget,
  onNavigated,
  onViewGraph,
}: {
  data: DependencyChipData;
  touchTarget: boolean;
  onNavigated: () => void;
  onViewGraph: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3">
      {data.dependsOn.length > 0 && (
        <section>
          <h4 className="px-2 pb-1 text-[10px] font-semibold uppercase text-muted-foreground">
            {t("task:dependencyBlockedBy")}
          </h4>
          {data.dependsOn.map((entry) => (
            <DependencyRow
              key={entry.id}
              ref={entry}
              showStatus
              touchTarget={touchTarget}
              onNavigated={onNavigated}
            />
          ))}
        </section>
      )}
      {data.blocks.length > 0 && (
        <section>
          <h4 className="px-2 pb-1 text-[10px] font-semibold uppercase text-muted-foreground">
            {t("task:dependencyBlocks")}
          </h4>
          {data.blocks.map((entry) => (
            <DependencyRow
              key={entry.id}
              ref={entry}
              showStatus={false}
              touchTarget={touchTarget}
              onNavigated={onNavigated}
            />
          ))}
        </section>
      )}
      {data.related.length > 0 && (
        <section>
          <h4 className="px-2 pb-1 text-[10px] font-semibold uppercase text-muted-foreground">
            {t("task:taskDependencyRelated")}
          </h4>
          {data.related.map((entry) => (
            <DependencyRow
              key={entry.id}
              ref={entry}
              showStatus={false}
              touchTarget={touchTarget}
              onNavigated={onNavigated}
            />
          ))}
        </section>
      )}
      {data.blockedReason === "failed" && (
        <p className="px-2 text-[11px] text-red-600 dark:text-red-400">
          {t("task:dependencyChainHalted")}
        </p>
      )}
      <ViewGraphAction touchTarget={touchTarget} onClick={onViewGraph} />
    </div>
  );
}

function ChipTriggerContent({ data }: { data: DependencyChipData }) {
  const { t } = useTranslation();
  return (
    <>
      {data.blocked ? <IconLock className="h-3.5 w-3.5" /> : <IconLink className="h-3.5 w-3.5" />}
      <span>
        {data.dependsOn.length > 0 &&
          t("task:dependencyCountBlockedBy", { count: data.dependsOn.length })}
        {data.dependsOn.length > 0 && data.blocks.length > 0 && " · "}
        {data.blocks.length > 0 && t("task:dependencyCountBlocks", { count: data.blocks.length })}
        {data.dependsOn.length === 0 &&
          data.blocks.length === 0 &&
          data.related.length > 0 &&
          t("task:taskDependencyRelatedCount", { count: data.related.length })}
      </span>
    </>
  );
}

/**
 * Pill styling follows the house formula for status chips: a fully rounded
 * outline, a 10%-opacity tint of the state colour, a 35%-opacity border of the
 * same colour, and the colour itself as the text — see the workspace badge in
 * `app-sidebar/sections/settings/workspaces-group.tsx`. A solid fill with muted
 * text reads as a disabled button next to the sibling chips in this row.
 *
 * Reds use the repo's soft pair `text-red-600 dark:text-red-400` (see
 * `settings/plugins/plugin-status-badge.tsx`): a flat red-500 is harsh against a
 * dark surface.
 *
 * Colour carries the state: red when the chain has halted on a failed
 * predecessor (needs a human), primary while merely waiting or purely
 * informational. Deliberately not amber — that is the Autopilot chip's colour and
 * both can sit in this row at once.
 */
const TRIGGER_BASE =
  "inline-flex h-[26px] shrink-0 cursor-pointer items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium leading-none transition-colors";

const TRIGGER_TONES = {
  failed: "border-red-500/40 bg-red-500/10 text-red-600 dark:text-red-400 hover:bg-red-500/15",
  waiting: "border-primary/35 bg-primary/10 text-primary hover:bg-primary/15",
} as const;

function triggerTone(data: DependencyChipData): string {
  return data.blockedReason === "failed" ? TRIGGER_TONES.failed : TRIGGER_TONES.waiting;
}

type ChipSurfaceProps = {
  data: DependencyChipData;
  open: boolean;
  setOpen: (open: boolean) => void;
  triggerRef: RefObject<HTMLButtonElement | null>;
  onViewGraph: () => void;
};

function ChipDrawer({ data, open, setOpen, triggerRef, onViewGraph }: ChipSurfaceProps) {
  const { t } = useTranslation();
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={t("task:dependencyChipLabel")}
        onClick={() => setOpen(true)}
        className={cn(TRIGGER_BASE, triggerTone(data))}
        data-testid="task-dependency-chip"
      >
        <ChipTriggerContent data={data} />
      </button>
      <DrawerContent data-testid="task-dependency-chip-drawer" className="max-h-[70vh]">
        <DrawerHeader className="flex flex-row items-center justify-between border-b py-2">
          <DrawerTitle className="text-sm">{t("task:dependencies")}</DrawerTitle>
          <DrawerDescription className="sr-only">{t("task:dependencyChipLabel")}</DrawerDescription>
          <DrawerClose asChild>
            <Button variant="ghost" size="sm">
              {t("common:close")}
            </Button>
          </DrawerClose>
        </DrawerHeader>
        <div className="overflow-y-auto p-2">
          <DependencyLists
            data={data}
            touchTarget
            onNavigated={() => setOpen(false)}
            onViewGraph={onViewGraph}
          />
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function ChipPopover({ data, open, setOpen, triggerRef, onViewGraph }: ChipSurfaceProps) {
  const { t } = useTranslation();
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          aria-label={t("task:dependencyChipLabel")}
          className={cn(TRIGGER_BASE, triggerTone(data))}
          data-testid="task-dependency-chip"
        >
          <ChipTriggerContent data={data} />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-2" data-testid="task-dependency-chip-popover">
        <DependencyLists
          data={data}
          touchTarget={false}
          onNavigated={() => setOpen(false)}
          onViewGraph={onViewGraph}
        />
      </PopoverContent>
    </Popover>
  );
}

export function TaskDependencyChip({ taskId }: { taskId: string | null }) {
  const data = useTaskDependencies(taskId);
  const usesMobileDrawer = useTouchDrawer();
  const { openTab } = useOpenTaskGraph();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  // No edges in either direction: nothing to say. Same null-when-not-applicable
  // contract as PRStatusChip.
  if (!data || !taskId) return null;

  const surface: ChipSurfaceProps = {
    data,
    open,
    setOpen,
    triggerRef,
    onViewGraph: () => {
      setOpen(false);
      openTab(taskId);
    },
  };
  return usesMobileDrawer ? <ChipDrawer {...surface} /> : <ChipPopover {...surface} />;
}

/** Badge form used where a chip trigger is not appropriate (e.g. compact rows). */
export function TaskDependencyBadge({ taskId }: { taskId: string | null }) {
  const data = useTaskDependencies(taskId);
  const { t } = useTranslation();
  if (!data?.blocked) return null;
  return (
    <Badge variant="outline" className="h-5 gap-1 text-xs">
      <IconLock className="h-3 w-3" />
      {t("task:dependencyCountBlockedBy", { count: data.dependsOn.length })}
    </Badge>
  );
}
