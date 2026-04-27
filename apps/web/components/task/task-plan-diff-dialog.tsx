"use client";

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
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
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
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

type DiffMode = "unified" | "split";

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
  const [mode, setMode] = useState<DiffMode>("unified");
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
        className="!max-w-6xl w-[95vw] max-h-[90vh] flex flex-col"
        data-testid="plan-revision-diff-dialog"
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription className="text-xs">
            Line-level diff between the two selected versions.
          </DialogDescription>
        </DialogHeader>
        <DiffBody
          before={before}
          after={after}
          loadContent={loadContent}
          mode={mode}
          setMode={setMode}
        />
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
  mode,
  setMode,
}: {
  before: TaskPlanRevision | null;
  after: TaskPlanRevision | null;
  loadContent: (revisionId: string) => Promise<string>;
  mode: DiffMode;
  setMode: (m: DiffMode) => void;
}) {
  const { beforeContent, afterContent, lines, error } = useDiffContent(before, after, loadContent);
  const summary = lines ? diffSummary(lines) : null;
  const sameRevision = before !== null && after !== null && before.id === after.id;
  return (
    <div className="flex flex-col flex-1 min-h-0 gap-2">
      <div className="flex items-center justify-between">
        <div className="text-[11px] text-muted-foreground" data-testid="plan-revision-diff-summary">
          {summary ? `${summary.added} added · ${summary.removed} removed` : "Loading…"}
        </div>
        <ToggleGroup
          type="single"
          size="sm"
          value={mode}
          onValueChange={(v) => {
            if (v === "unified" || v === "split") setMode(v);
          }}
          data-testid="plan-revision-diff-mode-toggle"
        >
          <ToggleGroupItem
            value="unified"
            className="text-xs px-2 cursor-pointer"
            data-testid="plan-revision-diff-mode-unified"
          >
            Unified
          </ToggleGroupItem>
          <ToggleGroupItem
            value="split"
            className="text-xs px-2 cursor-pointer"
            data-testid="plan-revision-diff-mode-split"
          >
            Split
          </ToggleGroupItem>
        </ToggleGroup>
      </div>
      <div
        className={cn(
          "flex-1 min-h-0 rounded border border-border bg-muted/20 font-mono text-xs",
          // Unified scrolls in both axes; split mode owns its own panes' scroll
          // and only needs vertical here so its synced horizontal scrollbars
          // sit inside the panes (one per side, scroll-linked).
          mode === "split" ? "overflow-y-auto overflow-x-hidden" : "overflow-auto",
        )}
        data-testid="plan-revision-diff-body"
      >
        <DiffBodyInner
          mode={mode}
          lines={lines}
          beforeContent={beforeContent}
          afterContent={afterContent}
          error={error}
          sameRevision={sameRevision}
          summary={summary}
        />
      </div>
    </div>
  );
}

function DiffBodyInner({
  mode,
  lines,
  beforeContent,
  afterContent,
  error,
  sameRevision,
  summary,
}: {
  mode: DiffMode;
  lines: DiffLine[] | null;
  beforeContent: string | null;
  afterContent: string | null;
  error: string | null;
  sameRevision: boolean;
  summary: { added: number; removed: number } | null;
}) {
  if (error) return <div className="p-3 text-destructive">{error}</div>;
  if (lines === null) return <DiffLoading />;
  if (sameRevision)
    return <DiffMessage>These are the same version; nothing to compare.</DiffMessage>;
  if (lines.length === 0) return <DiffMessage>(both versions are empty)</DiffMessage>;
  if (summary && summary.added === 0 && summary.removed === 0) {
    return <DiffMessage>No textual changes between these versions.</DiffMessage>;
  }
  if (mode === "split") {
    return <SplitDiff beforeContent={beforeContent ?? ""} afterContent={afterContent ?? ""} />;
  }
  return <UnifiedDiff lines={lines} />;
}

function UnifiedDiff({ lines }: { lines: DiffLine[] }) {
  return (
    // inline-block + min-w-full lets the row backgrounds extend past the
    // viewport so highlights stay solid when the user scrolls horizontally.
    <ul className="inline-block min-w-full" data-testid="plan-revision-diff-unified">
      {lines.map((line, i) => (
        <DiffLineRow key={i} line={line} />
      ))}
    </ul>
  );
}

