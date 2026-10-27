"use client";

import type { MutableRefObject } from "react";
import {
  TaskSwitcherDialogs,
  type MobileSubtaskTarget,
} from "./session-task-switcher-sheet-dialogs";
import type { MobileTaskLinking } from "./session-task-switcher-sheet";
import { useSheetActions, useSheetData } from "./session-task-switcher-sheet-hooks";
import { useMobileTaskRename } from "./use-mobile-task-rename";
import { useSidebarTaskEdit } from "../task-session-sidebar-edit";

export function SessionTaskSwitcherDialogHost({
  presentation,
  autoFocusNewTasks,
  opener,
  dialogOpen,
  onDialogOpenChange,
  workspaceId,
  workflowId,
  data,
  actions,
  rename,
  edit,
  linking,
  subtaskTarget,
  onSubtaskTargetChange,
}: {
  presentation: "sheet" | "drawer";
  autoFocusNewTasks: boolean;
  opener: MutableRefObject<HTMLElement | null>;
  dialogOpen: boolean;
  onDialogOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
  data: ReturnType<typeof useSheetData>;
  actions: ReturnType<typeof useSheetActions>;
  rename: ReturnType<typeof useMobileTaskRename>;
  edit: ReturnType<typeof useSidebarTaskEdit>;
  linking: MobileTaskLinking;
  subtaskTarget: MobileSubtaskTarget | null;
  onSubtaskTargetChange: (target: MobileSubtaskTarget | null) => void;
}) {
  return (
    <TaskSwitcherDialogs
      focusReturnRef={presentation === "drawer" && !autoFocusNewTasks ? opener : undefined}
      dialogOpen={dialogOpen}
      onDialogOpenChange={onDialogOpenChange}
      workspaceId={workspaceId}
      workflowId={workflowId}
      data={data}
      actions={actions}
      rename={rename}
      edit={edit}
      linking={linking}
      subtaskTarget={subtaskTarget}
      onSubtaskTargetChange={onSubtaskTargetChange}
    />
  );
}
