"use client";

import { useCallback, useEffect, useRef, type KeyboardEvent } from "react";
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
import type { TaskReviewTarget } from "./task-pr-open";

type TaskPRPickerDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  targets: TaskReviewTarget[];
  selectedIndex: number;
  onSelectedIndexChange: (index: number) => void;
  onActivateIndex: (index: number) => void;
};

type TaskReviewRowProps = {
  target: TaskReviewTarget;
  index: number;
  selected: boolean;
  onActivate: (index: number) => void;
};

function TaskReviewRow({ target, index, selected, onActivate }: TaskReviewRowProps) {
  const isMergeRequest = target.type === "mr";
  const testId = isMergeRequest
    ? `task-mr-picker-row-${target.review.id}`
    : `task-pr-picker-row-${target.review.id}`;
  const iconClass = target.type === "pr" ? getPRStatusColor(target.review) : "text-orange-500";
  const reviewLabel =
    target.type === "pr"
      ? `${target.review.repo} #${target.review.pr_number}`
      : `${target.review.project_path} !${target.review.mr_iid}`;
  const reviewTitle = target.type === "pr" ? target.review.pr_title : target.review.mr_title;

  return (
    <button
      key={target.key}
      type="button"
      data-pr-row
      data-row-index={index}
      data-selected={selected ? "true" : undefined}
      data-testid={testId}
      aria-current={selected ? "true" : undefined}
      onClick={() => onActivate(index)}
      className={cn(
        "flex cursor-pointer items-center gap-2 rounded-md border border-transparent px-2 py-2 text-left text-sm transition-colors hover:border-border hover:bg-muted/40 focus:border-primary/70 focus:outline-none",
        isMergeRequest && "min-h-11",
        selected && "border-primary/70 bg-muted/40",
      )}
    >
      <IconGitPullRequest className={cn("h-4 w-4 shrink-0", iconClass)} />
      <span className="min-w-0 flex-1 truncate">
        <span className="font-medium">{reviewLabel}</span>{" "}
        <span className="text-muted-foreground">{reviewTitle}</span>
      </span>
      <span className="shrink-0 text-xs capitalize text-muted-foreground">
        {target.review.state}
      </span>
    </button>
  );
}

/**
 * Picker shown by the open-task-PR shortcut when a task has several linked
 * reviews. A scrollable list of rows; ArrowUp/ArrowDown move the selected row
 * (wrapping), Enter or click opens it and closes the dialog.
 */
export function TaskPRPickerDialog({
  open,
  onOpenChange,
  targets,
  selectedIndex,
  onSelectedIndexChange,
  onActivateIndex,
}: TaskPRPickerDialogProps) {
  const listRef = useRef<HTMLDivElement>(null);
  const hasMergeRequests = targets.some((target) => target.type === "mr");

  const focusRow = useCallback((index: number) => {
    listRef.current
      ?.querySelector<HTMLButtonElement>(`button[data-pr-row][data-row-index="${index}"]`)
      ?.focus();
  }, []);

  useEffect(() => {
    if (open) focusRow(selectedIndex);
  }, [focusRow, open, selectedIndex]);

  const onListKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      onActivateIndex(selectedIndex);
      return;
    }
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    e.preventDefault();
    if (targets.length === 0) return;
    const delta = e.key === "ArrowDown" ? 1 : -1;
    onSelectedIndexChange((selectedIndex + delta + targets.length) % targets.length);
  };

  // Explicit focus keeps keyboard arrows and held-shortcut selection in sync.
  const focusSelectedRow = (e: Event) => {
    e.preventDefault();
    focusRow(selectedIndex);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="w-[calc(100vw-2rem)] sm:max-w-lg"
        enterConfirms={false}
        onOpenAutoFocus={focusSelectedRow}
      >
        <DialogHeader>
          <DialogTitle>{hasMergeRequests ? "Open code review" : "Open pull request"}</DialogTitle>
          <DialogDescription>
            {hasMergeRequests
              ? "Choose a linked pull request or merge request to open at its provider."
              : `This task has ${targets.length} linked pull requests. Choose one to open on GitHub.`}
          </DialogDescription>
        </DialogHeader>
        <div
          ref={listRef}
          data-testid="task-pr-picker-list"
          className="-mx-2 flex max-h-[50vh] flex-col gap-1 overflow-y-auto px-2"
          onKeyDown={onListKeyDown}
        >
          {targets.map((target, index) => (
            <TaskReviewRow
              key={target.key}
              target={target}
              index={index}
              selected={index === selectedIndex}
              onActivate={onActivateIndex}
            />
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
