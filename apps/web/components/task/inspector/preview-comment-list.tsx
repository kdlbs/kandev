"use client";

/**
 * The annotation comment panel's list: entries are numbered like their
 * markers, and opening one edits it in the floating comment box.
 */
export type PreviewCommentList = {
  numbers: ReadonlyMap<string, number>;
  /** The draft or saved item whose comment is open in the floating box. */
  activeId: string | null;
  selectedId: string | null;
  onOpen: (id: string) => void;
  onSelect: (id: string) => void;
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
