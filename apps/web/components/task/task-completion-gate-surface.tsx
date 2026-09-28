"use client";

import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import type { WorkflowStepperStep } from "@/components/task/workflow-step-disclosure";
import type {
  TaskCompletionGateHistory,
  TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";
import { TaskCompletionGateDetails } from "./task-completion-gate-details";

type TaskCompletionGateSurfaceProps = {
  taskId: string;
  taskUpdatedAt: string;
  isMobile: boolean;
  open: boolean;
  snapshot: TaskCompletionGateSnapshot | null;
  history: TaskCompletionGateHistory[];
  completionSteps: WorkflowStepperStep[];
  error: string | null;
  onOpenChange: (open: boolean) => void;
  onRefresh: () => Promise<void>;
  onOverride: (stepId: string, reason: string, revision: number) => Promise<boolean>;
};

export function TaskCompletionGateSurface(props: TaskCompletionGateSurfaceProps) {
  const { t } = useTranslation();
  const details = (
    <TaskCompletionGateDetails
      taskId={props.taskId}
      taskUpdatedAt={props.taskUpdatedAt}
      snapshot={props.snapshot}
      history={props.history}
      completionSteps={props.completionSteps}
      error={props.error}
      onRefresh={props.onRefresh}
      onOverride={props.onOverride}
    />
  );

  if (props.isMobile) {
    return (
      <Drawer open={props.open} onOpenChange={props.onOpenChange}>
        <DrawerContent
          className="flex max-h-[90dvh] flex-col overflow-hidden"
          data-testid="task-completion-gate-surface"
        >
          <DrawerHeader className="shrink-0 border-b px-4 py-3 text-left">
            <DrawerTitle>{t("task:completionGateTitle")}</DrawerTitle>
            <DrawerDescription>{t("task:completionGateDescription")}</DrawerDescription>
          </DrawerHeader>
          {details}
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        className="flex max-h-[85dvh] flex-col gap-0 overflow-hidden p-0"
        data-testid="task-completion-gate-surface"
      >
        <DialogHeader className="shrink-0 border-b p-4">
          <DialogTitle>{t("task:completionGateTitle")}</DialogTitle>
          <DialogDescription>{t("task:completionGateDescription")}</DialogDescription>
        </DialogHeader>
        {details}
      </DialogContent>
    </Dialog>
  );
}
