"use client";

import type { ReactNode } from "react";

/**
 * The annotation comment panel's list mode: entries are numbered like their
 * markers, and comments are written in place instead of in a floating form.
 */
export type PreviewInlineComments = {
  numbers: ReadonlyMap<string, number>;
  /** The draft or saved item whose comment form is open. */
  activeId: string | null;
  selectedId: string | null;
  onOpen: (id: string) => void;
  onSelect: (id: string) => void;
  renderComposer: (id: string) => ReactNode;
};

export function CaptureNumber({ value }: { value: number | undefined }) {
  if (value === undefined) return null;
  return (
    <span
      className="inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-primary px-1 text-[11px] font-semibold text-primary-foreground tabular-nums"
      data-testid="preview-capture-number"
    >
      {value}
    </span>
  );
}
