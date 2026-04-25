"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import type { TaskPlanRevision } from "@/lib/types/http";
import { lineDiff, diffSummary, type DiffLine, type DiffLineKind } from "./task-plan-diff";

type Props = {
  /** Revision pair in arbitrary user-pick order; the dialog re-orders them by
   * revision_number so the older one is always the "before" side. */
  pair: [TaskPlanRevision | null, TaskPlanRevision | null];
  loadContent: (revisionId: string) => Promise<string>;
  onClose: () => void;
  /** Restore the older revision (the "before" side) — null if the pair isn't
   * fully populated. */
  onRestoreOlder: (revisionId: string) => void;
};

const KIND_CLASS: Record<DiffLineKind, string> = {
  add: "bg-emerald-500/10 border-l-2 border-emerald-500/60",
  remove: "bg-rose-500/10 border-l-2 border-rose-500/60",
  context: "border-l-2 border-transparent",
};

const KIND_PREFIX: Record<DiffLineKind, string> = {
  add: "+",
  remove: "-",
  context: " ",
};

export function PlanRevisionDiffDialog({
  pair,
  loadContent,
  onClose,
  onRestoreOlder,
}: Props): ReactNode {
  const [before, after] = useMemo(() => orderPair(pair), [pair]);
  const open = before !== null && after !== null;
  const sameRevision = before !== null && after !== null && before.id === after.id;
  const title =
    before && after ? `Compare v${before.revision_number} → v${after.revision_number}` : "Compare";

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
    >
      <DialogContent
        className="max-w-3xl max-h-[80vh] flex flex-col"
        data-testid="plan-revision-diff-dialog"
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription className="text-xs">
            Line-level diff between the two selected versions.
          </DialogDescription>
        </DialogHeader>
        <DiffBody before={before} after={after} loadContent={loadContent} />
        <DialogFooter>
          <Button
            variant="outline"
            onClick={onClose}
            className="cursor-pointer"
            data-testid="plan-revision-diff-close"
          >
            Close
          </Button>
          {before && !sameRevision && (
            <Button
              onClick={() => onRestoreOlder(before.id)}
              className="cursor-pointer"
              data-testid="plan-revision-diff-restore"
            >
              Restore v{before.revision_number}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DiffBody({
  before,
  after,
  loadContent,
}: {
  before: TaskPlanRevision | null;
  after: TaskPlanRevision | null;
  loadContent: (revisionId: string) => Promise<string>;
}) {
  const { lines, error } = useDiffContent(before, after, loadContent);
  const summary = lines ? diffSummary(lines) : null;
  const sameRevision = before !== null && after !== null && before.id === after.id;
  return (
    <div className="flex flex-col flex-1 min-h-0 gap-2">
      <div className="text-[11px] text-muted-foreground" data-testid="plan-revision-diff-summary">
        {summary ? `${summary.added} added · ${summary.removed} removed` : "Loading…"}
      </div>
      <div
        className="flex-1 min-h-0 overflow-y-auto rounded border border-border bg-muted/20 font-mono text-xs"
        data-testid="plan-revision-diff-body"
      >
        <DiffBodyInner
          lines={lines}
          error={error}
          sameRevision={sameRevision}
          summary={summary}
        />
      </div>
    </div>
  );
}

function DiffBodyInner({
  lines,
  error,
  sameRevision,
  summary,
}: {
  lines: DiffLine[] | null;
  error: string | null;
  sameRevision: boolean;
  summary: { added: number; removed: number } | null;
}) {
  if (error) return <div className="p-3 text-destructive">{error}</div>;
  if (lines === null) return <DiffLoading />;
  if (sameRevision) return <DiffMessage>These are the same version; nothing to compare.</DiffMessage>;
  if (lines.length === 0) return <DiffMessage>(both versions are empty)</DiffMessage>;
  if (summary && summary.added === 0 && summary.removed === 0) {
    return <DiffMessage>No textual changes between these versions.</DiffMessage>;
  }
  return (
    <ul>
      {lines.map((line, i) => (
        <DiffLineRow key={i} line={line} />
      ))}
    </ul>
  );
}

function DiffLoading() {
  return (
    <div className="flex items-center gap-2 p-3 text-muted-foreground">
      <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
      Loading…
    </div>
  );
}

function DiffMessage({ children }: { children: ReactNode }) {
  return <div className="p-3 text-muted-foreground italic">{children}</div>;
}

function DiffLineRow({ line }: { line: DiffLine }) {
  return (
    <li
      className={cn(
        "flex gap-2 px-3 py-0.5 leading-relaxed whitespace-pre",
        KIND_CLASS[line.kind],
      )}
      data-testid="plan-revision-diff-line"
      data-line-kind={line.kind}
    >
      <span className="select-none text-muted-foreground w-3 shrink-0">
        {KIND_PREFIX[line.kind]}
      </span>
      <span className="flex-1 break-words">{line.text || " "}</span>
    </li>
  );
}

/** Sort a 2-slot pair by revision_number ascending so the older revision is
 * always the "before" side, regardless of pick order. */
function orderPair(
  pair: [TaskPlanRevision | null, TaskPlanRevision | null],
): [TaskPlanRevision | null, TaskPlanRevision | null] {
  const [a, b] = pair;
  if (!a || !b) return pair;
  return a.revision_number <= b.revision_number ? [a, b] : [b, a];
}

function useDiffContent(
  before: TaskPlanRevision | null,
  after: TaskPlanRevision | null,
  loadContent: (revisionId: string) => Promise<string>,
): { lines: DiffLine[] | null; error: string | null } {
  const [beforeContent, setBeforeContent] = useState<string | null>(null);
  const [afterContent, setAfterContent] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!before || !after) return;
    let cancelled = false;
    Promise.all([loadContent(before.id), loadContent(after.id)])
      .then(([b, a]) => {
        if (cancelled) return;
        setBeforeContent(b);
        setAfterContent(a);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "Failed to load revision content");
      });
    return () => {
      cancelled = true;
    };
  }, [before, after, loadContent]);

  const lines = useMemo(() => {
    if (beforeContent === null || afterContent === null) return null;
    return lineDiff(beforeContent, afterContent);
  }, [beforeContent, afterContent]);
  return { lines, error };
}
