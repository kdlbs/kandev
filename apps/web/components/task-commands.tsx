"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "./state-provider";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { useSidebarData, useTaskRowActions } from "./task/task-session-sidebar";
import { SidebarDialogs } from "./task/task-session-sidebar-dialogs";
import { useSidebarTaskLinking } from "./task/task-session-sidebar-task-linking";
import { TaskMoveOptionsSurface, useTaskMoveOptions } from "./task/task-move-context-menu";
import { buildSidebarTaskCommands } from "./task-command-items";
import { useTaskCommandChoices } from "./task-command-choices";
import type { TaskSwitcherItem } from "./task/task-switcher-types";
import type { CommandItem } from "@/lib/commands/types";

/** One task action host, shared by desktop and phone and independent of sessions. */
export function TaskCommands() {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const data = useSidebarData(workspaceId);
  const task = data.tasksWithRepositories.find((item) => item.id === data.activeTaskId);
  if (!workspaceId || !task || data.workspaceContextAccessDenied) return null;
  return (
    <TaskCommandsForTask
      key={`${workspaceId}:${task.id}`}
      task={task}
      workspaceId={workspaceId}
      data={data}
    />
  );
}

type TaskCommandsForTaskProps = {
  task: TaskSwitcherItem;
  workspaceId: string;
  data: ReturnType<typeof useSidebarData>;
};

function TaskCommandsForTask({ task, workspaceId, data }: TaskCommandsForTaskProps) {
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const actions = useTaskRowActions(store);
  const prefs = useSidebarTaskPrefs();
  const linkHandlers = useSidebarTaskLinking(workspaceId, actions);
  const repositories = useAppStore((state) => state.repositories.itemsByWorkspaceId[workspaceId]);
  const move = useTaskMoveOptions({
    taskId: task.id,
    workflowId: task.workflowId,
    steps: data.stepsByWorkflowId[task.workflowId ?? ""],
  });
  const choices = useTaskCommandChoices({
    task,
    workflows: data.workflows,
    stepsByWorkflowId: data.stepsByWorkflowId,
    linkHandlers,
    openMoveOptions: move.openMoveOptionsStep,
    moveImmediately: (step) => {
      void move.moveImmediately(step);
    },
  });
  const commands = useMemo(() => {
    const items = buildSidebarTaskCommands({
      task,
      t,
      ...choices,
      isPinned: prefs.pinnedTaskIds.includes(task.id),
      disabled: actions.isDeleting,
      onPin: () => prefs.togglePinnedTask(task.id),
      onEdit: () => actions.handleEditTask(task),
      onRename: () => actions.handleRenameTask(task.id, task.title),
      onDetach: () => actions.handleDetachTask(task.id),
      onDelete: () => actions.handleDeleteTask(task.id),
    });
    return guardTaskCommands(items, () => {
      const state = store.getState();
      return state.tasks.activeTaskId === task.id && state.workspaces.activeId === workspaceId;
    });
  }, [task, t, choices, prefs, actions, store, workspaceId]);
  useRegisterCommands(commands);
  return (
    <>
      <SidebarDialogs
        actions={actions}
        repositories={repositories ?? []}
        workspaceId={workspaceId}
        stepsByWorkflowId={data.stepsByWorkflowId}
      />
      <TaskMoveOptionsSurface
        step={move.moveOptionsStep}
        isMoving={move.isMoving}
        onClose={move.closeMoveOptions}
        autoFocusInstructions
        onSubmit={(options) => {
          const state = store.getState();
          if (
            state.tasks.activeTaskId !== task.id ||
            state.workspaces.activeId !== workspaceId ||
            task.isArchived
          )
            return false;
          return move.submitMoveOptions(options);
        }}
      />
    </>
  );
}

function guardTaskCommands(commands: CommandItem[], isCurrent: () => boolean): CommandItem[] {
  return commands.map((command) => ({
    ...command,
    children: command.children ? guardTaskCommands(command.children, isCurrent) : undefined,
    immediateAction: command.immediateAction
      ? () => {
          if (isCurrent() && !command.disabled) command.immediateAction?.();
        }
      : undefined,
    action: command.action
      ? () => {
          if (isCurrent() && !command.disabled) command.action?.();
        }
      : undefined,
  }));
}
