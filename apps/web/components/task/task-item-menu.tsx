"use client";

import {
  IconDots,
  IconTrash,
  IconCopy,
  IconEye,
  IconPencil,
  IconLoader,
  IconArchive,
} from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";

type TaskItemMenuProps = {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  onRename?: () => void;
  onDuplicate?: () => void;
  onReview?: () => void;
  onArchive?: () => void;
  onDelete?: () => void;
  isDeleting?: boolean;
};

export function TaskItemMenu({
  open,
  onOpenChange,
  onRename,
  onDuplicate,
  onReview,
  onArchive,
  onDelete,
  isDeleting,
}: TaskItemMenuProps) {
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange} modal={false}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className={cn(
            "flex h-6 w-6 items-center justify-center rounded-md cursor-pointer",
            "text-muted-foreground hover:text-foreground hover:bg-foreground/10",
            "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
            "transition-colors",
          )}
          onClick={(e) => e.stopPropagation()}
          aria-label="Task actions"
        >
          <IconDots className="h-4 w-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        <DropdownMenuItem
          onClick={(e) => {
            e.stopPropagation();
            onRename?.();
          }}
        >
          <IconPencil className="mr-2 h-4 w-4" />
          Rename
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={(e) => {
            e.stopPropagation();
            onDuplicate?.();
          }}
        >
          <IconCopy className="mr-2 h-4 w-4" />
          Duplicate
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={(e) => {
            e.stopPropagation();
            onReview?.();
          }}
        >
          <IconEye className="mr-2 h-4 w-4" />
          Review
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={(e) => {
            e.stopPropagation();
            onArchive?.();
          }}
        >
          <IconArchive className="mr-2 h-4 w-4" />
          Archive
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          disabled={isDeleting}
          onClick={(e) => {
            e.stopPropagation();
            onDelete?.();
          }}
        >
          {isDeleting ? (
            <IconLoader className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <IconTrash className="mr-2 h-4 w-4" />
          )}
          Delete
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
