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

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconLink, IconLock, IconAlertTriangle, IconCheck, IconClock } from "@tabler/icons-react";
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
import { useAppStore } from "@/components/state-provider";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { getTaskDependencies } from "@/lib/api/domains/task-dependencies-api";
import type { TaskDependencyRef } from "@/lib/state/slices/kanban/types";

type DependencyChipData = {
  blocked: boolean;
  blockedReason?: string;
  dependsOn: TaskDependencyRef[];
  blocks: TaskDependencyRef[];
};

/**
 * useTaskDependencies reads the derived dependency projection for a task from
 * the store. Returns null when the task is unknown or has no edges in either
 * direction, which is also the chip's "render nothing" signal.
 *
 * The selector returns the task itself — a stable reference — and the derived
 * shape is built in a memo afterwards. Returning a freshly-built object FROM the
 * selector compares unequal under Object.is on every store read, which loops
 * until React bails out and renders nothing at all.
 */
export function useTaskDependencies(taskId: string | null): DependencyChipData | null {
  const fromStore = useTaskDependenciesFromStore(taskId);
  const storeAnswered = fromStore !== undefined;
  const fetched = useFetchedTaskDependencies(taskId, storeAnswered);
  // `??` would be wrong here: the store's `null` is a real answer ("this task
  // has no edges"), and falling through on it would keep showing the previously
  // fetched task's dependencies after an edge is removed or after switching to
  // a task the store knows is edge-free. Only `undefined` means "ask the server".
  return storeAnswered ? fromStore : fetched;
}

/**
 * Fetches the projection when the store copy has none.
 *
 * `undefined` from the store means "this copy carries no dependency fields" (a
 * lightweight task.updated overwrote them, or landed before hydration); `null`
 * means "read it, there are no edges". Only the former needs a fetch.
 */
function useFetchedTaskDependencies(
  taskId: string | null,
  storeAnswered: boolean,
): DependencyChipData | null {
  const [data, setData] = useState<DependencyChipData | null>(null);
  useEffect(() => {
    if (!taskId || storeAnswered) {
      // Drop anything fetched for a previous task/state so it can never be
      // rendered again once the store can answer.
      setData(null);
      return;
    }
    let cancelled = false;
    void getTaskDependencies(taskId)
      .then((task) => {
        if (cancelled) return;
        const dependsOn = (task.depends_on ?? []) as TaskDependencyRef[];
        const blocks = (task.blocks ?? []) as TaskDependencyRef[];
        setData(
          dependsOn.length === 0 && blocks.length === 0
            ? null
            : {
                blocked: task.blocked ?? false,
                blockedReason: task.blocked_reason,
                dependsOn,
                blocks,
              },
        );
      })
      .catch(() => {
        // A failed read hides the chip rather than showing a wrong state.
        if (!cancelled) setData(null);
      });
    return () => {
      cancelled = true;
    };
  }, [taskId, storeAnswered]);
  return data;
}

/**
 * Store fast path. Returns `undefined` when the store copy carries no dependency
 * projection at all, so the caller knows to fetch rather than render nothing.
 */
function useTaskDependenciesFromStore(
  taskId: string | null,
): DependencyChipData | null | undefined {
  const task = useAppStore((state) => {
    if (!taskId) return undefined;
    // Nil-safe on both slices: this renders inside a task panel that can mount
    // before board hydration completes.
    const fromBoard = state.kanban?.tasks?.find((candidate) => candidate.id === taskId);
    if (fromBoard) return fromBoard;
    for (const snapshot of Object.values(state.kanbanMulti?.snapshots ?? {})) {
      const hit = snapshot.tasks?.find((candidate) => candidate.id === taskId);
      if (hit) return hit;
    }
    return undefined;
  });
  return useMemo(() => {
    if (!task) return undefined;
    // No projection on this copy: the caller must ask the server.
    if (task.dependsOn === undefined && task.blocks === undefined) return undefined;
    const dependsOn = task.dependsOn ?? [];
    const blocks = task.blocks ?? [];
    if (dependsOn.length === 0 && blocks.length === 0) return null;
    return {
      blocked: task.blocked ?? false,
      blockedReason: task.blockedReason,
      dependsOn,
      blocks,
    };
  }, [task]);
}

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
}: {
  ref: TaskDependencyRef;
  showStatus: boolean;
}) {
  const { t } = useTranslation();
  return (
    <a
      href={`/tasks/${entry.id}`}
      className="flex items-center gap-2 rounded px-2 py-1.5 text-xs hover:bg-accent min-w-0"
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
    </a>
  );
}

function DependencyLists({ data }: { data: DependencyChipData }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3">
      {data.dependsOn.length > 0 && (
        <section>
          <h4 className="px-2 pb-1 text-[10px] font-semibold uppercase text-muted-foreground">
            {t("task:dependencyBlockedBy")}
          </h4>
          {data.dependsOn.map((entry) => (
            <DependencyRow key={entry.id} ref={entry} showStatus />
          ))}
        </section>
      )}
      {data.blocks.length > 0 && (
        <section>
          <h4 className="px-2 pb-1 text-[10px] font-semibold uppercase text-muted-foreground">
            {t("task:dependencyBlocks")}
          </h4>
          {data.blocks.map((entry) => (
            <DependencyRow key={entry.id} ref={entry} showStatus={false} />
          ))}
        </section>
      )}
      {data.blockedReason === "failed" && (
        <p className="px-2 text-[11px] text-red-600 dark:text-red-400">
          {t("task:dependencyChainHalted")}
        </p>
      )}
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

export function TaskDependencyChip({ taskId }: { taskId: string | null }) {
  const data = useTaskDependencies(taskId);
  const usesMobileDrawer = useTouchDrawer();
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  // No edges in either direction: nothing to say. Same null-when-not-applicable
  // contract as PRStatusChip.
  if (!data) return null;

  const toneClass = triggerTone(data);

  if (usesMobileDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <button
          type="button"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={t("task:dependencyChipLabel")}
          onClick={() => setOpen(true)}
          className={cn(TRIGGER_BASE, toneClass)}
          data-testid="task-dependency-chip"
        >
          <ChipTriggerContent data={data} />
        </button>
        <DrawerContent data-testid="task-dependency-chip-drawer" className="max-h-[70vh]">
          <DrawerHeader className="flex flex-row items-center justify-between border-b py-2">
            <DrawerTitle className="text-sm">{t("task:dependencies")}</DrawerTitle>
            <DrawerDescription className="sr-only">
              {t("task:dependencyChipLabel")}
            </DrawerDescription>
            <DrawerClose asChild>
              <Button variant="ghost" size="sm">
                {t("common:close")}
              </Button>
            </DrawerClose>
          </DrawerHeader>
          <div className="overflow-y-auto p-2">
            <DependencyLists data={data} />
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={t("task:dependencyChipLabel")}
          className={cn(TRIGGER_BASE, toneClass)}
          data-testid="task-dependency-chip"
        >
          <ChipTriggerContent data={data} />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-2" data-testid="task-dependency-chip-popover">
        <DependencyLists data={data} />
      </PopoverContent>
    </Popover>
  );
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
