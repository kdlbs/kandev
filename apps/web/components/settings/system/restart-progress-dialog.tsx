"use client";

import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import type { KandevRestartPhase } from "@/hooks/domains/system/use-kandev-restart";

type RestartProgressDialogProps = {
  phase: KandevRestartPhase;
  errorMessage: string | null;
  onDismiss: () => void;
};

export function RestartProgressDialog({
  phase,
  errorMessage,
  onDismiss,
}: RestartProgressDialogProps) {
  if (phase === "idle") return null;
  const done = phase === "done";
  const failed = phase === "error";
  return (
    <Dialog open onOpenChange={(open) => !open && (done || failed) && onDismiss()}>
      <DialogContent
        className="sm:max-w-md"
        data-testid="restart-progress-dialog"
        data-phase={phase}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <RestartStatusIcon phase={phase} />
            {restartTitle(phase)}
          </DialogTitle>
          <DialogDescription>{restartDescription(phase, errorMessage)}</DialogDescription>
        </DialogHeader>
        {(done || failed) && (
          <DialogFooter>
            <Button
              variant={failed ? "outline" : "default"}
              className="w-full cursor-pointer sm:w-auto"
              onClick={done ? () => window.location.reload() : onDismiss}
            >
              {done ? "Reload page" : "Dismiss"}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}

function RestartStatusIcon({ phase }: { phase: KandevRestartPhase }) {
  if (phase === "done") return <IconCheck className="size-4 text-emerald-500" />;
  if (phase === "error") return <IconAlertTriangle className="size-4 text-destructive" />;
  return <Spinner className="size-4" />;
}

function restartTitle(phase: KandevRestartPhase): string {
  switch (phase) {
    case "starting":
      return "Requesting restart";
    case "restarting":
      return "Restarting Kandev";
    case "done":
      return "Kandev restarted";
    case "error":
      return "Restart failed";
    default:
      return "Restarting Kandev";
  }
}

function restartDescription(phase: KandevRestartPhase, errorMessage: string | null): string {
  switch (phase) {
    case "starting":
      return "Preparing the local supervisor restart request.";
    case "restarting":
      return "Kandev is stopping and starting again. This page will detect the new process automatically.";
    case "done":
      return "The backend is running again. Reload the page to reconnect with the latest feature toggle state.";
    case "error":
      return errorMessage ?? "The restart could not be completed.";
    default:
      return "";
  }
}
