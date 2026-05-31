"use client";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";

/**
 * Shared "Close terminal?" confirmation. Rendered before a destroy-on-close
 * when the terminal looks busy (a command is running) or is a script terminal.
 * Used by the dockview tab, the right-panel strip, and the mobile picker so
 * all three close paths warn consistently.
 */
export function CloseTerminalConfirmDialog({
  open,
  terminalName,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  terminalName: string;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Close terminal?</AlertDialogTitle>
          <AlertDialogDescription>
            {`This stops the “${terminalName}” shell and any command it's running.`}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={(event) => {
              event.preventDefault();
              void onConfirm();
            }}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Close terminal
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
