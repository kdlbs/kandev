"use client";

import { useMemo, useState, memo } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Button } from "@kandev/ui/button";
import { TaskSwitcher } from "../task-switcher";
import type { TaskSwitcherItem } from "../task-switcher";
import { applyView } from "@/lib/sidebar/apply-view";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { WorkspaceSwitcher } from "../workspace-switcher";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { TaskArchiveConfirmDialog } from "../task-archive-confirm-dialog";
import { useSheetData, useSheetActions } from "./session-task-switcher-sheet-hooks";
import type { Workspace } from "@/lib/types/http";

type SessionTaskSwitcherSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
};

function MobileTaskList({
  tasks,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onArchiveTask,
  onDeleteTask,
  deletingTaskId,
  isLoading,
}: {
  tasks: TaskSwitcherItem[];
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onArchiveTask: (taskId: string) => void;
  onDeleteTask: (taskId: string) => Promise<void> | void;
  deletingTaskId: string | null;
  isLoading?: boolean;
}) {
  const view = useEffectiveSidebarView();
  const { pinnedTaskIds, orderedTaskIds, togglePinnedTask, handleReorderGroup } =
    useSidebarTaskPrefs();
  const grouped = useMemo(
    () => applyView(tasks, view, { pinnedTaskIds, orderedTaskIds }),
    [tasks, view, pinnedTaskIds, orderedTaskIds],
  );
  return (
    <TaskSwitcher
      grouped={grouped}
      activeTaskId={activeTaskId}
      selectedTaskId={selectedTaskId}
      onSelectTask={onSelectTask}
      onArchiveTask={onArchiveTask}
      onDeleteTask={onDeleteTask}
      onTogglePin={togglePinnedTask}
      onReorderGroup={handleReorderGroup}
      pinnedTaskIds={pinnedTaskIds}
      deletingTaskId={deletingTaskId}
      isLoading={isLoading}
      totalTaskCount={tasks.length}
    />
  );
}

export const SessionTaskSwitcherSheet = memo(function SessionTaskSwitcherSheet({
  open,
  onOpenChange,
  workspaceId,
  workflowId,
}: SessionTaskSwitcherSheetProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const data = useSheetData(workspaceId, workflowId);
  const actions = useSheetActions(workspaceId, onOpenChange);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        showCloseButton={false}
        side="left"
        className="w-[85vw] max-w-sm p-0 flex flex-col"
      >
        <SheetHeader className="p-4 pb-2 border-b border-border">
          <div className="flex items-center justify-between">
            <SheetTitle className="text-base">Tasks</SheetTitle>
            <Button
              size="sm"
              variant="outline"
              className="h-7 gap-1 cursor-pointer"
              onClick={() => setDialogOpen(true)}
            >
              <IconPlus className="h-4 w-4" />
              New
            </Button>
          </div>
          <div className="pt-2">
            <WorkspaceSwitcher
              workspaces={data.workspaces.map((w: Workspace) => ({ id: w.id, name: w.name }))}
              activeWorkspaceId={workspaceId}
              onSelect={actions.handleWorkspaceChange}
            />
          </div>
        </SheetHeader>

        <div className="flex-1 min-h-0 overflow-y-auto p-2">
          <MobileTaskList
            tasks={data.tasksWithRepositories}
            activeTaskId={data.activeTaskId}
            selectedTaskId={data.selectedTaskId}
            onSelectTask={actions.handleSelectTask}
            onArchiveTask={actions.handleArchiveTask}
            onDeleteTask={actions.handleDeleteTask}
            deletingTaskId={actions.deletingTaskId}
            isLoading={data.tasksLoading}
          />
        </div>
      </SheetContent>

      <TaskCreateDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        mode="create"
        workspaceId={workspaceId}
        workflowId={workflowId}
        defaultStepId={data.dialogSteps[0]?.id ?? null}
        steps={data.dialogSteps}
        onSuccess={actions.handleTaskCreated}
      />

      <TaskArchiveConfirmDialog
        open={actions.archivingTask !== null}
        onOpenChange={(open) => {
          if (!open) actions.setArchivingTask(null);
        }}
        taskTitle={actions.archivingTask?.title ?? ""}
        isArchiving={actions.isArchiving}
        onConfirm={actions.handleArchiveConfirm}
      />
    </Sheet>
  );
});
