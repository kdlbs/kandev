import { IconLoader } from "@tabler/icons-react";
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

export function TaskDeleteConfirmDialog({
  open,
  onOpenChange,
  taskTitle,
  isBulkOperation,
  count,
  isDeleting,
  onConfirm,
  confirmTestId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskTitle?: string;
  isBulkOperation?: boolean;
  count?: number;
  isDeleting?: boolean;
  onConfirm: () => void;
  confirmTestId?: string;
}) {
  const safeCount = count ?? 0;
  const label = isBulkOperation ? `task${safeCount !== 1 ? "s" : ""}` : "task";
  const title = isBulkOperation ? `Delete ${safeCount} ${label}` : "Delete task";
  const description = isBulkOperation
    ? `Are you sure you want to delete ${safeCount} ${label}? This action cannot be undone.`
    : `Are you sure you want to delete "${taskTitle}"? This action cannot be undone.`;

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent onClick={(e) => e.stopPropagation()}>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={isDeleting}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            data-testid={confirmTestId}
            onClick={() => {
              if (isDeleting) return;
              onConfirm();
              onOpenChange(false);
            }}
          >
            {isDeleting ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
