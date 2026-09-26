"use client";

import { useCallback, useLayoutEffect, useRef } from "react";
import type { ChangeEvent } from "react";
import { clampTaskTitleInput } from "@/lib/task-title";

type TitleInputElement = HTMLInputElement | HTMLTextAreaElement;

type PendingSelection = { start: number; end: number; value: string };

/**
 * Re-pin the caret after React restores a controlled value whose render
 * bailed out. The epoch, ownership, and value checks keep a stale restore
 * from touching a newer value or a replaced element; the connection and focus
 * checks cover unmount and blur between scheduling and execution.
 */
function scheduleSameResultCaretRestore(options: {
  el: TitleInputElement;
  next: string;
  start: number;
  end: number;
  epoch: number;
  refs: {
    restoreEpochRef: { current: number };
    inputRef: { current: TitleInputElement | null };
  };
}) {
  const { el, next, start, end, epoch, refs } = options;
  queueMicrotask(() => {
    if (epoch !== refs.restoreEpochRef.current) return;
    if (refs.inputRef.current !== el) return;
    if (el.value !== next) return;
    if (!el.isConnected || document.activeElement !== el) return;
    const max = next.length;
    el.setSelectionRange(Math.min(start, max), Math.min(end, max));
  });
}

/**
 * Keeps the caret in place while a task-title field clamps its value at the
 * 60-character cap.
 *
 * `clampChange` truncates the typed value (dropping the tail beyond the cap)
 * and records where the caret was. Because a truncated value differs from the
 * DOM's current value, React rewrites the DOM and the browser resets the caret
 * to the end of the field; the layout effect re-pins the caret to the recorded
 * position (bounded by the clamped length) after that commit.
 *
 * The record is cleared on every non-truncating change so a stale caret from
 * a keystroke that never committed (typing at the very end while at the cap)
 * cannot be replayed by a later commit.
 *
 * When a truncating keystroke leaves the clamped value equal to the last
 * committed value (e.g. typing the same character into an all-same-char title
 * at the cap), `setValue` bails out of the render, so no layout effect runs —
 * but React still restores the controlled DOM value after the event, which
 * resets the caret to the end. That case is handled with an immediate
 * microtask restore instead of the commit-driven path.
 */
export function useTaskTitleSelectionRestore<T extends TitleInputElement = HTMLInputElement>(
  value: string,
) {
  const inputRef = useRef<T | null>(null);
  const pendingSelectionRef = useRef<PendingSelection | null>(null);
  const lastCommittedRef = useRef(value);
  const restoreEpochRef = useRef(0);

  const clampChange = useCallback((e: ChangeEvent<TitleInputElement>) => {
    const el = e.target;
    const next = clampTaskTitleInput(el.value);
    // Any new change supersedes bail-out restores scheduled by an earlier
    // change in the same turn.
    restoreEpochRef.current += 1;
    if (next !== el.value) {
      if (next !== lastCommittedRef.current) {
        // The commit will change the value: record the caret (bound to the
        // clamped value it belongs to) for the layout effect, which runs
        // after React rewrites the DOM.
        pendingSelectionRef.current = {
          start: el.selectionStart ?? el.value.length,
          end: el.selectionEnd ?? el.value.length,
          value: next,
        };
      } else {
        // A same-result keystroke supersedes any earlier commit-path record
        // that the parent never applied.
        pendingSelectionRef.current = null;
        // The clamped value equals the committed value: the render bails out,
        // but React still restores the controlled DOM value after the event
        // and the browser resets the caret to the end. Re-pin it after that
        // write via a microtask; there is no commit to hook into.
        scheduleSameResultCaretRestore({
          el,
          next,
          start: el.selectionStart ?? el.value.length,
          end: el.selectionEnd ?? el.value.length,
          epoch: restoreEpochRef.current,
          refs: { restoreEpochRef, inputRef },
        });
      }
    } else {
      pendingSelectionRef.current = null;
    }
    return next;
  }, []);

  useLayoutEffect(() => {
    lastCommittedRef.current = value;
    // A committed value change supersedes any pending bail-out restore.
    restoreEpochRef.current += 1;
    const selection = pendingSelectionRef.current;
    pendingSelectionRef.current = null;
    if (!selection) return;
    // The committed value must be the one the caret was computed for; a
    // parent that delayed, rejected, or superseded the clamped update must
    // not receive a stale caret.
    if (selection.value !== value) return;
    const el = inputRef.current;
    if (!el || document.activeElement !== el) return;
    const max = value.length;
    el.setSelectionRange(Math.min(selection.start, max), Math.min(selection.end, max));
  }, [value]);

  return { inputRef, clampChange };
}
