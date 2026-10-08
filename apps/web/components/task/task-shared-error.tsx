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
import {
  useTaskLaunchErrorContext,
  type TaskLaunchErrorContextValue,
} from "./task-launch-error-context";
import { TaskLaunchErrorEntry } from "./simple/components/task-launch-error-entry";

/** Task launch errors retain their task-wide presentation. Executor incidents
 * use the session composer recovery surface. */
function useSharedFailurePresentation() {
  const context = useTaskLaunchErrorContext();
  const error = statusSummaryTaskError(context?.statusSummary);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const { isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const preview = error?.preview ?? "";
  const attention = t("task:launchNeedsAttention");
  const errorStamp = error?.stamp ?? "";
  const announcement = useFailureAnnouncement(context, errorStamp, preview, attention);

  if (!context || !error) return null;

  return {
    context,
    error,
    detailsOpen,
    setDetailsOpen,
    isMobile,
    preview,
    attention,
    announcement,
  };
}

export function TaskSharedError({
  reserveMobileTopBar = false,
}: { reserveMobileTopBar?: boolean } = {}) {
  const presentation = useSharedFailurePresentation();
  const { t } = useTranslation();
  if (!presentation) return null;
  const { detailsOpen, setDetailsOpen, isMobile, preview, attention, announcement } = presentation;
  const details = <SharedFailureDetails presentation={presentation} />;

  return (
    <>
      <section
        className={cn(
          "flex min-w-0 shrink-0 items-start gap-3 border-b border-destructive/25 bg-destructive/5 px-4 py-2.5",
          isMobile && reserveMobileTopBar && "mt-[calc(3.5rem+1px+env(safe-area-inset-top,0px))]",
        )}
        data-testid="task-shared-error"
      >
        <IconAlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-destructive"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium text-destructive">{preview}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">{attention}</p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="min-h-11 shrink-0 cursor-pointer sm:min-h-7"
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
              <DrawerTitle>{preview}</DrawerTitle>
              <DrawerDescription>{attention}</DrawerDescription>
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
              <DialogTitle>{preview}</DialogTitle>
              <DialogDescription>{attention}</DialogDescription>
            </DialogHeader>
            {details}
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}

function useFailureAnnouncement(
  context: TaskLaunchErrorContextValue | null,
  stamp: string,
  preview: string,
  attention: string,
) {
  const localStamp = useRef<string | null>(null);
  const [announcement, setAnnouncement] = useState("");
  useEffect(() => {
    if (!context || !stamp) return;
    const shouldAnnounce = context.claimTaskErrorAnnouncement
      ? context.claimTaskErrorAnnouncement(stamp)
      : localStamp.current !== stamp;
    if (!shouldAnnounce) return;
    localStamp.current = stamp;
    setAnnouncement(`${preview}. ${attention}`);
  }, [context, stamp, preview, attention]);
  return announcement;
}

function SharedFailureDetails({
  presentation,
}: {
  presentation: NonNullable<ReturnType<typeof useSharedFailurePresentation>>;
}) {
  const { error, context } = presentation;
  return (
    <div className="space-y-4">
      {error && (
        <TaskLaunchErrorEntry
          isActive
          taskId={context.taskId}
          workspaceId={context.workspaceId}
          error={error}
          repositories={context.repositories}
        />
      )}
    </div>
  );
}
