"use client";

import { useState, useCallback, useEffect, useMemo, useRef, type ReactNode } from "react";
import { IconHistory, IconUser, IconRestore, IconLoader2 } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
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
import { formatPreciseTime } from "@/lib/utils";
import { AgentLogo } from "@/components/agent-logo";
import { useAppStore } from "@/components/state-provider";
import { PlanRevisionPreviewDialog } from "./task-plan-preview-dialog";
import { PlanRevisionDiffDialog } from "./task-plan-diff-dialog";

type ComparePair = [string | null, string | null];

type TaskPlanRevisionsProps = {
  taskId: string;
  revisions: TaskPlanRevision[];
  isLoading: boolean;
  isSaving: boolean;
  onOpen: () => void;
  onRevert: (revisionId: string) => Promise<TaskPlanRevision | null>;
  loadRevisionContent: (revisionId: string) => Promise<string>;
  previewRevisionId: string | null;
  setPreviewRevision: (revisionId: string | null) => void;
  /** comparePair / toggleCompareSelection / clearComparePair are still threaded
   * through so the diff dialog can carry an arbitrary pair, but the popover
   * UI no longer surfaces a selection mode — pairs are seeded from inside the
   * preview dialog ("Compare with previous" / "Compare with current"). */
  comparePair: ComparePair;
  toggleCompareSelection: (revisionId: string) => void;
  clearComparePair: () => void;
  disabled?: boolean;
};

/** Label rendered in the UI for any user-authored revision. Display-only —
 * the backend still stamps its own author_name on the row. */
const USER_DISPLAY_NAME = "You";

