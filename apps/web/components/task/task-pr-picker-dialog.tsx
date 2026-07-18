"use client";

import { useRef, type KeyboardEvent } from "react";
import { IconGitPullRequest } from "@tabler/icons-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { cn } from "@/lib/utils";
import { getPRStatusColor } from "@/components/github/pr-task-icon";
import { openExternalLink } from "@/lib/desktop/external-links";
import type { TaskPR } from "@/lib/types/github";

type TaskPRPickerDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  prs: TaskPR[];
};

/**
 * Picker shown by the open-task-PR shortcut when a task has several linked
 * PRs. A scrollable list of rows; ArrowUp/ArrowDown move focus (wrapping),
 * Enter or click opens the focused PR on GitHub and closes the dialog.
 */
export function TaskPRPickerDialog({ open, onOpenChange, prs }: TaskPRPickerDialogProps) {
  const listRef = useRef<HTMLDivElement>(null);

  const openPR = (pr: TaskPR) => {
    void openExternalLink(pr.pr_url).catch(() => undefined);
    onOpenChange(false);
  };

  const onListKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    e.preventDefault();
    const rows = Array.from(
      listRef.current?.querySelectorAll<HTMLButtonElement>("button[data-pr-row]") ?? [],
    );
    if (rows.length === 0) return;
    const idx = rows.findIndex((row) => row === document.activeElement);
    const delta = e.key === "ArrowDown" ? 1 : -1;
    if (idx === -1) {
      rows[delta === 1 ? 0 : rows.length - 1].focus();
      return;
    }
    rows[(idx + delta + rows.length) % rows.length].focus();
  };

  // Focus the first PR row explicitly on open instead of relying on Radix's
  // implicit first-focusable ordering, so keyboard flow (ArrowDown/Enter)
  // stays stable if the dialog template's DOM order changes.
  const focusFirstRow = (e: Event) => {
    e.preventDefault();
    listRef.current?.querySelector<HTMLButtonElement>("button[data-pr-row]")?.focus();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] sm:max-w-lg" onOpenAutoFocus={focusFirstRow}>
        <DialogHeader>
          <DialogTitle>Open pull request</DialogTitle>
          <DialogDescription>
            This task has {prs.length} linked pull requests. Choose one to open on GitHub.
          </DialogDescription>
        </DialogHeader>
        <div
          ref={listRef}
          data-testid="task-pr-picker-list"
          className="-mx-2 flex max-h-[50vh] flex-col gap-1 overflow-y-auto px-2"
          onKeyDown={onListKeyDown}
        >
          {prs.map((pr) => (
            <button
              key={pr.id}
              type="button"
              data-pr-row
              data-testid={`task-pr-picker-row-${pr.id}`}
              onClick={() => openPR(pr)}
              className="flex cursor-pointer items-center gap-2 rounded-md border border-transparent px-2 py-2 text-left text-sm transition-colors hover:border-border hover:bg-accent/40 focus:border-primary/70 focus:outline-none"
            >
              <IconGitPullRequest className={cn("h-4 w-4 shrink-0", getPRStatusColor(pr))} />
              <span className="min-w-0 flex-1 truncate">
                <span className="font-medium">
                  {pr.repo} #{pr.pr_number}
                </span>{" "}
                <span className="text-muted-foreground">{pr.pr_title}</span>
              </span>
              <span className="shrink-0 text-xs capitalize text-muted-foreground">{pr.state}</span>
            </button>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
