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
import type { ActiveSessionInfo } from "@/lib/types/agent-profile-errors";

type AgentProfileDeleteDialogProps = {
  activeSessions: ActiveSessionInfo[] | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function AgentProfileDeleteDialog({
  activeSessions,
  onOpenChange,
  onConfirm,
}: AgentProfileDeleteDialogProps) {
  const tasks = activeSessions?.filter((s) => !s.is_ephemeral) ?? [];
  const quickChats = activeSessions?.filter((s) => s.is_ephemeral) ?? [];

  return (
    <AlertDialog open={!!activeSessions} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete agent profile?</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>This profile is currently in use. Deleting it will affect the following:</p>
              {tasks.length > 0 && (
                <div className="mt-2">
                  <p className="font-medium text-sm">Tasks:</p>
                  <ul className="list-disc list-inside mt-1 space-y-0.5">
                    {tasks.map((t) => (
                      <li key={t.task_id} className="text-sm">
                        {t.task_title || "Untitled task"}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {quickChats.length > 0 && (
                <div className="mt-2">
                  <p className="font-medium text-sm">Quick Chats:</p>
                  <ul className="list-disc list-inside mt-1 space-y-0.5">
                    {quickChats.map((t) => (
                      <li key={t.task_id} className="text-sm">
                        {t.task_title || "Untitled quick chat"}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              <p className="mt-2">
                These sessions will no longer be able to use this profile. This action cannot be
                undone.
              </p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Delete Anyway
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
