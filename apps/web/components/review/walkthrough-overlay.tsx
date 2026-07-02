"use client";

import { useEffect, useRef, useState } from "react";
import { IconRoute, IconColumns } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { getTaskWalkthrough } from "@/lib/api/domains/walkthrough-api";
import { WalkthroughFloatingWindow } from "@/components/diff/walkthrough-floating-window";

type WalkthroughOverlayProps = {
  /** The task whose walkthrough launcher should be shown. */
  taskId: string | null;
  /** Unused — kept for call-site compatibility. */
  sessionId?: string | null;
  /** Opens a file's current state in an editor tab (for editor-mode tours). */
  onSelectFile?: (path: string, repo?: string) => void | Promise<void>;
};

/**
 * Task-level launcher for an agent-authored walkthrough. It (1) backfills the
 * walkthrough into the store on mount — a live `task.walkthrough.created` event
 * can fire before the page's WS subscription exists — and (2) offers two entry
 * points: open the review (diff-anchored step cards) or an editor-mode floating
 * window that opens each step's file in its current state (works for unchanged
 * files). The rich step card is shared (see WalkthroughStepInner).
 */
export function WalkthroughOverlay({ taskId, onSelectFile }: WalkthroughOverlayProps) {
  const walkthrough = useAppStore((s) => (taskId ? s.walkthroughs.byTaskId[taskId] : null));
  const activeStep = useAppStore((s) =>
    taskId ? (s.walkthroughs.activeStepByTaskId[taskId] ?? 0) : 0,
  );
  const setWalkthrough = useAppStore((s) => s.setWalkthrough);
  const [editorMode, setEditorMode] = useState(false);

  const fetchedRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (!taskId || fetchedRef.current.has(taskId)) return;
    fetchedRef.current.add(taskId);
    getTaskWalkthrough(taskId)
      .then((wt) => {
        if (wt) setWalkthrough(taskId, wt);
      })
      .catch(() => {});
  }, [taskId, setWalkthrough]);

  if (!taskId || !walkthrough) return null;

  return (
    <>
      {editorMode && onSelectFile ? (
        <WalkthroughFloatingWindow openFile={onSelectFile} onExit={() => setEditorMode(false)} />
      ) : null}
      <div
        data-testid="walkthrough-launcher"
        className="fixed bottom-6 right-6 z-[60] flex items-center gap-1 rounded-full border border-primary/40 bg-card px-2 py-1 text-xs font-medium shadow-lg"
      >
        <button
          type="button"
          data-testid="walkthrough-launcher-review"
          onClick={() => window.dispatchEvent(new CustomEvent("open-review-dialog"))}
          className="flex cursor-pointer items-center gap-1.5 rounded-full px-1.5 py-0.5 hover:bg-accent"
        >
          <IconRoute className="size-4 text-primary" />
          Walkthrough
          <span className="text-muted-foreground">
            {activeStep + 1}/{walkthrough.steps.length}
          </span>
        </button>
        {onSelectFile ? (
          <button
            type="button"
            aria-label="Open walkthrough in editor"
            title="Open in editor (works for unchanged files)"
            data-testid="walkthrough-launcher-editor"
            onClick={() => setEditorMode(true)}
            className="flex cursor-pointer items-center rounded-full p-1 hover:bg-accent"
          >
            <IconColumns className="size-4 text-muted-foreground" />
          </button>
        ) : null}
      </div>
    </>
  );
}
