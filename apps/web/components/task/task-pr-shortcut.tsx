"use client";

import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { openExternalLink } from "@/lib/desktop/external-links";
import { buildTaskReviewTargets, type TaskReviewTarget } from "./task-pr-open";
import { TaskPRPickerDialog } from "./task-pr-picker-dialog";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useTaskReviewShortcut } from "./use-task-review-shortcut";

/**
 * Task-screen keybinding (default Cmd/Ctrl+Shift+G) that jumps straight to the
 * task's GitHub pull request. One linked PR opens directly; several open a
 * picker dialog; none shows a toast.
 */
export function TaskPRShortcut({ taskId }: { taskId: string | null }) {
  const { toast } = useToast();
  const { t } = useTranslation("task");
  const { prs } = useTaskPR(taskId);
  const mrs = useTaskMRs(taskId);
  const overrides = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const shortcut = getShortcut("OPEN_TASK_PR", overrides);
  const targets = useMemo(() => buildTaskReviewTargets(prs, mrs), [mrs, prs]);
  const onNoTargets = useCallback(
    () => toast({ description: t("task:noPullOrMergeRequestLinked") }),
    [t, toast],
  );
  const onOpenTarget = useCallback((target: TaskReviewTarget) => {
    void openExternalLink(target.url).catch(() => undefined);
  }, []);
  const reviewShortcut = useTaskReviewShortcut({
    targets,
    shortcut,
    onNoTargets,
    onOpenTarget,
  });

  useKeyboardShortcut(
    shortcut,
    reviewShortcut.handleShortcut,
    // Capture + stopPropagation so the binding wins over focus-trapped
    // surfaces (xterm.js, editors) — mirrors useEditorKeybinds. Disabled
    // until the task id resolves so a transient null doesn't toast.
    { capture: true, stopPropagation: true, enabled: !!taskId },
  );

  return (
    <TaskPRPickerDialog
      open={reviewShortcut.pickerOpen}
      onOpenChange={reviewShortcut.setPickerOpen}
      targets={targets}
      selectedIndex={reviewShortcut.selectedIndex}
      onSelectedIndexChange={reviewShortcut.setSelectedIndex}
      onActivateIndex={reviewShortcut.openTargetAtIndex}
    />
  );
}
