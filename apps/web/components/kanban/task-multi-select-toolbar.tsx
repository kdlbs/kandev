"use client";

import { useCallback, useMemo, useState } from "react";
import { IconTrash, IconArchive, IconChevronRight, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { BulkTaskColorPicker } from "@/components/task/bulk-task-color-picker";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import type { WorkflowStep } from "@/components/kanban-column";
import { useTranslation } from "react-i18next";
import type { BulkTaskActionSelection } from "@/hooks/use-task-multi-select";

type BulkTaskActionOptions = {
  cascade?: boolean;
  discardWorktreeChanges?: boolean;
};

interface TaskMultiSelectToolbarProps {
  selectedIds: Set<string>;
  steps: WorkflowStep[];
  isProcessing: boolean;
  canMove?: boolean;
  onClearSelection: () => void;
  getEligibleSelectedIds: (ids: string[]) => string[];
  onBulkDelete: (
    opts?: BulkTaskActionOptions,
    selection?: BulkTaskActionSelection,
  ) => Promise<void>;
  onBulkArchive: (
    opts?: BulkTaskActionOptions,
    selection?: BulkTaskActionSelection,
  ) => Promise<void>;
  onBulkMove: (targetStepId: string) => Promise<void>;
}

function useBulkExecutorTypes(taskIds: string[]): Array<string | null | undefined> {
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const fallbackTasks = useAppStore((state) => state.kanban.tasks);
  return useMemo(
    () =>
      taskIds.map(
        (id) => findTaskInSnapshots(id, snapshots, fallbackTasks)?.primaryExecutorType ?? null,
      ),
    [taskIds, snapshots, fallbackTasks],
  );
}

function captureBulkSelection(
  taskIds: string[],
  getEligibleSelectedIds: (ids: string[]) => string[],
): BulkTaskActionSelection | null {
  const allIds = [...taskIds];
  const eligibleIds = getEligibleSelectedIds(allIds);
  return eligibleIds.length > 0 ? { allIds, eligibleIds } : null;
}

function useBulkConfirmationSelection(
  taskIds: string[],
  getEligibleSelectedIds: (ids: string[]) => string[],
) {
  const [open, setOpen] = useState(false);
  const [selection, setSelection] = useState<BulkTaskActionSelection | null>(null);
  const openConfirmation = useCallback(() => {
    const nextSelection = captureBulkSelection(taskIds, getEligibleSelectedIds);
    if (!nextSelection) return;
    setSelection(nextSelection);
    setOpen(true);
  }, [getEligibleSelectedIds, taskIds]);
  const handleOpenChange = useCallback((next: boolean) => {
    setOpen(next);
    if (!next) setSelection(null);
  }, []);
  return { open, selection, openConfirmation, handleOpenChange };
}

function BulkArchiveDialog({
  count,
  taskIds,
  getEligibleSelectedIds,
  isProcessing,
  onConfirm,
}: {
  count: number;
  taskIds: string[];
  getEligibleSelectedIds: (ids: string[]) => string[];
  isProcessing: boolean;
  onConfirm: (opts: { cascade: boolean }, selection: BulkTaskActionSelection) => void;
}) {
  const { t } = useTranslation();
  const { open, selection, openConfirmation, handleOpenChange } = useBulkConfirmationSelection(
    taskIds,
    getEligibleSelectedIds,
  );
  const eligibleTaskIds = selection?.eligibleIds ?? [];
  const executorTypes = useBulkExecutorTypes(eligibleTaskIds);

  return (
    <>
      <Button
        size="default"
        variant="outline"
        className="cursor-pointer gap-1.5"
        disabled={isProcessing}
        onClick={openConfirmation}
        data-testid="bulk-archive-button"
      >
        <IconArchive className="h-4 w-4" />
        {t("kanban:archiveCount", { count })}
      </Button>
      <TaskArchiveConfirmDialog
        open={open}
        onOpenChange={handleOpenChange}
        isBulkOperation
        count={eligibleTaskIds.length}
        taskIds={eligibleTaskIds}
        executorTypes={executorTypes}
        isArchiving={isProcessing}
        onConfirm={(opts) => {
          if (selection) onConfirm(opts, selection);
        }}
        confirmTestId="bulk-archive-confirm"
      />
    </>
  );
}

function BulkDeleteDialog({
  count,
  taskIds,
  getEligibleSelectedIds,
  isProcessing,
  onConfirm,
}: {
  count: number;
  taskIds: string[];
  getEligibleSelectedIds: (ids: string[]) => string[];
  isProcessing: boolean;
  onConfirm: (
    opts: { cascade: boolean; discardWorktreeChanges: boolean },
    selection: BulkTaskActionSelection,
  ) => void;
}) {
  const { t } = useTranslation();
  const { open, selection, openConfirmation, handleOpenChange } = useBulkConfirmationSelection(
    taskIds,
    getEligibleSelectedIds,
  );
  const eligibleTaskIds = selection?.eligibleIds ?? [];
  const executorTypes = useBulkExecutorTypes(eligibleTaskIds);

  return (
    <>
      <Button
        size="default"
        variant="destructive"
        className="cursor-pointer gap-1.5"
        disabled={isProcessing}
        onClick={openConfirmation}
        data-testid="bulk-delete-button"
      >
        <IconTrash className="h-4 w-4" />
        {t("kanban:deleteCount", { count })}
      </Button>
      <TaskDeleteConfirmDialog
        open={open}
        onOpenChange={handleOpenChange}
        isBulkOperation
        count={eligibleTaskIds.length}
        taskIds={eligibleTaskIds}
        executorTypes={executorTypes}
        isDeleting={isProcessing}
        onConfirm={(opts) => {
          if (selection) onConfirm(opts, selection);
        }}
        confirmTestId="bulk-delete-confirm"
      />
    </>
  );
}

export function TaskMultiSelectToolbar({
  selectedIds,
  steps,
  isProcessing,
  canMove = true,
  getEligibleSelectedIds,
  onClearSelection,
  onBulkDelete,
  onBulkArchive,
  onBulkMove,
}: TaskMultiSelectToolbarProps) {
  const { t } = useTranslation();
  const taskIds = useMemo(() => [...selectedIds], [selectedIds]);
  const { isMobile } = useResponsiveBreakpoint();
  const [actionsOpen, setActionsOpen] = useState(false);

  if (selectedIds.size === 0) return null;

  const count = selectedIds.size;

  return (
    <div
      className={cn(
        "fixed bottom-[calc(1.5rem+var(--app-status-bar-height)+env(safe-area-inset-bottom))] left-1/2 z-50 -translate-x-1/2",
        isMobile && "w-[calc(100%_-_1rem)] max-w-sm",
        "flex items-center gap-2 px-4 py-2 rounded-xl shadow-lg border border-border",
        "bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/75",
      )}
      data-testid="multi-select-toolbar"
    >
      <span className="min-w-0 text-sm font-medium text-muted-foreground mr-1">
        {t("kanban:bulkSelected", { count })}
      </span>
      <BulkTaskColorPicker taskIds={taskIds} />
      <BulkActionsSurface isMobile={isMobile} open={actionsOpen} onOpenChange={setActionsOpen}>
        {steps.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                size="default"
                variant="outline"
                className="cursor-pointer gap-1.5"
                disabled={isProcessing || !canMove}
                title={!canMove ? t("kanban:cannotMoveTasksFromDifferentWorkflows") : undefined}
                data-testid="bulk-move-button"
              >
                {t("kanban:moveTo")}
                <IconChevronRight className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="center" side="top">
              {steps.map((step) => (
                <DropdownMenuItem
                  key={step.id}
                  className="cursor-pointer"
                  onClick={() => onBulkMove(step.id)}
                  data-testid={`bulk-move-step-${step.id}`}
                >
                  <div className={cn("w-2 h-2 rounded-full mr-2 shrink-0", step.color)} />
                  {step.title}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}

        <BulkArchiveDialog
          count={count}
          taskIds={taskIds}
          getEligibleSelectedIds={getEligibleSelectedIds}
          isProcessing={isProcessing}
          onConfirm={({ cascade }, selection) => onBulkArchive({ cascade }, selection)}
        />

        <BulkDeleteDialog
          count={count}
          taskIds={taskIds}
          getEligibleSelectedIds={getEligibleSelectedIds}
          isProcessing={isProcessing}
          onConfirm={({ cascade, discardWorktreeChanges }, selection) =>
            onBulkDelete({ cascade, discardWorktreeChanges }, selection)
          }
        />
      </BulkActionsSurface>
      <Button
        size="icon"
        variant="ghost"
        className="cursor-pointer ml-1"
        onClick={onClearSelection}
        disabled={isProcessing}
        aria-label={t("kanban:clearSelection")}
        data-testid="bulk-clear-selection"
      >
        <IconX className="h-4 w-4" />
      </Button>
    </div>
  );
}

function BulkActionsSurface({
  isMobile,
  open,
  onOpenChange,
  children,
}: {
  isMobile: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: React.ReactNode;
}) {
  const { t } = useTranslation();
  if (!isMobile) return <>{children}</>;
  return (
    <>
      <Button
        variant="outline"
        onClick={() => onOpenChange(true)}
        data-testid="bulk-actions-button"
      >
        {t("kanban:bulkActions")}
      </Button>
      <MobilePickerSheet
        open={open}
        onOpenChange={onOpenChange}
        title={t("kanban:bulkActions")}
        confirmationHost
      >
        <div className="flex flex-col gap-2">{children}</div>
      </MobilePickerSheet>
    </>
  );
}
