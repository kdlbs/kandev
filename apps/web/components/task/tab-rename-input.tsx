"use client";

import { useEffect, useRef, useState } from "react";

/**
 * Inline rename input rendered in place of a dockview tab body.
 * Commits on Enter/blur, cancels on Escape. Shared by the terminal
 * and session tabs.
 */
export function TabRenameInput({
  initial,
  seqBadge,
  onCommit,
  onCancel,
  testId = "tab-rename-input",
  maxLength,
}: {
  initial: string;
  seqBadge: number | null;
  onCommit: (next: string) => void;
  onCancel: () => void;
  testId?: string;
  /** Hard cap typed input to match a server-side name length limit. */
  maxLength?: number;
}) {
  const [value, setValue] = useState(initial);
  const inputRef = useRef<HTMLInputElement>(null);
  // Enter/Escape resolve the rename synchronously, but the resulting unmount
  // also fires the input's blur — guard so Escape can't be followed by a
  // stale onCommit (and Enter can't commit twice).
  const doneRef = useRef(false);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  return (
    <div
      className="flex h-full items-center gap-1 px-2"
      // Stop the click from selecting the tab while we type.
      onClick={(e) => e.stopPropagation()}
      onMouseDown={(e) => e.stopPropagation()}
    >
      {seqBadge != null && (
        <span className="text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5">
          {seqBadge}
        </span>
      )}
      <input
        ref={inputRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            doneRef.current = true;
            onCommit(value);
          } else if (e.key === "Escape") {
            e.preventDefault();
            doneRef.current = true;
            onCancel();
          }
          // Don't let dockview see typed keys as shortcuts.
          e.stopPropagation();
        }}
        onBlur={() => {
          if (doneRef.current) return;
          onCommit(value);
        }}
        maxLength={maxLength}
        data-testid={testId}
        className="h-5 min-w-[6rem] max-w-[14rem] rounded border border-input bg-background px-1 text-xs outline-none focus:ring-1 focus:ring-ring"
      />
    </div>
  );
}