export function TaskPlanRevisions(props: TaskPlanRevisionsProps) {
  const [open, setOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<TaskPlanRevision | null>(null);
  const [diffOpen, setDiffOpen] = useState(false);
  const agentName = useActiveAgentBackendName();
  const handleOpenChange = useTriggerOnFirstOpen(setOpen, props.onOpen);

  const previewRevision = useMemo(
    () => props.revisions.find((r) => r.id === props.previewRevisionId) ?? null,
    [props.revisions, props.previewRevisionId],
  );
  const compareRevisions = useMemo<[TaskPlanRevision | null, TaskPlanRevision | null]>(
    () => [
      props.revisions.find((r) => r.id === props.comparePair[0]) ?? null,
      props.revisions.find((r) => r.id === props.comparePair[1]) ?? null,
    ],
    [props.revisions, props.comparePair],
  );
  const headRevision = props.revisions[0] ?? null;

  return (
    <>
      <RevisionsPopover
        open={open}
        onOpenChange={handleOpenChange}
        agentName={agentName}
        confirmTargetSetter={setConfirmTarget}
        {...props}
      />
      <RevisionsDialogStack
        revisions={props.revisions}
        previewRevision={previewRevision}
        compareRevisions={compareRevisions}
        diffOpen={diffOpen}
        confirmTarget={confirmTarget}
        isSaving={props.isSaving}
        loadRevisionContent={props.loadRevisionContent}
        headRevision={headRevision}
        onRevert={props.onRevert}
        setConfirmTarget={setConfirmTarget}
        setDiffOpen={setDiffOpen}
        setPreviewRevision={props.setPreviewRevision}
        clearComparePair={props.clearComparePair}
        toggleCompareSelection={props.toggleCompareSelection}
        closePopover={() => setOpen(false)}
      />
    </>
  );
}

/** Lazy-load revisions on the first popover open; subsequent opens reuse the
 * cached list. Wraps the open setter so callers don't need a ref of their own. */
function useTriggerOnFirstOpen(setOpen: (v: boolean) => void, onOpen: () => void) {
  const loadedRef = useRef(false);
  return useCallback(
    (next: boolean) => {
      setOpen(next);
      if (next && !loadedRef.current) {
        loadedRef.current = true;
        onOpen();
      }
    },
    [setOpen, onOpen],
  );
}

function RevisionsPopover({
  open,
  onOpenChange,
  agentName,
  confirmTargetSetter,
  revisions,
  isLoading,
  setPreviewRevision,
  disabled = false,
}: TaskPlanRevisionsProps & {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  agentName: string | null;
  confirmTargetSetter: (rev: TaskPlanRevision | null) => void;
}) {
  const hasRevisions = revisions.length > 0;
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
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
        // Override the popover's default `gap-4` between flex children — it
        // was rendering as visible empty space above the top-most row.
        className="w-96 p-0 gap-0"
        data-testid="plan-revisions-popover"
      >
        <div className="flex items-center justify-between px-3 py-2 border-b">
          <span className="text-sm font-medium">Plan history</span>
          {isLoading && <IconLoader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
        </div>
        {/* Plain overflow-y-auto is more reliable than ScrollArea inside a Popover
         * — the popover doesn't constrain its child by default, so a fixed
         * max-height with native scroll keeps the list bounded. */}
        <div className="max-h-96 overflow-y-auto">
          <RevisionList
            revisions={revisions}
            isLoading={isLoading}
            agentName={agentName}
            onRevertClick={confirmTargetSetter}
            onRowClick={(rev) => setPreviewRevision(rev.id)}
          />
        </div>
      </PopoverContent>
    </Popover>
  );
}

type DialogStackProps = {
  revisions: TaskPlanRevision[];
  previewRevision: TaskPlanRevision | null;
  compareRevisions: [TaskPlanRevision | null, TaskPlanRevision | null];
  diffOpen: boolean;
  confirmTarget: TaskPlanRevision | null;
  isSaving: boolean;
  loadRevisionContent: (revisionId: string) => Promise<string>;
  headRevision: TaskPlanRevision | null;
  onRevert: (revisionId: string) => Promise<TaskPlanRevision | null>;
  setConfirmTarget: (rev: TaskPlanRevision | null) => void;
  setDiffOpen: (v: boolean) => void;
  setPreviewRevision: (id: string | null) => void;
  clearComparePair: () => void;
  toggleCompareSelection: (id: string) => void;
  closePopover: () => void;
};

function RevisionsDialogStack({
  revisions,
  previewRevision,
  compareRevisions,
  diffOpen,
  confirmTarget,
  isSaving,
  loadRevisionContent,
  headRevision,
  onRevert,
  setConfirmTarget,
  setDiffOpen,
  setPreviewRevision,
  clearComparePair,
  toggleCompareSelection,
  closePopover,
}: DialogStackProps) {
  const isPreviewCurrent = previewRevision !== null && previewRevision.id === headRevision?.id;
  const previousRevision = useMemo(() => {
    if (!previewRevision) return null;
    // revisions are sorted newest-first by revision_number; previous = the
    // first entry whose revision_number is strictly less than the previewed one.
    return revisions.find((r) => r.revision_number < previewRevision.revision_number) ?? null;
  }, [revisions, previewRevision]);

  const handleRevert = useCallback(
    async (revision: TaskPlanRevision) => {
      try {
        const result = await onRevert(revision.id);
        if (result) {
          toast.success(`Plan restored to v${revision.revision_number}`);
          closePopover();
          setPreviewRevision(null);
          setDiffOpen(false);
        } else {
          // `revertTo` (the hook impl) swallows errors and returns null, so
          // we surface the failure here too — without this branch the dialog
          // closes silently on failure and the user has no feedback.
          toast.error("Failed to restore plan");
        }
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to restore plan");
      }
    },
    [onRevert, setPreviewRevision, setDiffOpen, closePopover],
  );

  const seedComparePair = useCallback(
    (a: TaskPlanRevision, b: TaskPlanRevision) => {
      // Replace any existing pair with [a, b] so the diff opens immediately.
      clearComparePair();
      toggleCompareSelection(a.id);
      if (a.id !== b.id) toggleCompareSelection(b.id);
      setPreviewRevision(null);
      setDiffOpen(true);
    },
    [clearComparePair, toggleCompareSelection, setPreviewRevision, setDiffOpen],
  );

  const onCompareWithCurrent = useCallback(() => {
    if (!previewRevision || !headRevision) return;
    seedComparePair(previewRevision, headRevision);
  }, [previewRevision, headRevision, seedComparePair]);

  const onCompareWithPrevious = useMemo(() => {
    if (!previewRevision || !previousRevision) return null;
    return () => seedComparePair(previousRevision, previewRevision);
  }, [previewRevision, previousRevision, seedComparePair]);

  return (
    <>
      <PlanRevisionPreviewDialog
        // Remount on revision change so loader state resets without setState-in-effect.
        key={`preview-${previewRevision?.id ?? "none"}`}
        revision={previewRevision}
        authorLabel={previewRevision ? authorLabel(previewRevision) : ""}
        loadContent={loadRevisionContent}
        onClose={() => setPreviewRevision(null)}
        onRestore={() => {
          if (previewRevision) setConfirmTarget(previewRevision);
        }}
        onCompareWithPrevious={onCompareWithPrevious}
        onCompareWithCurrent={onCompareWithCurrent}
        isCurrent={isPreviewCurrent}
      />
      <PlanRevisionDiffDialog
        // Remount on pair change so the diff loader resets cleanly.
        key={`diff-${compareRevisions[0]?.id ?? ""}-${compareRevisions[1]?.id ?? ""}`}
        pair={diffOpen ? compareRevisions : [null, null]}
        loadContent={loadRevisionContent}
        onClose={() => setDiffOpen(false)}
        onRestoreOlder={(revisionId) => {
          const rev = revisions.find((r) => r.id === revisionId);
          if (rev) setConfirmTarget(rev);
        }}
      />
      <RevertConfirmDialog
        target={confirmTarget}
        isSaving={isSaving}
        onCancel={() => setConfirmTarget(null)}
        onConfirm={async () => {
          if (!confirmTarget) return;
          try {
            await handleRevert(confirmTarget);
          } finally {
            setConfirmTarget(null);
          }
        }}
      />
    </>
  );
}

/** Display label for an author. User revisions always render as "You";
 * agent revisions use the stored author_name (the agent profile display name). */
function authorLabel(revision: TaskPlanRevision): string {
  if (revision.author_kind === "user") return USER_DISPLAY_NAME;
  return revision.author_name || "Agent";
}

/** Resolve the active session's agent backend name (e.g. "claude", "codex") so
 * agent revisions can render the same logo we use elsewhere. Returns null when
 * there is no active session or its snapshot doesn't carry an agent_name. */
function useActiveAgentBackendName(): string | null {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const snapshot = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId]?.agent_profile_snapshot : null,
  );
  return useMemo(() => {
    const name = (snapshot as { agent_name?: unknown } | null | undefined)?.agent_name;
    return typeof name === "string" && name.length > 0 ? name : null;
  }, [snapshot]);
}