function DiffLineRow({ line }: { line: DiffLine }) {
  return (
    <li
      className={cn(
        "flex gap-2 px-3 py-0.5 leading-relaxed whitespace-pre min-w-full w-max",
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

/** Side-by-side diff with two equal-width panes that scroll horizontally
 * together. The dialog stays at its fixed width; each pane shows up to half
 * of it and gets its own horizontal scrollbar for long lines. A scroll
 * listener keeps both panes' `scrollLeft` in lockstep so corresponding rows
 * stay aligned visually. */
function SplitDiff({
  beforeContent,
  afterContent,
}: {
  beforeContent: string;
  afterContent: string;
}) {
  const rows = useMemo(
    () => alignSplit(beforeContent, afterContent),
    [beforeContent, afterContent],
  );
  const beforeRef = useRef<HTMLDivElement>(null);
  const afterRef = useRef<HTMLDivElement>(null);
  // Re-entrancy guard: when we set the partner pane's scrollLeft, that fires
  // the partner's onScroll, which would bounce back here without this flag.
  const syncingRef = useRef(false);

  useEffect(() => {
    const before = beforeRef.current;
    const after = afterRef.current;
    if (!before || !after) return;
    const sync = (source: HTMLDivElement, target: HTMLDivElement) => () => {
      if (syncingRef.current) return;
      syncingRef.current = true;
      target.scrollLeft = source.scrollLeft;
      // Release on the next frame so the bounced onScroll has fired and
      // returned early, but we don't permanently disable syncing.
      requestAnimationFrame(() => {
        syncingRef.current = false;
      });
    };
    const onBefore = sync(before, after);
    const onAfter = sync(after, before);
    before.addEventListener("scroll", onBefore, { passive: true });
    after.addEventListener("scroll", onAfter, { passive: true });
    return () => {
      before.removeEventListener("scroll", onBefore);
      after.removeEventListener("scroll", onAfter);
    };
  }, [rows]);

  return (
    <div
      className="grid grid-cols-2 divide-x divide-border min-w-0"
      data-testid="plan-revision-diff-split"
    >
      <SplitPane ref={beforeRef} rows={rows} side="before" testIdSuffix="before" />
      <SplitPane ref={afterRef} rows={rows} side="after" testIdSuffix="after" />
    </div>
  );
}

const SplitPane = ({
  ref,
  rows,
  side,
  testIdSuffix,
}: {
  ref: React.RefObject<HTMLDivElement | null>;
  rows: SplitRow[];
  side: "before" | "after";
  testIdSuffix: string;
}) => (
  <div
    ref={ref}
    className="min-w-0 overflow-x-auto"
    data-testid={`plan-revision-diff-split-pane-${testIdSuffix}`}
  >
    {/* w-max lets each pane's intrinsic width be the longest line on that
     * side; combined with the parent grid-cols-2, the visible portion is
     * still capped at 50% of the dialog while horizontal scroll reveals
     * the rest. */}
    <div className="w-max min-w-full">
      {rows.map((row, i) => (
        <SplitCell key={i} side={side} line={side === "before" ? row.before : row.after} />
      ))}
    </div>
  </div>
);

function SplitCell({ side, line }: { side: "before" | "after"; line: DiffLine | null }) {
  // Empty cells (a removal on the after side, or an addition on the before
  // side) get a subtle striped background so the row alignment is visible.
  if (!line) {
    return (
      <div
        className="px-3 py-0.5 leading-relaxed bg-muted/40 border-l-2 border-transparent min-h-[1.25rem]"
        data-testid="plan-revision-diff-split-cell"
        data-line-kind="empty"
        data-side={side}
      />
    );
  }
  return (
    <div
      className={cn("flex gap-2 px-3 py-0.5 leading-relaxed whitespace-pre", KIND_CLASS[line.kind])}
      data-testid="plan-revision-diff-split-cell"
      data-line-kind={line.kind}
      data-side={side}
    >
      <span className="select-none text-muted-foreground w-3 shrink-0">
        {KIND_PREFIX[line.kind]}
      </span>
      <span className="flex-1">{line.text || " "}</span>
    </div>
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

/** Sort a 2-slot pair by revision_number ascending so the older revision is
 * always the "before" side, regardless of pick order. */
function orderPair(
  pair: [TaskPlanRevision | null, TaskPlanRevision | null],
): [TaskPlanRevision | null, TaskPlanRevision | null] {
  const [a, b] = pair;
  if (!a || !b) return pair;
  return a.revision_number <= b.revision_number ? [a, b] : [b, a];
}

type SplitRow = { before: DiffLine | null; after: DiffLine | null };

/** Align the line-diff into side-by-side rows: each row contains at most one
 * before-line and one after-line. Removals render on the left only; adds on
 * the right only; context lines align across both sides. Adjacent
 * remove/add chunks are zipped together so a single-line edit shows up on the
 * same row. */
function alignSplit(beforeContent: string, afterContent: string): SplitRow[] {
  const lines = lineDiff(beforeContent, afterContent);
  const rows: SplitRow[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.kind === "context") {
      rows.push({ before: line, after: line });
      i++;
      continue;
    }
    // Collect a contiguous remove block, then any adjacent add block.
    const removes: DiffLine[] = [];
    while (i < lines.length && lines[i].kind === "remove") {
      removes.push(lines[i]);
      i++;
    }
    const adds: DiffLine[] = [];
    while (i < lines.length && lines[i].kind === "add") {
      adds.push(lines[i]);
      i++;
    }
    const max = Math.max(removes.length, adds.length);
    for (let k = 0; k < max; k++) {
      rows.push({ before: removes[k] ?? null, after: adds[k] ?? null });
    }
  }
  return rows;
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
