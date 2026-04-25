"use client";

import dynamic from "next/dynamic";
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
import { formatRelativeTime } from "@/lib/utils";
import { lineDiff, diffSummary, type DiffLine, type DiffLineKind } from "./task-plan-diff";

const PlanReadOnlyMarkdown = dynamic(
  () =>
    import("@/components/editors/tiptap/tiptap-plan-readonly").then(
      (mod) => mod.PlanReadOnlyMarkdown,
    ),
  { ssr: false },
);

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
        className="max-w-4xl max-h-[85vh] flex flex-col"
        data-testid="plan-revision-diff-dialog"
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription className="text-xs">
            Rendered markdown from each version, plus a line-level breakdown of the changes.
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
  const { beforeContent, afterContent, lines, error } = useDiffContent(before, after, loadContent);
  const summary = lines ? diffSummary(lines) : null;
  const sameRevision = before !== null && after !== null && before.id === after.id;
  return (
    <div className="flex flex-col flex-1 min-h-0 gap-3 overflow-y-auto">
      <div className="text-[11px] text-muted-foreground" data-testid="plan-revision-diff-summary">
        {summary ? `${summary.added} added · ${summary.removed} removed` : "Loading…"}
      </div>

      <DiffRenderedSections
        before={before}
        after={after}
        beforeContent={beforeContent}
        afterContent={afterContent}
        error={error}
        sameRevision={sameRevision}
      />

      <DiffLinesSection
        lines={lines}
        error={error}
        sameRevision={sameRevision}
        summary={summary}
      />
    </div>
  );
}

function DiffRenderedSections({
  before,
  after,
  beforeContent,
  afterContent,
  error,
  sameRevision,
}: {
  before: TaskPlanRevision | null;
  after: TaskPlanRevision | null;
  beforeContent: string | null;
  afterContent: string | null;
  error: string | null;
  sameRevision: boolean;
}) {
  if (error) return null;
  if (sameRevision) return null;
  return (
    <div
      className="grid grid-cols-1 md:grid-cols-2 gap-3"
      data-testid="plan-revision-diff-rendered"
    >
      <RenderedSide
        revision={before}
        content={beforeContent}
        side="before"
        label="Before"
      />
      <RenderedSide revision={after} content={afterContent} side="after" label="After" />
    </div>
  );
}

function RenderedSide({
  revision,
  content,
  side,
  label,
}: {
  revision: TaskPlanRevision | null;
  content: string | null;
  side: "before" | "after";
  label: string;
}) {
  return (
    <div
      className={cn(
        "rounded border p-3 bg-card",
        side === "before"
          ? "border-rose-500/30 bg-rose-500/[0.02]"
          : "border-emerald-500/30 bg-emerald-500/[0.02]",
      )}
      data-testid={`plan-revision-diff-${side}`}
    >
      <div className="flex items-center justify-between mb-2 text-[11px] text-muted-foreground">
        <span className="font-semibold uppercase tracking-wide">
          {label}
          {revision ? ` · v${revision.revision_number}` : ""}
        </span>
        {revision && <span>{formatRelativeTime(revision.updated_at)}</span>}
      </div>
      <RenderedSideBody content={content} />
    </div>
  );
}

function RenderedSideBody({ content }: { content: string | null }) {
  if (content === null) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
        Loading…
      </div>
    );
  }
  if (content.trim() === "") {
    return <div className="text-xs text-muted-foreground italic">(empty)</div>;
  }
  return <PlanReadOnlyMarkdown content={content} />;
}

function DiffLinesSection({
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
  return (
    <div className="rounded border border-border bg-muted/20" data-testid="plan-revision-diff-body">
      <div className="px-3 py-1.5 border-b text-[11px] text-muted-foreground font-medium">
        Line-level changes
      </div>
      <DiffLinesInner
        lines={lines}
        error={error}
        sameRevision={sameRevision}
        summary={summary}
      />
    </div>
  );
}

function DiffLinesInner({
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
  if (error) return <div className="p-3 text-destructive text-xs">{error}</div>;
  if (lines === null) return <DiffLoading />;
  if (sameRevision) return <DiffMessage>These are the same version; nothing to compare.</DiffMessage>;
  if (lines.length === 0) return <DiffMessage>(both versions are empty)</DiffMessage>;
  if (summary && summary.added === 0 && summary.removed === 0) {
    return <DiffMessage>No textual changes between these versions.</DiffMessage>;
  }
  return (
    <ul className="font-mono text-xs">
      {lines.map((line, i) => (
        <DiffLineRow key={i} line={line} />
      ))}
    </ul>
  );
}

function DiffLoading() {
  return (
    <div className="flex items-center gap-2 p-3 text-muted-foreground text-xs">
      <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
      Loading…
    </div>
  );
}

function DiffMessage({ children }: { children: ReactNode }) {
  return <div className="p-3 text-muted-foreground italic text-xs">{children}</div>;
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
): {
  beforeContent: string | null;
  afterContent: string | null;
  lines: DiffLine[] | null;
  error: string | null;
} {
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
  return { beforeContent, afterContent, lines, error };
}
