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
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import type { TaskManagementClaimSnapshot } from "@/lib/api/domains/task-management-claims-api";
import {
  TaskManagementClaimActions,
  TaskManagementClaimDetails,
} from "./task-management-claim-details";

type TaskManagementClaimSurfaceProps = {
  taskId: string;
  taskUpdatedAt: string;
  isMobile: boolean;
  open: boolean;
  snapshot: TaskManagementClaimSnapshot | null;
  error: string | null;
  onOpenChange: (open: boolean) => void;
  onRefresh: () => Promise<void>;
  onError: (message: string | null) => void;
};

export function TaskManagementClaimSurface(props: TaskManagementClaimSurfaceProps) {
  const { t } = useTranslation();
  const title = t("task:managementClaimTitle");
  const description = t("task:managementClaimDescription");
  const details = (
    <TaskManagementClaimDetails
      snapshot={props.snapshot}
      error={props.error}
      onRefresh={props.onRefresh}
    />
  );
  const actions = (
    <TaskManagementClaimActions
      taskId={props.taskId}
      taskUpdatedAt={props.taskUpdatedAt}
      snapshot={props.snapshot}
      error={props.error}
      onRefresh={props.onRefresh}
      onError={props.onError}
    />
  );

  if (props.isMobile) {
    return (
      <Drawer open={props.open} onOpenChange={props.onOpenChange}>
        <DrawerContent
          className="flex max-h-[90dvh] flex-col overflow-hidden"
          data-testid="task-management-claim-surface"
        >
          <DrawerHeader className="shrink-0 border-b px-4 py-3 text-left">
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
          {details}
          <DrawerFooter className="shrink-0 border-t p-0 pb-[max(1rem,env(safe-area-inset-bottom))]">
            {actions}
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        className="flex max-h-[85dvh] flex-col gap-0 overflow-hidden p-0"
        data-testid="task-management-claim-surface"
      >
        <DialogHeader className="shrink-0 border-b p-4">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {details}
        <div className="shrink-0 border-t">{actions}</div>
      </DialogContent>
    </Dialog>
  );
}
