"use client";

import dynamic from "next/dynamic";
import { useEffect, useState, type ReactNode } from "react";
import { IconLoader2, IconRestore } from "@tabler/icons-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import type { TaskPlanRevision } from "@/lib/types/http";
import { formatPreciseTime } from "@/lib/utils";

const PlanReadOnlyMarkdown = dynamic(
  () =>
    import("@/components/editors/tiptap/tiptap-plan-readonly").then(
      (mod) => mod.PlanReadOnlyMarkdown,
    ),
  { ssr: false },
);

type Props = {
  revision: TaskPlanRevision | null;
  authorLabel: string;
  loadContent: (revisionId: string) => Promise<string>;
  onClose: () => void;
  onRestore: () => void;
  /** Open the diff dialog comparing the previewed revision with the previous
   * revision (vN-1). Disabled if the previewed one is the oldest. */
  onCompareWithPrevious: (() => void) | null;
  /** Open the diff dialog comparing the previewed revision with current HEAD. */
  onCompareWithCurrent: () => void;
  /** True when the revision is the current HEAD — Restore + Compare-with-current
   * CTAs hide. */
  isCurrent: boolean;
};

/** Read-only modal preview of a single plan revision. The parent should give
 * this component a `key` derived from the revision id so it remounts (and
 * resets its loader state) when the user opens a different revision. */
export function PlanRevisionPreviewDialog({
  revision,
  authorLabel,
  loadContent,
  onClose,
  onRestore,
  onCompareWithPrevious,
  onCompareWithCurrent,
  isCurrent,
}: Props): ReactNode {
  const open = revision !== null;
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
    >
      <DialogContent
        className="!max-w-5xl w-[92vw] max-h-[88vh] flex flex-col"
        data-testid="plan-revision-preview-dialog"
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <span>Version {revision?.revision_number}</span>
            {isCurrent && (
              <Badge variant="secondary" className="h-4 text-[10px] px-1.5">
                current
              </Badge>
            )}
          </DialogTitle>
          <DialogDescription className="flex items-center gap-2 text-xs">
            <span>{authorLabel}</span>
            <span>·</span>
            <span>{revision ? formatPreciseTime(revision.updated_at) : ""}</span>
            {revision?.revert_of_revision_id && (
              <span className="flex items-center gap-1 text-muted-foreground">
                <IconRestore className="h-3 w-3" /> restored from earlier version
              </span>
            )}
          </DialogDescription>
        </DialogHeader>

        <PreviewBody revision={revision} loadContent={loadContent} />

        <DialogFooter>
          <Button
            variant="outline"
            onClick={onClose}
            className="cursor-pointer"
            data-testid="plan-revision-preview-close"
          >
            Close
          </Button>
          {!isCurrent && revision && (
            <>
              {onCompareWithPrevious && (
                <Button
                  variant="outline"
                  onClick={onCompareWithPrevious}
                  className="cursor-pointer"
                  data-testid="plan-revision-preview-compare-with-previous"
                >
                  Compare with previous
                </Button>
              )}
              <Button
                variant="outline"
                onClick={onCompareWithCurrent}
                className="cursor-pointer"
                data-testid="plan-revision-preview-compare-with-current"
              >
                Compare with current
              </Button>
              <Button
                onClick={onRestore}
                className="cursor-pointer"
                data-testid="plan-revision-preview-restore"
              >
                Restore this version
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PreviewBody({
  revision,
  loadContent,
}: {
  revision: TaskPlanRevision | null;
  loadContent: (revisionId: string) => Promise<string>;
}) {
  const [content, setContent] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!revision) return;
    let cancelled = false;
    loadContent(revision.id)
      .then((c) => {
        if (!cancelled) setContent(c);
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Failed to load revision");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [revision, loadContent]);

  return (
    <div
      className="flex-1 min-h-0 overflow-y-auto rounded border border-border bg-card p-4"
      data-testid="plan-revision-preview-body"
    >
      <PreviewBodyInner content={content} error={error} />
    </div>
  );
}

function PreviewBodyInner({ content, error }: { content: string | null; error: string | null }) {
  if (error) {
    return <div className="text-destructive text-xs">{error}</div>;
  }
  if (content === null) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
        Loading…
      </div>
    );
  }
  if (content.trim() === "") {
    return <div className="text-xs text-muted-foreground italic">(empty plan)</div>;
  }
  return <PlanReadOnlyMarkdown content={content} />;
}
