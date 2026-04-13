"use client";

import { useState } from "react";
import { IconLoader, IconTrash, IconArchive, IconChevronRight, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import type { WorkflowStep } from "@/components/kanban-column";

interface TaskMultiSelectToolbarProps {
  selectedIds: Set<string>;
  steps: WorkflowStep[];
  isProcessing: boolean;
  onClearSelection: () => void;
  onBulkDelete: () => Promise<void>;
  onBulkArchive: () => Promise<void>;
  onBulkMove: (targetStepId: string) => Promise<void>;
}

function BulkDeleteDialog({
  count,
  isProcessing,
  onConfirm,
}: {
  count: number;
  isProcessing: boolean;
  onConfirm: () => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button
        size="sm"
        variant="destructive"
        className="cursor-pointer gap-1.5"
        disabled={isProcessing}
        onClick={() => setOpen(true)}
        data-testid="bulk-delete-button"
      >
        <IconTrash className="h-4 w-4" />
        Delete {count}
      </Button>
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {count} tasks</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete {count} task{count !== 1 ? "s" : ""}? This action
              cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={isProcessing}
              className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => {
                onConfirm();
                setOpen(false);
              }}
              data-testid="bulk-delete-confirm"
            >
              {isProcessing ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

export function TaskMultiSelectToolbar({
  selectedIds,
  steps,
  isProcessing,
  onClearSelection,
  onBulkDelete,
  onBulkArchive,
  onBulkMove,
}: TaskMultiSelectToolbarProps) {
  if (selectedIds.size === 0) return null;

  const count = selectedIds.size;

  return (
    <div
      className={cn(
        "fixed bottom-6 left-1/2 -translate-x-1/2 z-50",
        "flex items-center gap-2 px-4 py-2 rounded-xl shadow-lg border border-border",
        "bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/75",
      )}
      data-testid="multi-select-toolbar"
    >
      <span className="text-sm font-medium text-muted-foreground mr-1">
        {count} selected
      </span>

      {steps.length > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              size="sm"
              variant="outline"
              className="cursor-pointer gap-1.5"
              disabled={isProcessing}
              data-testid="bulk-move-button"
            >
              Move to
              <IconChevronRight className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="center" side="top">
            {steps.map((step) => (
              <DropdownMenuItem
                key={step.id}
                className="cursor-pointer"
                onClick={() => onBulkMove(step.id)}
                data-testid={`bulk-move-step-${step.id}`}
              >
                <div className={cn("w-2 h-2 rounded-full mr-2 shrink-0", step.color)} />
                {step.title}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}

      <Button
        size="sm"
        variant="outline"
        className="cursor-pointer gap-1.5"
        disabled={isProcessing}
        onClick={() => onBulkArchive()}
        data-testid="bulk-archive-button"
      >
        <IconArchive className="h-4 w-4" />
        Archive {count}
      </Button>

      <BulkDeleteDialog
        count={count}
        isProcessing={isProcessing}
        onConfirm={() => onBulkDelete()}
      />

      <Button
        size="sm"
        variant="ghost"
        className="cursor-pointer ml-1"
        onClick={onClearSelection}
        disabled={isProcessing}
        aria-label="Clear selection"
        data-testid="bulk-clear-selection"
      >
        <IconX className="h-4 w-4" />
      </Button>
    </div>
  );
}
