"use client";

import { useCallback, useEffect, useRef, useState } from "react";
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

// previewLoader fetches the count of tasks that would be deleted. The
// dialog calls it once when it opens; the result is cached for the
// remainder of the open session so the user doesn't see the number jump.
type PreviewLoader = () => Promise<{ taskCount: number }>;

// onConfirm executes the destructive reset. Errors are surfaced via the
// parent's toast — the dialog just stays open until onConfirm resolves so
// the user can't double-fire it.
type ConfirmHandler = () => Promise<void>;

type ResetWatchDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // integrationLabel is shown in the title (e.g. "Jira watcher").
  integrationLabel: string;
  previewLoader: PreviewLoader;
  onConfirm: ConfirmHandler;
};

// ResetWatchDialog is the shared confirmation dialog for the destructive
// "Reset" action on every integration's watch row. It:
//   - Loads the preview count on open so the body reads "delete N task(s)"
//   - Falls back to a generic description if the preview fails
//   - Disables the confirm button while either the preview or the reset
//     is in flight, so double-clicks can't fire two resets
//
// All integrations (GitHub issue/review, Jira, Linear, Sentry) reuse this
// component to keep the confirmation copy and behaviour consistent.
export function ResetWatchDialog({
  open,
  onOpenChange,
  integrationLabel,
  previewLoader,
  onConfirm,
}: ResetWatchDialogProps) {
  const [count, setCount] = useState<number | null>(null);
  const [previewError, setPreviewError] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [confirming, setConfirming] = useState(false);

  useEffect(() => {
    if (!open) {
      // Reset state between opens so a previous error or count doesn't
      // bleed into the next confirmation.
      setCount(null);
      setPreviewError(false);
      setPreviewLoading(false);
      setConfirming(false);
      return;
    }
    let ignore = false;
    setPreviewLoading(true);
    previewLoader()
      .then((res) => {
        if (ignore) return;
        setCount(res.taskCount);
        setPreviewError(false);
      })
      .catch(() => {
        if (ignore) return;
        setPreviewError(true);
      })
      .finally(() => {
        if (!ignore) setPreviewLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, [open, previewLoader]);

  const description = renderDescription({ previewLoading, previewError, count });
  const confirmDisabled = previewLoading || confirming;

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent data-testid="reset-watch-dialog">
        <AlertDialogHeader>
          <AlertDialogTitle>Reset {integrationLabel}?</AlertDialogTitle>
          <AlertDialogDescription data-testid="reset-watch-dialog-description">
            {description}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer" disabled={confirming}>
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            data-testid="reset-watch-dialog-confirm"
            disabled={confirmDisabled}
            onClick={async (e) => {
              // Prevent the AlertDialog from auto-closing before onConfirm
              // resolves — the parent closes the dialog itself after the
              // toast fires so an error keeps the dialog open.
              e.preventDefault();
              setConfirming(true);
              try {
                await onConfirm();
              } finally {
                setConfirming(false);
              }
            }}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {confirming ? "Resetting…" : "Reset"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// useWatchResetController encapsulates the reset confirmation flow shared
// by every integration's settings section:
//   - tracks which watch is awaiting confirmation
//   - exposes a stable previewLoader / confirmReset pair for ResetWatchDialog
//   - closes the dialog on success and keeps it open on failure so the user
//     can retry after seeing the parent's toast
//
// Caller supplies preview(watch) and reset(watch). resetting carries the
// whole watch row so per-row workspaceId stays available even if the
// underlying list re-renders mid-dialog.
//
// opts is captured by ref, NOT included in the callbacks' deps. Call sites
// pass `useWatchResetController({ preview: ..., reset: ... })` with an
// inline object literal, so opts is a fresh reference on every parent
// render. Including it in the deps would re-create previewLoader each
// render, re-firing ResetWatchDialog's `useEffect([open, previewLoader])`
// and re-issuing the preview API call mid-dialog. The ref keeps the
// closure stable while still seeing the latest callbacks.
export function useWatchResetController<TWatch extends { id: string }>(opts: {
  preview: (watch: TWatch) => Promise<{ taskCount: number }>;
  reset: (watch: TWatch) => Promise<void>;
}) {
  const [resetting, setResetting] = useState<TWatch | null>(null);
  const optsRef = useRef(opts);
  // Sync the ref in an effect rather than during render: the ESLint
  // react-x/no-access-state-in-setstate rule (React 19 docs) flags
  // `ref.current = value` at render time as a stale-closure footgun.
  useEffect(() => {
    optsRef.current = opts;
  }, [opts]);

  const previewLoader = useCallback(() => {
    if (!resetting) {
      // useEffect inside ResetWatchDialog only runs when open=true, which
      // only happens when resetting != null, so this branch is unreachable
      // in practice. The check keeps the closure typed cleanly.
      return Promise.resolve({ taskCount: 0 });
    }
    return optsRef.current.preview(resetting);
  }, [resetting]);

  const confirmReset = useCallback(async () => {
    if (!resetting) return;
    try {
      await optsRef.current.reset(resetting);
      setResetting(null);
    } catch {
      // Keep dialog open on error so the user sees the toast and can retry.
    }
  }, [resetting]);

  const onOpenChange = useCallback((open: boolean) => {
    if (!open) setResetting(null);
  }, []);

  return { resetting, setResetting, previewLoader, confirmReset, onOpenChange };
}

function renderDescription({
  previewLoading,
  previewError,
  count,
}: {
  previewLoading: boolean;
  previewError: boolean;
  count: number | null;
}): string {
  const tail =
    " The watch's polling cursor is also cleared so the next check re-imports every currently-matching item. This cannot be undone.";
  if (previewLoading) return "Checking how many tasks would be deleted…";
  if (previewError) {
    return (
      "This will delete every task previously created by the watch, including archived tasks." +
      tail
    );
  }
  if (count === 0) {
    return (
      "No tasks were created by this watch yet — only the polling state will be cleared." + tail
    );
  }
  const plural = count === 1 ? "task" : "tasks";
  return `This will delete ${count} ${plural} previously created by the watch, including archived ${plural}.${tail}`;
}
