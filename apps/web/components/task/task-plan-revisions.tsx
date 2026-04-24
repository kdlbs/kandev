"use client";

import { useState, useCallback, useEffect, useRef, type ReactNode } from "react";
import { IconHistory, IconRobot, IconUser, IconRestore, IconLoader2 } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { ScrollArea } from "@kandev/ui/scroll-area";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { toast } from "sonner";
import type { TaskPlanRevision } from "@/lib/types/http";
import { formatRelativeTime } from "@/lib/utils";

type TaskPlanRevisionsProps = {
  taskId: string;
  revisions: TaskPlanRevision[];
  isLoading: boolean;
  isSaving: boolean;
  onOpen: () => void;
  onRevert: (revisionId: string) => Promise<TaskPlanRevision | null>;
  disabled?: boolean;
};

export function TaskPlanRevisions({
  revisions,
  isLoading,
  isSaving,
  onOpen,
  onRevert,
  disabled = false,
}: TaskPlanRevisionsProps) {
  const [open, setOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<TaskPlanRevision | null>(null);

  const loadedRef = useRef(false);
  const handleOpenChange = useCallback(
    (next: boolean) => {
      setOpen(next);
      if (next && !loadedRef.current) {
        loadedRef.current = true;
        onOpen();
      }
    },
    [onOpen],
  );

  const hasRevisions = revisions.length > 0;

  return (
    <>
      <Popover open={open} onOpenChange={handleOpenChange}>
        <PopoverTrigger asChild>
          <Button
            size="icon"
            variant="ghost"
            className="h-7 w-7 cursor-pointer"
            disabled={disabled || !hasRevisions}
            data-testid="plan-rewind-button"
            title="View plan history"
          >
            <IconHistory className="h-4 w-4" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="end"
          className="w-96 p-0"
          data-testid="plan-revisions-popover"
        >
          <div className="flex items-center justify-between px-3 py-2 border-b">
            <span className="text-sm font-medium">Plan history</span>
            {isLoading && <IconLoader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
          </div>
          <ScrollArea className="max-h-96">
            <RevisionList
              revisions={revisions}
              isLoading={isLoading}
              onRevertClick={setConfirmTarget}
            />
          </ScrollArea>
        </PopoverContent>
      </Popover>

      <RevertConfirmDialog
        target={confirmTarget}
        isSaving={isSaving}
        onCancel={() => setConfirmTarget(null)}
        onConfirm={async () => {
          if (!confirmTarget) return;
          const result = await onRevert(confirmTarget.id);
          setConfirmTarget(null);
          if (result) {
            toast.success(`Plan restored to v${confirmTarget.revision_number}`);
            setOpen(false);
          }
        }}
      />
    </>
  );
}

function RevisionList({
  revisions,
  isLoading,
  onRevertClick,
}: {
  revisions: TaskPlanRevision[];
  isLoading: boolean;
  onRevertClick: (rev: TaskPlanRevision) => void;
}) {
  if (revisions.length === 0 && !isLoading) {
    return (
      <div className="px-3 py-6 text-xs text-muted-foreground text-center">
        No revisions yet. Edits will appear here.
      </div>
    );
  }
  return (
    <ul className="divide-y">
      {revisions.map((rev, i) => (
        <RevisionRow
          key={rev.id}
          revision={rev}
          isCurrent={i === 0}
          onRevertClick={onRevertClick}
        />
      ))}
    </ul>
  );
}

function RevisionRow({
  revision,
  isCurrent,
  onRevertClick,
}: {
  revision: TaskPlanRevision;
  isCurrent: boolean;
  onRevertClick: (rev: TaskPlanRevision) => void;
}) {
  const AuthorIcon = revision.author_kind === "agent" ? IconRobot : IconUser;
  const [relative, setRelative] = useState(() => formatRelativeTime(revision.updated_at));
  useEffect(() => {
    const id = setInterval(() => setRelative(formatRelativeTime(revision.updated_at)), 30_000);
    return () => clearInterval(id);
  }, [revision.updated_at]);

  return (
    <li
      className="px-3 py-2.5 flex items-start gap-3 hover:bg-accent/30"
      data-testid="plan-revision-row"
      data-revision-id={revision.id}
      data-revision-number={revision.revision_number}
    >
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-xs font-semibold">v{revision.revision_number}</span>
          <AuthorIcon className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
          <span
            className="text-xs text-foreground truncate"
            data-testid="plan-revision-author"
          >
            {revision.author_name}
          </span>
          <span
            className="text-xs text-muted-foreground"
            data-testid="plan-revision-time"
          >
            {relative}
          </span>
          {isCurrent && (
            <Badge
              variant="secondary"
              className="h-4 text-[10px] px-1.5"
              data-testid="plan-revision-current-badge"
            >
              current
            </Badge>
          )}
        </div>
        {revision.revert_of_revision_id && (
          <div
            className="text-[11px] text-muted-foreground mt-1 flex items-center gap-1"
            data-testid="plan-revision-revert-marker"
          >
            <IconRestore className="h-3 w-3" />
            restored from earlier version
          </div>
        )}
      </div>
      {!isCurrent && (
        <Button
          size="sm"
          variant="ghost"
          className="h-7 px-2 text-xs cursor-pointer"
          onClick={() => onRevertClick(revision)}
          data-testid="plan-revision-revert-button"
        >
          Revert
        </Button>
      )}
    </li>
  );
}

function RevertConfirmDialog({
  target,
  isSaving,
  onCancel,
  onConfirm,
}: {
  target: TaskPlanRevision | null;
  isSaving: boolean;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
}): ReactNode {
  const open = target !== null;
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onCancel();
      }}
    >
      <DialogContent data-testid="plan-revert-confirm-dialog">
        <DialogHeader>
          <DialogTitle>Restore to version {target?.revision_number}?</DialogTitle>
          <DialogDescription>
            This creates a new version with v{target?.revision_number}&#39;s content. Nothing is
            lost — the current version stays in history.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={onCancel}
            disabled={isSaving}
            className="cursor-pointer"
            data-testid="plan-revert-confirm-cancel"
          >
            Cancel
          </Button>
          <Button
            onClick={onConfirm}
            disabled={isSaving}
            className="cursor-pointer"
            data-testid="plan-revert-confirm-ok"
          >
            {isSaving ? "Restoring..." : "Restore"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
