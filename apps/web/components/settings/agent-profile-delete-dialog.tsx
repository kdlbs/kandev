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
import type { ActiveSessionInfo, WatcherReference } from "@/lib/types/agent-profile-errors";

const WATCHER_KIND_LABELS: Record<WatcherReference["kind"], string> = {
  linear: "Linear",
  jira: "Jira",
  github_issue: "GitHub Issues",
  github_review: "GitHub PR Reviews",
};

type AgentProfileDeleteConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function AgentProfileDeleteConfirmDialog({
  open,
  onOpenChange,
  onConfirm,
}: AgentProfileDeleteConfirmDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete agent profile?</AlertDialogTitle>
          <AlertDialogDescription>
            This will permanently delete this profile. This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// AgentProfileDeleteConflict carries the structured 409 payload from the
// backend. `open` is separate from the lists so a watcher-only conflict
// (no active sessions) still pops the dialog.
export type AgentProfileDeleteConflict = {
  activeSessions: ActiveSessionInfo[];
  watchers: WatcherReference[];
};

type AgentProfileDeleteConflictDialogProps = {
  conflict: AgentProfileDeleteConflict | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function AgentProfileDeleteConflictDialog({
  conflict,
  onOpenChange,
  onConfirm,
}: AgentProfileDeleteConflictDialogProps) {
  const tasks = conflict?.activeSessions.filter((s) => !s.is_ephemeral) ?? [];
  const quickChats = conflict?.activeSessions.filter((s) => s.is_ephemeral) ?? [];
  const watchers = conflict?.watchers ?? [];
  const watchersByKind = groupWatchersByKind(watchers);

  return (
    <AlertDialog open={!!conflict} onOpenChange={onOpenChange}>
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
              {watchers.length > 0 && (
                <div className="mt-2">
                  <p className="font-medium text-sm">Watchers (will be disabled):</p>
                  <ul className="list-disc list-inside mt-1 space-y-0.5">
                    {Object.entries(watchersByKind).map(([kind, items]) => (
                      <li key={kind} className="text-sm">
                        <span className="font-medium">
                          {WATCHER_KIND_LABELS[kind as WatcherReference["kind"]] ?? kind}:
                        </span>{" "}
                        {items.map((w) => w.label || w.id).join(", ")}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              <p className="mt-2">
                These sessions will no longer be able to use this profile and the listed watchers
                will be disabled. This action cannot be undone.
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

function groupWatchersByKind(watchers: WatcherReference[]): Record<string, WatcherReference[]> {
  return watchers.reduce<Record<string, WatcherReference[]>>((acc, w) => {
    (acc[w.kind] ??= []).push(w);
    return acc;
  }, {});
}