function RevisionList({
  revisions,
  isLoading,
  agentName,
  onRevertClick,
  onRowClick,
}: {
  revisions: TaskPlanRevision[];
  isLoading: boolean;
  agentName: string | null;
  onRevertClick: (rev: TaskPlanRevision) => void;
  onRowClick: (rev: TaskPlanRevision) => void;
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
          agentName={agentName}
          onRevertClick={onRevertClick}
          onRowClick={onRowClick}
        />
      ))}
    </ul>
  );
}

function RevisionAuthor({
  revision,
  agentName,
}: {
  revision: TaskPlanRevision;
  agentName: string | null;
}) {
  if (revision.author_kind === "agent") {
    return (
      <>
        {agentName ? <AgentLogo agentName={agentName} size={14} className="shrink-0" /> : null}
        <span className="text-xs text-foreground truncate" data-testid="plan-revision-author">
          {revision.author_name}
        </span>
      </>
    );
  }
  return (
    <>
      <IconUser className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
      <span className="text-xs text-foreground truncate" data-testid="plan-revision-author">
        {USER_DISPLAY_NAME}
      </span>
    </>
  );
}

function RevisionRow({
  revision,
  isCurrent,
  agentName,
  onRevertClick,
  onRowClick,
}: {
  revision: TaskPlanRevision;
  isCurrent: boolean;
  agentName: string | null;
  onRevertClick: (rev: TaskPlanRevision) => void;
  onRowClick: (rev: TaskPlanRevision) => void;
}) {
  // Force re-render every 30s so the precise timestamp ("5m ago", "Today,
  // 14:32", …) refreshes as the revision ages — `formatPreciseTime` derives
  // from `revision.updated_at` on every render.
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 30_000);
    return () => clearInterval(id);
  }, []);
  const timestamp = formatPreciseTime(revision.updated_at);

  return (
    <li
      // items-center keeps the Restore button vertically centered against the
      // row body, even when the body grows taller (timestamp + restored-from
      // marker on revert rows).
      className="px-3 py-2.5 flex items-center gap-3 hover:bg-accent/30"
      data-testid="plan-revision-row"
      data-revision-id={revision.id}
      data-revision-number={revision.revision_number}
    >
      <button
        type="button"
        onClick={() => onRowClick(revision)}
        className="flex-1 min-w-0 text-left cursor-pointer"
        data-testid="plan-revision-row-body"
      >
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-xs font-semibold">v{revision.revision_number}</span>
          <RevisionAuthor revision={revision} agentName={agentName} />
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
        <div className="text-[11px] text-muted-foreground mt-1" data-testid="plan-revision-time">
          {timestamp}
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
      </button>
      {!isCurrent && (
        <Button
          size="sm"
          variant="ghost"
          className="h-7 px-2 text-xs cursor-pointer shrink-0 gap-1"
          onClick={(e) => {
            e.stopPropagation();
            onRevertClick(revision);
          }}
          data-testid="plan-revision-revert-button"
        >
          <IconRestore className="h-3.5 w-3.5" />
          Restore
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
            lost; the current version stays in history.
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
