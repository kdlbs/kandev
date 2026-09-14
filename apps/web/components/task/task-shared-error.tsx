"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
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
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { statusSummaryTaskError } from "@/lib/task-status-summary";
import { cn } from "@/lib/utils";
import { useTaskLaunchErrorContext } from "./task-launch-error-context";
import { TaskLaunchErrorEntry } from "./simple/components/task-launch-error-entry";

/** Persistent task-owned failure surface. It is mounted above the task tabs so
 * session changes and Dockview panel changes cannot hide the recovery affordance. */
export function TaskSharedError({
  reserveMobileTopBar = false,
}: {
  reserveMobileTopBar?: boolean;
} = {}) {
  const context = useTaskLaunchErrorContext();
  const error = statusSummaryTaskError(context?.statusSummary);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const localAnnouncementStampRef = useRef<string | null>(null);
  const { isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const launchNeedsAttention = t("task:launchNeedsAttention");
  const errorStamp = error?.stamp ?? "";
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    if (!context || !error || !errorStamp) return;
    const shouldAnnounce = context.claimTaskErrorAnnouncement
      ? context.claimTaskErrorAnnouncement(errorStamp)
      : localAnnouncementStampRef.current !== errorStamp;
    if (!shouldAnnounce) return;
    localAnnouncementStampRef.current = errorStamp;
    setAnnouncement(`${error.preview}. ${launchNeedsAttention}`);
  }, [context, error, errorStamp, launchNeedsAttention]);

  if (!context || !error) return null;

  const details = (
    <TaskLaunchErrorEntry
      taskId={context.taskId}
      workspaceId={context.workspaceId}
      error={error}
      repositories={context.repositories}
    />
  );

  return (
    <>
      <section
        className={cn(
          "flex min-w-0 shrink-0 items-start gap-3 border-b border-destructive/25 bg-destructive/5 px-4 py-2.5",
          isMobile && reserveMobileTopBar && "mt-[calc(3.5rem+env(safe-area-inset-top,0px))]",
        )}
        data-testid="task-shared-error"
      >
        <IconAlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-destructive"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium text-destructive">{error.preview}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">{launchNeedsAttention}</p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="min-h-11 shrink-0 cursor-pointer sm:min-h-8"
          onClick={() => setDetailsOpen(true)}
          data-testid="task-shared-error-details"
        >
          {t("task:showDetails")}
        </Button>
      </section>
      <p
        aria-atomic="true"
        aria-live="assertive"
        className="sr-only"
        data-testid="task-shared-error-announcement"
      >
        {announcement}
      </p>

      {isMobile ? (
        <Drawer open={detailsOpen} onOpenChange={setDetailsOpen} direction="bottom">
          <DrawerContent
            className="overflow-hidden pb-[max(1rem,env(safe-area-inset-bottom))] data-[vaul-drawer-direction=bottom]:mb-2 data-[vaul-drawer-direction=bottom]:max-h-[calc(100dvh-1rem)]"
            data-testid="task-shared-error-drawer"
          >
            <DrawerHeader>
              <DrawerTitle>{error.preview}</DrawerTitle>
              <DrawerDescription>{launchNeedsAttention}</DrawerDescription>
            </DrawerHeader>
            <div className="min-h-0 overflow-y-auto overscroll-contain px-4 pb-4" data-vaul-no-drag>
              {details}
            </div>
          </DrawerContent>
        </Drawer>
      ) : (
        <Dialog open={detailsOpen} onOpenChange={setDetailsOpen}>
          <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>{error.preview}</DialogTitle>
              <DialogDescription>{launchNeedsAttention}</DialogDescription>
            </DialogHeader>
            {details}
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
