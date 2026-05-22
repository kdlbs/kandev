"use client";

import { useState, useRef, useEffect } from "react";
import type { Terminal } from "@/hooks/domains/session/use-terminals";

/**
 * Compact dropdown that lists every parked (hidden but PTY-alive) terminal
 * for the active task. Click an item to resume — the tab reappears in the
 * main strip with its sequence number and PTY status preserved.
 *
 * Right-click an item to destroy (kill PTY + remove the DB row).
 *
 * Renders nothing when the parked list is empty; the parent decides
 * whether to mount it.
 */
export function ParkedTerminalsMenu({
  parkedTerminals,
  onResume,
  onDestroy,
}: {
  parkedTerminals: Terminal[];
  onResume: (id: string) => Promise<void> | void;
  onDestroy: (id: string) => Promise<void> | void;
}) {
  const [open, setOpen] = useState(false);
  const wrapperRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDocumentClick = (event: globalThis.MouseEvent) => {
      if (!wrapperRef.current) return;
      if (!wrapperRef.current.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDocumentClick);
    return () => document.removeEventListener("mousedown", onDocumentClick);
  }, [open]);

  if (parkedTerminals.length === 0) return null;

  return (
    <div ref={wrapperRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        data-testid="parked-terminals-button"
        className="inline-flex items-center gap-1 rounded-sm px-2 py-1 h-6 text-xs text-muted-foreground hover:bg-muted cursor-pointer"
        aria-haspopup="menu"
        aria-expanded={open}
        title="Parked terminals"
      >
        <span className="font-mono">⌃</span>
        <span>Parked ({parkedTerminals.length})</span>
      </button>
      {open && (
        <div
          role="menu"
          data-testid="parked-terminals-menu"
          className="absolute right-0 top-7 z-50 min-w-[220px] rounded-md border border-border bg-popover text-popover-foreground shadow-md py-1"
        >
          {parkedTerminals.map((t) => (
            <button
              key={t.id}
              type="button"
              role="menuitem"
              data-testid={`parked-terminal-item-${t.id}`}
              onClick={() => {
                setOpen(false);
                void onResume(t.id);
              }}
              onContextMenu={(e) => {
                e.preventDefault();
                if (window.confirm(`Destroy "${t.label}"? This kills the PTY and removes it.`)) {
                  setOpen(false);
                  void onDestroy(t.id);
                }
              }}
              className="w-full flex items-center justify-between gap-2 px-3 py-1.5 text-xs hover:bg-muted cursor-pointer text-left"
            >
              <span className="flex items-center gap-1.5 min-w-0">
                {t.seq && (
                  <span className="inline-flex items-center justify-center rounded-sm bg-muted text-muted-foreground text-[10px] font-mono leading-none px-1 py-0.5">
                    #{t.seq}
                  </span>
                )}
                <span className="truncate">{t.label}</span>
              </span>
              <span
                className={
                  "shrink-0 text-[10px] font-mono px-1 py-0.5 rounded-sm " +
                  (t.ptyStatus === "running"
                    ? "bg-emerald-500/20 text-emerald-600 dark:text-emerald-400"
                    : "bg-muted text-muted-foreground")
                }
              >
                {t.ptyStatus ?? "stopped"}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
